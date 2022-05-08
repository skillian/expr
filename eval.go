package expr

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/davecgh/go-spew/spew"
)

const (
	debug = false
)

var (
	ErrInvalidType = errors.New("invalid type")
)

// FloatTolerance defines how close two floats must be to be considered equal.
type FloatTolerance float64

// FloatToleranceFromContext retrieves the float tolerance from the context.
// If no float tolerance is present, 0 is returned.
func FloatToleranceFromContext(ctx context.Context) float64 {
	t, _ := ctx.Value((*FloatTolerance)(nil)).(float64)
	return t
}

// AddToContext adds the float tolerance to the context.
func (t FloatTolerance) AddToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, (*FloatTolerance)(nil), float64(t))
}

// Eval evaluates an expression.
func Eval(ctx context.Context, e Expr) (res interface{}, err error) {
	// defer func() { catchError(&err, recover()) }()
	eev := exprEvaluators.Get()
	ee := eev.(*exprEvaluator)
	if len(ee.valueStack.typs) != 0 {
		panic("non-empty stack")
	}
	ee.pushExpr(e)
	ee.eval(ctx)
	if len(ee.valueStack.typs) != 1 {
		panic(fmt.Errorf("unbalanced stack: %v", spew.Sdump(ee.valueStack)))
	}
	res, _ = ee.valueStack.popAny()
	exprEvaluators.Put(eev)
	return
}

var exprEvaluators = sync.Pool{
	New: func() any {
		const arbCap = 8
		ee := &exprEvaluator{
			valueStack: eeValueStack{
				typs: make([]eeType, 0, arbCap),
				nums: make([]eeNum, 0, arbCap),
				strs: make([]string, 0, arbCap),
				anys: make([]interface{}, 0, arbCap),
			},
		}
		// get a *big.Rat and put it back to initialize:
		ee.putRat(ee.getRat())
		return ee
	},
}

type exprEvaluator struct {
	exprStack  []Expr
	valueStack eeValueStack
	free       struct {
		rats []*big.Rat
	}
}

func (ee *exprEvaluator) eval(ctx context.Context) {
	e := ee.peekExpr(0)
	if debug {
		fmt.Println(e, "->", ee.valueStack.String())
		defer func() { fmt.Println(e, "<-", ee.valueStack.String()) }()
	}
	switch e := e.(type) {
	case Not:
		ee.pushExpr(e.Operand())
		ee.eval(ctx)
		ee.valueStack.pushBool(!ee.valueStack.popBool())
	case Binary:
		ops := e.Operands()
		// pop expects to get the first operand first, so
		// when evaluating, we have to push them in the
		// opposite order:
		ee.pushExpr(ops[1])
		ee.eval(ctx)
		ee.pushExpr(ops[0])
		ee.eval(ctx)
		if _, ok := e.(interface{ diffTypesOK() }); !ok {
			ee.coalesceTypes(2)
		}
		et := ee.valueStack.peekType(0)
		nt, _ := et.(eeNumType)
		ct := eeCmpType(nt)
		if ct == nil {
			ct, _ = et.(eeCmpType)
		}
		mt, _ := et.(eeMemType)
		switch e := e.(type) {
		case Eq:
			ee.valueStack.pushBool(et.eq(ee))
		case Ne:
			ee.valueStack.pushBool(!et.eq(ee))
		case Gt:
			ee.valueStack.pushBool(ct.cmp(ee) > 0)
		case Ge:
			ee.valueStack.pushBool(ct.cmp(ee) >= 0)
		case Lt:
			ee.valueStack.pushBool(ct.cmp(ee) < 0)
		case Le:
			ee.valueStack.pushBool(ct.cmp(ee) <= 0)
		case And:
			a, b := ee.valueStack.popBool(), ee.valueStack.popBool()
			ee.valueStack.pushBool(a && b)
		case Or:
			a, b := ee.valueStack.popBool(), ee.valueStack.popBool()
			ee.valueStack.pushBool(a || b)
		case Add:
			nt.add(ee)
		case Sub:
			nt.sub(ee)
		case Mul:
			nt.mul(ee)
		case Div:
			nt.div(ee)
		case Mem:
			var ref eeRef
			if _, ok := e[0].(Mem); ok {
				v, _ := ee.valueStack.popAny()
				ref = v.(eeRef)
			} else {
				ref = mt.mem(ee)
			}
			if _, ok := ee.peekExpr(1).(Mem); ok {
				ee.valueStack.pushAny(ref)
			} else {
				ref.get(ee)
			}
		}
	default:
		eeTypeOf(e).push(&ee.valueStack, e)
	}
	_ = ee.popExpr()
}

func (ee *exprEvaluator) peekExpr(fromTop int) (e Expr) {
	if len(ee.exprStack) > fromTop {
		e = ee.exprStack[len(ee.exprStack)-1-fromTop]
	}
	return
}

func (ee *exprEvaluator) popExpr() (e Expr) {
	e = ee.exprStack[len(ee.exprStack)-1]
	ee.exprStack = ee.exprStack[:len(ee.exprStack)-1]
	return
}

func (ee *exprEvaluator) pushExpr(e Expr) {
	ee.exprStack = append(ee.exprStack, e)
}

// coalesceTypes converts the top n values on the stack to the same
// type.  The only allowed conversions are from ints and floats to
// *big.Rat.
func (ee *exprEvaluator) coalesceTypes(n int) {
	if n < 2 {
		return
	}
	ets := make([]eeType, n)
	nts := make([]eeNumType, n)
	allSame, allNums := true, true
	for i := 0; i < n; i++ {
		ets[i] = ee.valueStack.peekType(i)
		if allNums {
			nts[i], allNums = ets[i].(eeNumType)
		}
		if allSame && i > 0 {
			allSame = ets[i-1] == ets[i]
		}
	}
	if allSame {
		return
	}
	if allNums {
		rs := make([]*big.Rat, n)
		for i := 0; i < n; i++ {
			r := ee.getRat()
			rs[i] = r
			nts[i].rat(ee, &rs[i])
			if rs[i] != r {
				ee.putRat(r)
			}
		}
		for i := n - 1; i >= 0; i-- {
			ee.valueStack.pushRat(rs[i])
		}
		return
	}
	names := make([]string, n)
	for i, et := range ets {
		names[i] = fmt.Sprint(et)
	}
	panic(fmt.Errorf("cannot coalesce types: %v", strings.Join(names, ", ")))
}

func (ee *exprEvaluator) getRat() (r *big.Rat) {
	if len(ee.free.rats) == 0 {
		if cap(ee.free.rats) == 0 {
			ee.free.rats = make([]*big.Rat, 8)
		} else {
			// we ran out, so grow the capacity to the next size:
			ee.free.rats = append(ee.free.rats[:cap(ee.free.rats)], nil)
			ee.free.rats = ee.free.rats[:cap(ee.free.rats)]
		}
		// micro-optimization:
		type bigInt struct {
			bool
			words []uint
		}
		type bigRat [2]bigInt
		const num, den = 2, 2
		ratCache := make([]bigRat, len(ee.free.rats))
		natCache := make([]uint, (num+den)*len(ee.free.rats))
		for i := range ee.free.rats {
			start := i * (num + den)
			startDen := start + num
			endDen := startDen + den
			br := &ratCache[i]
			br[0].words = natCache[start:start:startDen]
			br[1].words = natCache[startDen:startDen:endDen]
			ee.free.rats[len(ee.free.rats)-1-i] = (*big.Rat)(unsafe.Pointer(br))
		}
	}
	r = ee.free.rats[len(ee.free.rats)-1]
	ee.free.rats = ee.free.rats[:len(ee.free.rats)-1]
	return
}

func (ee *exprEvaluator) putRat(r *big.Rat) {
	ee.free.rats = append(ee.free.rats, r)
}

func catchError(p *error, v interface{}) {
	if v == nil {
		*p = nil
		return
	}
	if err, ok := v.(error); ok {
		*p = err
		return
	}
	*p = fmt.Errorf("%v", v)
}

func makeInvalidType(expect, actual any) error {
	return fmt.Errorf(
		"%[1]w: expected %[2]T, actual: %[3]v (type: %[3]T)",
		ErrInvalidType, expect, actual,
	)
}

type eeValueStack struct {
	typs []eeType
	nums []eeNum
	strs []string
	anys []interface{}
}

func (s *eeValueStack) peekAny() interface{} {
	return s.peekType(0).peek(s)
}

func (s *eeValueStack) popAny() (v interface{}, t eeType) {
	t = s.peekType(0)
	v = t.pop(s)
	return
}

func (s *eeValueStack) pushAny(v interface{}) {
	eeTypeOf(v).push(s, v)
}

func (s *eeValueStack) peekBool() bool {
	s.mustPeekType(boolType{})
	n, _ := s.peekNum()
	return *n.int() != 0
}

func (s *eeValueStack) popBool() bool {
	s.mustPeekType(boolType{})
	n, _ := s.popNum()
	return *n.int() != 0
}

func (s *eeValueStack) pushBool(b bool) {
	s.pushNum(eeNumBool(b), boolType{})
}

func (s *eeValueStack) peekNum() (n eeNum, t eeType) {
	return s.nums[len(s.nums)-1], s.peekType(0)
}

func (s *eeValueStack) popNum() (n eeNum, t eeType) {
	n, t = s.peekNum()
	s.nums = s.nums[:len(s.nums)-1]
	s.typs = s.typs[:len(s.typs)-1]
	return
}

func (s *eeValueStack) pushNum(n eeNum, t eeType) {
	s.nums = append(s.nums, n)
	s.typs = append(s.typs, t)
	return
}

func (s *eeValueStack) peekInt() int {
	s.mustPeekType(intType{})
	n, _ := s.peekNum()
	return *n.int()
}

func (s *eeValueStack) popInt() int {
	s.mustPeekType(intType{})
	n, _ := s.popNum()
	return *n.int()
}

func (s *eeValueStack) pushInt(i int) {
	s.pushNum(eeNumInt(i), intType{})
}

func (s *eeValueStack) peekRat() (r *big.Rat) {
	s.mustPeekType(bigRatType{})
	return s.anys[len(s.anys)-1].(*big.Rat)
}

func (s *eeValueStack) popRat() (r *big.Rat) {
	r = s.peekRat()
	s.anys = s.anys[:len(s.anys)-1]
	s.typs = s.typs[:len(s.typs)-1]
	return
}

func (s *eeValueStack) pushRat(r *big.Rat) (i int) {
	i = len(s.anys)
	s.anys = append(s.anys, r)
	s.typs = append(s.typs, bigRatType{})
	return
}

func (s *eeValueStack) peekString() string {
	s.mustPeekType(stringType{})
	return s.strs[len(s.strs)-1]
}

func (s *eeValueStack) popString() (v string) {
	v = s.peekString()
	s.strs = s.strs[:len(s.strs)-1]
	s.typs = s.typs[:len(s.typs)-1]
	return
}

func (s *eeValueStack) pushString(v string) (i int) {
	i = len(s.strs)
	s.strs = append(s.strs, v)
	s.typs = append(s.typs, stringType{})
	return
}

func (s *eeValueStack) peekType(offset int) eeType {
	return s.typs[len(s.typs)-1-offset]
}

func (s *eeValueStack) mustPeekType(expect eeType) {
	if !debug {
		return
	}
	actual := s.peekType(0)
	eeTypeAssert(expect, actual)
}

func (s *eeValueStack) String() string {
	if len(s.typs) == 0 {
		return ""
	}
	write := func(sb *strings.Builder, v interface{}, t eeType) {
		sb.WriteByte('(')
		sb.WriteString(fmt.Sprintf("%#v", t))
		sb.WriteByte(' ')
		sb.WriteString(fmt.Sprintf("%#v", v))
		sb.WriteByte(')')
	}
	dup := *s // copy the slices
	sb := &strings.Builder{}
	v, t := dup.popAny()
	write(sb, v, t)
	for len(dup.typs) > 0 {
		sb.WriteByte(' ')
		v, t = dup.popAny()
		write(sb, v, t)
	}
	return sb.String()
}

func eeTypeAssert(expect, actual eeType) {
	if expect != actual {
		panic(fmt.Errorf(
			"expected type %[1]v (%[1]T), but actual: "+
				"%[2]v (%[2]T)", expect, actual))
	}
}

type eeNum struct {
	data uint64
}

func eeNumBool(x bool) (n eeNum) {
	if x {
		n.data--
	}
	return
}

func eeNumInt(x int) (n eeNum) {
	*n.int() = x
	return
}

func (n *eeNum) int() *int { return (*int)(unsafe.Pointer(&n.data)) }

type eeType interface {
	peek(stack *eeValueStack) interface{}

	// pop a value from the stack as an interface{}
	pop(stack *eeValueStack) interface{}

	// push a value of the current type to the expression
	// stack.
	push(stack *eeValueStack, v interface{})

	// eq pops two values of the current type and compares them for
	// equality.
	eq(ee *exprEvaluator) bool
}

var eeTypes = func() (m sync.Map) { // reflect.Type -> eeType
	m.Store(reflect.TypeOf(true), boolType{})
	m.Store(reflect.TypeOf((*big.Rat)(nil)), bigRatType{})
	m.Store(reflect.TypeOf(int(0)), intType{})
	m.Store(reflect.TypeOf(""), stringType{})
	return
}()

func eeTypeOf(v interface{}) eeType {
	newType := func(rt reflect.Type) eeType {
		if rt.Kind() == reflect.Ptr {
			rt = rt.Elem()
		}
		switch rt.Kind() {
		case reflect.Map:
			return mapType{}
		case reflect.Struct:
			return &structType{reflectType{}, rt}
		}
		panic(fmt.Sprint("unknown type:", rt))
	}
	rt := reflect.TypeOf(v)
	var key interface{} = rt
	if v, loaded := eeTypes.Load(key); loaded {
		return v.(eeType)
	}
	et := newType(rt)
	if v, loaded := eeTypes.LoadOrStore(key, et); loaded {
		return v.(eeType)
	}
	return et
}

func RegisterMapType[TKey comparable, TValue any]() {
	_, _ = eeTypes.LoadOrStore(
		reflect.TypeOf(map[TKey]TValue{}),
		genMapType[TKey, TValue]{},
	)
}

type eeCmpType interface {
	cmp(ee *exprEvaluator) int
}

type eeNumType interface {
	eeCmpType

	// rat is passed in a non-nil pointer to a non-nil *big.Rat.
	// The implementation can either set the *big.Rat or overwrite
	// the **big.Rat with its own.
	rat(ee *exprEvaluator, target **big.Rat)

	add(ee *exprEvaluator)
	sub(ee *exprEvaluator)
	mul(ee *exprEvaluator)
	div(ee *exprEvaluator)
}

type eeMemType interface {
	mem(ee *exprEvaluator) eeRef
}

type eeRef interface {
	get(ee *exprEvaluator)
}

type eeMutRef interface {
	set(ee *exprEvaluator)
}

type bigRatType struct{}

var _ interface {
	eeType
	eeNumType
} = bigRatType{}

func (bigRatType) peek(stack *eeValueStack) interface{} {
	return stack.peekRat()
}

func (bigRatType) pop(stack *eeValueStack) interface{} {
	return stack.popRat()
}

func (bigRatType) push(stack *eeValueStack, v interface{}) {
	stack.pushRat(v.(*big.Rat))
}

func (bigRatType) pop2(ee *exprEvaluator) (left, right *big.Rat) {
	left = ee.valueStack.popRat()
	right = ee.valueStack.popRat()
	return
}

func (bigRatType) add(ee *exprEvaluator) {
	left, right := bigRatType{}.pop2(ee)
	ee.valueStack.pushRat(ee.getRat().Add(left, right))
}

func (bigRatType) cmp(ee *exprEvaluator) int {
	left, right := bigRatType{}.pop2(ee)
	return left.Cmp(right)
}

func (bigRatType) div(ee *exprEvaluator) {
	left, right := bigRatType{}.pop2(ee)
	ee.valueStack.pushRat(ee.getRat().Quo(left, right))
}

func (bigRatType) eq(ee *exprEvaluator) bool {
	return bigRatType{}.cmp(ee) == 0
}

func (bigRatType) mul(ee *exprEvaluator) {
	left, right := bigRatType{}.pop2(ee)
	ee.valueStack.pushRat(ee.getRat().Mul(left, right))
}

func (bigRatType) rat(ee *exprEvaluator, target **big.Rat) {
	*target = ee.valueStack.popRat()
}

func (bigRatType) sub(ee *exprEvaluator) {
	left, right := bigRatType{}.pop2(ee)
	ee.valueStack.pushRat(ee.getRat().Sub(left, right))
}

type intType struct{}

var _ interface {
	eeType
	eeNumType
} = intType{}

func (intType) peek(stack *eeValueStack) interface{} {
	return stack.peekInt()
}

func (intType) pop(stack *eeValueStack) interface{} {
	return stack.popInt()
}

func (intType) push(stack *eeValueStack, v interface{}) {
	stack.pushInt(v.(int))
}

func (intType) pop2(ee *exprEvaluator) (left, right int) {
	left = ee.valueStack.popInt()
	right = ee.valueStack.popInt()
	return
}

func (intType) add(ee *exprEvaluator) {
	left, right := intType{}.pop2(ee)
	ee.valueStack.pushInt(left + right)
}

func (intType) cmp(ee *exprEvaluator) int {
	left, right := intType{}.pop2(ee)
	return left - right
}

func (intType) div(ee *exprEvaluator) {
	left, right := intType{}.pop2(ee)
	ee.valueStack.pushInt(left / right)
}

func (intType) eq(ee *exprEvaluator) bool {
	left, right := intType{}.pop2(ee)
	return left == right
}

func (intType) mul(ee *exprEvaluator) {
	left, right := intType{}.pop2(ee)
	ee.valueStack.pushInt(left * right)
}

func (intType) rat(ee *exprEvaluator, target **big.Rat) {
	(*target).SetInt64(int64(ee.valueStack.popInt()))
}

func (intType) sub(ee *exprEvaluator) {
	left, right := intType{}.pop2(ee)
	ee.valueStack.pushInt(left - right)
}

type boolType struct{}

var _ eeType = boolType{}

func (boolType) eq(ee *exprEvaluator) bool {
	return ee.valueStack.popBool() == ee.valueStack.popBool()
}

func (boolType) peek(stack *eeValueStack) interface{} {
	return stack.peekBool()
}

func (boolType) pop(stack *eeValueStack) interface{} {
	return stack.popBool()
}

func (boolType) push(stack *eeValueStack, v interface{}) {
	stack.pushBool(v.(bool))
}

type stringType struct{}

var _ interface {
	eeType
	eeCmpType
} = stringType{}

func (stringType) pop2(ee *exprEvaluator) (left, right string) {
	left = ee.valueStack.popString()
	right = ee.valueStack.popString()
	return
}

func (stringType) cmp(ee *exprEvaluator) int {
	left, right := stringType{}.pop2(ee)
	return strings.Compare(left, right)
}

func (stringType) eq(ee *exprEvaluator) bool {
	left, right := stringType{}.pop2(ee)
	return left == right
}

func (stringType) peek(stack *eeValueStack) interface{} {
	return stack.peekString()
}

func (stringType) pop(stack *eeValueStack) interface{} {
	return stack.popString()
}

func (stringType) push(stack *eeValueStack, v interface{}) {
	stack.pushString(v.(string))
}

type eeAnyType struct{}

func (eeAnyType) peek(stack *eeValueStack) interface{} {
	return stack.anys[len(stack.anys)-1]
}

func (eeAnyType) pop(stack *eeValueStack) interface{} {
	v := eeAnyType{}.peek(stack)
	stack.anys = stack.anys[:len(stack.anys)-1]
	stack.typs = stack.typs[:len(stack.typs)-1]
	return v
}

func (eeAnyType) push(stack *eeValueStack, v interface{}) {
	stack.anys = append(stack.anys, v)
	stack.typs = append(stack.typs, reflectType{})
}

type reflectType struct{ eeAnyType }

var _ eeType = reflectType{}

func (reflectType) pop2(ee *exprEvaluator) (left, right interface{}) {
	left, _ = ee.valueStack.popAny()
	right, _ = ee.valueStack.popAny()
	return
}

func (reflectType) eq(ee *exprEvaluator) (ok bool) {
	left, right := reflectType{}.pop2(ee)
	defer func(p *bool, a, b interface{}) {
		if recover() != nil {
			left := *((*[2]uintptr)(unsafe.Pointer(&a)))
			right := *((*[2]uintptr)(unsafe.Pointer(&b)))
			*p = left == right
		}
	}(&ok, left, right)
	return left == right
}

type mapType struct{ reflectType }

var _ interface {
	eeType
	eeMemType
} = mapType{}

func (mapType) mem(ee *exprEvaluator) eeRef {
	m, key := mapType{}.pop2(ee)
	return mapRef{
		m: reflect.ValueOf(m),
		k: reflect.ValueOf(key),
	}
}

func (mapType) push(stack *eeValueStack, v interface{}) {
	stack.anys = append(stack.anys, v)
	stack.typs = append(stack.typs, mapType{})
}

type mapRef struct {
	m reflect.Value
	k reflect.Value
}

func (m mapRef) get(ee *exprEvaluator) {
	v := m.m.MapIndex(m.k).Interface()
	eeTypeOf(v).push(&ee.valueStack, v)
}

func (m mapRef) set(ee *exprEvaluator) {
	v, _ := ee.valueStack.popAny()
	m.m.SetMapIndex(m.k, reflect.ValueOf(v))
}

type genMapType[TKey comparable, TValue any] struct{ reflectType }

var _ interface {
	eeType
	eeMemType
} = genMapType[int, int]{}

func (genMapType[TKey, TValue]) mem(ee *exprEvaluator) eeRef {
	m, _ := ee.valueStack.popAny()
	k, _ := ee.valueStack.popAny()
	return genMapRef[TKey, TValue]{
		m: m.(map[TKey]TValue),
		k: k.(TKey),
	}
}

type genMapRef[TKey comparable, TValue any] struct {
	m map[TKey]TValue
	k TKey
}

func (m genMapRef[TKey, TValue]) get(ee *exprEvaluator) {
	v := m.m[m.k]
	eeTypeOf(v).push(&ee.valueStack, v)
}

func (m genMapRef[TKey, TValue]) set(ee *exprEvaluator) {
	v, _ := ee.valueStack.popAny()
	m.m[m.k] = v.(TValue)
}

type structType struct {
	reflectType
	tp reflect.Type
}

var _ interface {
	eeType
	eeMemType
} = (*structType)(nil)

func (t *structType) mem(ee *exprEvaluator) eeRef {
	v, _ := ee.valueStack.popAny()
	self := reflect.ValueOf(v)
	index := ee.valueStack.popInt()
	return structMem{self: self, index: index}
}

func (t *structType) push(stack *eeValueStack, v interface{}) {
	stack.anys = append(stack.anys, v)
	stack.typs = append(stack.typs, t)
}

type structMem struct {
	self  reflect.Value
	index int
}

var _ interface {
	eeRef
	eeMutRef
	eeMemType
} = structMem{}

func (m structMem) get(ee *exprEvaluator) {
	v := m.self.Elem().Field(m.index).Interface()
	eeTypeOf(v).push(&ee.valueStack, v)
}

func (m structMem) set(ee *exprEvaluator) {
	v, _ := ee.valueStack.popAny()
	m.self.Elem().Field(m.index).Set(reflect.ValueOf(v))
}

func (m structMem) mem(ee *exprEvaluator) eeRef {
	return structMem{
		self:  m.self.Elem().Field(m.index).Addr(),
		index: ee.valueStack.popInt(),
	}
}
