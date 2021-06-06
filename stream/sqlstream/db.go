package sqlstream

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/davecgh/go-spew/spew"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqltypes"
	"github.com/skillian/logging"
)

var (
	logger = logging.GetLogger("expr/stream/sqlstream")
)

// DB wraps a *sql.DB and adds ORM functionality on top.
type DB struct {
	mu          sync.Mutex
	DB          *sql.DB
	dialect     Dialect
	columnNamer Namer
	tableNamer  Namer
	tables      map[reflect.Type]*table
	freelists   struct {
		ifaces [][]interface{}
		strs   [][]string
	}
}

// sqlCmdImpl executes SQL commands like statements and queries.
type sqlCmdImpl interface {
	ExecContext(ctx context.Context, stmt string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, stmt string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, stmt string, args ...interface{}) *sql.Row
}

// DBOption configures a DB
type DBOption func(db *DB) error

// NewDB creates a DB object from a *sql.DB.  At the moment, the dialect is
// required
func NewDB(sqlDB *sql.DB, options ...DBOption) (*DB, error) {
	db := &DB{
		DB:     sqlDB,
		tables: make(map[reflect.Type]*table),
	}
	// Build some initial empty []interface{} and []string slices:
	{
		const arbitraryCapacity = 8
		//
		c := arbitraryCapacity
		db.freelists.ifaces = make([][]interface{}, 0, c)
		db.freelists.strs = make([][]string, 0, c)
		ifaces := make([]interface{}, c*c)
		strs := make([]string, cap(ifaces))
		for i := range db.freelists.ifaces {
			a := i * c
			b := (i + 1) * c
			db.freelists.ifaces[i] = ifaces[a:a:b]
			db.freelists.strs[i] = strs[a:a:b]
		}
	}
	for _, opt := range options {
		if err := opt(db); err != nil {
			return nil, err
		}
	}
	if db.dialect == nil {
		logger.Error(
			"No dialect was specified and it is required.  It " +
				"may be possible to automatically determine a " +
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
	var fields []interface{}
	var idFields []interface{}
	var isID map[interface{}]struct{}
	var idSQLNames []string
	if idm, ok := db.IDOf(m); ok {
		db.mu.Lock()
		fields = m.AppendFields(db.getifacesLocked())
		idFields = idm.AppendFields(db.getifacesLocked())
		db.mu.Unlock()
		logger.Verbose2("fields: %#v, idFields:%#v", fields, idFields)
		isID = make(map[interface{}]struct{}, len(idFields))
		for _, f := range idFields {
			isID[f] = struct{}{}
		}
		idSQLNames = db.getstrings()
		defer db.putifaces(fields, idFields)
		defer db.putstrings(idSQLNames)
	}
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
		if len(idFields) > 0 {
			if _, ok := isID[fields[i]]; ok {
				idSQLNames = append(idSQLNames, c.sqlName)
			}
		}
	}
	if len(idSQLNames) > 0 {
		ws(", CONSTRAINT ")
		ws(db.dialect.Escape("PK_" + t.rawName))
		ws(" PRIMARY KEY (")
		ws(idSQLNames[0])
		for _, sn := range idSQLNames[1:] {
			ws(", ")
			ws(sn)
		}
		ws(")")
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
	t := db.getTableLocked(v, m)
	L.Unlock()
	if !strings.HasPrefix(t.columns[0].rawName, t.rawName) || !strings.HasSuffix(t.columns[0].rawName, "ID") {
		return nil, false
	}
	var f *int64
	var n string
	{
		L.Lock()
		fs := m.AppendFields(db.getifacesLocked())
		ns := m.AppendNames(db.getstringsLocked())
		f, ok = fs[0].(*int64)
		n = ns[0]
		db.putifacesLocked(fs)
		db.putstringsLocked(ns)
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

// Save some records.  The records do not have to be the same type.  Nested
// slices of []interface{} or []Model are recursively unpacked and saved.
func (db *DB) Save(ctx context.Context, vs ...interface{}) error {
	ids, ws := db.getifaces(), db.getifaces()
	defer db.putifaces(ws, ids)
	var cmder sqlCmdImpl
	if tx, ok := TxFromContext(ctx); ok {
		cmder = tx
	} else {
		cmder = db.DB
	}
	for _, v := range vs {
		switch v := v.(type) {
		case []interface{}:
			if err := db.Save(ctx, v...); err != nil {
				return err
			}
		case []Model:
			for _, x := range v {
				ws = append(ws, x)
			}
			if err := db.Save(ctx, ws...); err != nil {
				return err
			}
			ws = ws[:0]
		}
		m, err := ModelOf(v)
		if err != nil {
			return errors.Errorf1From(
				err, "failed to create a model from "+
					"%[1]v (type: %[1]T)", v)
		}
		t := db.getTable(v, m)
		idm, ok := db.IDOf(m)
		if ok {
			ids = idm.AppendValues(ids[:0])
			logger.Verbose2(
				"current ID values of %v: %v",
				m, spew.Sdump(ids))
			if !valuesAreZero(ids) {
				logger.Verbose1("one or more non-zero ID "+
					"values.  Updating %v...", m)
				ws = append(m.AppendValues(ws[:0]), ids...)
				res, err := cmder.ExecContext(
					ctx, t.sqlUpdate, ws...)
				if err != nil {
					return errors.Errorf2From(
						err, "failed to update model "+
							"with SQL:\n\n%s\n\n"+
							"Args:\n\n%v",
						t.sqlUpdate, ws)
				}
				n, err := res.RowsAffected()
				if err != nil {
					return errors.Errorf2From(
						err, "failed to determine the "+
							"number of rows "+
							"updated by\n\t%s\n\n"+
							"Args:\n\t%v",
						t.sqlUpdate, ws)
				}
				if n > 1 {
					return errors.Errorf2(
						"SQL:\n\t%v\n\nArgs:\n\t%v "+
							"updated %d rows, not "+
							"%d",
						n, 1)
				}
				if n == 1 {
					continue
				}
				logger.Verbose0(
					"No rows updated.  Attempting " +
						"insert...")
			}
		}
		_, err = t.insert(ctx, db, cmder, m)
		if err != nil {
			return errors.Errorf2From(
				err, "error while inserting %v into %v", m, db)
		}
	}
	return nil
}

// WithTx executes f with a *sql.Tx.  If there's already a transaction
// associated with the context, f uses that transaction.  Otherwise a new one
// is created and used.  If a new one was created and used, WithTx will commit
// it.
func (db *DB) WithTx(ctx context.Context, f func(ctx context.Context, tx *sql.Tx) error) (err error) {
	tx, borrowed := TxFromContext(ctx)
	if !borrowed {
		tx, err = db.DB.BeginTx(ctx, nil)
		if err != nil {
			return
		}
		ctx = AddTxToContext(ctx, tx)
	}
	if err := f(ctx, tx); err != nil {
		return tx.Rollback()
	}
	if !borrowed {
		return tx.Commit()
	}
	return nil
}

func cleanifaces(ifss ...[]interface{}) [][]interface{} {
	for i, ifs := range ifss {
		ifs = ifs[:cap(ifs)]
		for j := range ifs {
			ifs[j] = nil
		}
		ifss[i] = ifs[:0]
	}
	return ifss
}

func (db *DB) getifaces() (ifs []interface{}) {
	db.mu.Lock()
	ifs = db.getifacesLocked()
	db.mu.Unlock()
	if ifs == nil {
		ifs = make([]interface{}, 0, 8)
	}
	return
}

func (db *DB) getifacesLocked() (ifs []interface{}) {
	if len(db.freelists.ifaces) > 0 {
		i := len(db.freelists.ifaces) - 1
		ifs = db.freelists.ifaces[i]
		db.freelists.ifaces = db.freelists.ifaces[:i]
	}
	return
}

func (db *DB) putifaces(ifss ...[]interface{}) {
	ifss = cleanifaces(ifss...)
	db.mu.Lock()
	db.freelists.ifaces = append(db.freelists.ifaces, ifss...)
	db.mu.Unlock()
}

func (db *DB) putifacesLocked(ifss ...[]interface{}) {
	ifss = cleanifaces(ifss...)
	db.freelists.ifaces = append(db.freelists.ifaces, ifss...)
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
	strs = db.getstringsLocked()
	db.mu.Unlock()
	if strs == nil {
		strs = make([]string, 0, 0)
	}
	return
}

func (db *DB) getstringsLocked() (strs []string) {
	i := len(db.freelists.strs)
	if i > 0 {
		i--
		strs = db.freelists.strs[i]
		db.freelists.strs = db.freelists.strs[:i]
	}
	return
}

func (db *DB) putstrings(strss ...[]string) {
	strss = cleanstrings(strss...)
	db.mu.Lock()
	db.putstringsLocked(strss...)
	db.mu.Unlock()
}

func (db *DB) putstringsLocked(strss ...[]string) {
	db.freelists.strs = append(db.freelists.strs, strss...)
}

type table struct {
	sqlUpdate  string
	sqlInsert  string
	insertFunc func(t *table, ctx context.Context, db *DB, cmder sqlCmdImpl, m Model) (ids Model, err error)
	// idColNum is the column number (index + 1) of the ID column in the
	// table.  It's used to calculate which field to skip on inserts
	// into tables with auto-incrementing IDs.
	idColNum int
	columns  []column
	sqlName  string
	rawName  string
	goType   reflect.Type
}

// getTable gets or creates a table from the given value and model.  The value
// is passed separately from the model in case the model is a runtime-generated
// wrapper around a simple struct
func (db *DB) getTable(v interface{}, m Model) *table {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.getTableLocked(v, m)
}

func (db *DB) getTableLocked(v interface{}, m Model) *table {
	v = interfaceOf(v)
	tp := reflect.TypeOf(v)
	t, ok := db.tables[tp]
	if !ok {
		t = &table{
			goType: tp.Elem(),
		}
		db.tables[tp] = t
		t.initWithUnsafeDB(db, m, tp)
	}
	return t
}

// initWithUnsafeDB initializes a table within a DB.  This function
// must be run while holding db's mutex.
func (t *table) initWithUnsafeDB(db *DB, m Model, tp reflect.Type) {
	v, ok := interfaceOf2(m)
	if ok {
		tp = reflect.TypeOf(v)
	}
	logger.Verbose1("creating table for type %v", tp)
	if tnr, ok := v.(interface{ TableName() string }); ok {
		t.rawName = tnr.TableName()
	} else {
		switch {
		// TODO: Might need additional checks?
		case tp.Kind() == reflect.Ptr:
			t.rawName = tp.Elem().Name()
		default:
			t.rawName = tp.Name()
		}
	}
	t.sqlName = db.dialect.Escape(
		db.tableNamer.Apply(
			DefaultCase.Parse(t.rawName)))
	logger.Verbose3(
		"name of table for type %v: (raw: %v, sql: %v)",
		tp, t.rawName, t.sqlName)
	fields := m.AppendFields(db.getifacesLocked())
	names := m.AppendNames(db.getstringsLocked())
	sqlTypes := m.AppendSQLTypes(make([]sqltypes.Type, 0, len(fields)))
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
	idNames := db.getstringsLocked()
	ids, ok := db.idOf(nopLocker{}, m)
	if ok {
		idNames = ids.AppendNames(idNames)
	}
	idSet := make(map[string]struct{}, len(idNames))
	for _, n := range idNames {
		idSet[n] = struct{}{}
	}
	sb := db.getstringsLocked()
	defer db.putstringsLocked(idNames, sb)
	defer cleanstrings(idNames, sb)
	sb = append(sb, "INSERT INTO ", t.sqlName, " ( ")
	var nameIndexSkipID int
	for i, n := range names {
		// only omit single primary keys.  Composites are not ignored.
		if _, ok := idSet[n]; ok && len(idSet) == 1 {
			t.idColNum = i + 1
			continue
		}
		if nameIndexSkipID > 0 {
			sb = append(sb, ", ")
		}
		nameIndexSkipID++
		sb = append(sb, fieldNameToSQLName[n])
	}
	sb = append(sb, " ) ")
	if len(idNames) > 0 && db.dialect.Flags().HasAll(DialectOutputInfix) {
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
	// composite keys don't get excluded from insertion values
	delta := 0
	if len(idNames) == 1 {
		delta++
	}
	sb = append(sb, "VALUES ( ", qmarkAndCommas[:(len(names)-delta)*3-2], " )")
	if len(idNames) > 0 && db.dialect.Flags().HasAll(DialectReturningSuffix) {
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
		nameIndexSkipID = 0
		sb = append(sb, "UPDATE ", t.sqlName, " SET ")
		for _, n := range names {
			if _, ok := idSet[n]; ok && len(idNames) == 1 {
				continue
			}
			if nameIndexSkipID > 0 {
				sb = append(sb, ", ")
			}
			nameIndexSkipID++
			sb = append(sb, fieldNameToSQLName[n], " = ?")
		}
		sb = append(sb, " WHERE ")
		for i, n := range idNames {
			if i > 0 {
				sb = append(sb, " AND ")
			}
			sb = append(sb, fieldNameToSQLName[n], " = ?")
		}
		sb = append(sb, ";")
		t.sqlUpdate = strings.Join(sb, "")
	}
	logger.Verbose1("initialized table: %#v", t)
}

func (t *table) insert(ctx context.Context, db *DB, cmder sqlCmdImpl, m Model) (Model, error) {
	logger.Verbose2("inserting %v into %v...", spew.Sdump(m), db)
	ids, err := t.insertFunc(t, ctx, db, cmder, m)
	logger.Verbose4(
		"result of inserting %v into %v: (%v, %v)",
		spew.Sdump(m), db, spew.Sdump(ids), err)
	return ids, err
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
func (t *table) insertQuery(ctx context.Context, db *DB, cmder sqlCmdImpl, m Model) (ids Model, err error) {
	defer deferredInsertError(&err, m)
	ids, ok := db.IDOf(m)
	if !ok {
		return nil, errCannotDetermineID
	}
	logger.Verbose2("inserting %v with SQL:\n\t%v", m, t.sqlInsert)
	vs, fs := db.getifaces(), db.getifaces()
	defer db.putifaces(vs, fs)
	defer cleanifaces(vs, fs)
	err = cmder.QueryRowContext(ctx, t.sqlInsert, m.AppendValues(vs)...).Scan(ids.AppendFields(fs))
	logger.Verbose1("insert result: %v", m)
	return
}

// insertLastInsertID obtains the inserted result from res.LastInsertId
func (t *table) insertLastInsertID(ctx context.Context, db *DB, cmder sqlCmdImpl, m Model) (ids Model, err error) {
	defer deferredInsertError(&err, m)
	vs := db.getifaces()
	defer db.putifaces(vs)
	ids, ok := db.IDOf(m)
	if !ok {
		return nil, errCannotDetermineID
	}
	vs = ids.AppendFields(vs[:0])
	v := vs[0]
	pi64, ok := v.(*int64)
	if !ok {
		return nil, errors.Errorf(
			"cannot use %v with non-int64 ID: %T",
			errors.CallerName(0), v)
	}
	vs = m.AppendValues(vs[:0])
	switch t.idColNum {
	case 0:
		//pass
	case 1:
		vs = vs[1:]
	default:
		copy(vs[t.idColNum-1:], vs[t.idColNum:])
		vs = vs[:len(vs)-1]
	}
	logger.Verbose3(
		"inserting %v with SQL:\n\t%v\n\nArgs:\n\t%v",
		m, t.sqlInsert, vs)
	res, err := cmder.ExecContext(ctx, t.sqlInsert, vs...)
	if err != nil {
		return nil, errors.Errorf2From(
			err, "error while executing SQL:\n\n%s\n\nArgs:\n\n%v",
			t.sqlInsert, vs)
	}
	*pi64, err = res.LastInsertId()
	if err != nil {
		return nil, errors.Errorf0From(
			err, "failed to get last insert ID")
	}
	logger.Verbose1("inserted: %v", m)
	return ids, err
}

type column struct {
	// sqlName holds the pre-escaped column name
	sqlName string

	// sqlTypeName is the data type name of the column and varies depending
	// on the Dialect of the DB that owns this column's table.
	sqlTypeName string

	sqlType sqltypes.Type

	// rawName holds the un-escaped name of the column.
	rawName string

	// sqlDataType defines the Go type that data needs to be converted
	// to to be stored to SQL or converted from when retrieved from SQL.
	sqlConvType reflect.Type
}

/*
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
*/
// sqlDriverValueTypes maps GoSQLDataTypeKinds to their Go types.
var sqlDriverValueTypes = [...]reflect.Type{
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
	for _, t := range sqlDriverValueTypes {
		if t == from {
			return t
		}
	}
	for _, t := range sqlDriverValueTypes {
		if from.AssignableTo(t) && t.AssignableTo(from) {
			return t
		}
	}
	for _, t := range sqlDriverValueTypes {
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
