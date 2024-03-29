package expr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"reflect"
	"strings"
	"sync"

	"github.com/skillian/ctxutil"
	"github.com/skillian/logging"
)

const (
	// PkgName gets this package's name as a string constant
	PkgName = "github.com/skillian/expr"

	// Debug is set when expensive assertions should be checked.
	Debug = true

	// MoreUnsafe enables potentially unsupported unsafe code
	MoreUnsafe = true
)

var (
	logger = logging.GetLogger(PkgName)
)

// Expr is a basic expression
type Expr interface{}

var exprType = reflect.TypeOf((*Expr)(nil)).Elem()

// Tuple is a finite ordered sequence of expressions.
type Tuple []Expr

func (t Tuple) Eq(t2 Tuple) bool {
	if len(t) != len(t2) {
		return false
	}
	var vs eeValues
	for i, x := range t {
		vs.push(t2[i])
		et := typeOf(x)
		et.push(&vs, x)
		vs.pushType(et)
		if !et.eq(&vs) {
			return false
		}
	}
	return true
}

func (t Tuple) Operands() []Expr { return []Expr(t) }

// Unary is a unary (single-operand) expression, for example the negation
// operator.
type Unary interface {
	Operand() Expr
}

// Not negates its operand.
type Not [1]Expr

func (x Not) Operand() Expr { return x[0] }

func (x Not) String() string { return buildString(x) }

func (x Not) appendString(sb *strings.Builder) {
	sb.WriteString("(not ")
	appendString(sb, x[0])
	sb.WriteByte(')')
}

// Binary represents a binary (two-operand) expression such as addition in
// a + b.
type Binary interface {
	Operands() [2]Expr
}

// Eq checks for equality (i.e. a == b)
type Eq [2]Expr

func (x Eq) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Eq) String() string { return buildString(x) }

func (x Eq) appendString(sb *strings.Builder) { appendBinary(sb, "eq", x) }

// Ne checks for inequality (i.e. a != b)
type Ne [2]Expr

func (x Ne) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Ne) String() string { return buildString(x) }

func (x Ne) appendString(sb *strings.Builder) { appendBinary(sb, "ne", x) }

// Gt checks if the first operand is greater than the second (i.e. a > b)
type Gt [2]Expr

func (x Gt) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Gt) String() string { return buildString(x) }

func (x Gt) appendString(sb *strings.Builder) { appendBinary(sb, "gt", x) }

// Ge checks if the first operand is greater than or equal to the second
// (i.e. a >= b)
type Ge [2]Expr

func (x Ge) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Ge) String() string { return buildString(x) }

func (x Ge) appendString(sb *strings.Builder) { appendBinary(sb, "ge", x) }

// Lt checks if the first operand is greater than the second (i.e. a < b)
type Lt [2]Expr

func (x Lt) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Lt) String() string { return buildString(x) }

func (x Lt) appendString(sb *strings.Builder) { appendBinary(sb, "lt", x) }

// Le checks if the first operand is less than or equal to the second
// (i.e. a <= b)
type Le [2]Expr

func (x Le) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Le) String() string { return buildString(x) }

func (x Le) appendString(sb *strings.Builder) { appendBinary(sb, "le", x) }

// And performs a boolean AND operation of its two operands.
//
//	false && false == false
//	false && true == false
//	true && false == false
//	true && true == true
type And [2]Expr

func (x And) Operands() [2]Expr { return ([2]Expr)(x) }

func (x And) String() string { return buildString(x) }

func (x And) appendString(sb *strings.Builder) { appendBinary(sb, "and", x) }

// Or performs a boolean OR operation of its two operands.
//
//	false || false == false
//	false || true == true
//	true || false == true
//	true || true == true
type Or [2]Expr

func (x Or) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Or) String() string { return buildString(x) }

func (x Or) appendString(sb *strings.Builder) { appendBinary(sb, "or", x) }

// Add performs an arithmetic addition of its two operands (i.e. a + b)
type Add [2]Expr

func (x Add) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Add) String() string { return buildString(x) }

func (x Add) appendString(sb *strings.Builder) { appendBinary(sb, "+", x) }

// Sub performs an arithmetic addition of its two operands (i.e. a - b)
type Sub [2]Expr

func (x Sub) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Sub) String() string { return buildString(x) }

func (x Sub) appendString(sb *strings.Builder) { appendBinary(sb, "-", x) }

// Mul performs an arithmetic addition of its two operands (i.e. a * b)
type Mul [2]Expr

func (x Mul) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Mul) String() string { return buildString(x) }

func (x Mul) appendString(sb *strings.Builder) { appendBinary(sb, "*", x) }

// Div performs an arithmetic addition of its two operands (i.e. a /+ b)
type Div [2]Expr

func (x Div) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Div) String() string { return buildString(x) }

func (x Div) appendString(sb *strings.Builder) { appendBinary(sb, "/", x) }

// Mem selects a member of a source expression.
type Mem [2]Expr

var mems sync.Map // map[reflect.Type][]reflect.StructField

func MemOf(e Expr, base, field interface{}) Mem {
	getPtr := func(v interface{}) (rv reflect.Value, p uintptr) {
		rv = reflect.ValueOf(v)
		if !rv.IsValid() {
			panic("MemOf: base or field is invalid")
		}
		if rv.Kind() != reflect.Ptr {
			panic(fmt.Sprintf(
				"MemOf: base and field must be pointer, not %T",
				base,
			))
		}
		if rv.IsNil() {
			panic("MemOf: base or field is nil")
		}
		p = rv.Pointer()
		return
	}
	getStructFields := func(t reflect.Type) *[]reflect.StructField {
		k := interface{}(t)
		v, loaded := mems.Load(k)
		if loaded {
			return v.(*[]reflect.StructField)
		}
		offs := new([]reflect.StructField)
		*offs = make([]reflect.StructField, 0, t.NumField())
		for i := 0; i < cap(*offs); i++ {
			f := t.Field(i)
			if f.Type.Size() == 0 {
				continue
			}
			*offs = append(*offs, f)
		}
		v, loaded = mems.LoadOrStore(k, offs)
		if loaded {
			return v.(*[]reflect.StructField)
		}
		return offs
	}
	bv, bp := getPtr(base)
	fv, fp := getPtr(field)
	ft := fv.Elem().Type()
	foff := fp - bp
	structFields := *getStructFields(bv.Type().Elem())
	// TODO: Maybe binary search would be better, but I'm assuming tiny
	// numbers of fields for now:
	for i := range structFields {
		sf := &structFields[i]
		if sf.Offset == foff && sf.Type == ft {
			return Mem{e, sf}
		}
	}
	panic(fmt.Errorf(
		"unknown %T field at offset %d in %T",
		field, foff, base,
	))
}

func (x Mem) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Mem) String() string { return buildString(x) }

func (x Mem) appendString(sb *strings.Builder) { appendBinary(sb, ".", x) }

// Var is a placeholder for a runtime-determined value in an expression.
type Var interface {
	Expr

	// Var differentiates a Var from an Expr but could just
	// return itself.
	Var() Var
}

// Values is an ordered mapping of Vars to their values.
type Values interface {
	// Get the value associated with the given variable.
	Get(ctx context.Context, v Var) (interface{}, error)

	// Set the value associated with the given variable.
	Set(ctx context.Context, v Var, x interface{}) error

	// Vars returns an iterator through the values' variables.  The
	// returned iterator might be a VarValueIter.
	Vars() VarIter
}

// AddValuesToContext adds Values to the context so that expression
// evaluation can use them further down the stack
func AddValuesToContext(ctx context.Context, vs Values) context.Context {
	return ctxutil.WithValue(ctx, ValuesContextKey(), vs)
}

// ValuesFromContextOK attempts to retrieve the values from the context
// and returns false if the values cannot be found.
func ValuesFromContextOK(ctx context.Context) (vs Values, ok bool) {
	vs, ok = ctxutil.Value(ctx, ValuesContextKey()).(Values)
	return
}

var errNoValuesInContext = fmt.Errorf(
	"%w: failed to get values from context",
	ErrNotFound,
)

// ValuesFromContext attempts to retrieve expression variable values
// from a context and returns an error if the values are not found.
func ValuesFromContext(ctx context.Context) (vs Values, err error) {
	vs, ok := ValuesFromContextOK(ctx)
	if !ok {
		return nil, errNoValuesInContext
	}
	return
}

func GetOrAddValuesToContext(ctx context.Context) (context.Context, Values) {
	vs, ok := ValuesFromContextOK(ctx)
	if !ok {
		vs = NewValues()
		ctx = ctxutil.WithValue(ctx, ValuesContextKey(), vs)
	}
	return ctx, vs
}

// ValuesContextKey is the key value to context.Context.Value to retrieve
// Values from the context.
func ValuesContextKey() interface{} { return (*Values)(nil) }

// VarIter is a variable iterator.  Next must be called before Var to retrieve
// a valid Var.
type VarIter interface {
	// Next retrieves the next Var from the VarIter.
	Next(context.Context) error

	// Var retrieves the variable from the iterator.
	Var() Var
}

// VarValueIter is a VarIter that can also retrieve the value associated
// with the current variable.
type VarValueIter interface {
	VarIter
	VarValue(context.Context) VarValue
}

// valueList is an implementation of Values that keeps its keys
// and values in slices that are scanned sequentially.
type valueList struct {
	keys []Var
	vals []interface{}
}

var _ interface {
	Values
} = (*valueList)(nil)

// VarValue pairs together a variable and its value
type VarValue struct {
	Var   Var
	Value interface{}
}

// NewValues creates Values from a sequence of Vars and their values.
func NewValues(ps ...VarValue) Values {
	_, capacity := minMaxInt(1<<bits.Len(uint(len(ps))), 4)
	vs := &valueList{
		keys: make([]Var, len(ps), capacity),
		vals: make([]interface{}, len(ps), capacity),
	}
	for i, p := range ps {
		vs.keys[i] = p.Var
		vs.vals[i] = p.Value
	}
	return vs
}

// ErrNotFound indicates something wasn't found; it should be wrapped
// in another error that provides more context.
var ErrNotFound = errors.New("not found")

func (vs *valueList) Get(ctx context.Context, v Var) (interface{}, error) {
	for i, k := range vs.keys {
		if k == v {
			return vs.vals[i], nil
		}
	}
	return nil, ErrNotFound
}

func (vs *valueList) Set(ctx context.Context, v Var, x interface{}) error {
	for i, k := range vs.keys {
		if k == v {
			vs.vals[i] = x
			return nil
		}
	}
	vs.keys = append(vs.keys, v)
	vs.vals = append(vs.vals, x)
	return nil
}

type valueListIter struct {
	vs *valueList
	i  int
}

func (vs *valueList) Vars() VarIter { return &valueListIter{vs, 0} }

func (vli *valueListIter) Next(context.Context) error {
	if vli.i >= len(vli.vs.keys) {
		return io.EOF
	}
	vli.i++
	return nil
}

func (vli *valueListIter) Reset(context.Context) error {
	vli.i = 0
	return nil
}

func (vli *valueListIter) Var() Var {
	return vli.vs.keys[vli.i-1]
}

func (vli *valueListIter) VarValue(context.Context) VarValue {
	return VarValue{
		Var:   vli.vs.keys[vli.i-1],
		Value: vli.vs.vals[vli.i-1],
	}
}

type noValues struct{}

// NoValues returns a dummy Values that holds no values and cannot hold
// any values.
func NoValues() Values { return noValues{} }

func (noValues) ContextKey() interface{} { return ValuesContextKey() }
func (noValues) Get(context.Context, Var) (interface{}, error) {
	return nil, ErrNotFound
}
func (noValues) Set(context.Context, Var, interface{}) error {
	return ErrNotFound
}
func (noValues) Vars() VarIter                     { return noValues{} }
func (noValues) Next(context.Context) error        { return io.EOF }
func (noValues) Var() Var                          { return nil }
func (noValues) VarValue(context.Context) VarValue { return VarValue{} }

type varValueIter struct {
	Values
	VarIter
}

// EachVarValue iterates through the values and calls f with each Var and the
// value associated with that Var.
func EachVarValue(ctx context.Context, vs Values, f func(Var, interface{}) error) error {
	vvi := VarValueIterOf(vs)
	for {
		err := vvi.Next(ctx)
		if err == nil {
			vv := vvi.VarValue(ctx)
			err = f(vv.Var, vv.Value)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// MakeVarValueSlice makes a []VarValue slice from the vars and values in
// vs.
func MakeVarValueSlice(ctx context.Context, vs Values) ([]VarValue, error) {
	const arbitraryCapacity = 8
	length := arbitraryCapacity
	if vsLen, ok := tryLenOK(vs); ok {
		length = vsLen
	}
	vvs := make([]VarValue, length)
	err := EachVarValue(ctx, vs, func(v Var, i interface{}) error {
		vvs = append(vvs, VarValue{v, i})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create VarValue slice from %[1]v "+
				"(type: %[1]T): %[2]w",
			vs, err,
		)
	}
	return vvs, nil
}

// tryLenOK and tryLenErr attempt to get the length of _something_.
// tryLenOK doesn't waste resources building an error if the value
// doesn't have a concept of length.  tryLenErr will report an error
// message if the value doesn't have a length.
var tryLenOK, _ = func() (func(v interface{}) (int, bool), func(v interface{}) (int, error)) {
	type tryLenErr int
	const (
		ok tryLenErr = iota
		lenErr
		nilPtr
		badType
		tooDeep
	)
	var tryLenImpl func(v interface{}, depth int) (int, tryLenErr, error)
	tryLenImpl = func(v interface{}, depth int) (int, tryLenErr, error) {
		switch v := v.(type) {
		case interface{ Len() int }:
			return v.Len(), ok, nil
		case interface{ Len() (int, error) }:
			n, err := v.Len()
			return n, lenErr, err
		default:
			rv := reflect.ValueOf(v)
			if !rv.IsValid() {
				return 0, nilPtr, nil
			}
			switch rv.Kind() {
			case reflect.Ptr:
				if depth > 0 {
					return 0, tooDeep, nil
				}
				if rv.IsNil() {
					return 0, nilPtr, nil
				}
				rv = rv.Elem()
				if !rv.CanInterface() {
					return 0, badType, nil
				}
				return tryLenImpl(rv.Interface(), depth+1)
			case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
				return rv.Len(), ok, nil
			}
			return 0, badType, nil
		}
	}
	return func(v interface{}) (int, bool) {
			length, tryErr, _ := tryLenImpl(v, 0)
			if tryErr != ok {
				return 0, false
			}
			return length, true
		}, func(v interface{}) (int, error) {
			length, tryErr, err := tryLenImpl(v, 0)
			if err != nil {
				return 0, err
			}
			switch tryErr {
			case ok:
				return length, nil
			case lenErr:
				return 0, err
			case nilPtr:
				return 0, fmt.Errorf(
					"%w: cannot get length of nil pointer",
					ErrInvalidType,
				)
			case badType:
				return 0, fmt.Errorf("%w: %T", ErrInvalidType, v)
			case tooDeep:
				return 0, fmt.Errorf(
					"%w: cannot get length of pointer to "+
						"pointer: %[2]v (type: %[2]T)",
					ErrInvalidType, v,
				)
			}
			panic(fmt.Errorf("unhandled switch case: %v", tryErr))
		}
}()

// VarValueIterOf creates a VarValueIter from the values.  If the Values'
// Vars function already returns a VarValueIter implementation, that
// implementation is returned directly.  If the implementation is only a
// VarIter, a wrapper is returned that implements VarValueIter by calling
// Get for each Var returned by the iterator.
func VarValueIterOf(vs Values) VarValueIter {
	vi := vs.Vars()
	if vvi, ok := vi.(VarValueIter); ok {
		return vvi
	}
	return varValueIter{vs, vi}
}

func (vvi varValueIter) VarValue(ctx context.Context) VarValue {
	va := vvi.Var()
	v, _ := vvi.Get(ctx, va)
	return VarValue{Var: va, Value: v}
}

func buildString(x interface{}) string {
	sb := strings.Builder{}
	appendString(&sb, x)
	return sb.String()
}

func appendString(sb *strings.Builder, x interface{}) {
	switch x := x.(type) {
	case interface{ appendString(sb *strings.Builder) }:
		x.appendString(sb)
	default:
		sb.WriteString(fmt.Sprint(x))
	}
}

func appendBinary(sb *strings.Builder, op string, b Binary) {
	ops := b.Operands()
	sb.WriteByte('(')
	sb.WriteString(op)
	sb.WriteByte(' ')
	appendString(sb, ops[0])
	sb.WriteByte(' ')
	appendString(sb, ops[1])
	sb.WriteByte(')')
}

// HasOperands returns true if the expression has operands
func HasOperands(e Expr) bool {
	switch e.(type) {
	case Unary, Binary:
		return true
	case interface{ Operands() []Expr }:
		return true
	}
	return false
}

// Operands gets the operands of the given expression as a slice.
// If the expression has no operands, nil is returned.
func Operands(e Expr) []Expr {
	switch e := e.(type) {
	case Unary:
		return []Expr{e.Operand()}
	case Binary:
		ops := e.Operands()
		return ops[:]
	case interface{ Operands() []Expr }:
		return e.Operands()
	}
	return nil
}

// Rewrite an expression tree by passing each node to f.
func Rewrite(e Expr, f func(Expr) Expr) Expr {
	switch e2 := e.(type) {
	case Unary:
		op := e2.Operand()
		op2 := f(op)
		if !eq(op, op2) {
			arr := reflect.New(reflect.TypeOf(e)).Elem()
			arr.Index(0).Set(reflect.ValueOf(op2))
			e = arr.Interface()
		}
	case Binary:
		ops := e2.Operands()
		ops2 := [2]Expr{f(ops[0]), f(ops[1])}
		if !eq(ops[0], ops2[0]) || !eq(ops[1], ops2[1]) {
			arr := reflect.New(reflect.TypeOf(e)).Elem()
			arr.Index(0).Set(reflect.ValueOf(ops2[0]))
			arr.Index(1).Set(reflect.ValueOf(ops2[1]))
			e = arr.Interface()
		}
	default:
	}
	return f(e)
}

type Visitor interface {
	Visit(context.Context, Expr) (Visitor, error)
}

type VisitFunc func(context.Context, Expr) (Visitor, error)

func (f VisitFunc) Visit(ctx context.Context, e Expr) (Visitor, error) {
	return f(ctx, e)
}

func Walk(ctx context.Context, e Expr, v Visitor, options ...WalkOption) error {
	var cfg walkConfig
	for _, opt := range options {
		opt(&cfg)
	}
	w, err := v.Visit(ctx, e)
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	ops := Operands(e)
	if cfg.flags&walkBackwards == walkBackwards {
		for i := len(ops) - 1; i >= 0; i-- {
			if err = Walk(ctx, ops[i], w, options...); err != nil {
				return err
			}
		}
	} else {
		for _, op := range ops {
			if err = Walk(ctx, op, w, options...); err != nil {
				return err
			}
		}
	}
	_, err = w.Visit(ctx, nil)
	return err
}

type walkConfig struct {
	flags walkFlag
}

type walkFlag uint8

const (
	walkBackwards walkFlag = 1 << iota
)

// WalkOption is an option to the Walk function.
type WalkOption func(c *walkConfig)

// WalkOperandsBackwards walks the operands of the expression backwards
// which is useful for the internal compilation process which generates
// virtual machine instructions.
func WalkOperandsBackwards() WalkOption {
	return func(c *walkConfig) {
		c.flags |= walkBackwards
	}
}
