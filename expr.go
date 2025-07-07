package expr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"reflect"
	"strings"

	"github.com/skillian/ctxutil"
	"github.com/skillian/logging"
	"github.com/skillian/unsafereflect"
)

const (
	// PkgName gets this package's name as a string constant
	PkgName = "github.com/skillian/expr"

	// Debug is set when expensive assertions should be checked.
	Debug = true

	// MoreUnsafe enables potentially unsupported unsafe code
	MoreUnsafe = true
)

// Expr is a basic expression
type Expr interface{}

var (
	exprType = reflect.TypeOf((*Expr)(nil)).Elem()
	logger   = logging.GetLogger(PkgName)
)

// Tuple is a finite ordered sequence of expressions.
type Tuple []Expr

func MakeTuple(length int) Tuple {
	return make(Tuple, length)
}

func (t Tuple) Eq(t2 Tuple) bool {
	if len(t) != len(t2) {
		return false
	}
	vs := quickSimpleEEValues()
	for i, x := range t {
		vs.push(t2[i])
		vs.push(x)
		if !vs.peekType().eq(vs) {
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

func (x Not) writeStringToStringBuilder(sb *strings.Builder) {
	sb.WriteString("(not ")
	writeStringToStringBuilder(sb, x[0])
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

func (x Eq) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "eq", x) }

// Ne checks for inequality (i.e. a != b)
type Ne [2]Expr

func (x Ne) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Ne) String() string { return buildString(x) }

func (x Ne) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "ne", x) }

// Gt checks if the first operand is greater than the second (i.e. a > b)
type Gt [2]Expr

func (x Gt) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Gt) String() string { return buildString(x) }

func (x Gt) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "gt", x) }

// Ge checks if the first operand is greater than or equal to the second
// (i.e. a >= b)
type Ge [2]Expr

func (x Ge) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Ge) String() string { return buildString(x) }

func (x Ge) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "ge", x) }

// Lt checks if the first operand is greater than the second (i.e. a < b)
type Lt [2]Expr

func (x Lt) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Lt) String() string { return buildString(x) }

func (x Lt) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "lt", x) }

// Le checks if the first operand is less than or equal to the second
// (i.e. a <= b)
type Le [2]Expr

func (x Le) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Le) String() string { return buildString(x) }

func (x Le) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "le", x) }

// And performs a boolean AND operation of its two operands.
//
//	false && false == false
//	false && true == false
//	true && false == false
//	true && true == true
type And [2]Expr

func (x And) Operands() [2]Expr { return ([2]Expr)(x) }

func (x And) String() string { return buildString(x) }

func (x And) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "and", x) }

// Or performs a boolean OR operation of its two operands.
//
//	false || false == false
//	false || true == true
//	true || false == true
//	true || true == true
type Or [2]Expr

func (x Or) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Or) String() string { return buildString(x) }

func (x Or) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "or", x) }

// Add performs an arithmetic addition of its two operands (i.e. a + b)
type Add [2]Expr

func (x Add) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Add) String() string { return buildString(x) }

func (x Add) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "+", x) }

// Sub performs an arithmetic addition of its two operands (i.e. a - b)
type Sub [2]Expr

func (x Sub) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Sub) String() string { return buildString(x) }

func (x Sub) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "-", x) }

// Mul performs an arithmetic addition of its two operands (i.e. a * b)
type Mul [2]Expr

func (x Mul) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Mul) String() string { return buildString(x) }

func (x Mul) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "*", x) }

// Div performs an arithmetic addition of its two operands (i.e. a /+ b)
type Div [2]Expr

func (x Div) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Div) String() string { return buildString(x) }

func (x Div) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, "/", x) }

// Mem selects a member of a source expression.
type Mem [2]Expr

func ReflectStructFieldsOfType(rt reflect.Type) []reflect.StructField {
	return unsafereflect.TypeFromReflectType(rt).ReflectStructFields()
}

// ReflectStructFieldOf takes a pointer to a struct and a pointer to
// a field within the struct to get that field's `*reflect.StructField`.
// This `*reflect.StructField` is cached, so do not mutate it.
func ReflectStructFieldOf(base, field interface{}) (sf *reflect.StructField) {
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
	bv, bp := getPtr(base)
	fv, fp := getPtr(field)
	ft := fv.Elem().Type()
	offset := fp - bp
	structFields := ReflectStructFieldsOfType(bv.Type().Elem())
	// TODO: Maybe binary search would be better, but I'm assuming tiny
	// numbers of fields for now:
	for i := range structFields {
		sf := &structFields[i]
		if sf.Offset == offset && sf.Type == ft {
			return sf
		}
	}
	panic(fmt.Errorf(
		"unknown %T field at offset %d in %T",
		field, offset, base,
	))
}

func MemOf(e Expr, base, field interface{}) Mem {
	return Mem{e, ReflectStructFieldOf(base, field)}
}

func (x Mem) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Mem) String() string { return buildString(x) }

func (x Mem) writeStringToStringBuilder(sb *strings.Builder) { appendBinary(sb, ".", x) }

// Var is a placeholder for a runtime-determined value in an expression.
type Var interface {
	Expr

	// Var differentiates a Var from an Expr but could just
	// return itself.
	Var() Var
}

type NamedVar interface {
	Var
	Name() string
}

type Ident string

func (n Ident) Var() Var     { return n }
func (n Ident) Name() string { return string(n) }

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
	"expr.Values %w in context",
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
	varValues []VarValue
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
	return &valueList{append(make([]VarValue, 0, capacity), ps...)}
}

// ErrNotFound indicates something wasn't found; it should be wrapped
// in another error that provides more context.
var ErrNotFound = errors.New("not found")

func (vs *valueList) Get(ctx context.Context, v Var) (interface{}, error) {
	for i := range vs.varValues {
		if vs.varValues[i].Var == v {
			return vs.varValues[i].Value, nil
		}
	}
	return nil, ErrNotFound
}

func (vs *valueList) Set(ctx context.Context, v Var, x interface{}) error {
	for i := range vs.varValues {
		if vs.varValues[i].Var == v {
			vs.varValues[i].Value = x
			return nil
		}
	}
	vs.varValues = append(vs.varValues, VarValue{v, x})
	return nil
}

type valueListIter struct {
	vs *valueList
	i  int
}

func (vs *valueList) Vars() VarIter { return &valueListIter{vs, 0} }

func (vli *valueListIter) Next(context.Context) error {
	if vli.i >= len(vli.vs.varValues) {
		return io.EOF
	}
	vli.i++
	return nil
}

func (vli *valueListIter) Reset(context.Context) error {
	vli.i = 0
	return nil
}

// Var is not meant to make this type implement the Var interface.
func (vli *valueListIter) Var() Var {
	return vli.vs.varValues[vli.i-1].Var
}

func (vli *valueListIter) VarValue(context.Context) VarValue {
	return vli.vs.varValues[vli.i-1]
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
	if vsLen, err := tryLen(vs, 1); err == nil {
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

var (
	errNilPtr         = errors.New("pointer is nil")
	errInvalidValue   = errors.New("value is invalid")
	errRecursionLimit = errors.New("max recursion limit reached")
)

// tryLen attempts to get the length of _something_.
func tryLen(v interface{}, maxPtrDepth int) (length int, err error) {
	if maxPtrDepth < 0 {
		return 0, errRecursionLimit
	}
	switch v := v.(type) {
	case interface{ Len() int }:
		return v.Len(), nil
	case interface{ Len() int64 }:
		return int(v.Len()), nil
	case interface{ Len() int32 }:
		return int(v.Len()), nil
	case interface{ Len() int16 }:
		return int(v.Len()), nil
	case interface{ Len() int8 }:
		return int(v.Len()), nil
	case interface{ Len() uint64 }:
		return int(v.Len()), nil
	case interface{ Len() uint32 }:
		return int(v.Len()), nil
	case interface{ Len() uint16 }:
		return int(v.Len()), nil
	case interface{ Len() uint8 }:
		return int(v.Len()), nil
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return 0, errNilPtr
	}
	switch rv.Kind() {
	case reflect.Ptr:
		if rv.IsNil() {
			return 0, errNilPtr
		}
		rv = rv.Elem()
		if !rv.CanInterface() {
			return 0, errInvalidValue
		}
		return tryLen(rv.Interface(), maxPtrDepth-1)
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len(), nil
	}
	return 0, errInvalidValue
}

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
	writeStringToStringBuilder(&sb, x)
	return sb.String()
}

func writeStringToStringBuilder(sb *strings.Builder, x interface{}) {
	switch x := x.(type) {
	case interface{ writeStringToStringBuilder(sb *strings.Builder) }:
		x.writeStringToStringBuilder(sb)
	default:
		sb.WriteString(fmt.Sprint(x))
	}
}

func appendBinary(sb *strings.Builder, op string, b Binary) {
	ops := b.Operands()
	sb.WriteByte('(')
	sb.WriteString(op)
	sb.WriteByte(' ')
	writeStringToStringBuilder(sb, ops[0])
	sb.WriteByte(' ')
	writeStringToStringBuilder(sb, ops[1])
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

var errNilExpr = errors.New("nil expression")

func Walk(ctx context.Context, e Expr, v Visitor, options ...WalkOption) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("expr.Walk: %v: %w", e, err)
		}
	}()
	if e == nil {
		return errNilExpr
	}
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
