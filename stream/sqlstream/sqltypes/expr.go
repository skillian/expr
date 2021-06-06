package sqltypes

import (
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/internal"
	"github.com/skillian/logging"

	"github.com/araddon/dateparse"
)

var (
	logger = logging.GetLogger("expr/stream/sqlstream/sqltypes")
)

// Type defines a SQL Type
type Type interface {
	fmt.Stringer
}

// Parse an expr/stream/sqlstream-specific data type specification into a
// sqltype.Type.  This is a common representation meant to be specified in
// struct field tags that can be parsed into RDBMS-specific types.
func Parse(s string) (t Type, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "bool" {
		return Bool, nil
	}
	var ok bool
	if s, ok = removeSuffix(s, ")"); !ok {
		return nil, errors.Errorf1("invalid type spec: %q", s)
	}
	if s, ok = removePrefix(s, "nullable("); ok {
		inner, err := Parse(s)
		if err != nil {
			return nil, err
		}
		return Nullable{inner}, nil
	}
	if s, ok = removePrefix(s, "int("); ok {
		bits, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		return IntType{Bits: bits}, nil
	}
	if s, ok = removePrefix(s, "float("); ok {
		man, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		return FloatType{Mantissa: man}, nil
	}
	if s, ok = removePrefix(s, "date("); ok {
		m, err := parseKeyValues(s)
		if err != nil {
			return nil, err
		}
		var min time.Time
		if x, ok := m["min"]; ok {
			min, err = dateparse.ParseAny(x)
			if err != nil {
				return nil, err
			}
		}
		var max time.Time
		if x, ok := m["max"]; ok {
			max, err = dateparse.ParseAny(x)
			if err != nil {
				return nil, err
			}
		}
		var prec time.Duration
		if x, ok := m["prec"]; ok {
			prec, err = time.ParseDuration(x)
			if err != nil {
				return nil, err
			}
		}
		return TimeType{Min: min, Max: max, Prec: prec}, nil
	}
	if s, ok = removePrefix(s, "string("); ok {
		m, err := parseKeyValues(s)
		if err != nil {
			return nil, err
		}
		st := StringType{}
		if x, ok := m["length"]; ok {
			st.Length, err = strconv.Atoi(x)
			if err != nil {
				return nil, errors.Errorf1From(
					err, "failed to parse %q as string length",
					x)
			}
		}
		if x, ok := m["var"]; ok {
			st.Var, err = strconv.ParseBool(x)
			if err != nil {
				return nil, errors.Errorf1From(
					err, "failed to parse %q as a string's variable-length flag",
					x)
			}
		}
		return st, nil
	}
	if s, ok = removePrefix(s, "bytes("); ok {
		m, err := parseKeyValues(s)
		if err != nil {
			return nil, err
		}
		bt := BytesType{}
		if x, ok := m["length"]; ok {
			bt.Length, err = strconv.Atoi(x)
			if err != nil {
				return nil, errors.Errorf1From(
					err, "failed to parse %q as bytes length",
					x)
			}
		}
		if x, ok := m["var"]; ok {
			bt.Var, err = strconv.ParseBool(x)
			if err != nil {
				return nil, errors.Errorf1From(
					err, "failed to parse %q as bytes' variable-length flag",
					x)
			}
		}
		return bt, nil
	}
	return nil, errors.Errorf1("invalid type spec: %q", s)
}

func removePrefix(v, prefix string) (value string, ok bool) {
	if strings.HasPrefix(v, prefix) {
		return v[len(prefix):], true
	}
	return v, false
}

func removeSuffix(v, suffix string) (value string, ok bool) {
	if strings.HasSuffix(v, suffix) {
		return v[:len(v)-len(suffix)], true
	}
	return v, false
}

// Nullable can wrap a type definition to make the definition nullable
type Nullable [1]Type

var isNullable error = &internal.Sentinel{}

// IsNullable checks if the type is nullable
func IsNullable(t Type) (ok bool) {
	return IterInners(t, func(t Type) error {
		if _, ok = t.(Nullable); ok {
			return isNullable
		}
		return nil
	}) == isNullable
}

// IterInners digs into the innermost type and then calls f on each type as it
// returns back out.
func IterInners(t Type, f func(t Type) error) error {
	switch t := t.(type) {
	case Nullable:
		if err := IterInners(t[0], f); err != nil {
			return err
		}
	}
	return f(t)
}

func (t Nullable) String() string {
	return strings.Join([]string{"nullable(", t[0].String(), ")"}, "")
}

var (
	// Bool represents some SQL type that can hold a boolean value
	Bool Type = BoolType{}

	// Int64 represents some SQL type that can hold an int64 value
	Int64 Type = IntType{Bits: 64}

	// Float64 represents some SQL type that can hold a float64 value
	Float64 Type = FloatType{Mantissa: 53}
)

// BoolType is a "metatype" of the Bool SQL type
type BoolType struct{}

func (BoolType) String() string { return "bool" }

// IntType is the "metatype" of Int SQL types
type IntType struct{ Bits int }

func (t IntType) String() string { return fmt.Sprintf("int(%d)", t.Bits) }

// FloatType is the "metatype" of floating point SQL types
type FloatType struct{ Mantissa int }

func (t FloatType) String() string { return fmt.Sprintf("float(%d)", t.Mantissa) }

// DecimalType is the "metatype" of precise decimal SQL types
type DecimalType struct{ Scale, Prec int }

func (t DecimalType) String() string {
	return fmt.Sprintf("decimal(scale: %d, prec: %d)", t.Scale, t.Prec)
}

// TimeType is the "metatype" of date(time) SQL types
type TimeType struct {
	Min, Max time.Time
	Prec     time.Duration
}

func (t TimeType) String() string {
	return fmt.Sprintf(
		"time(min: %s, max: %s, prec: %s)",
		t.Min, t.Max, t.Prec,
	)
}

// StringType is the "metatype" of string SQL types
type StringType struct {
	Length int
	Var    bool
}

func (t StringType) String() string {
	return fmt.Sprintf("string(length: %d, var: %t)", t.Length, t.Var)
}

// BytesType is the "metatype" of binary SQL types
type BytesType struct {
	Length int
	Var    bool
}

func (t BytesType) String() string {
	return fmt.Sprintf("bytes(length: %d, var: %t)", t.Length, t.Var)
}

func parseKeyValues(keyValues string) (map[string]string, error) {
	m := make(map[string]string)
	keyValues = strings.TrimSpace(keyValues)
	if len(keyValues) == 0 {
		return m, nil
	}
	var key, value string
	var err error
	for len(keyValues) > 0 {
		pivot := strings.IndexByte(keyValues, ':')
		if pivot == -1 {
			return nil, errors.Errorf1(
				"invalid key value: %q", keyValues)
		}
		key, keyValues = strings.TrimSpace(keyValues[:pivot]), keyValues[pivot+1:]
		value, keyValues, err = dequoteUntil(keyValues, ',')
		if err != nil {
			return nil, err
		}
		m[key] = value
	}
	logger.Verbose("parsed values: %#v", m)
	return m, nil
}

// dequote handles parsing a double-quoted string into an "unescaped" version.
func dequoteUntil(v string, stopAt rune) (dequoted, remaining string, err error) {
	v = strings.TrimSpace(v)
	var quote rune
	switch {
	case strings.HasPrefix(v, "\""):
		quote = '"'
	case strings.HasPrefix(v, "'"):
		quote = '\''
	}
	if quote == 0 {
		end := strings.IndexRune(v, stopAt)
		if end == -1 {
			return strings.TrimSpace(v), "", nil
		}
		return strings.TrimSpace(v[:end]), v[end+utf8.RuneLen(stopAt):], nil
	}
	v = v[1:]
	rs := make([]rune, 0, capFromLen(len(v)))
	var escaping, done bool
	for _, r := range v {
		if done {
			if unicode.IsSpace(r) {
				continue
			}
			if r != stopAt {
				return "", "", errors.Errorf2(
					"additional character %q after "+
						"string closing %q", r, v)
			}
			break
		}
		if escaping {
			escaping = false
			switch r {
			case 'n':
				r = '\n'
			case 'r':
				r = '\r'
			case 't':
				r = '\t'
			}
			rs = append(rs, r)
		}
		switch {
		case r == '\\':
			escaping = true
		case r == quote:
			done = true
		default:
			rs = append(rs, r)
		}
	}
	if escaping || !done {
		err = errors.Errorf1("unclosed string: %q", v)
		return
	}
	dequoted = string(rs)
	remaining = strings.TrimSpace(v[len(dequoted)+1+utf8.RuneLen(stopAt):])
	return
}

func capFromLen(length int) int { return 1 << int(bits.Len(uint(length))) }
