package expr

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/skillian/ctxutil"
	"github.com/skillian/errors"
)

// FloatTolerance can be added to a context so that comparisons of
// floats with differences less than this amount are considered equal.
// For example, with a float tolerance of 0.01, 0.021 and 0.028 would
// be considered equal.
type FloatTolerance float64

func (t FloatTolerance) ContextKey() interface{} {
	return (*FloatTolerance)(nil)
}

// Eval evaluates the given expression.
func Eval(ctx context.Context, e Expr, vs Values) (interface{}, error) {
	fc, ok := ctxutil.FromContextKey(ctx, (*funcCache)(nil)).(*funcCache)
	var fk funcKey
	if ok {
		fk = makeFuncKey(ctx, e, vs)
		if fn, ok := fc.load(fk); ok {
			return fn.Call(ctx, vs)
		}
	}
	fn, err := funcFromExpr(ctx, e, vs)
	if err != nil {
		return nil, err
	}
	if ok {
		fn, _ = fc.loadOrStore(fk, fn)
	}
	ctx = ctxutil.AddToContext(ctx, exprEvaluators.Get().(*exprEvaluator))
	return fn.Call(ctx, vs)
}

var exprEvaluators = sync.Pool{
	New: func() any {
		ee := &exprEvaluator{
			ctx: eeCtx{
				strcmp: strings.Compare,
			},
		}
		ee.putMathBigRat(ee.getMathBigRat())
		ee.stack.init(8)
		return ee
	},
}

type exprEvaluator struct {
	stack eeStack
	ctx   eeCtx
	free  struct {
		mathBigRats []*big.Rat
	}
}

func (ee *exprEvaluator) ContextKey() interface{} { return (*exprEvaluator)(nil) }

var errInvalidOp = errors.New("invalid op code")

func (ee *exprEvaluator) evalFunc(ctx context.Context, f *opFunc, vs Values) (err error) {
	defer func(p *error) {
		v := recover()
		if v == nil {
			return
		}
		err, ok := v.(error)
		if !ok {
			err = fmt.Errorf("%v", v)
		}
		*p = multierrorOf(*p, err)
	}(&err)
	for pc := 0; pc < len(f.ops); pc++ {
		op := f.ops[pc]
		if op == opNop {
			continue
		}
		var et eeType
		if len(ee.stack.types) > 0 {
			et = ee.stack.peekType()
		}
		switch op {
		case opNot:
			n := ee.stack.popNum()
			ee.stack.pushNum(eeNumBool(!(*n.bool())))
			ee.stack.pushType(boolType)
		case opAnd:
			a, b := eeBoolType{}.pop2Bool(&ee.stack)
			ee.stack.pushNum(eeNumBool(a && b))
			ee.stack.pushType(boolType)
		case opOr:
			a, b := eeBoolType{}.pop2Bool(&ee.stack)
			ee.stack.pushNum(eeNumBool(a || b))
			ee.stack.pushType(boolType)
		case opEq:
			ee.stack.pushNum(eeNumBool(et.eq(ee)))
			ee.stack.pushType(boolType)
		case opNe:
			ee.stack.pushNum(eeNumBool(!et.eq(ee)))
			ee.stack.pushType(boolType)
		case opGt:
			ee.stack.pushNum(eeNumBool(et.(eeCmpType).cmp(ee) > 0))
			ee.stack.pushType(boolType)
		case opGe:
			ee.stack.pushNum(eeNumBool(et.(eeCmpType).cmp(ee) >= 0))
			ee.stack.pushType(boolType)
		case opLt:
			ee.stack.pushNum(eeNumBool(et.(eeCmpType).cmp(ee) < 0))
			ee.stack.pushType(boolType)
		case opLe:
			ee.stack.pushNum(eeNumBool(et.(eeCmpType).cmp(ee) <= 0))
			ee.stack.pushType(boolType)
		case opAdd:
			et.(eeNumType).add(ee)
		case opSub:
			et.(eeNumType).sub(ee)
		case opMul:
			et.(eeNumType).mul(ee)
		case opDiv:
			et.(eeNumType).div(ee)
		case opConv1:
			fallthrough
		case opConv2:
			pc++
			i64, n := getIntFromOpCodes(f.ops[pc:])
			pc += n - 1 // loop will increment 1
			target := f.consts.types[int(i64)]
			var conver eeConvType
			if op == opConv2 {
				pc++
				i64, n = getIntFromOpCodes(f.ops[pc:])
				pc += n - 1
				conver = f.consts.anys[int(i64)].(eeConvType)
			} else {
				conver = target.(eeConvType)
			}
			conver.conv(ee, target)
		case opLdc:
			fallthrough
		case opLdv:
			pc++
			i64, n := getIntFromOpCodes(f.ops[pc:])
			pc += n - 1 // loop will increment 1
			k := f.constkeys[int(i64)]
			et = f.consts.types[k.typeIndex]
			et.pushValue(&ee.stack, &f.consts, k.valIndex)
			ee.stack.pushType(et)
			if op == opLdv {
				va := et.pop(&ee.stack).(Var)
				v, err := vs.Get(ctx, va)
				if err != nil {
					panic(err)
				}
				ee.stack.push(v, nil)
			}
		default:
			panic(fmt.Errorf("%w: %v", errInvalidOp, op))
		}
	}
	return nil
}

func (ee *exprEvaluator) getMathBigRat() (r *big.Rat) {
	if len(ee.free.mathBigRats) > 0 {
		p := &ee.free.mathBigRats[len(ee.free.mathBigRats)-1]
		r = *p
		*p = nil
		ee.free.mathBigRats = ee.free.mathBigRats[:len(ee.free.mathBigRats)-1]
		return
	}
	if cap(ee.free.mathBigRats) == 0 {
		const arbitraryCapacity = 8
		ee.free.mathBigRats = make([]*big.Rat, arbitraryCapacity)
	} else {
		// Let append's behavior determine the next size
		// increase:
		ee.free.mathBigRats = append(ee.free.mathBigRats, nil)
		ee.free.mathBigRats = ee.free.mathBigRats[:cap(ee.free.mathBigRats)]
	}
	// oh boy, here come the micro-optimizations!
	// these must keep in sync with math/big:
	type bigInt struct {
		_   bool
		nat []uint
	}
	type bigRat struct {
		a, b bigInt
	}
	natCache := make([]uint, cap(ee.free.mathBigRats)*4)
	ratCache := make([]bigRat, cap(ee.free.mathBigRats))
	for i := range ee.free.mathBigRats {
		r := &ratCache[i]
		r.a.nat = natCache[i*4 : i*4 : i*4+2]
		r.b.nat = natCache[i*4+2 : i*4+2 : i*4+4]
		ee.free.mathBigRats[len(ee.free.mathBigRats)-1-i] = (*big.Rat)(unsafe.Pointer(r))
	}
	p := &ee.free.mathBigRats[len(ee.free.mathBigRats)-1]
	r = *p
	*p = nil
	ee.free.mathBigRats = ee.free.mathBigRats[:len(ee.free.mathBigRats)-1]
	return
}

func (ee *exprEvaluator) putMathBigRat(r *big.Rat) {
	r.SetInt64(0)
	ee.free.mathBigRats = append(ee.free.mathBigRats, r)
}

// eeCtx holds contextual information used when evaluating an
// expression.  It's similar to context.Context, but context.Context
// can be wrapped with context.WithValue, context.WithTimeout, etc.
// within the same expression, so it has to be passed normally.  Things
// like FloatTolerance and string comparison makes sense (to me,
// anyway) to make static throughout the expression.
type eeCtx struct {
	// floatTol is set from the FloatTolerance
	floatTol float64
	streqfn  func(a, b string) bool
	strcmp   func(a, b string) int
}

func (c eeCtx) streq(a, b string) bool {
	if c.streqfn != nil {
		return c.streqfn(a, b)
	}
	return c.strcmp(a, b) == 0
}

// eeValues is an ordered collection of values of arbitrary types,
// analogous to an []interface{} slice, but should avoid boxing common
// types like ints, float64s, etc.
type eeValues struct {
	types []eeType

	// nums holds numeric values that fit within a single machine
	// word (i.e. all int, uint, and float types).
	nums []eeNum

	// strs holds strings
	strs []string

	// anys holds everything else.
	anys []interface{}
}

func (vs *eeValues) init(caps int) {
	vs.types = make([]eeType, 0, caps)
	vs.nums = make([]eeNum, 0, caps)
	vs.strs = make([]string, 0, caps)
	vs.anys = make([]interface{}, 0, caps)
}

func (vs *eeValues) reset() {
	for i := range vs.types {
		vs.types[i] = nil
	}
	vs.types = vs.types[:0]
	vs.nums = vs.nums[:0]
	for i := range vs.strs {
		vs.strs[i] = ""
	}
	vs.strs = vs.strs[:0]
	for i := range vs.anys {
		vs.anys[i] = nil
	}
	vs.anys = vs.anys[:0]
}

func (vs *eeValues) append(v interface{}, t eeType) (k eeValueKey) {
	if t == nil {
		t = eeTypeOf(v)
	}
	k.typeIndex = vs.appendType(t)
	k.valIndex = t.append(vs, v)
	return
}

func (vs *eeValues) appendType(v eeType) (i int) {
	i = len(vs.types)
	vs.types = append(vs.types, v)
	return
}

func (vs *eeValues) appendNum(v eeNum) (i int) {
	i = len(vs.nums)
	vs.nums = append(vs.nums, v)
	return
}

func (vs *eeValues) appendStr(v string) (i int) {
	i = len(vs.strs)
	vs.strs = append(vs.strs, v)
	return
}

func (vs *eeValues) appendAny(v interface{}) (i int) {
	i = len(vs.anys)
	vs.anys = append(vs.anys, v)
	return
}

func (vs *eeValues) get(k eeValueKey) interface{} {
	return vs.types[k.typeIndex].get(vs, k.valIndex)
}

func (vs *eeValues) set(k eeValueKey, v interface{}) {
	vs.types[k.typeIndex].set(vs, k.valIndex, v)
}

type eeValueKey struct {
	typeIndex int
	valIndex  int
}

type eeStack eeValues

func (es *eeStack) init(caps int) {
	((*eeValues)(es)).init(caps)
}

func (es *eeStack) reset() {
	((*eeValues)(es)).reset()
}

func (es *eeStack) peek() (v interface{}, t eeType) {
	t = es.peekType()
	v = t.peek(es)
	return
}

func (es *eeStack) pop() (v interface{}, t eeType) {
	t = es.popType()
	v = t.pop(es)
	return
}

func (es *eeStack) push(v interface{}, t eeType) {
	_ = ((*eeValues)(es)).append(v, t)
}

func (es *eeStack) peekType() eeType {
	return es.types[len(es.types)-1]
}

func (es *eeStack) popType() (v eeType) {
	v = es.peekType()
	es.types = es.types[:len(es.types)-1]
	return
}

func (es *eeStack) pushType(v eeType) {
	_ = ((*eeValues)(es)).appendType(v)
}

func (es *eeStack) peekNum() eeNum {
	return es.nums[len(es.nums)-1]
}

func (es *eeStack) popNum() (v eeNum) {
	v = es.peekNum()
	es.nums = es.nums[:len(es.nums)-1]
	return
}

func (es *eeStack) pushNum(v eeNum) {
	_ = ((*eeValues)(es)).appendNum(v)
}

func (es *eeStack) peekStr() string {
	return es.strs[len(es.strs)-1]
}

func (es *eeStack) popStr() (v string) {
	v = es.peekStr()
	es.strs = es.strs[:len(es.strs)-1]
	return
}

func (es *eeStack) pushStr(v string) {
	_ = ((*eeValues)(es)).appendStr(v)
}

func (es *eeStack) peekAny() interface{} {
	return es.anys[len(es.anys)-1]
}

func (es *eeStack) popAny() (v interface{}) {
	v = es.peekAny()
	es.anys = es.anys[:len(es.anys)-1]
	return
}

func (es *eeStack) pushAny(v interface{}) {
	_ = ((*eeValues)(es)).appendAny(v)
}

type eeNum struct {
	data uint64
}

func eeNumBool(v bool) (n eeNum) {
	*n.bool() = v
	return
}

func eeNumInt(v int) (n eeNum) {
	*n.int() = v
	return
}

func eeNumInt64(v int64) (n eeNum) {
	*n.int64() = v
	return
}

func eeNumFloat64(v float64) (n eeNum) {
	*n.float64() = v
	return
}

func (n *eeNum) bool() *bool       { return (*bool)(unsafe.Pointer(&n.data)) }
func (n *eeNum) int() *int         { return (*int)(unsafe.Pointer(&n.data)) }
func (n *eeNum) int64() *int64     { return (*int64)(unsafe.Pointer(&n.data)) }
func (n *eeNum) float64() *float64 { return (*float64)(unsafe.Pointer(&n.data)) }

type eeType interface {
	// eq compares two values at the top of the stack of type eeType
	// for equality
	eq(*exprEvaluator) bool

	// append appends a value of type eeType into the eeValues and
	// returns the index in the eeValues subslice that the value
	// was appended into.
	append(*eeValues, interface{}) int

	// appendZero appends the zero value for the type into the
	// values and returns the subslice index like append.
	appendZero(*eeValues) int

	// get retrieves a value of type eeType from an eeValues
	// subslice at the given index.  Implementations can assume
	// the value at that index is of type eeType.
	get(*eeValues, int) interface{}

	// set assigns a value at the eeValues subslice index.
	set(*eeValues, int, interface{})

	// peek a value of type eeType off of the eeStack.
	peek(*eeStack) interface{}

	// pop a value of type eeType from the eeStack
	pop(*eeStack) interface{}

	// pushValue retrieves a value from the eeValues (like get)
	// and pushes it to the eeStack (like eeStack.push).  The only
	// difference is the value is not boxed into an interface{}
	// where it might escape to the heap in between.
	pushValue(*eeStack, *eeValues, int)

	// typeCheck is used during function generation to check if the
	// given operation is compatible with the other operand.
	// Incompatible operations should return a nil eeType.
	typeCheck(e Expr, t2 eeType) eeType
}

var (
	boolType eeType = eeBoolType{}
	varType  eeType = eeVarType{}

	intType interface {
		eeType
		eeNumType
	} = &numType{intTypeImpl{}}
	int64Type interface {
		eeType
		eeNumType
	} = &numType{int64TypeImpl{}}
	stringType interface {
		eeType
		eeCmpType
	} = eeStringType{}

	timeType interface {
		eeType
		eeNumType
	} = eeTimeType{}
	timeDurationType interface {
		eeType
		eeNumType
	} = &numType{timeDurationTypeImpl{}}
	float64Type interface {
		eeType
		eeNumType
	} = &eeFloat64Type{numType{float64TypeImpl{}}}
	mathBigRatType interface {
		eeType
		eeNumType
	} = eeMathBigRatType{}

	numTypePromotions = []eeNumType{
		intType,
		int64Type,
		float64Type,
		mathBigRatType,
	}

	eeTypes = func() (m sync.Map) { // reflect.Type -> eeType
		m.Store(reflect.TypeOf(false), boolType)
		m.Store(reflect.TypeOf(int(0)), intType)
		m.Store(reflect.TypeOf(int64(0)), int64Type)
		m.Store(reflect.TypeOf(float64(0)), float64Type)
		m.Store(reflect.TypeOf(string("")), stringType)
		m.Store(reflect.TypeOf((*big.Rat)(nil)), mathBigRatType)
		return
	}()
)

func eeTypeOf(v interface{}) eeType {
	rt := reflect.TypeOf(v)
	return eeTypeFromReflectType(rt)
}

func eeTypeFromReflectType(rt reflect.Type) eeType {
	// newType creates a new eeType.  The type is not initialized
	// until the init function is called, which is needed for
	// recursive types, e.g.:
	//
	//	type Node struct {
	//		Next *Node
	//	}
	//
	newType := func(rt reflect.Type) (et eeType, init func(et eeType, rt reflect.Type)) {
		switch rt.Kind() {
		case reflect.Map:
			t := &eeReflectMapType{}
			return t, func(et eeType, rt reflect.Type) {
				et.(*eeReflectMapType).valueType = eeTypeFromReflectType(
					rt.Elem(),
				)
			}
		}
		panic("not implemented")
	}
	var key interface{} = rt
	v, loaded := eeTypes.Load(key)
	if loaded {
		return v.(eeType)
	}
	et, init := newType(rt)
	v, loaded = eeTypes.LoadOrStore(key, et)
	if loaded {
		return v.(eeType)
	}
	init(et, rt)
	return et
}

type eeCmpType interface {
	cmp(*exprEvaluator) int
}

type eeConvType interface {
	conv(*exprEvaluator, eeType)
}

type eeNumType interface {
	add(*exprEvaluator)
	sub(*exprEvaluator)
	mul(*exprEvaluator)
	div(*exprEvaluator)
}

type eeMapType interface {
	key(ee *exprEvaluator)
	setKey(ee *exprEvaluator)
}

type eeMemType interface {
	mem(ee *exprEvaluator) eeMem
}

type eeBoolType struct{}

var _ interface {
	eeType
} = eeBoolType{}

func (eeBoolType) append(vs *eeValues, v interface{}) int {
	return vs.appendNum(eeNumBool(v.(bool)))
}

func (eeBoolType) appendZero(vs *eeValues) int {
	return vs.appendNum(eeNumBool(false))
}

func (eeBoolType) eq(ee *exprEvaluator) bool {
	a, b := eeBoolType{}.pop2Bool(&ee.stack)
	return a == b
}

func (eeBoolType) get(vs *eeValues, i int) interface{} {
	return *vs.nums[i].bool()
}

func (eeBoolType) set(vs *eeValues, i int, v interface{}) {
	*vs.nums[i].bool() = v.(bool)
}

func (eeBoolType) peek(es *eeStack) interface{} {
	n := es.peekNum()
	return *n.bool()
}

func (eeBoolType) pop(es *eeStack) interface{} {
	n := es.popNum()
	return *n.bool()
}

func (eeBoolType) pushValue(es *eeStack, vs *eeValues, i int) {
	es.pushNum(vs.nums[i])
}

func (eeBoolType) typeCheck(e Expr, t2 eeType) eeType {
	switch e.(type) {
	case Not:
		if t2 == nil {
			return boolType
		}
	case And, Or, Eq, Ne:
		if t2 == boolType {
			return boolType
		}
	}
	return nil
}

func (eeBoolType) pop2Bool(es *eeStack) (a, b bool) {
	_, _ = es.popType(), es.popType()
	an, bn := es.popNum(), es.popNum()
	return *an.bool(), *bn.bool()
}

type numType struct {
	impl numTypeImpl
}

type numTypeImpl interface {
	eq(a, b eeNum) bool
	cmp(a, b eeNum) int

	add(a, b eeNum) eeNum
	sub(a, b eeNum) eeNum
	mul(a, b eeNum) eeNum
	div(a, b eeNum) eeNum

	valToNum(interface{}) eeNum
	numToVal(eeNum) interface{}
}

var _ interface {
	eeType
	eeNumType
} = (*numType)(nil)

func (t *numType) add(ee *exprEvaluator)                      { ee.stack.pushNum(t.impl.add(t.pop2Num(&ee.stack, false))) }
func (t *numType) append(vs *eeValues, v interface{}) int     { return vs.appendNum(t.impl.valToNum(v)) }
func (t *numType) appendZero(vs *eeValues) int                { return vs.appendNum(eeNum{}) }
func (t *numType) cmp(ee *exprEvaluator) int                  { return t.impl.cmp(t.pop2Num(&ee.stack, true)) }
func (t *numType) div(ee *exprEvaluator)                      { ee.stack.pushNum(t.impl.div(t.pop2Num(&ee.stack, false))) }
func (t *numType) eq(ee *exprEvaluator) bool                  { return t.impl.eq(t.pop2Num(&ee.stack, true)) }
func (t *numType) get(vs *eeValues, i int) interface{}        { return t.impl.numToVal(vs.nums[i]) }
func (t *numType) mul(ee *exprEvaluator)                      { ee.stack.pushNum(t.impl.mul(t.pop2Num(&ee.stack, false))) }
func (t *numType) peek(es *eeStack) interface{}               { return t.impl.numToVal(es.peekNum()) }
func (t *numType) pop(es *eeStack) interface{}                { return t.impl.numToVal(es.popNum()) }
func (t *numType) pushValue(es *eeStack, vs *eeValues, i int) { es.pushNum(vs.nums[i]) }
func (t *numType) set(vs *eeValues, i int, v interface{})     { vs.nums[i] = t.impl.valToNum(v) }
func (t *numType) sub(ee *exprEvaluator)                      { ee.stack.pushNum(t.impl.sub(t.pop2Num(&ee.stack, false))) }

func (t *numType) typeCheck(e Expr, t2 eeType) eeType {
	// by default, only support comparisson and arithmetic,
	// and return the same type.
	switch e.(type) {
	case Eq, Ne, Gt, Ge, Lt, Le, Add, Sub, Mul, Div:
		if t == t2 {
			return t2
		}
	}
	return nil
}

func (t *numType) pop2Num(es *eeStack, bothTypes bool) (a, b eeNum) {
	_ = es.popType()
	if bothTypes {
		_ = es.popType()
	}
	a, b = es.popNum(), es.popNum()
	return
}

type intTypeImpl struct{}

var _ interface {
	numTypeImpl
} = intTypeImpl{}

func (intTypeImpl) eq(a, b eeNum) bool { return *a.int() == *b.int() }
func (intTypeImpl) cmp(a, b eeNum) int { return *a.int() - *b.int() }

func (intTypeImpl) add(a, b eeNum) eeNum { return eeNumInt(*a.int() + *b.int()) }
func (intTypeImpl) sub(a, b eeNum) eeNum { return eeNumInt(*a.int() - *b.int()) }
func (intTypeImpl) mul(a, b eeNum) eeNum { return eeNumInt(*a.int() * *b.int()) }
func (intTypeImpl) div(a, b eeNum) eeNum { return eeNumInt(*a.int() / *b.int()) }

func (intTypeImpl) numToVal(n eeNum) interface{}     { return *n.int() }
func (intTypeImpl) valToNum(v interface{}) (n eeNum) { *n.int() = v.(int); return }

type int64TypeImpl struct{}

var _ interface {
	numTypeImpl
} = int64TypeImpl{}

func (int64TypeImpl) eq(a, b eeNum) bool { return *a.int64() == *b.int64() }
func (int64TypeImpl) cmp(a, b eeNum) int {
	cmp := *a.int64() - *b.int64()
	if cmp == 0 {
		return 0
	}
	if cmp < 0 {
		return -1
	}
	return 1
}

func (int64TypeImpl) add(a, b eeNum) eeNum { return eeNumInt64(*a.int64() + *b.int64()) }
func (int64TypeImpl) sub(a, b eeNum) eeNum { return eeNumInt64(*a.int64() - *b.int64()) }
func (int64TypeImpl) mul(a, b eeNum) eeNum { return eeNumInt64(*a.int64() * *b.int64()) }
func (int64TypeImpl) div(a, b eeNum) eeNum { return eeNumInt64(*a.int64() / *b.int64()) }

func (int64TypeImpl) numToVal(n eeNum) interface{}     { return *n.int64() }
func (int64TypeImpl) valToNum(v interface{}) (n eeNum) { *n.int64() = v.(int64); return }

type eeFloat64Type struct{ numType }

func (eeFloat64Type) cmp(ee *exprEvaluator) int {
	_, _ = ee.stack.popType(), ee.stack.popType()
	a, b := ee.stack.popNum(), ee.stack.popNum()
	cmp := math.Abs(*a.float64() - *b.float64())
	if cmp <= ee.ctx.floatTol {
		return 0
	}
	if cmp < 0 {
		return -1
	}
	return 1
}

func (eeFloat64Type) eq(ee *exprEvaluator) bool {
	cmp := eeFloat64Type{}.cmp(ee)
	return cmp == 0
}

type float64TypeImpl struct{}

var _ interface {
	numTypeImpl
} = float64TypeImpl{}

func (float64TypeImpl) eq(a, b eeNum) bool { return false } // not used; eeFloat64Type is used instead.
func (float64TypeImpl) cmp(a, b eeNum) int { return 0 }     // not used; eeFloat64Type is used instead.

func (float64TypeImpl) add(a, b eeNum) eeNum { return eeNumFloat64(*a.float64() + *b.float64()) }
func (float64TypeImpl) sub(a, b eeNum) eeNum { return eeNumFloat64(*a.float64() - *b.float64()) }
func (float64TypeImpl) mul(a, b eeNum) eeNum { return eeNumFloat64(*a.float64() * *b.float64()) }
func (float64TypeImpl) div(a, b eeNum) eeNum { return eeNumFloat64(*a.float64() / *b.float64()) }

func (float64TypeImpl) numToVal(n eeNum) interface{}     { return *n.float64() }
func (float64TypeImpl) valToNum(v interface{}) (n eeNum) { *n.float64() = v.(float64); return }

type eeNumConvType struct{}

var _ interface {
	eeConvType
} = eeNumConvType{}

var (
	bigInt1        = big.NewInt(1)
	bigInt2        = big.NewInt(2)
	bigIntMaxInt64 = big.NewInt(math.MaxInt64)
)

func (eeNumConvType) conv(ee *exprEvaluator, t eeType) {
	int64Of := func(v interface{}) (int64, bool) {
		switch v := v.(type) {
		case int:
			return int64(v), true
		case int8:
			return int64(v), true
		case int16:
			return int64(v), true
		case int32:
			return int64(v), true
		case int64:
			return v, true
		case uint8:
			return int64(v), true
		case uint16:
			return int64(v), true
		case uint32:
			return int64(v), true
		}
		return 0, false
	}
	float64Of := func(v interface{}) (float64, bool) {
		switch v := v.(type) {
		case float32:
			return float64(v), true
		case float64:
			return v, true
		}
		return 0, false
	}
	v, _ := ee.stack.pop()
	if i64, ok := int64Of(v); ok {
		switch t {
		case intType:
			ee.stack.pushNum(eeNumInt(int(i64)))
		case int64Type:
			ee.stack.pushNum(eeNumInt64(i64))
		case float64Type:
			ee.stack.pushNum(eeNumFloat64(float64(i64)))
		case mathBigRatType:
			r := ee.getMathBigRat()
			r.SetInt64(i64)
			ee.stack.pushAny(r)
		}
	} else if f64, ok := float64Of(v); ok {
		switch t {
		case intType:
			ee.stack.pushNum(eeNumInt(int(f64)))
		case int64Type:
			ee.stack.pushNum(eeNumInt64(int64(f64)))
		case float64Type:
			ee.stack.pushNum(eeNumFloat64(f64))
		case mathBigRatType:
			r := ee.getMathBigRat()
			r.SetFloat64(f64)
			ee.stack.pushAny(r)
		}
	} else if r, ok := v.(*big.Rat); ok {
		switch t {
		case intType:
			fallthrough
		case int64Type:
			var bi *big.Int
			if !r.IsInt() {
				// TODO: Context option to control type of rounding?
				quo, rem := (&big.Int{}).QuoRem(r.Num(), r.Denom(), (&big.Int{}))
				rem = rem.Mul(rem, bigInt2)
				if rem.Cmp(r.Denom()) >= 0 {
					quo = quo.Add(quo, bigInt1)
				}
				bi = quo
			} else {
				bi = r.Num()
			}
			i64 := bi.Int64()
			if t == intType {
				ee.stack.pushNum(eeNumInt(int(i64)))
			} else {
				ee.stack.pushNum(eeNumInt64(i64))
			}
		case float64Type:
			f, _ := r.Float64()
			ee.stack.pushNum(eeNumFloat64(f))
		case mathBigRatType:
			ee.stack.pushAny(v)
		}
	} else if u64, ok := v.(uint64); ok {
		switch t {
		case intType:
			ee.stack.pushNum(eeNumInt(int(u64)))
		case int64Type:
			ee.stack.pushNum(eeNumInt64(int64(u64)))
		case float64Type:
			ee.stack.pushNum(eeNumFloat64(float64(u64)))
		case mathBigRatType:
			r := ee.getMathBigRat()
			r.SetUint64(u64)
			ee.stack.pushAny(r)
		}
	} else if u32, ok := v.(uint32); ok {
		switch t {
		case intType:
			ee.stack.pushNum(eeNumInt(int(u32)))
		case int64Type:
			ee.stack.pushNum(eeNumInt64(int64(u32)))
		case float64Type:
			ee.stack.pushNum(eeNumFloat64(float64(u32)))
		case mathBigRatType:
			r := ee.getMathBigRat()
			r.SetUint64(uint64(u32))
			ee.stack.pushAny(r)
		}
	}
	ee.stack.pushType(t)
	return
}

type timeDurationTypeImpl struct{ int64TypeImpl }

func (timeDurationTypeImpl) numToVal(n eeNum) interface{} { return time.Duration(*n.int64()) }
func (timeDurationTypeImpl) valToNum(v interface{}) (n eeNum) {
	*n.int64() = int64(v.(time.Duration))
	return
}

type eeTimeType struct{}

var _ interface {
	eeType
	eeNumType
} = eeTimeType{}

func (eeTimeType) add(ee *exprEvaluator) {
	_, _ = ee.stack.popType(), ee.stack.popType()
	t := ee.stack.popAny().(time.Time)
	n := ee.stack.popNum()
	d := time.Duration(*n.int64())
	ee.stack.push(t.Add(d), timeType)
}

func (eeTimeType) append(vs *eeValues, v interface{}) int { return vs.appendAny(v) }
func (eeTimeType) appendZero(vs *eeValues) int            { return vs.appendAny((*time.Time)(nil)) }
func (eeTimeType) div(ee *exprEvaluator)                  { panic("cannot divide a time") }

func (eeTimeType) eq(ee *exprEvaluator) bool {
	_, _ = ee.stack.popType(), ee.stack.popType()
	return ee.stack.popAny().(time.Time).Equal(ee.stack.popAny().(time.Time))
}

func (eeTimeType) get(vs *eeValues, i int) interface{}        { return vs.anys[i] }
func (eeTimeType) mul(ee *exprEvaluator)                      { panic("cannot multiply a time") }
func (eeTimeType) peek(es *eeStack) interface{}               { return es.peekAny() }
func (eeTimeType) pop(es *eeStack) interface{}                { return es.popAny() }
func (eeTimeType) pushValue(es *eeStack, vs *eeValues, i int) { es.pushAny(vs.anys[i]) }

func (eeTimeType) set(vs *eeValues, i int, v interface{}) {
	vs.anys[i] = v
}

func (eeTimeType) sub(ee *exprEvaluator) {
	_ = ee.stack.popType()
	t := ee.stack.popAny().(time.Time)
	et := ee.stack.popType()
	switch et {
	case timeType:
		t2 := ee.stack.popAny().(time.Time)
		ee.stack.push(t.Sub(t2), timeDurationType)
		return
	case timeDurationType:
		n := ee.stack.popNum()
		d := time.Duration(*n.int64())
		ee.stack.push(t.Add(-d), timeType)
		return
	}
	panic(fmt.Errorf(
		"cannot subtract %[1]v (type: %[1]T) from "+
			"%[2]v (type: %[2]T)",
		et.pop(&ee.stack), t,
	))
}

func (eeTimeType) typeCheck(e Expr, t2 eeType) eeType {
	switch e.(type) {
	case Eq, Ne, Gt, Ge, Lt, Le:
		if t2 == timeType {
			return timeType
		}
	case Add:
		if t2 == timeDurationType {
			return timeType
		}
	case Sub:
		switch t2 {
		case timeType:
			return timeDurationType
		case timeDurationType:
			return timeType
		}
	}
	return nil
}

type eeAnyType struct{}

func (eeAnyType) append(vs *eeValues, v interface{}) int { return vs.appendAny(v) }
func (eeAnyType) appendZero(vs *eeValues) int            { return vs.appendAny(nil) }

func (eeAnyType) eq(ee *exprEvaluator) bool {
	_, _ = ee.stack.popType(), ee.stack.popType()
	a, b := ee.stack.popAny(), ee.stack.popAny()
	return a == b
}

func (eeAnyType) get(vs *eeValues, i int) interface{}        { return vs.anys[i] }
func (eeAnyType) set(vs *eeValues, i int, v interface{})     { vs.anys[i] = v }
func (eeAnyType) peek(es *eeStack) interface{}               { return es.peekAny() }
func (eeAnyType) pop(es *eeStack) interface{}                { return es.popAny() }
func (eeAnyType) pushValue(es *eeStack, vs *eeValues, i int) { es.pushAny(vs.anys[i]) }

type eeMathBigRatType struct{ eeAnyType }

var _ interface {
	eeType
	eeNumType
} = eeMathBigRatType{}

func (eeMathBigRatType) add(ee *exprEvaluator) {
	a, b := eeMathBigRatType{}.pop2BigRat(&ee.stack, false)
	ee.stack.push(a.Add(a, b), mathBigRatType)
	ee.putMathBigRat(b)
}

func (eeMathBigRatType) cmp(ee *exprEvaluator) int {
	a, b := eeMathBigRatType{}.pop2BigRat(&ee.stack, true)
	return a.Cmp(b)
}

func (eeMathBigRatType) div(ee *exprEvaluator) {
	a, b := eeMathBigRatType{}.pop2BigRat(&ee.stack, false)
	ee.stack.push(a.Quo(a, b), mathBigRatType)
	ee.putMathBigRat(b)
}

func (eeMathBigRatType) eq(ee *exprEvaluator) bool {
	return eeMathBigRatType{}.cmp(ee) == 0
}

func (eeMathBigRatType) mul(ee *exprEvaluator) {
	a, b := eeMathBigRatType{}.pop2BigRat(&ee.stack, false)
	ee.stack.push(a.Mul(a, b), mathBigRatType)
	ee.putMathBigRat(b)
}

func (eeMathBigRatType) sub(ee *exprEvaluator) {
	a, b := eeMathBigRatType{}.pop2BigRat(&ee.stack, false)
	ee.stack.push(a.Sub(a, b), mathBigRatType)
	ee.putMathBigRat(b)
}

func (eeMathBigRatType) typeCheck(e Expr, t2 eeType) eeType {
	if t2 != mathBigRatType {
		return nil
	}
	switch e.(type) {
	case Eq, Ne, Gt, Ge, Lt, Le:
		return boolType
	case Add, Sub, Mul, Div:
		return mathBigRatType
	}
	return nil
}

func (eeMathBigRatType) pop2BigRat(es *eeStack, bothTypes bool) (a, b *big.Rat) {
	_ = es.popType()
	if bothTypes {
		_ = es.popType()
	}
	a = es.popAny().(*big.Rat)
	b = es.popAny().(*big.Rat)
	return
}

type eeStringType struct{}

func (eeStringType) append(vs *eeValues, v interface{}) int {
	vs.strs = append(vs.strs, v.(string))
	return len(vs.strs) - 1
}

func (eeStringType) appendZero(vs *eeValues) int { return vs.appendStr("") }

func (eeStringType) cmp(ee *exprEvaluator) int {
	a, b := eeStringType{}.pop2String(&ee.stack)
	cmp := ee.ctx.strcmp(a, b)
	return cmp
}

func (eeStringType) eq(ee *exprEvaluator) bool {
	a, b := eeStringType{}.pop2String(&ee.stack)
	return ee.ctx.streq(a, b)
}

func (eeStringType) get(vs *eeValues, i int) interface{}        { return vs.strs[i] }
func (eeStringType) peek(es *eeStack) interface{}               { return es.peekStr() }
func (eeStringType) pop(es *eeStack) interface{}                { return es.popStr() }
func (eeStringType) pushValue(es *eeStack, vs *eeValues, i int) { es.pushStr(vs.strs[i]) }
func (eeStringType) set(vs *eeValues, i int, v interface{})     { vs.strs[i] = v.(string) }
func (eeStringType) typeCheck(e Expr, t2 eeType) eeType {
	switch e.(type) {
	case Eq, Ne, Gt, Ge, Lt, Le:
		if t2 == stringType {
			return boolType
		}
	case Add:
		if t2 == stringType {
			return stringType
		}
	}
	return nil
}

func (eeStringType) pop2String(es *eeStack) (a, b string) {
	_, _ = es.popType(), es.popType()
	a = es.popStr()
	b = es.popStr()
	return
}

type eeVarType struct {
	eeAnyType
	valueType eeType
}

var _ interface {
	eeType
} = (*eeVarType)(nil)

func (t eeVarType) typeCheck(e Expr, t2 eeType) eeType {
	return t.valueType.typeCheck(e, t2)
}

type eeMem interface {
	memType() eeType
	get(ee *exprEvaluator)
	ref(ee *exprEvaluator)
	set(ee *exprEvaluator)
}

type eeReflectType struct {
	eeAnyType
	mems  []eeMem
	rtype reflect.Type
}

var _ interface {
	eeType
	eeMemType
} = (*eeReflectType)(nil)

func (rt *eeReflectType) mem(ee *exprEvaluator) eeMem {
	_ = ee.stack.popType()
	n := ee.stack.popNum()
	return rt.mems[*n.int()]
}

func (rt *eeReflectType) typeCheck(e Expr, t2 eeType) eeType {
	return rt.mems[e.(Mem)[1].(int)].memType()
}

type eeStructField struct {
	et eeType
	ix []int
}

var _ interface {
	eeMem
} = (*eeStructField)(nil)

func (sf *eeStructField) get(ee *exprEvaluator) {
	ee.stack.push(sf.field(ee).Interface(), sf.et)
}

func (sf *eeStructField) ref(ee *exprEvaluator) {
	ee.stack.push(sf.field(ee).Addr().Interface(), sf.et)
}

func (sf *eeStructField) set(ee *exprEvaluator) {
	v, _ := ee.stack.pop()
	sf.field(ee).Set(reflect.ValueOf(v))
}

func (sf *eeStructField) memType() eeType { return sf.et }

func (sf *eeStructField) field(ee *exprEvaluator) reflect.Value {
	v, _ := ee.stack.pop()
	return reflect.ValueOf(v).Elem().FieldByIndex(sf.ix)
}

type eeReflectMapType struct {
	eeAnyType
	valueType eeType
}

var _ interface {
	eeType
	eeMapType
} = (*eeReflectMapType)(nil)

func (t *eeReflectMapType) key(ee *exprEvaluator) {
	m, _ := ee.stack.pop()
	k, _ := ee.stack.pop()
	v := reflect.ValueOf(m).MapIndex(reflect.ValueOf(k))
	ee.stack.push(v.Interface(), t.valueType)
}

func (t *eeReflectMapType) setKey(ee *exprEvaluator) {
	m, _ := ee.stack.pop()
	k, _ := ee.stack.pop()
	v, _ := ee.stack.pop()
	reflect.ValueOf(m).SetMapIndex(
		reflect.ValueOf(k),
		reflect.ValueOf(v),
	)
}

func (t *eeReflectMapType) typeCheck(e Expr, t2 eeType) eeType {
	switch e.(type) {
	case Eq, Ne:
		return boolType
	case Mem:
		return t.valueType
	}
	return nil
}
