package sqlstream

import (
	"math/bits"
	"reflect"
	"sync"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqltype"
)

type fieldsAppender interface {
	// AppendFields appends pointers to the fields of the model.  These
	// pointers are then passed to (*sql.Rows).Scan when scanning database
	// results into the model.
	AppendFields(fs []interface{}) []interface{}
}

type fieldsAppenderFunc func(fs []interface{}) []interface{}

func (fa fieldsAppenderFunc) AppendFields(fs []interface{}) []interface{} {
	return fa(fs)
}

type namesAppender interface {
	// AppendNames appends the names of the model's fields to ns.  The
	// *DB type has a column renamer that will be applied to the columns.
	AppendNames(ns []string) []string
}

type namesAppenderFunc func(ns []string) []string

func (na namesAppenderFunc) AppendNames(ns []string) []string {
	return na(ns)
}

type valuesAppender interface {
	// AppendValues appends the values of the fields in the model to vs.
	AppendValues(vs []interface{}) []interface{}
}

type valuesAppenderFunc func(vs []interface{}) []interface{}

func (va valuesAppenderFunc) AppendValues(vs []interface{}) []interface{} {
	return va(vs)
}

type sqlTypesAppender interface {
	// AppendSQLTypes appends the SQL types for the model's fields.
	AppendSQLTypes(ts []sqltype.Type) []sqltype.Type
}

// Model describes data that can be queried as rows from a relational
// database.
type Model interface {
	fieldsAppender
	namesAppender
	valuesAppender
	sqlTypesAppender
}

// IDModeler returns a Model implementation that describes its unique
// ID.
type IDModeler interface {
	ID() Model
}

// zeroValues is a hash set of zero values for common SQL field types
var zeroValues = map[interface{}]struct{}{
	false:     struct{}{},
	int(0):    struct{}{},
	int8(0):   struct{}{},
	int16(0):  struct{}{},
	int32(0):  struct{}{},
	int64(0):  struct{}{},
	uint(0):   struct{}{},
	uint8(0):  struct{}{},
	uint16(0): struct{}{},
	uint32(0): struct{}{},
	uint64(0): struct{}{},
	"":        struct{}{},
}

// valuesAreZero checks if all of the values in the slice are zero
// values.
func valuesAreZero(vs []interface{}) bool {
	if len(vs) == 0 {
		return true
	}
	for _, v := range vs {
		if v == nil {
			continue
		}
		if _, ok := zeroValues[v]; ok {
			continue
		}
		switch v := v.(type) {
		case []byte:
			if len(v) > 0 {
				return false
			}
			continue
		}
		return false
	}
	return true
}

// Scanner is implemented by *sql.Row and *sql.Rows in the go database/sql
// package
type Scanner interface {
	Scan(vs ...interface{}) error
}

// CompoundModel is an ordered sequence of models that together are treated as
// a single model
type CompoundModel []Model

// Compound creates a CompoundModel from other models
func Compound(ms ...Model) CompoundModel {
	cm := make(CompoundModel, 0, 1<<bits.Len(uint(len(ms))))
	for _, m := range ms {
		if c, ok := m.(CompoundModel); ok {
			cm = append(cm, c...)
			continue
		}
		cm = append(cm, m)
	}
	return cm
}

// AppendFields appends the fields of all the inner models to fs.
func (c CompoundModel) AppendFields(fs []interface{}) []interface{} {
	for _, m := range c {
		fs = m.AppendFields(fs)
	}
	return fs
}

// AppendNames implements the Model interface.
func (c CompoundModel) AppendNames(ns []string) []string {
	for _, m := range c {
		ns = m.AppendNames(ns)
	}
	return ns
}

// AppendValues appends the values of all the inner models to vs.
func (c CompoundModel) AppendValues(vs []interface{}) []interface{} {
	for _, m := range c {
		vs = m.AppendValues(vs)
	}
	return vs
}

// AppendSQLTypes appends the SQL types of all the inner models to ts.
func (c CompoundModel) AppendSQLTypes(ts []sqltype.Type) []sqltype.Type {
	for _, m := range c {
		ts = m.AppendSQLTypes(ts)
	}
	return ts
}

type reflectModel struct {
	self interface{}
	rmt  *reflectModelType
	fa   fieldsAppender
}

// ModelOf creates a model from the given value.
func ModelOf(v interface{}) (Model, error) {
	switch v := v.(type) {
	case Model:
		return v, nil
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, errors.Errorf0("value is invalid")
	}
	if rv.Kind() != reflect.Ptr {
		return nil, errors.Errorf0("value must be a pointer to struct")
	}
	if rv.IsNil() {
		return nil, errors.Errorf0("pointer cannot be nil")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return nil, errors.Errorf1(
			"model must be a pointer to struct, not %T", v)
	}
	rt := rv.Type()
	var k interface{} = rt
	var rmt *reflectModelType
	x, loaded := reflectModelTypes.Load(k)
	if !loaded {
		rmt = newReflectModelType(v, rt)
		x, loaded = reflectModelTypes.LoadOrStore(k, rmt)
	}
	if loaded {
		rmt = x.(*reflectModelType)
	}
	// fieldsAppender is handled per-model instead of by the reflectModelType
	// because we can create this fields slice once for each model and keep
	// reusing it without resorting to reflection ever again.
	fa, ok := v.(fieldsAppender)
	if !ok {
		fields := make([]interface{}, len(rmt.fields))
		for i, f := range rmt.fields {
			fields[i] = rv.FieldByIndex(f.Index).Addr().Interface()
		}
		fa = fieldsAppenderFunc(func(fs []interface{}) []interface{} {
			return append(fs, fields...)
		})
	}
	m := &reflectModel{
		self: v,
		rmt:  rmt,
		fa:   fa,
	}
	return m, nil
}

// MustModelOf must succeed in creating a model from a value or else it will
// panic.
func MustModelOf(v interface{}) Model {
	m, err := ModelOf(v)
	if err != nil {
		panic(err)
	}
	return m
}

func (m reflectModel) AppendFields(fs []interface{}) []interface{} {
	return m.fa.AppendFields(fs)
}

func (m reflectModel) AppendNames(ns []string) []string {
	if m.rmt.na == nil {
		return m.self.(namesAppender).AppendNames(ns)
	}
	return m.rmt.na.bind(&m).AppendNames(ns)
}

func (m reflectModel) AppendValues(vs []interface{}) []interface{} {
	if m.rmt.va == nil {
		return m.self.(valuesAppender).AppendValues(vs)
	}
	return m.rmt.va.bind(&m).AppendValues(vs)
}

func (m reflectModel) AppendSQLTypes(ts []sqltype.Type) []sqltype.Type {
	if m.rmt.ta == nil {
		return m.self.(sqlTypesAppender).AppendSQLTypes(ts)
	}
	return m.rmt.ta.bind(&m).AppendSQLTypes(ts)
}

func (m reflectModel) Interface() interface{} { return m.self }

var (
	reflectModelTypes sync.Map

	// typeOfSQLDecimal's definition is from https://github.com/golang-sql/decomposer
	typeOfSQLDecimal = reflect.TypeOf((*interface {
		Decompose(buf []byte) (form byte, negative bool, coefficient []byte, exponent int32)
		Compose(form byte, negative bool, coefficient []byte, exponent int32) error
	})(nil)).Elem()

	typeOfByteSlice = reflect.TypeOf(([]byte)(nil))
)

type reflectModelType struct {
	fields []reflectStructField
	fa     reflectModelUnboundFieldsAppenderFunc
	va     reflectModelUnboundValuesAppenderFunc
	na     reflectModelUnboundNamesAppenderFunc
	ta     reflectModelUnboundSQLTypesAppenderFunc
}

func newReflectModelType(v interface{}, rt reflect.Type) *reflectModelType {
	nf := rt.NumField()
	// try to keep data close:
	indexes := make([]int, 0, 16)
	indexStartStops := make([][2]int, nf)
	rmt := &reflectModelType{
		fields: make([]reflectStructField, 0, nf),
	}
	for i := 0; i < nf; i++ {
		sf := rt.Field(i)
		start := len(indexes)
		indexes = append(indexes, sf.Index...)
		indexStartStops[i] = [2]int{start, len(indexes)}
		fi := len(rmt.fields)
		rmt.fields = append(rmt.fields, reflectStructField{
			Type: sf.Type,
		})
		rsf, name := &rmt.fields[fi], sf.Name
		rsf.convertTo = getConvertType(rsf.Type)
		if rsf.convertTo != nil {
			logger.Verbose(
				"%v.%s (type: %v, index: %v) will convert to %v",
				v, name, rsf.Type,
				indexes[indexStartStops[fi][0]:indexStartStops[fi][1]],
				rsf.convertTo)
		}
	}
	for i := range rmt.fields {
		startStop := indexStartStops[i]
		rmt.fields[i].Index = indexes[startStop[0]:startStop[1]]
	}
	// Note: fieldsAppender is handled per-model
	if _, ok := v.(valuesAppender); !ok {
		rmt.va = reflectModelUnboundValuesAppenderFunc(func(m *reflectModel, vs []interface{}) []interface{} {
			rv := reflect.ValueOf(m.self).Elem()
			for _, f := range rmt.fields {
				vs = append(vs, rv.FieldByIndex(f.Index).Interface())
			}
			return vs
		})
	}
	if _, ok := v.(namesAppender); !ok {
		names := make([]string, len(rmt.fields))
		for i, f := range rmt.fields {
			sf := rt.FieldByIndex(f.Index)
			if tag, ok := sf.Tag.Lookup("sqlstreamname"); ok {
				names[i] = tag
			} else {
				names[i] = sf.Name
			}
		}
		// TODO: Nothing here benefits from being "unbound"
		// Maybe it was a premature optimization?
		rmt.na = reflectModelUnboundNamesAppenderFunc(func(m *reflectModel, ns []string) []string {
			return append(ns, names...)
		})
	}
	if _, ok := v.(sqlTypesAppender); !ok {
		sqltypes := make([]sqltype.Type, len(rmt.fields))
		for i, f := range rmt.fields {
			sf := rt.FieldByIndex(f.Index)
			if tag, ok := sf.Tag.Lookup("sqlstreamtype"); ok {
				st, err := sqltype.Parse(tag)
				if err != nil {
					panic(errors.Errorf2From(
						err, "failed to parse SQL "+
							"type from struct "+
							"field %q of type %q",
						sf.Name, rt))
				}
				sqltypes[i] = st
				continue
			}
			primary := false
			switch sf.Type.Kind() {
			case reflect.Bool:
				sqltypes[i] = sqltype.Bool
			case reflect.Int, reflect.Int64:
				sqltypes[i] = sqltype.Int64
				primary = i == 0
			case reflect.Int32:
				sqltypes[i] = sqltype.IntType{Bits: 32}
				primary = i == 0
			case reflect.Int16:
				sqltypes[i] = sqltype.IntType{Bits: 16}
				primary = i == 0
			case reflect.Int8:
				sqltypes[i] = sqltype.IntType{Bits: 8}
			case reflect.Float32:
				sqltypes[i] = sqltype.FloatType{Mantissa: 24}
			case reflect.Float64:
				sqltypes[i] = sqltype.FloatType{Mantissa: 53}
			case reflect.String:
				sqltypes[i] = sqltype.StringType{Var: true}
				primary = i == 0
			default:
				switch {
				case sf.Type.AssignableTo(typeOfSQLDecimal):
					sqltypes[i] = sqltype.DecimalType{}
					primary = i == 0
				case sf.Type.AssignableTo(typeOfByteSlice):
					sqltypes[i] = sqltype.BytesType{Var: true}
					primary = i == 0
				default:
					panic(errors.Errorf2(
						"cannot determine the SQL "+
							"type of field %q in "+
							"type %v",
						sf.Name, rt))
				}
			}
			if primary {
				sqltypes[i] = sqltype.Primary{sqltypes[i]}
			}
		}
		// TODO: Nothing here benefits from being "unbound"
		// Maybe it was a premature optimization?
		rmt.ta = reflectModelUnboundSQLTypesAppenderFunc(func(m *reflectModel, ts []sqltype.Type) []sqltype.Type {
			return append(ts, sqltypes...)
		})
	}
	return rmt
}

type reflectModelUnboundFieldsAppenderFunc func(m *reflectModel, fs []interface{}) []interface{}

func (f reflectModelUnboundFieldsAppenderFunc) bind(m *reflectModel) reflectModelBoundFieldsAppender {
	return reflectModelBoundFieldsAppender{m: m, f: f}
}

type reflectModelBoundFieldsAppender struct {
	m *reflectModel
	f reflectModelUnboundFieldsAppenderFunc
}

func (fa reflectModelBoundFieldsAppender) AppendFields(fs []interface{}) []interface{} {
	return fa.f(fa.m, fs)
}

type reflectModelUnboundValuesAppenderFunc func(m *reflectModel, vs []interface{}) []interface{}

func (f reflectModelUnboundValuesAppenderFunc) bind(m *reflectModel) reflectModelBoundValuesAppender {
	return reflectModelBoundValuesAppender{m: m, f: f}
}

type reflectModelBoundValuesAppender struct {
	m *reflectModel
	f reflectModelUnboundValuesAppenderFunc
}

func (va reflectModelBoundValuesAppender) AppendValues(vs []interface{}) []interface{} {
	return va.f(va.m, vs)
}

type reflectModelUnboundNamesAppenderFunc func(m *reflectModel, ns []string) []string

func (f reflectModelUnboundNamesAppenderFunc) bind(m *reflectModel) reflectModelBoundNamesAppender {
	return reflectModelBoundNamesAppender{m: m, f: f}
}

type reflectModelBoundNamesAppender struct {
	m *reflectModel
	f reflectModelUnboundNamesAppenderFunc
}

func (na reflectModelBoundNamesAppender) AppendNames(ns []string) []string {
	return na.f(na.m, ns)
}

type reflectModelUnboundSQLTypesAppenderFunc func(m *reflectModel, ts []sqltype.Type) []sqltype.Type

func (f reflectModelUnboundSQLTypesAppenderFunc) bind(m *reflectModel) reflectModelBoundSQLTypesAppenderFunc {
	return reflectModelBoundSQLTypesAppenderFunc{m: m, f: f}
}

type reflectModelBoundSQLTypesAppenderFunc struct {
	m *reflectModel
	f reflectModelUnboundSQLTypesAppenderFunc
}

func (ta reflectModelBoundSQLTypesAppenderFunc) AppendSQLTypes(ts []sqltype.Type) []sqltype.Type {
	return ta.f(ta.m, ts)
}

type reflectStructField struct {
	convertTo reflect.Type
	Type      reflect.Type
	Index     []int
}

// Int64Model can be returned by models' ID functions
type Int64Model struct {
	ID   *int64
	Name string
}

// AppendValues appends the ID to the slice of values
func (m Int64Model) AppendValues(vs []interface{}) []interface{} { return append(vs, *m.ID) }

// AppendNames appends the ID's Name to the names
func (m Int64Model) AppendNames(ns []string) []string { return append(ns, m.Name) }

// AppendFields appends the address of the ID to the fields
func (m Int64Model) AppendFields(fs []interface{}) []interface{} { return append(fs, m.ID) }

// AppendSQLTypes appends sqltype.Int64 to ts
func (m Int64Model) AppendSQLTypes(ts []sqltype.Type) []sqltype.Type {
	return append(ts, sqltype.Int64)
}

type nopID struct {
	m Model
}

func (m nopID) AppendFields(fs []interface{}) []interface{} {
	return m.m.AppendFields(fs)
}

func (m nopID) AppendNames(ns []string) []string {
	return m.m.AppendNames(ns)
}

func (m nopID) AppendValues(vs []interface{}) []interface{} {
	return m.m.AppendFields(vs)
}

func (m nopID) AppendSQLTypes(vs []sqltype.Type) []sqltype.Type {
	return m.m.AppendSQLTypes(vs)
}
