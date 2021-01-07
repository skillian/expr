package sqlstream

import (
	"context"
	"math/bits"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqltype"
)

// Dialect tweaks how queries/statements are built depending on the RDBMS's
// "flavor" of SQL.
type Dialect interface {
	// DialectFlags gets the set of boolean dialect options
	DialectFlags() DialectFlags

	// Escape an identifier so it can safely be used in raw SQL.  This is
	// not safe from malicious misuse.
	Escape(id string) string

	// DataTypeName generates the SQL name of the given datatype
	DataTypeName(t sqltype.Type) (string, error)

	// tableExists checks if a table exists
	tableExists(ctx context.Context, db *DB, t *table) (exists bool, err error)
}

// WithDialect configures the SQL dialect for a DB
func WithDialect(d Dialect) DBOption {
	return func(db *DB) error {
		db.dialect = d
		return nil
	}
}

// DialectFlags holds dialect boolean flag options
type DialectFlags uint64

// HasAll checks if the DialectFlags have all of the flags being queried
func (fs DialectFlags) HasAll(f DialectFlags) bool { return fs&f == f }

const (
	// DialectLimitSuffix is set when " LIMIT n" is suffixed to a query to
	// limit the number of results affected/returned by a query.
	DialectLimitSuffix DialectFlags = 1 << iota

	// DialectTopInfix is set when " TOP(n) " is infixed into statements
	// to limit the number of results affected/returned by a query.
	DialectTopInfix

	// DriverSupportsLastRowID is set if the driver supports getting the
	// last inserted row's ID.
	DriverSupportsLastRowID

	// DialectReturningSuffix is defined in SQL variants where the IDs
	// of inserted rows can be returned with a "RETURNING id_col" suffix.
	DialectReturningSuffix

	// DialectOutputInfix is for SQL variants where IDs of inserted rows
	// can be returned with an " OUTPUT INSERTED.id_col" infix.
	DialectOutputInfix
)

var (
	// MSSQL is a built-in syntax type for Microsoft SQL Server.
	MSSQL Dialect = mssql{}

	// SQLite3 is a built-in syntax type for SQLite3.
	SQLite3 Dialect = sqlite3{}
)

// TimeTypeName pairs together a sqltype.TimeType and the name of the type that
// matches that criteria.
type TimeTypeName struct {
	sqltype.TimeType
	Name string
}

func selectTimeTypeName(t sqltype.TimeType, ttns []TimeTypeName) (string, error) {
	for _, ttn := range ttns {
		if (ttn.Min == time.Time{} || t.Min.After(ttn.Min) || t.Min.Equal(ttn.Min)) &&
			(ttn.Max == time.Time{} || t.Max.Before(ttn.Max) || t.Max.Equal(ttn.Max)) &&
			(ttn.Prec == 0 || t.Prec <= ttn.Prec) {
			return ttn.Name, nil
		}
	}
	return "", errors.Errorf1("no time type for this SQL variant can handle %v", t)
}

type defaultDialect struct{}

func (d defaultDialect) DialectFlags() DialectFlags { return DialectLimitSuffix }

func (d defaultDialect) Escape(id string) string {
	if strings.Contains(id, "\"\"") {
		id = strings.Join(strings.Split(id, "\""), "\"\"")
	}
	return strings.Join([]string{"\"", "\""}, id)
}

type mssql struct{ defaultDialect }

func (mssql) DialectFlags() DialectFlags { return DialectTopInfix | DialectOutputInfix }

var mssqlTimeTypes = []TimeTypeName{
	{TimeType: sqltype.TimeType{
		Min:  time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
		Max:  time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC),
		Prec: 24 * time.Hour,
	}, Name: "date"},
	{TimeType: sqltype.TimeType{
		Min:  time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
		Max:  time.Date(2079, 6, 6, 0, 0, 0, 0, time.UTC),
		Prec: 1 * time.Minute,
	}, Name: "smalldatetime"},
	{TimeType: sqltype.TimeType{
		Min:  time.Date(1753, 1, 1, 0, 0, 0, 0, time.UTC),
		Max:  time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
		Prec: 100 * time.Nanosecond,
	}, Name: "datetime"},
	{TimeType: sqltype.TimeType{
		Min:  time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
		Max:  time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC),
		Prec: 100 * time.Nanosecond,
	}, Name: "datetime2"},
}

func (mssql) DataTypeName(t sqltype.Type) (string, error) {
	sb := make([]string, 0, 8)
	err := sqltype.IterInners(t, func(t sqltype.Type) error {
		switch t := t.(type) {
		case sqltype.Nullable:
			sb = append(sb[:len(sb)-1], " null")
			return nil
		case sqltype.Primary:
			sb = append(sb, " primary key")
			return nil
		case sqltype.BoolType:
			sb = append(sb, "bit", " not null")
			return nil
		case sqltype.IntType:
			if t.Bits <= 32 {
				sb = append(sb, "int", " not null")
				return nil
			}
			sb = append(sb, "bigint", " not null")
			return nil
		case sqltype.FloatType:
			if 0 < t.Mantissa && t.Mantissa <= 24 {
				sb = append(sb, "float(24)", " not null")
				return nil
			}
			if t.Mantissa > 53 {
				return errors.Errorf1(
					"Mantissa value %d out of range [0, 53]",
					t.Mantissa)
			}
			sb = append(sb, "float(53)", " not null")
			return nil
		case sqltype.DecimalType:
			sb = append(sb,
				"decimal(",
				strconv.Itoa(t.Scale),
				", ",
				strconv.Itoa(t.Prec),
				")", " not null")
			return nil
		case sqltype.TimeType:
			s, err := selectTimeTypeName(t, mssqlTimeTypes)
			if err != nil {
				return err
			}
			sb = append(sb, s, " not null")
		case sqltype.StringType:
			if t.Var {
				sb = append(sb, "var")
			}
			sb = append(sb, "nchar(")
			length := "max"
			if t.Length > 0 {
				length = strconv.Itoa(t.Length)
			}
			sb = append(sb, length, ")", " not null")
			return nil
		case sqltype.BytesType:
			if t.Var {
				sb = append(sb, "var")
			}
			sb = append(sb, "binary(")
			length := "max"
			if t.Length > 0 {
				length = strconv.Itoa(t.Length)
			}
			sb = append(sb, length, ")", " not null")
			return nil
		}
		return errors.Errorf1("cannot create data type name for %v", t)
	})
	if err != nil {
		return "", err
	}
	return strings.Join(sb, ""), nil
}

func (mssql) tableExists(ctx context.Context, db *DB, t *table) (exists bool, err error) {
	var count int64
	err = db.DB.QueryRowContext(ctx, "SELECT COUNT(1) FROM sys.tables WHERE t.\"name\" = ?;", t.rawName).Scan(&count)
	exists = count == 1
	return
}

type sqlite3 struct{ defaultDialect }

func (sqlite3) DialectFlags() DialectFlags { return DialectLimitSuffix | DriverSupportsLastRowID }

var sqlite3TimeTypeNames = []TimeTypeName{
	{TimeType: sqltype.TimeType{
		Min: func() time.Time { // Maybe this will work for BCE years?
			t := time.Date(4714, 11, 24, 0, 0, 0, 0, time.UTC)
			t.AddDate(-2*t.Year(), 0, 0)
			return t
		}(),
	}, Name: "REAL"},
	{TimeType: sqltype.TimeType{
		Min:  time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		Prec: 1 * time.Second,
	}, Name: "INTEGER"},
	{TimeType: sqltype.TimeType{}, Name: "TEXT"},
}

func (sqlite3) DataTypeName(t sqltype.Type) (string, error) {
	sb := make([]string, 0, 4)
	err := sqltype.IterInners(t, func(t sqltype.Type) error {
		switch t := t.(type) {
		case sqltype.Primary:
			sb = append(sb, " PRIMARY KEY")
			return nil
		case sqltype.Nullable:
			sb = append(sb[:len(sb)-1], " NULL")
			return nil
		case sqltype.BoolType:
			sb = append(sb, "INTEGER", " NOT NULL")
			return nil
		case sqltype.IntType:
			sb = append(sb, "INTEGER", " NOT NULL")
			return nil
		case sqltype.FloatType:
			sb = append(sb, "REAL", " NOT NULL")
			return nil
		case sqltype.DecimalType:
			return errors.Errorf0("decimal type is not implemented")
		case sqltype.TimeType:
			s, err := selectTimeTypeName(t, sqlite3TimeTypeNames)
			if err != nil {
				return err
			}
			sb = append(sb, s, " NOT NULL")
			return nil
		case sqltype.StringType:
			sb = append(sb, "TEXT", " NOT NULL")
			return nil
		case sqltype.BytesType:
			sb = append(sb, "BLOB", " NOT NULL")
			return nil
		}
		return errors.Errorf1("Unknown type: %T", t)
	})
	if err != nil {
		return "", err
	}
	return strings.Join(sb, ""), nil
}

func (sqlite3) tableExists(ctx context.Context, db *DB, t *table) (exists bool, err error) {
	var count int64
	err = db.DB.QueryRowContext(ctx, "SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?;", t.rawName).Scan(&count)
	exists = count == 1
	return
}

// Namer can both apply a rename rule or parse a name under that rule into
// separate words.  Case is ignored when applying and Parse's result is
// lowercase.:
//
//	Apply("thing name") -> "ThingName"
//
// and
//
//	Parse("ThingName") -> "thing name"
//
type Namer interface {
	Apply(s string) string
	Parse(s string) string
}

// WithTableNamer uses the given Namer to rename models to a given naming
// convention
func WithTableNamer(n Namer) DBOption {
	return func(db *DB) error {
		db.tableNamer = n
		return nil
	}
}

// WithColumnNamer uses the given namer to rename model field names to a
// given naming convention
func WithColumnNamer(n Namer) DBOption {
	return func(db *DB) error {
		db.columnNamer = n
		return nil
	}
}

var (
	// CamelCase renames string names into camelCase
	CamelCase Namer = caseNamer{capInitial: false}

	// DefaultCase is a built-in naming convention
	DefaultCase = DefaultNamer{
		acronyms: map[string]string{
			"Id":  "ID",
			"Sql": "SQL",
		},
	}

	// PascalCase renames string names into PascalCase
	PascalCase Namer = caseNamer{capInitial: true}

	// SnakeCase renames strings into snake_case
	SnakeCase Namer = snakeCaseNamer{}
)

type caseNamer struct {
	capInitial bool
}

func (cr caseNamer) Apply(s string) string {
	rs := make([]rune, 0, len(s))
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inSpace = true
			continue
		}
		if !unicode.IsOneOf([]*unicode.RangeTable{unicode.Letter, unicode.Number}, r) {
			continue
		}
		if len(rs) == 0 {
			if cr.capInitial {
				r = unicode.ToUpper(r)
			} else {
				r = unicode.ToLower(r)
			}
		} else if inSpace {
			r = unicode.ToUpper(r)
		} else {
			r = unicode.ToLower(r)
		}
		inSpace = false
		rs = append(rs, r)
	}
	return string(rs)
}

func (caseNamer) Parse(s string) string {
	s = strings.TrimSpace(s)
	rs := make([]rune, 0, 1<<bits.Len(uint(len(s))))
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			rs = append(rs, ' ')
		}
		rs = append(rs, unicode.ToLower(r))
	}
	return string(rs)
}

// DefaultNamer is a default Namer convention that follows Go's identifier
// naming convention.
type DefaultNamer struct {
	mu       sync.Mutex
	acronyms map[string]string
}

// Apply a the naming convention to s
func (n *DefaultNamer) Apply(s string) string {
	fields := strings.Fields(s)
	for i, f := range fields {
		first, n := utf8.DecodeRuneInString(f)
		fields[i] = string(unicode.ToUpper(first)) + f[n:]
	}
	// TODO: Maybe RCU?
	n.mu.Lock()
	for i, f := range fields {
		if u, ok := n.acronyms[f]; ok {
			fields[i] = u
		}
	}
	n.mu.Unlock()
	return strings.Join(fields, "")
}

// Parse a name into separate words
func (n *DefaultNamer) Parse(s string) string {
	if len(s) == 0 {
		return ""
	}
	fields := make([][]rune, 0, 1<<(bits.Len(uint(len(s)))-1))
	rs := make([]rune, 0, cap(fields))
	for _, r := range s {
		if !unicode.IsOneOf([]*unicode.RangeTable{unicode.Letter, unicode.Digit}, r) {
			continue
		}
		if unicode.IsUpper(r) && len(rs) > 0 {
			rs2 := make([]rune, len(rs))
			copy(rs2, rs)
			fields = append(fields, rs2)
			rs = rs[:0]
		}
		rs = append(rs, r)
	}
	if len(rs) > 0 {
		fields = append(fields, rs)
	}
	if len(fields) == 0 {
		return ""
	}
	for i := len(fields) - 1; i > 0; i-- {
		cur, prev := fields[i], &fields[i-1]
		if len(*prev) == 1 && unicode.IsUpper((*prev)[0]) {
			curUpper := true
			for _, r := range cur {
				if !unicode.IsUpper(r) {
					curUpper = false
					break
				}
			}
			if curUpper {
				*prev = append(*prev, cur...)
				copy(fields[i:], fields[i+1:])
				fields = fields[:len(fields)-1]
			}
		}
	}
	length := 0
	for _, rs = range fields {
		length += len(rs)
	}
	rs = make([]rune, 0, length)
	for i, part := range fields {
		if i > 0 {
			rs = append(rs, ' ')
		}
		for j, r := range part {
			part[j] = unicode.ToLower(r)
		}
		rs = append(rs, part...)
	}
	return string(rs)
}

type snakeCaseNamer struct{}

func (snakeCaseNamer) Apply(s string) string {
	//rs := make([]rune, 0, 1<<(bits.Len(uint(len(s)))-1))
	started := false
	inSpace := false
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			if !started {
				return -1
			}
			if inSpace {
				return -1
			}
			inSpace = true
			return '_'
		}
		if !started {
			started = true
			return r
		}
		inSpace = false
		return unicode.ToLower(r)
	}, s)
}

func (snakeCaseNamer) Parse(s string) string {
	return strings.Join(strings.Split(s, "_"), " ")
}
