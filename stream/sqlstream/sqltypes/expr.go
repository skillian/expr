package sqltypes

import (
	"math/bits"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/skillian/expr/errors"

	"github.com/araddon/dateparse"
)

// Type defines a SQL Type
type Type interface{}

// Parse an expr/stream/sqlstream-specific data type specification into a
// sqltype.Type.  This is a common representation meant to be specified in
// struct field tags that can be parsed into RDBMS-specific types.
func Parse(v string) (t Type, err error) {
	const (
		nullablePrefix = "nullable("
		intPrefix      = "int("
		floatPrefix    = "float("
		datePrefix     = "date("
		stringPrefix   = "string("
		bytesPrefix    = "bytes("
	)
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "bool" {
		return Bool, nil
	}
	var ok bool
	if strings.HasSuffix(v, ")") {
		v = v[:len(v)-len(")")]
		if v, ok = removePrefix(v, nullablePrefix); ok {
			inner, err := Parse(v)
			if err != nil {
				return nil, err
			}
			return Nullable{inner}, nil
		}
		if v, ok = removePrefix(v, intPrefix); ok {
			bits, err := strconv.Atoi(v)
			if err != nil {
				return nil, err
			}
			return IntType{Bits: bits}, nil
		}
		if v, ok = removePrefix(v, floatPrefix); ok {
			man, err := strconv.Atoi(v)
			if err != nil {
				return nil, err
			}
			return FloatType{Mantissa: man}, nil
		}
		if v, ok = removePrefix(v, datePrefix); ok {
			m, err := parseKeyValues(v)
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
		if v, ok = removePrefix(v, stringPrefix); ok {
			m, err := parseKeyValues(v)
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
		if v, ok = removePrefix(v, bytesPrefix); ok {
			m, err := parseKeyValues(v)
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
	}
	return nil, errors.Errorf1("invalid type spec: %q", v)
}

func removePrefix(v, prefix string) (value string, ok bool) {
	if strings.HasPrefix(v, prefix) {
		return v[len(prefix):], true
	}
	return v, false
}

// Nullable can wrap a type definition to make the definition nullable
type Nullable [1]Type

// IsNullable checks if the type is nullable
func IsNullable(t Type) (ok bool) {
	IterInners(t, func(t Type) error {
		_, ok = t.(Nullable)
		return nil
	})
	return
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

// IntType is the "metatype" of Int SQL types
type IntType struct{ Bits int }

// FloatType is the "metatype" of floating point SQL types
type FloatType struct{ Mantissa int }

// DecimalType is the "metatype" of precise decimal SQL types
type DecimalType struct{ Scale, Prec int }

// TimeType is the "metatype" of date(time) SQL types
type TimeType struct {
	Min, Max time.Time
	Prec     time.Duration
}

// StringType is the "metatype" of string SQL types
type StringType struct {
	Length int
	Var    bool
}

// BytesType is the "metatype" of binary SQL types
type BytesType struct {
	Length int
	Var    bool
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
	return m, nil
}

// dequote handles parsing a double-quoted string into an "unescaped" version.
func dequoteUntil(v string, stopAt rune) (dequoted, remaining string, err error) {
	if !strings.HasPrefix(v, "\"") {
		end := strings.IndexRune(v, stopAt)
		if end == -1 {
			return strings.TrimSpace(v), "", nil
		}
		return strings.TrimSpace(v[:end]), v[end+utf8.RuneLen(stopAt):], nil
	}
	rs := make([]rune, 0, capFromLen(len(v)))
	var escaping, done bool
	for _, r := range v {
		if done {
			if unicode.IsSpace(r) {
				continue
			}
			if r != stopAt {
				return "", "", errors.Errorf1("additional characters after string closing %q", v)
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
		switch r {
		case '\\':
			escaping = true
		case '"':
			done = true
		}
	}
	dequoted = string(rs)
	remaining = v[len(dequoted):]
	return
}

func capFromLen(length int) int { return 1 << int(bits.Len(uint(length))) }
