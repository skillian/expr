package sqlstream

import (
	"math/bits"
	"reflect"
	"sync"

	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/internal"
	"github.com/skillian/expr/stream/sqlstream/sqltypes"
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
	AppendSQLTypes(ts []sqltypes.Type) []sqltypes.Type
}

type UnnamedModel interface {
	fieldsAppender
	valuesAppender
	sqlTypesAppender
}

// ModelWithNames creates a model from a type that implements
// the model interface except for AppendNames
func ModelWithNames(m UnnamedModel, names ...string) Model {
	return unnamedModel{m, names}
}

type unnamedModel struct {
	UnnamedModel
	names []string
}

func (um unnamedModel) AppendNames(ns []string) []string {
	return append(ns, um.names...)
}

// Model describes rows of data, such as those returned from tables, views,
// stored procedures, functions, etc.
//
// AppendValues and AppendFields are used to access and mutate, respectively,
// the fields within the model.  AppendNames and AppendSQLTypes are used to
// match fields/values together with result sets coming from the underlying
// driver(s).
//
// These functions append to slices instead of allocating new slices so that
// callers can pre-allocate slices or easily compound slices of multiple models.
type Model interface {
	fieldsAppender
	namesAppender
	valuesAppender
	sqlTypesAppender
}

// Modeler is implemented by types that aren't models, but can produce
// their models.
type Modeler interface {
	Model() Model
}

// IDModeler returns a Model implementation that describes its unique
// ID.  The ID model must be a subset of the Model itself and its fields must
// occur in the same order as its owning Model's fields.
type IDModeler interface {
	ID() Model
}

// zeroValues is a hash set of zero values for common SQL field types
var zeroValues = map[interface{}]struct{}{
	false:     {},
	int(0):    {},
	int8(0):   {},
	int16(0):  {},
	int32(0):  {},
	int64(0):  {},
	uint(0):   {},
	uint8(0):  {},
	uint16(0): {},
	uint32(0): {},
	uint64(0): {},
	"":        {},
}

// valuesAreZero checks if all of the values in the slice are zero
// values.
//
// This function is used by, for example, the (*DB).Save function to check if
// a model already has values for its ID fields.  If so, it is assumed that
// Save should update existing records.  Otherwise, Save should insert new
// records.
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
// package.  By defining it here, the same code can work with both database/sql
// types.
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

// CompoundOf converts its operands to Models
func CompoundOf(vs ...interface{}) (cm CompoundModel, err error) {
	cm = make(CompoundModel, len(vs))
	for i, v := range vs {
		if cm[i], err = ModelOf(v); err != nil {
			return nil, errors.Errorf2From(
				err, "failed to create model from parameter "+
					"%d: %v",
				i, v)
		}
	}
	return
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
func (c CompoundModel) AppendSQLTypes(ts []sqltypes.Type) []sqltypes.Type {
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
	case Modeler:
		return v.Model(), nil
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
			fv := rv.FieldByIndex(f.Index)
			ft := fv.Type()
			ct := reflect.PtrTo(getConvertType(ft))
			fp := fv.Addr()
			fields[i] = fp.Convert(ct).Interface()
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

// ModelsOf creates a slice of models from the given values.
func ModelsOf(vs ...interface{}) ([]Model, error) {
	ms := make([]Model, len(vs))
	for i, v := range vs {
		m, err := ModelOf(v)
		if err != nil {
			return nil, err
		}
		ms[i] = m
	}
	return ms, nil
}

// ModelsOf2 calls ModelOf on each parameter and returns their models in order.
func ModelsOf2(v0, v1 interface{}) (m0, m1 Model, err error) {
	if m0, err = ModelOf(v0); err != nil {
		return
	}
	m1, err = ModelOf(v1)
	return
}

// ModelsOf3 calls ModelOf on each parameter and returns their models in order.
func ModelsOf3(v0, v1, v2 interface{}) (m0, m1, m2 Model, err error) {
	if m0, m1, err = ModelsOf2(v0, v1); err != nil {
		return
	}
	m2, err = ModelOf(v2)
	return
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

var (
	modelFieldDefs sync.Map
	int64PtrType   = reflect.TypeOf((*int64)(nil))
)

// ModelField creates a model from the given field.  It's useful for defining
// a struct's ID model.
func ModelField(struc, field interface{}) Model {
	s := internal.IfaceOf(struc)
	f := internal.IfaceOf(field)
	type key struct {
		structType internal.IfaceType
		offset     uintptr
	}
	type value struct {
		sf reflect.StructField
	}
	k := key{s.Type(), f.Uintptr() - s.Uintptr()}
	var ki interface{} = k
	var v *value
	x, loaded := modelFieldDefs.Load(ki)
	if !loaded {
		rt := reflect.TypeOf(struc).Elem()
		nf := rt.NumField()
		for i := 0; i < nf; i++ {
			sf := rt.Field(i)
			if sf.Offset == k.offset {
				v = &value{sf: sf}
				break
			}
		}
		if v == nil {
			panic(errors.Errorf2(
				"failed to find %[1]p in %[2]p (type: %[2]T)",
				field, struc))
		}
		x, loaded = modelFieldDefs.LoadOrStore(ki, v)
	}
	if loaded {
		v = x.(*value)
	}
	switch f := field.(type) {
	case *int64:
		return Int64Model{ID: f, Name: v.sf.Name}
	}
	// TODO:
	// handle converibles to *int64 And struct ptrs whose first fields are
	// convertible to int64
	panic("TODO")
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

func (m reflectModel) AppendSQLTypes(ts []sqltypes.Type) []sqltypes.Type {
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
		var idFields []interface{}
		var idNames []string
		var idTypes []sqltypes.Type
		if idm, ok := v.(IDModeler); ok {
			ids := idm.ID()
			idFields = ids.AppendFields(idFields)
			idNames = ids.AppendNames(idNames)
			idTypes = ids.AppendSQLTypes(idTypes)
			logger.Verbose3(
				"%[1]v (type: %[1]T) idFields: %[2]v, "+
					"idTypes: %[3]v",
				v, idFields, idTypes)
		}
		sqlTypes := make([]sqltypes.Type, len(rmt.fields))
		rv := reflect.ValueOf(v).Elem()
		for i, f := range rmt.fields {
			sf := rt.FieldByIndex(f.Index)
			st := &sqlTypes[i]
			id := -1
			{
				fp := rv.FieldByIndex(sf.Index).Addr().Pointer()
				for i, idf := range idFields {
					eq := fp == reflect.ValueOf(idf).Pointer() &&
						sf.Name == idNames[i]
					logger.Verbose2("%v is ID: %v", sf, eq)
					if eq {
						id = i
						break
					}
				}
			}
			if id != -1 {
				*st = idTypes[id]
				logger.Verbose1(
					"Determined that %[1]v (type: %[1]T) "+
						"is ID via IDModeler", *st)
				continue
			}
			if tag, ok := sf.Tag.Lookup("sqlstreamtype"); ok {
				sqlt, err := sqltypes.Parse(tag)
				if err != nil {
					panic(errors.Errorf2From(
						err, "failed to parse SQL "+
							"type from struct "+
							"field %q of type %q",
						sf.Name, rt))
				}
				*st = sqlt
				continue
			}
			switch sf.Type.Kind() {
			case reflect.Bool:
				*st = sqltypes.Bool
			case reflect.Int, reflect.Int64:
				*st = sqltypes.Int64
			case reflect.Int32:
				*st = sqltypes.IntType{Bits: 32}
			case reflect.Int16:
				*st = sqltypes.IntType{Bits: 16}
			case reflect.Int8:
				*st = sqltypes.IntType{Bits: 8}
			case reflect.Float32:
				*st = sqltypes.FloatType{Mantissa: 24}
			case reflect.Float64:
				*st = sqltypes.FloatType{Mantissa: 53}
			case reflect.String:
				*st = sqltypes.StringType{Var: true}
			default:
				switch {
				case sf.Type.AssignableTo(typeOfSQLDecimal):
					*st = sqltypes.DecimalType{}
				case sf.Type.AssignableTo(typeOfByteSlice):
					*st = sqltypes.BytesType{Var: true}
				default:
					panic(errors.Errorf2(
						"cannot determine the SQL "+
							"type of field %q in "+
							"type %v",
						sf.Name, rt))
				}
			}
		}
		// TODO: Nothing here benefits from being "unbound"
		// Maybe it was a premature optimization?
		rmt.ta = reflectModelUnboundSQLTypesAppenderFunc(func(m *reflectModel, ts []sqltypes.Type) []sqltypes.Type {
			return append(ts, sqlTypes...)
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

type reflectModelUnboundSQLTypesAppenderFunc func(m *reflectModel, ts []sqltypes.Type) []sqltypes.Type

func (f reflectModelUnboundSQLTypesAppenderFunc) bind(m *reflectModel) reflectModelBoundSQLTypesAppenderFunc {
	return reflectModelBoundSQLTypesAppenderFunc{m: m, f: f}
}

type reflectModelBoundSQLTypesAppenderFunc struct {
	m *reflectModel
	f reflectModelUnboundSQLTypesAppenderFunc
}

func (ta reflectModelBoundSQLTypesAppenderFunc) AppendSQLTypes(ts []sqltypes.Type) []sqltypes.Type {
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

// AppendSQLTypes appends sqltypes.Int64 to ts
func (m Int64Model) AppendSQLTypes(ts []sqltypes.Type) []sqltypes.Type {
	return append(ts, sqltypes.Int64)
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

func (m nopID) AppendSQLTypes(vs []sqltypes.Type) []sqltypes.Type {
	return m.m.AppendSQLTypes(vs)
}
