package expr

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/skillian/ctxutil"
)

var (
	// ErrNotFound indicates something wasn't found; it should be
	// wrapped in another error that provides more context.
	ErrNotFound = errors.New("not found")

	// ErrInvalidType indicates that a value of the incorrect type
	// was found.
	ErrInvalidType = errors.New("invalid type")
)

// Kind returns a non-generic "kind" of operation corresponding
// to the generic Expr implementation.
type Kind uint

const (
	// NoKind is an invalid Kind that is checked for debugging.
	NoKind Kind = iota
	ConstKind
	VarKind
	NotKind
	EqKind
	NeKind
	GtKind
	GeKind
	LtKind
	LeKind
	AndKind
	OrKind
	AddKind

	// CallKind indicates that an expression is a function call
	CallKind
)

// AnyExpr is a non-generic expression.
type AnyExpr interface {
	EvalAny(context.Context) (any, error)
	Kind() Kind
	Type() reflect.Type
}

// Expr is any expression that evaluates to a value of a specific type.
type Expr[T any] interface {
	AnyExpr
	AsExpr() Expr[T]
	Eval(context.Context) (T, error)
}

// Const wraps a value into a constant expression.
func Const[T any](value T) ConstExpr[T] { return ConstExpr[T]{V: value} }

// AnyConstExpr is any expression that evaluates to a constant value
type AnyConstExpr interface {
	// AnyValue retrieves the constant's value as an untyped
	// interface{}
	AnyValue() any
}

// ConstExpr is a constant expression with a specific type.
type ConstExpr[T any] struct {
	// V is the inner constant value
	V T
}

var _ interface {
	Expr[int]
	AnyConstExpr
} = ConstExpr[int]{}

func (c ConstExpr[T]) AnyValue() any                            { return c.V }
func (c ConstExpr[T]) AsExpr() Expr[T]                          { return c }
func (c ConstExpr[T]) Eval(context.Context) (T, error)          { return c.V, nil }
func (c ConstExpr[T]) EvalAny(ctx context.Context) (any, error) { return c.Eval(ctx) }
func (c ConstExpr[T]) Kind() Kind                               { return ConstKind }
func (c ConstExpr[T]) Type() reflect.Type                       { return reflect.TypeOf(&c.V).Elem() }

// AnyUnaryExpr is any expression with a single operand
type AnyUnaryExpr interface {
	AnyOperand() AnyExpr
}

// UnaryExpr is a typed unary expression
type UnaryExpr[TOperand, TResult any] interface {
	Expr[TResult]
	AnyUnaryExpr
	Operand() Expr[TOperand]
}

var boolType = reflect.TypeOf(false)

// NotExpr is a boolean NOT which evaluates to the opposite of its
// operand.
type NotExpr [1]Expr[bool]

// Not returns a NotExpr of a boolean operand
func Not(e Expr[bool]) NotExpr { return NotExpr{e} }

var _ interface {
	UnaryExpr[bool, bool]
} = NotExpr{}

func (x NotExpr) AsExpr() Expr[bool]  { return x }
func (x NotExpr) AnyOperand() AnyExpr { return x.Operand() }
func (x NotExpr) Eval(ctx context.Context) (bool, error) {
	v, err := x.Operand().Eval(ctx)
	return !v, err
}
func (x NotExpr) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x NotExpr) Kind() Kind                               { return NotKind }
func (x NotExpr) Operand() Expr[bool]                      { return x[0] }
func (x NotExpr) Type() reflect.Type                       { return boolType }

// AnyBinaryExpr is any expression with two operands that resolves to
// a single value.
type AnyBinaryExpr interface {
	AnyOperands() [2]AnyExpr
}

// BinaryExpr is a typed binary expression that resolves to a single
// value.
type BinaryExpr[TLeft, TRight, TOut any] interface {
	Expr[TOut]
	AnyBinaryExpr
	Operands() (left Expr[TLeft], right Expr[TRight])
}

// binarySameExpr is a helper type whose input parameters are of the
// same type, but whose output can be different.
type binarySameExpr[TIn, TOut any] [2]Expr[TIn]

var _ interface {
	BinaryExpr[int, int, bool]
} = binarySameExpr[int, bool]{}

func (x binarySameExpr[TIn, TOut]) AsExpr() Expr[TOut]                           { return x }
func (x binarySameExpr[TIn, TOut]) AnyOperands() [2]AnyExpr                      { return anyOperands2(x.Operands()) }
func (x binarySameExpr[TIn, TOut]) Eval(ctx context.Context) (y TOut, err error) { return }
func (x binarySameExpr[TIn, TOut]) EvalAny(ctx context.Context) (any, error)     { return nil, nil }
func (c binarySameExpr[TIn, TOut]) Kind() Kind                                   { return NoKind }
func (x binarySameExpr[TIn, TOut]) Operands() (left, right Expr[TIn])            { return x[0], x[1] }
func (c binarySameExpr[TIn, TOut]) Type() reflect.Type {
	var to TOut
	return reflect.TypeOf(&to).Elem()
}

// Eqer can compare if two values are "equal"
type Eqer[T any] interface {
	Eq(ctx context.Context, a, b T) (bool, error)
}

// DefaultEqer returns a default equality checker for Go.
func DefaultEqer[T comparable]() Eqer[T] {
	return MakeEqer(func(ctx context.Context, a, b T) (bool, error) {
		return a == b, nil
	})
}

// MakeEqer creates an Eqer[T] from a function.
func MakeEqer[T any](f func(ctx context.Context, a, b T) (bool, error)) Eqer[T] {
	return EqerFunc[T](f)
}

// EqerFunc implements the Eqer[T] interface via a function.
type EqerFunc[T any] func(ctx context.Context, a, b T) (bool, error)

func (f EqerFunc[T]) Eq(ctx context.Context, a, b T) (bool, error) {
	return f(ctx, a, b)
}

// EqerContextKey returns a key that can be used to retrieve an Eqer[T]
// implementation from a context.Context.
//
//	eqer, ok := ctx.Value(EqerContextKey[int]()).(Eqer[int])
//
func EqerContextKey[T any]() interface{} { return (*Eqer[T])(nil) }

func eqExprHelper[T any](ctx context.Context, a, b Expr[T]) (bool, error) {
	eqer, err := ifaceFromCtx[Eqer[T]](ctx)
	if err != nil {
		return false, err
	}
	left, right, err := eval2(ctx, a, b)
	if err != nil {
		return false, err
	}
	return eqer.Eq(ctx, left, right)
}

// EqExpr is an equality comparison expression.
type EqExpr[T any] binarySameExpr[T, bool]

// Eq returns an equality expression.
func Eq[T any](a, b Expr[T]) EqExpr[T] { return EqExpr[T]{a, b} }

var _ interface {
	BinaryExpr[int, int, bool]
} = EqExpr[int]{}

func (x EqExpr[T]) AsExpr() Expr[bool]      { return x }
func (x EqExpr[T]) AnyOperands() [2]AnyExpr { return anyOperands2(x.Operands()) }
func (x EqExpr[T]) Eval(ctx context.Context) (bool, error) {
	left, right := x.Operands()
	return eqExprHelper(ctx, left, right)
}
func (x EqExpr[T]) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x EqExpr[T]) Kind() Kind                               { return EqKind }
func (x EqExpr[T]) Operands() (left, right Expr[T])          { return x[0], x[1] }
func (x EqExpr[T]) Type() reflect.Type                       { return boolType }

// NeExpr is an inequality expression
type NeExpr[T any] binarySameExpr[T, bool]

// Ne creates and returns an inequality expression.
func Ne[T any](a, b Expr[T]) NeExpr[T] { return NeExpr[T]{a, b} }

var _ interface {
	BinaryExpr[int, int, bool]
} = NeExpr[int]{}

func (x NeExpr[T]) AnyOperands() [2]AnyExpr { return anyOperands2(x.Operands()) }
func (x NeExpr[T]) AsExpr() Expr[bool]      { return x }
func (x NeExpr[T]) Eval(ctx context.Context) (bool, error) {
	left, right := x.Operands()
	eq, err := eqExprHelper(ctx, left, right)
	return !eq, err
}
func (x NeExpr[T]) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x NeExpr[T]) Kind() Kind                               { return NeKind }
func (x NeExpr[T]) Operands() (left, right Expr[T])          { return x[0], x[1] }
func (x NeExpr[T]) Type() reflect.Type                       { return boolType }

// Cmper compares its operands.
type Cmper[T any] interface {
	Cmp(ctx context.Context, a, b T) (int, error)
}

// CmperContextKey can be used as a key to `context.Context.Value` to
// retrieve an instance of the Cmper[T] from the context.
//
//	cmper, ok := ctx.Value(CmperContextKey[int]()).(Cmper[int])
//
func CmperContextKey[T any]() interface{} { return (*Cmper[T])(nil) }

func cmpExprHelper[T any](ctx context.Context, a, b Expr[T]) (int, error) {
	cmper, err := ifaceFromCtx[Cmper[T]](ctx)
	if err != nil {
		return 0, err
	}
	left, right, err := eval2(ctx, a, b)
	if err != nil {
		return 0, err
	}
	return cmper.Cmp(ctx, left, right)
}

// GtExpr is a greater than comparison.
type GtExpr[T comparable] binarySameExpr[T, bool]

// Gt returns a Greater than comparison.
func Gt[T comparable](a, b Expr[T]) GtExpr[T] { return GtExpr[T]{a, b} }

var _ interface {
	BinaryExpr[int, int, bool]
} = GtExpr[int]{}

func (x GtExpr[T]) AnyOperands() [2]AnyExpr { return anyOperands2(x.Operands()) }
func (x GtExpr[T]) AsExpr() Expr[bool]      { return x }
func (x GtExpr[T]) Eval(ctx context.Context) (bool, error) {
	left, right := x.Operands()
	cmp, err := cmpExprHelper(ctx, left, right)
	return cmp > 0, err
}
func (x GtExpr[T]) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x GtExpr[T]) Kind() Kind                               { return GtKind }
func (x GtExpr[T]) Operands() (left, right Expr[T])          { return x[0], x[1] }
func (x GtExpr[T]) Type() reflect.Type                       { return boolType }

// GeExpr is a greater than expression.
type GeExpr[T comparable] binarySameExpr[T, bool]

// Ge creates a GeExpr with two operands.
func Ge[T comparable](a, b Expr[T]) GeExpr[T] { return GeExpr[T]{a, b} }

var _ interface {
	BinaryExpr[int, int, bool]
} = GeExpr[int]{}

func (x GeExpr[T]) AnyOperands() [2]AnyExpr { return anyOperands2(x.Operands()) }
func (x GeExpr[T]) AsExpr() Expr[bool]      { return x }
func (x GeExpr[T]) Eval(ctx context.Context) (bool, error) {
	left, right := x.Operands()
	cmp, err := cmpExprHelper(ctx, left, right)
	return cmp >= 0, err
}
func (x GeExpr[T]) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x GeExpr[T]) Kind() Kind                               { return GeKind }
func (x GeExpr[T]) Operands() (left, right Expr[T])          { return x[0], x[1] }
func (x GeExpr[T]) Type() reflect.Type                       { return boolType }

// LtExpr is a less than expression.
type LtExpr[T comparable] binarySameExpr[T, bool]

// Lt creates a LtExpr with two operands.
func Lt[T comparable](a, b Expr[T]) LtExpr[T] { return LtExpr[T]{a, b} }

var _ interface {
	BinaryExpr[int, int, bool]
} = LtExpr[int]{}

func (x LtExpr[T]) AnyOperands() [2]AnyExpr { return anyOperands2(x.Operands()) }
func (x LtExpr[T]) AsExpr() Expr[bool]      { return x }
func (x LtExpr[T]) Eval(ctx context.Context) (bool, error) {
	left, right := x.Operands()
	cmp, err := cmpExprHelper(ctx, left, right)
	return cmp > 0, err
}
func (x LtExpr[T]) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x LtExpr[T]) Kind() Kind                               { return LtKind }
func (x LtExpr[T]) Operands() (left, right Expr[T])          { return x[0], x[1] }
func (x LtExpr[T]) Type() reflect.Type                       { return boolType }

// LtExpr is a less than expression.
type LeExpr[T comparable] binarySameExpr[T, bool]

// Lt creates a LtExpr with two operands.
func Le[T comparable](a, b Expr[T]) LeExpr[T] { return LeExpr[T]{a, b} }

var _ interface {
	BinaryExpr[int, int, bool]
} = LeExpr[int]{}

func (x LeExpr[T]) AnyOperands() [2]AnyExpr { return anyOperands2(x.Operands()) }
func (x LeExpr[T]) AsExpr() Expr[bool]      { return x }
func (x LeExpr[T]) Eval(ctx context.Context) (bool, error) {
	left, right := x.Operands()
	cmp, err := cmpExprHelper(ctx, left, right)
	return cmp > 0, err
}
func (x LeExpr[T]) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x LeExpr[T]) Kind() Kind                               { return LeKind }
func (x LeExpr[T]) Operands() (left, right Expr[T])          { return x[0], x[1] }
func (x LeExpr[T]) Type() reflect.Type                       { return boolType }

// AndExpr is a logical AND expression.
type AndExpr binarySameExpr[bool, bool]

// And creates an AndExpr.
func And(a, b Expr[bool]) AndExpr { return AndExpr{a, b} }

var _ interface {
	BinaryExpr[bool, bool, bool]
} = AndExpr{}

func (x AndExpr) AnyOperands() [2]AnyExpr { return anyOperands2(x.Operands()) }
func (x AndExpr) AsExpr() Expr[bool]      { return x }
func (x AndExpr) Eval(ctx context.Context) (bool, error) {
	a, b := x.Operands()
	left, right, err := eval2(ctx, a, b)
	return left && right, err
}
func (x AndExpr) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x AndExpr) Kind() Kind                               { return AndKind }
func (x AndExpr) Operands() (left, right Expr[bool])       { return x[0], x[1] }
func (x AndExpr) Type() reflect.Type                       { return boolType }

// OrExpr is a logical OR expression.
type OrExpr binarySameExpr[bool, bool]

// Or creates an OrExpr.
func Or(a, b Expr[bool]) OrExpr { return OrExpr{a, b} }

var _ interface {
	BinaryExpr[bool, bool, bool]
} = OrExpr{}

func (x OrExpr) AnyOperands() [2]AnyExpr { return anyOperands2(x.Operands()) }
func (x OrExpr) AsExpr() Expr[bool]      { return x }
func (x OrExpr) Eval(ctx context.Context) (bool, error) {
	a, b := x.Operands()
	left, right, err := eval2(ctx, a, b)
	return left || right, err
}
func (x OrExpr) EvalAny(ctx context.Context) (any, error) { return x.Eval(ctx) }
func (x OrExpr) Kind() Kind                               { return OrKind }
func (x OrExpr) Operands() (left, right Expr[bool])       { return x[0], x[1] }
func (x OrExpr) Type() reflect.Type                       { return boolType }

// Added adds an addend to an augend and returns a result.  Programming
// languages sometimes allow operands of different types (e.g. adding
// a duration to a time).
type Adder[TAugend, TAddend, TResult any] interface {
	Add(context.Context, TAugend, TAddend) (TResult, error)
}

// AdderContextKey returns a value that can be used as a key to
// context.Context.Value to lookup an Adder implementation.
//
//	adder, ok := ctx.Value(AdderContextKey[int, int, int]()).(Adder[int, int, int])
//
func AdderContextKey[TAugend, TAddend, TResult any]() interface{} {
	return (*Adder[TAddend, TAddend, TResult])(nil)
}

type numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// DefaultAdder is an Adder implementation that uses Go's built-in
// behavior.
func DefaultAdder[T numeric]() Adder[T, T, T] {
	return MakeAdder(func(ctx context.Context, a, b T) (T, error) {
		return a + b, nil
	})
}

// AdderFunc implements Adder via a wrapped function.
type AdderFunc[TAugend, TAddend, TResult any] func(context.Context, TAugend, TAddend) (TResult, error)

// MakeAdder creates an Adder implementation from a function.
func MakeAdder[TAugend, TAddend, TResult any](f func(context.Context, TAugend, TAddend) (TResult, error)) Adder[TAugend, TAddend, TResult] {
	return AdderFunc[TAugend, TAddend, TResult](f)
}

func (f AdderFunc[TAugend, TAddend, TResult]) Add(ctx context.Context, a TAugend, b TAddend) (TResult, error) {
	return f(ctx, a, b)
}

// AddExpr creates an add expression.
type AddExpr[TAugend, TAddend, TResult any] struct {
	// note that "augend" is an obsolete term, but I'm using it
	// here because arithmetic can be performed on different types
	// e.g. a date plus a duration yields another date.  A date
	// minus a date yields a duration, etc.

	// Augend is the number-like object to be augmented by the
	// addend.
	Augend Expr[TAugend]

	// Addend is the number-like object to be added to the augend.
	Addend Expr[TAddend]
}

func Add[TAugend, TAddend, TResult any](a Expr[TAugend], b Expr[TAddend], ar Adder[TAugend, TAddend, TResult]) AddExpr[TAugend, TAddend, TResult] {
	return AddExpr[TAugend, TAddend, TResult]{a, b}
}

var _ interface {
	BinaryExpr[int, int, int]
} = AddExpr[int, int, int]{}

func (x AddExpr[TAugend, TAddend, TResult]) AnyOperands() [2]AnyExpr {
	return anyOperands2(x.Operands())
}
func (x AddExpr[TAugend, TAddend, TResult]) AsExpr() Expr[TResult] { return x }
func (x AddExpr[TAugend, TAddend, TResult]) Eval(ctx context.Context) (TResult, error) {
	adder, err := ifaceFromCtx[Adder[TAugend, TAddend, TResult]](ctx)
	if err != nil {
		var res TResult
		return res, err
	}
	a, b := x.Operands()
	left, right, err := eval2(ctx, a, b)
	if err != nil {
		var res TResult
		return res, err
	}
	return adder.Add(ctx, left, right)
}
func (x AddExpr[TAugend, TAddend, TResult]) EvalAny(ctx context.Context) (any, error) {
	return x.Eval(ctx)
}
func (x AddExpr[TAugend, TAddend, TResult]) Kind() Kind { return AddKind }
func (x AddExpr[TAugend, TAddend, TResult]) Operands() (left Expr[TAugend], right Expr[TAddend]) {
	return x.Augend, x.Addend
}
func (x AddExpr[TAugend, TAddend, TResult]) Type() reflect.Type {
	var res TResult
	return reflect.TypeOf(&res).Elem()
}

// AnyFunc is an interface common to any function
type AnyFunc interface {
	AnyBody() AnyExpr
	AnyVars() []AnyVar
}

type Func1xAny1[TIn any] interface {
	Var0() Var[TIn]
}

// Func1x1 is a function with one input and one output.  During a
// function invocation, the operand result is mapped to the
// variable, V0 which can be used in the function body.
type Func1x1[T0, TResult any] struct {
	// Body is the expression body that may (or may not) use the
	// parameter to the function, V0, to calculate its result.
	Body Expr[TResult]

	// V0 is the first parameter to the function
	V0 Var[T0]
}

// NewFunc1x1 is a utility function that can create a Func1x1.  The
// factory function is passed in the function parameter Var and must
// return the function body expression.
func NewFunc1x1[T0, TResult any](factory func(v Var[T0]) Expr[TResult]) (f Func1x1[T0, TResult]) {
	f.V0 = &ptrVar[T0]{}
	f.Body = factory(f.V0)
	return f
}

func (f Func1x1[T0, TResult]) AnyBody() AnyExpr  { return f.Body }
func (f Func1x1[T0, TResult]) AnyVars() []AnyVar { return []AnyVar{f.V0} }
func (f Func1x1[T0, TResult]) Var0() Var[T0]     { return f.V0 }

// Call1x1Expr is a function call expression of a Func1x1.
type Call1x1Expr[T0, TResult any] struct {
	// Func is the Func1x1 that this call expression is calling.
	Func Func1x1[T0, TResult]

	// V0 is the expression passed in as the function call's
	// operand.
	V0 Expr[T0]
}

// Call1x1 creates a call expression that calls a Func1x1.
func Call1x1[T0, TResult any](ctx context.Context, f Func1x1[T0, TResult], v0 Expr[T0]) Call1x1Expr[T0, TResult] {
	return Call1x1Expr[T0, TResult]{Func: f, V0: v0}
}

var _ interface {
	UnaryExpr[int, string]
} = Call1x1Expr[int, string]{Func1x1[int, string]{}, Const(0)}

func (c Call1x1Expr[T0, T1]) AnyOperand() AnyExpr { return c.Operand() }
func (c Call1x1Expr[T0, T1]) AsExpr() Expr[T1]    { return c }
func (c Call1x1Expr[T0, T1]) Eval(ctx context.Context) (r0 T1, err error) {
	v0, err := c.Operand().Eval(ctx)
	if err != nil {
		return r0, err
	}
	return c.Func.Body.Eval(ctxutil.WithValue(ctx, c.Func.V0, v0))
}
func (c Call1x1Expr[T0, T1]) EvalAny(ctx context.Context) (any, error) { return c.Eval(ctx) }
func (c Call1x1Expr[T0, T1]) Kind() Kind                               { return CallKind }
func (c Call1x1Expr[T0, T1]) Operand() Expr[T0]                        { return c.V0 }
func (c Call1x1Expr[T0, T1]) Type() reflect.Type {
	var t1 T1
	return reflect.TypeOf(&t1).Elem()
}

// AnyVar is implemented by any variable expression.
type AnyVar interface {
	AnyVar() AnyVar
}

// Var is a type-safe variable.
type Var[T any] interface {
	AnyVar
	Expr[T]
	Var() Var[T]
}

type ptrVar[T any] struct{ _ byte }

func (v *ptrVar[T]) AsExpr() Expr[T]                          { return v }
func (v *ptrVar[T]) AnyVar() AnyVar                           { return v }
func (v *ptrVar[T]) EvalAny(ctx context.Context) (any, error) { return v.Eval(ctx) }
func (v *ptrVar[T]) Eval(ctx context.Context) (T, error)      { return EvalVar(ctx, v.Var()) }
func (v *ptrVar[T]) Kind() Kind                               { return VarKind }
func (v *ptrVar[T]) Var() Var[T]                              { return v }
func (v *ptrVar[T]) Type() reflect.Type {
	var t T
	return reflect.TypeOf(&t).Elem()
}

func (v *ptrVar[T]) GoString() string {
	return fmt.Sprintf("%[1]T(%[1]p)", v)
}

func EvalVar[T any](ctx context.Context, va Var[T]) (t T, err error) {
	v := ctxutil.Value(ctx, va)
	if v == nil {
		return t, ErrNotFound
	}
	t, ok := v.(T)
	if !ok {
		return t, fmt.Errorf(
			"%[1]w: expected %[2]v, but actual: "+
				"%[3]v (type: %[3]T)",
			ErrInvalidType, reflect.TypeOf(&t).Elem(), v,
		)
	}
	return t, nil
}

func eval2[T0, T1 any](ctx context.Context, e0 Expr[T0], e1 Expr[T1]) (v0 T0, v1 T1, err error) {
	v0, err = e0.Eval(ctx)
	if err == nil {
		v1, err = e1.Eval(ctx)
	}
	return
}

func anyOperands2[T0, T1 any](e0 Expr[T0], e1 Expr[T1]) [2]AnyExpr { return [2]AnyExpr{e0, e1} }

func ifaceFromCtx[T any](ctx context.Context) (T, error) {
	t, ok := ctx.Value((*T)(nil)).(T)
	if !ok {
		return t, fmt.Errorf(
			"%w: failed to get %v from context",
			ErrNotFound, reflect.TypeOf(&t).Elem(),
		)
	}
	return t, nil
}

// ErrorPexpectActual creates an ErrInvalidType from a pointer to an
// expected type and a value of the actual type.  A pointer to the
// expected type must be passed so that interface types can be expected.
func ErrorPexpectActual(expect, actual interface{}) error {
	return fmt.Errorf(
		"%[1]w: expected %[2]v value, but actual: %[3]v (type: %[3]T)",
		ErrInvalidType, reflect.TypeOf(expect).Elem(), actual,
	)
}

// WalkFunc is used by Walk when traversing an expression tree.  It is
// called by Walk with a non-nil expression when entering an expression
// and with nil when exiting.  For example, the following expression
// tree:
//
//	Add(Const(1), Const(2))
//
//would result in the following call stack:
//
//	f(AddExpr[int, int, int]{...})
//	f(ConstExpr[int]{1})
//	f(nil)	// exiting Const(1)
//	f(ConstExpr[int]{2})
//	f(nil)	// exiting Const(2)
//	f(nil)	// exiting Add(...)
//
type WalkFunc func(e AnyExpr) bool

// Walk the expression tree, e, with WalkFunc, f.  See WalkFunc's
// documention for more info.
func Walk(e AnyExpr, f WalkFunc) bool {
	if !f(e) {
		return false
	}
	switch e := e.(type) {
	case AnyUnaryExpr:
		if !Walk(e.AnyOperand(), f) {
			return false
		}
	case AnyBinaryExpr:
		for _, e := range e.AnyOperands() {
			if !Walk(e, f) {
				return false
			}
		}
	}
	return f(nil)
}
