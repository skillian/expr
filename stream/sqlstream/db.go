package sqlstream

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqltype"
	"github.com/skillian/logging"
)

var (
	logger = logging.GetLogger("expr/stream/sqlstream")
)

// DB is a SQL database
type DB struct {
	mu          sync.Mutex
	DB          *sql.DB
	dialect     Dialect
	columnNamer Namer
	tableNamer  Namer
	tables      map[reflect.Type]*table
	ifacefree   [][]interface{}
	stringfree  [][]string
}

// DBOption configures a DB
type DBOption func(db *DB) error

// NewDB creates a DB object from a *sql.DB.  At the moment, the dialect is
// required
func NewDB(sqlDB *sql.DB, options ...DBOption) (*DB, error) {
	db := &DB{
		DB:         sqlDB,
		tables:     make(map[reflect.Type]*table),
		ifacefree:  append(make([][]interface{}, 0, 8), make([]interface{}, 0, 8)),
		stringfree: append(make([][]string, 0, 8), make([]string, 0, 8)),
	}
	c := cap(db.ifacefree)
	ifaces := make([]interface{}, c*c)
	strs := make([]string, cap(ifaces))
	for i := range db.ifacefree {
		a := i * c
		b := (i + 1) * c
		db.ifacefree[i] = ifaces[a:a:b]
		db.stringfree[i] = strs[a:a:b]
	}
	for _, opt := range options {
		if err := opt(db); err != nil {
			return nil, err
		}
	}
	if db.dialect == nil {
		logger.Error(
			"No dialect was specified and it is required.  It " +
				"may be possible to automatically determine a" +
				"default dialect based on the driver in the " +
				"future (which is why the dialect is not " +
				"passed explicitly), but for now, it must be " +
				"specified manually.")
		return nil, errors.Errorf0("dialect is required.")
	}
	if db.columnNamer == nil {
		db.columnNamer = &DefaultCase
	}
	if db.tableNamer == nil {
		db.tableNamer = &DefaultCase
	}
	return db, nil
}

// CreateCollection creates a collection (a SQL Table) for the given model.
func (db *DB) CreateCollection(ctx context.Context, v interface{}) error {
	m, err := ModelOf(v)
	if err != nil {
		return errors.Errorf1From(err, "failed to create model for %v", v)
	}
	names := m.AppendNames(db.getstrings())
	var idNames []string
	if idm, ok := m.(IDModeler); ok {
		idNames = idm.ID().AppendNames(db.getstrings())
	}
	defer db.putstrings(names, idNames)
	t := db.getTable(v, m)
	sb := strings.Builder{}
	ws := sb.WriteString
	ws("CREATE TABLE ")
	ws(t.sqlName)
	ws(" ( ")
	for i, c := range t.columns {
		if i > 0 {
			ws(", ")
		}
		ws(c.sqlName)
		ws(" ")
		ws(c.sqlTypeName)
	}
	ws(" );")
	sqlStr := sb.String()
	logger.Info1("creating table with SQL:\n\n%s", sqlStr)
	if _, err := db.DB.ExecContext(ctx, sqlStr); err != nil {
		return errors.Errorf2From(
			err, "failed to create table %s with SQL:\n\n%s",
			t.sqlName, sqlStr)
	}
	return nil
}

// IDOf gets the ID model of a model.
func (db *DB) IDOf(m Model) (Model, bool) {
	return db.idOf(&db.mu, m)
}

func (db *DB) idOf(L sync.Locker, m Model) (Model, bool) {
	idm, ok := m.(IDModeler)
	if ok {
		return idm.ID(), true
	}
	var v interface{}
	if ir, ok := m.(interface{ Interface() interface{} }); ok {
		v = ir.Interface()
		if idm, ok = v.(IDModeler); ok {
			return idm.ID(), true
		}
	} else {
		v = m
	}
	L.Lock()
	t := db.getTableUnsafe(v, m)
	L.Unlock()
	if !strings.HasPrefix(t.columns[0].rawName, t.rawName) || !strings.HasSuffix(t.columns[0].rawName, "ID") {
		return nil, false
	}
	var f *int64
	var n string
	{
		L.Lock()
		fs := m.AppendFields(db.getifacesUnsafe())
		ns := m.AppendNames(db.getstringsUnsafe())
		f, ok = fs[0].(*int64)
		n = ns[0]
		db.putifacesUnsafe(fs)
		db.putstringsUnsafe(ns)
		L.Unlock()
	}
	if !ok {
		return nil, false
	}
	return Int64Model{ID: f, Name: n}, true
}

type dbcmd struct {
	db    *DB
	limit int64
}

var qmarkAndCommas = strings.Repeat("?, ", 256)

// Save some records.  The records do not have to be the same type.
func (db *DB) Save(ctx context.Context, vs ...interface{}) error {
	ids, vs := db.getifaces(), db.getifaces()
	defer db.putifaces(vs, ids)
	for _, v := range vs {
		m, err := ModelOf(v)
		if err != nil {
			return errors.Errorf1From(
				err, "failed to create a model from "+
					"%[1]v (type: %[1]T)", v)
		}
		t := db.getTable(v, m)
		idm, ok := db.IDOf(m)
		if !ok {
			_, err = t.insert(ctx, db, m)
		} else if err == nil {
			ids = idm.AppendValues(ids[:0])
			if valuesAreZero(ids) {
				_, err = t.insert(ctx, db, m)
			} else {
				vs = append(m.AppendValues(vs[:0]), ids...)
				if _, err = db.DB.ExecContext(ctx, t.sqlUpdate, vs...); err != nil {
					return errors.Errorf2From(
						err, "failed to update model with SQL:\n\n%s\n\nArgs:\n\n%v",
						t.sqlUpdate, vs)
				}
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func cleanifaces(ifss ...[]interface{}) [][]interface{} {
	for i, ifs := range ifss {
		for j := range ifs {
			ifs[j] = nil
		}
		ifss[i] = ifs[:0]
	}
	return ifss
}

func (db *DB) getifaces() (ifs []interface{}) {
	db.mu.Lock()
	ifs = db.getifacesUnsafe()
	db.mu.Unlock()
	if ifs == nil {
		ifs = make([]interface{}, 0, 8)
	}
	return
}

func (db *DB) getifacesUnsafe() (ifs []interface{}) {
	if len(db.ifacefree) > 0 {
		i := len(db.ifacefree) - 1
		ifs = db.ifacefree[i]
		db.ifacefree = db.ifacefree[:i]
	}
	return
}

func (db *DB) putifaces(ifss ...[]interface{}) {
	ifss = cleanifaces(ifss...)
	db.mu.Lock()
	db.ifacefree = append(db.ifacefree, ifss...)
	db.mu.Unlock()
}

func (db *DB) putifacesUnsafe(ifss ...[]interface{}) {
	ifss = cleanifaces(ifss...)
	db.ifacefree = append(db.ifacefree, ifss...)
}

func cleanstrings(strss ...[]string) [][]string {
	for i, strs := range strss {
		for j := range strs {
			strs[j] = ""
		}
		strss[i] = strs[:0]
	}
	return strss
}

func (db *DB) getstrings() (strs []string) {
	db.mu.Lock()
	strs = db.getstringsUnsafe()
	db.mu.Unlock()
	if strs == nil {
		strs = make([]string, 0, 0)
	}
	return
}

func (db *DB) getstringsUnsafe() (strs []string) {
	i := len(db.stringfree)
	if i > 0 {
		i--
		strs = db.stringfree[i]
		db.stringfree = db.stringfree[:i]
	}
	return
}

func (db *DB) putstrings(strss ...[]string) {
	strss = cleanstrings(strss...)
	db.mu.Lock()
	db.putstringsUnsafe(strss...)
	db.mu.Unlock()
}

func (db *DB) putstringsUnsafe(strss ...[]string) {
	db.stringfree = append(db.stringfree, strss...)
}

type table struct {
	sqlName    string
	sqlInsert  string
	insertFunc func(t *table, ctx context.Context, db *DB, m Model) (ids Model, err error)
	sqlUpdate  string
	columns    []column
	rawName    string
	goType     reflect.Type
}

func (db *DB) getTable(v interface{}, m Model) *table {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.getTableUnsafe(v, m)
}

func (db *DB) getTableUnsafe(v interface{}, m Model) *table {
	tp := reflect.TypeOf(v)
	t, ok := db.tables[tp]
	if !ok {
		t = &table{
			goType: tp,
		}
		db.tables[tp] = t
		t.init(db, m, tp)
	}
	return t
}

func (t *table) init(db *DB, m Model, tp reflect.Type) {
	var v interface{} = m
	if ir, ok := v.(interface{ Interface() interface{} }); ok {
		v = ir.Interface()
		tp = reflect.TypeOf(v)
	}
	logger.Verbose1("creating table for type %v", tp)
	if tnr, ok := v.(interface{ TableName() string }); ok {
		t.rawName = tnr.TableName()
	} else {
		t.rawName = tp.Elem().Name()
	}
	t.sqlName = db.dialect.Escape(db.tableNamer.Apply(DefaultCase.Parse(t.rawName)))
	logger.Verbose3(
		"name of table for type %v: (raw: %v, sql: %v)",
		tp, t.rawName, t.sqlName)
	fields := m.AppendFields(db.getifacesUnsafe())
	names := m.AppendNames(make([]string, 0, len(fields)))
	sqlTypes := m.AppendSQLTypes(make([]sqltype.Type, 0, len(fields)))
	fieldNameToSQLName := make(map[string]string, len(names))
	t.columns = make([]column, len(fields))
	for i := range fields {
		rawName := db.columnNamer.Apply(DefaultCase.Parse(names[i]))
		sqlName := db.dialect.Escape(rawName)
		sqlTypeName, err := db.dialect.DataTypeName(sqlTypes[i])
		if err != nil {
			// TODO: Is failing to get a way to represent a type
			// when creating a table a recoverable error?
			panic(errors.Errorf2From(
				err, "failed to get SQL data type name for "+
					"dialect %v of %#v",
				db.dialect, sqlTypes[i]))
		}
		t.columns[i] = column{
			sqlName:     sqlName,
			sqlTypeName: sqlTypeName,
			sqlType:     sqlTypes[i],
			rawName:     rawName,
		}
		fieldNameToSQLName[names[i]] = sqlName
	}
	idNames := db.getstringsUnsafe()
	ids, ok := db.idOf(nopLocker{}, m)
	if ok {
		idNames = ids.AppendNames(idNames)
	}
	idSet := make(map[string]struct{}, len(idNames))
	for _, n := range idNames {
		idSet[n] = struct{}{}
	}
	sb := db.getstringsUnsafe()
	defer db.putstringsUnsafe(idNames, sb)
	sb = append(sb, "INSERT INTO ", t.sqlName, " ( ")
	var i int
	for _, n := range names {
		if _, ok := idSet[n]; ok {
			continue
		}
		if i > 0 {
			sb = append(sb, ", ")
		}
		i++
		sb = append(sb, fieldNameToSQLName[n])
	}
	sb = append(sb, ") ")
	if len(idNames) > 0 && db.dialect.DialectFlags().HasAll(DialectOutputInfix) {
		sb = append(sb, "OUTPUT ")
		for i, name := range idNames {
			if i > 0 {
				sb = append(sb, ", ")
			}
			sb = append(sb, "INSERTED.", fieldNameToSQLName[name])
		}
		sb = append(sb, " ")
		t.insertFunc = (*table).insertQuery
	}
	logger.Verbose3("%v names: %v, idNames: %v", m, names, idNames)
	sb = append(sb, "VALUES ( ", qmarkAndCommas[:(len(names)-len(idNames))*3-2], " )")
	if len(idNames) > 0 && db.dialect.DialectFlags().HasAll(DialectReturningSuffix) {
		sb = append(sb, "RETURNING ")
		for i, name := range idNames {
			if i > 0 {
				sb = append(sb, ", ")
			}
			sb = append(sb, fieldNameToSQLName[name])
		}
		sb = append(sb, " ")
		t.insertFunc = (*table).insertQuery
	}
	sb = append(sb, ";")
	t.sqlInsert = strings.Join(sb, "")
	if t.insertFunc == nil {
		t.insertFunc = (*table).insertLastInsertID
	}
	sb = sb[:0]
	if len(idNames) > 0 {
		sb = append(sb, "UPDATE ", t.sqlName, " SET ")
		i = 0
		for _, n := range names {
			if _, ok := idSet[n]; ok {
				continue
			}
			if i > 0 {
				sb = append(sb, ", ")
			}
			i++
			sb = append(sb, fieldNameToSQLName[n], " = ?")
		}
		sb = append(sb, " WHERE ")
		for i, n := range idNames {
			if i > 0 {
				sb = append(sb, ", ")
			}
			sb = append(sb, fieldNameToSQLName[n], " = ?")
		}
		sb = append(sb, ";")
		t.sqlUpdate = strings.Join(sb, "")
	}
	logger.Verbose1("initialized table: %#v", t)
}

func (t *table) insert(ctx context.Context, db *DB, m Model) (Model, error) {
	return t.insertFunc(t, ctx, db, m)
}

var errCannotDetermineID = errors.New("cannot determine ID")

func deferredInsertError(p *error, m Model) {
	if *p != nil {
		*p = errors.Errorf1From(*p, "failed to insert %v", m)
	}
}

// insertQuery obtains the inserted ID(s) by executing the insert as a query
// and selecting the results out via either the OUTPUT ... or RETURNING ...
// parameters.
func (t *table) insertQuery(ctx context.Context, db *DB, m Model) (ids Model, err error) {
	defer deferredInsertError(&err, m)
	ids, ok := db.IDOf(m)
	if !ok {
		return nil, errCannotDetermineID
	}
	err = db.DB.QueryRowContext(ctx, t.sqlInsert, m.AppendValues(nil)...).Scan(ids.AppendFields(nil))
	return
}

// insertLastInsertID obtains the inserted result from res.LastInsertId
func (t *table) insertLastInsertID(ctx context.Context, db *DB, m Model) (ids Model, err error) {
	defer deferredInsertError(&err, m)
	ids, ok := db.IDOf(m)
	if !ok {
		return nil, errCannotDetermineID
	}
	v := ids.AppendFields(nil)[0]
	pi64, ok := v.(*int64)
	if !ok {
		return nil, errors.Errorf(
			"cannot use %v with non-int64 ID: %T",
			errors.CallerName(0), v)
	}
	args := m.AppendValues(nil)
	res, err := db.DB.ExecContext(ctx, t.sqlInsert, args...)
	if err != nil {
		return nil, errors.Errorf2From(
			err, "error while executing SQL:\n\n%s\n\nArgs:\n\n%v",
			t.sqlInsert, args)
	}
	*pi64, err = res.LastInsertId()
	if err != nil {
		return nil, errors.Errorf0From(
			err, "failed to get last insert ID")
	}
	return ids, err
}

type column struct {
	// sqlName holds the pre-escaped column name
	sqlName string

	// sqlTypeName is the data type name of the column and varies depending
	// on the Dialect of the DB that owns this column's table.
	sqlTypeName string

	sqlType sqltype.Type

	// rawName holds the un-escaped name of the column.
	rawName string

	// sqlDataType defines the Go type that data needs to be converted
	// to to be stored to SQL or converted from when retrieved from SQL.
	sqlConvType reflect.Type
}

// GoSQLDataTypeKind maps Go types to SQL kinds of data.
//
// Go database/sql drivers don't have to handle every type, just
type GoSQLDataTypeKind byte

const (
	// Undefined is an unknown data type
	Undefined GoSQLDataTypeKind = iota

	// Bool is for columns that can be represented as Go bools.
	Bool

	// Int64 is for columns that can be represented as Go int64s.
	Int64

	// Float64 is for columns that can be represented as Go float64s.
	Float64

	// Time is for columns that can be represented as Go time.Time.
	Time

	// DecimalDecomposer is a kind for types that implement
	// database/sql/driver.decimalDecomposer.
	DecimalDecomposer

	// String is for columns that can be represented as Go string.
	String

	// Bytes is for columns that can be implemented as Go []byte.
	Bytes

	numGoSQLDataTypeKinds
)

// GoSQLDataTypeKindFunc is the type of the members of the
// GoSQLDataTypeKindFuncs types.  The meaning of its parameters changes
// depending on which GoSQLDataTypeKind the func is for.
type GoSQLDataTypeKindFunc func(length, prec int64) string

// sqlDriverValueTypes maps GoSQLDataTypeKinds to their Go types.
var sqlDriverValueTypes = [int(numGoSQLDataTypeKinds)]reflect.Type{
	nil,
	reflect.TypeOf(false),
	reflect.TypeOf(int64(0)),
	reflect.TypeOf(float64(0)),
	reflect.TypeOf(time.Time{}),
	reflect.TypeOf((*interface {
		Decompose(buf []byte) (form byte, negative bool, coefficient []byte, exponent int32)
	})(nil)).Elem(),
	reflect.TypeOf(""),
	reflect.TypeOf([]byte(nil)),
}

func getConvertType(from reflect.Type) (to reflect.Type) {
	for _, t := range sqlDriverValueTypes[1:] {
		if t == from {
			return t
		}
	}
	for _, t := range sqlDriverValueTypes[1:] {
		if from.AssignableTo(t) && t.AssignableTo(from) {
			return t
		}
	}
	for _, t := range sqlDriverValueTypes[1:] {
		if from.ConvertibleTo(t) && t.ConvertibleTo(from) {
			return t
		}
	}
	return nil
}

func isAlphanumeric(r rune) bool {
	return unicode.IsOneOf([]*unicode.RangeTable{unicode.Letter, unicode.Number}, r)
}

type nopLocker struct{}

func (nopLocker) Lock()   {}
func (nopLocker) Unlock() {}
