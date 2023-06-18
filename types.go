package expr

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// RegisterIntType registers an int-kind type
func RegisterIntType[T ~int]() {
	var v T
	rt := reflect.TypeOf(v)
	eeTypes.LoadOrStore(rt, eeIntKindType[T]{rt: rt})
}

// RegisterInt64Type registers an int64-kind type
func RegisterInt64Type[T ~int64]() {
	var v T
	rt := reflect.TypeOf(v)
	eeTypes.LoadOrStore(rt, eeInt64KindType[T]{rt: rt})
}

// RegisterInt64Type registers an int64-kind type
func RegisterStringType[T ~string]() {
	var v T
	rt := reflect.TypeOf(v)
	st, _ := eeTypes.LoadOrStore(rt, eeStringKindType[T]{rt: rt})
	var m map[T]interface{}
	rt = reflect.TypeOf(m)
	eeTypes.LoadOrStore(rt, eeMapStrAnyKindType[T]{
		eeReflectType: eeReflectType{rt: rt},
		keyType:       st.(eeStringKindType[T]),
	})
}

func init() {
	RegisterInt64Type[time.Duration]()
}

// eeType is the definition of a type that is used by the
// exprEvaluator.
type eeType interface {
	// checkType checks to see if the current eeType supports the
	// given expression.  If e is a binary operation, then t2 is
	// the type of the second operand.  The returned type is the
	// type of the result of the operation.
	checkType(e Expr, t2 eeType) (eeType, error)

	// eq compares two operands on the top of the stack for
	// equality.  eq must pop the types and their values.
	eq(vs *eeValues) bool

	// kind gets the kind of the type (opAny, opBool, opInt, etc.).
	kind() opKind

	// get a value from the values a the given substack index
	get(vs *eeValues, index int) interface{}

	// pop a value from the correct eeValues "substack."  This
	// does not deal with the type substack.
	pop(vs *eeValues) interface{}

	// push a value to the correct eeValues "substack."  This does
	// not deal with the type substack.
	push(vs *eeValues, v interface{}) int

	// pushElem is like push but it expects a pointer to the value
	// to be passed.
	//
	// This is useful for pushing values in a loop; ptr will escape
	// but the element values can keep being stored into the same
	// ptr so, it's one escape not n escapes.
	pushElem(vs *eeValues, ptr interface{}) int

	// reflectType gets the reflect.Type
	reflectType() reflect.Type
}

// typeOf is like reflect.ValueOf to get the eeType
func typeOf(v interface{}) eeType {
	return getType(reflect.TypeOf(v))
}

// getType is like reflect.TypeOf to get the eeType
func getType(rt reflect.Type) eeType {
	initMethods := func(et *eeReflectType) {
		numMethods := rt.NumMethod()
		et.fns = make([]eeReflectFunc, numMethods)
		// preallocate single slice for all fn parameters:
		numInOutTypes := 0
		for i := 0; i < numMethods; i++ {
			m := rt.Method(i)
			numInOutTypes += m.Type.NumIn() + m.Type.NumOut()
		}
		inOutTypes := make([]eeType, numInOutTypes)
		numInOutTypes = 0
		for i := 0; i < numMethods; i++ {
			m := rt.Method(i)
			// init et.fns:
			{
				numIn := m.Type.NumIn()
				for j := 0; j < numIn; j++ {
					inOutTypes[numInOutTypes+j] = getType(m.Type.In(j))
				}
				numOut := m.Type.NumOut()
				for j := 0; j < numOut; j++ {
					inOutTypes[numInOutTypes+numIn+j] = getType(m.Type.Out(j))
				}
				et.fns[i] = eeReflectFunc{
					inOutTypes: inOutTypes[numInOutTypes : numIn : numIn+numOut],
					fn:         m.Func,
				}
			}
			// init et.<name>
			switch m.Name {
			case "Eq", "Equal", "Equals":
				et.eqOrd = i + 1
			case "Cmp", "Compare":
				et.cmpOrd = i + 1
			case "Add":
				et.addOrd = i + 1
			case "Sub":
				et.subOrd = i + 1
			case "Mul":
				et.mulOrd = i + 1
			case "Div":
				et.divOrd = i + 1
			}
		}
	}
	initStruct := func(et *eeReflectType) {
		et.numFields = et.reflectType().NumField()
		initMethods(et)
	}
	createType := func(rt reflect.Type) (t eeType) {
		switch rt.Kind() {
		case reflect.Bool, reflect.Float64, reflect.Int, reflect.Int64, reflect.Map, reflect.String:
			panic("bool, float64, int, int64, map, and string kind types must be registered")
		case reflect.Pointer:
			if rt.ConvertibleTo(bigRatType) {
				return eeRatKindType{rt: rt}
			}
			ptrRt := rt
			rt = rt.Elem()
			pt := &eePtrType{
				eeReflectType: eeReflectType{
					rt: ptrRt,
				},
				derefType: getType(rt),
			}
			pt.eeReflectType.eeTypeInit.fn = func() {
				initMethods(&pt.eeReflectType)
			}
			return pt
		default:
			et := &eeReflectType{rt: rt}
			switch rt.Kind() {
			case reflect.Struct:
				et.eeTypeInit.fn = func() {
					initStruct(et)
				}
			default:
				et.eeTypeInit.fn = func() {
					initMethods(et)
				}
			}
			return et
		}
	}
	k := interface{}(rt)
	v, ok := eeTypes.Load(k)
	if ok {
		return v.(eeType)
	}
	et := createType(rt)
	v, ok = eeTypes.LoadOrStore(k, et)
	if ok {
		et = v.(eeType)
	}
	return et
}

type eeCmpType interface {
	// cmp compares the top two operands on the stack.  cmp must
	// pop the types and their values.
	cmp(vs *eeValues) int
}

type eeMemType interface {
	// getMem pops the eeMemType instance and its member reference
	// from the stack and pushes a copy of the value of the
	// associated member.
	getMem(vs *eeValues)

	// setMem the value associated with the ref from the stack.
	setMem(vs *eeValues)
}

// eeAddType is implemented by types whose operands can be added
// together
//
// E.g. string + string = string
type eeAddType interface {
	add(vs *eeValues)
}

// eeSubType is implemented by types whose operands can be subtracted
//
// E.g. time.Time - time.Time = time.Duration
type eeSubType interface {
	sub(vs *eeValues)
}

// eeNumType is implemented by types that can have arithmetic operations
// such as addition and division on their operands.
type eeNumType interface {
	eeAddType
	eeSubType
	mul(vs *eeValues)
	div(vs *eeValues)
}

var (
	bigInts = func() (rs [3]big.Int) {
		for i := range rs {
			rs[i].SetInt64(int64(i))
		}
		return
	}()

	anyType       = reflect.TypeOf((*interface{})(nil)).Elem()
	bigRatType    = reflect.TypeOf((*big.Rat)(nil))
	boolType      = reflect.TypeOf(false)
	errorType     = reflect.TypeOf((*error)(nil)).Elem()
	float64Type   = reflect.TypeOf(float64(0))
	intType       = reflect.TypeOf(int(0))
	int64Type     = reflect.TypeOf(int64(0))
	stringType    = reflect.TypeOf("")
	mapStrAnyType = reflect.TypeOf(map[string]interface{}(nil))

	defaultTypeInit = func() (ti eeTypeInit) {
		ti.fn = func() {}
		return
	}

	eeAnyType   eeType = eeAnyKindType{}
	eeBoolType  eeType = eeBoolKindType[bool]{rt: boolType}
	eeErrorType eeType = &eeReflectType{
		rt:         errorType,
		eeTypeInit: defaultTypeInit(),
	}
	eeFloat64Type eeType = eeFloatKindType[float64]{rt: float64Type}
	eeIntType     eeType = eeIntKindType[int]{rt: intType}
	eeInt64Type   eeType = eeInt64KindType[int64]{rt: int64Type}
	eeRatType     eeType = eeRatKindType{rt: bigRatType}
	eeStringType  eeType = eeStringKindType[string]{rt: stringType}

	eeMapStrAnyType eeType = &eeMapStrAnyKindType[string]{
		eeReflectType: eeReflectType{
			rt:         mapStrAnyType,
			eeTypeInit: defaultTypeInit(),
		},
		keyType: eeStringType.(eeStringKindType[string]),
	}

	eeTypes = func() (m sync.Map) {
		for _, et := range []eeType{
			eeBoolType,
			eeErrorType,
			eeFloat64Type,
			eeIntType,
			eeInt64Type,
			eeRatType,
			eeStringType,
			eeMapStrAnyType,
		} {
			m.Store(et.reflectType(), et)
		}
		return
	}()
)

type eeAnyKindType struct{}

func (eeAnyKindType) checkType(e Expr, t2 eeType) (eeType, error) {
	switch e.(type) {
	case Eq:
	case Ne:
		return eeBoolType, nil
	}
	return nil, ErrInvalidType
}

func (et eeAnyKindType) eq(vs *eeValues) (equal bool) {
	a := vs.popType().pop(vs)
	b := vs.popType().pop(vs)
	defer func() {
		if recover() != nil {
			ad := *((*[2]uintptr)(unsafe.Pointer(&a)))
			bd := *((*[2]uintptr)(unsafe.Pointer(&b)))
			equal = ad == bd
		}
	}()
	return a == b
}

func (eeAnyKindType) get(vs *eeValues, i int) interface{}  { return vs.anys[i] }
func (eeAnyKindType) kind() opKind                         { return opAny }
func (eeAnyKindType) pop(vs *eeValues) interface{}         { return vs.popAny() }
func (eeAnyKindType) push(vs *eeValues, v interface{}) int { return vs.pushAny(v) }
func (eeAnyKindType) pushElem(vs *eeValues, ptr interface{}) int {
	return vs.pushAny(*(ptr.(*interface{})))
}
func (eeAnyKindType) reflectType() reflect.Type { return anyType }

type eeReflectFunc struct {
	// inOutTypes holds input parameter types in [0:len(inOutTypes)]
	// and output parameter types in [len(inOutTypes):cap(inOutTypes)]
	inOutTypes []eeType
	fn         reflect.Value
}

type eeTypeInit struct {
	once sync.Once
	fn   func()
}

func (ti *eeTypeInit) init() { ti.once.Do(ti.fn) }

type eePtrType struct {
	eeReflectType
	derefType eeType
}

var _ eeType = (*eePtrType)(nil)

func (et *eePtrType) checkType(e Expr, t2 eeType) (eeType, error) {
	switch e := e.(type) {
	case Mem:
		return et.derefType.checkType(e, t2)
	}
	return et.eeReflectType.checkType(e, t2)
}

type eeReflectType struct {
	eeAnyKindType
	eeTypeInit
	rt        reflect.Type
	numFields int
	fns       []eeReflectFunc

	// the Ords below are "ordinals" for entries in fns.  ord 1
	// corresponds to fns[0], etc.  This allows the ord 0 to
	// indicate the lack of a function.

	eqOrd  int
	cmpOrd int
	addOrd int
	subOrd int
	mulOrd int
	divOrd int
}

func eeReflectTypeFn[TArg, TRes any](et *eeReflectType, arg TArg, anyFn, boolFn, floatFn, intFn, ratFn, strFn func(TArg) TRes) TRes {
	if et == nil {
		panic("et is nil")
	}
	rt := et.reflectType()
	if rt == nil {
		panic(fmt.Errorf(
			"%#[1]v (@%[1]p) reflectType is nil",
			et,
		))
	}
	switch et.reflectType().Kind() {
	case reflect.Bool:
		return boolFn(arg)
	case reflect.Float32, reflect.Float64:
		return floatFn(arg)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intFn(arg)
	case reflect.Pointer:
		if et.reflectType().ConvertibleTo(bigRatType) {
			return ratFn(arg)
		}
	case reflect.String:
		return strFn(arg)
	}
	return anyFn(arg)
}

func (et *eeReflectType) call(fnOrd int, vs *eeValues) {
	var params [2]reflect.Value
	params[0] = reflect.ValueOf(vs.popType().pop(vs))
	params[1] = reflect.ValueOf(vs.popType().pop(vs))
	res := et.fns[fnOrd-1].fn.Call(params[:])
	if len(res) > 1 {
		vs.pushAny(res[1].Interface())
		vs.pushType(eeErrorType)
	}
	vs.push(res[0].Interface())
}

func (et *eeReflectType) add(vs *eeValues) { et.call(et.addOrd, vs) }

func (et *eeReflectType) checkType(e Expr, t2 eeType) (eeType, error) {
	getOrdFn := func(et *eeReflectType, ord int) (eeType, error) {
		if ord == 0 {
			return nil, ErrInvalidType
		}
		iots := et.fns[ord-1].inOutTypes
		return iots[:cap(iots)][len(iots)], nil
	}
	et.init()
	switch e := e.(type) {
	case Eq, Ne:
		return eeBoolType, nil
	case Lt, Le, Ge, Gt:
		if et.cmpOrd != 0 {
			return eeBoolType, nil
		}
	case Add:
		return getOrdFn(et, et.addOrd)
	case Sub:
		return getOrdFn(et, et.subOrd)
	case Mul:
		return getOrdFn(et, et.mulOrd)
	case Div:
		return getOrdFn(et, et.divOrd)
	case Mem:
		if t2 != eeIntType {
			return nil, fmt.Errorf(
				"%w: Cannot get %[2]v (type: %[2]T) member of %[3]v (type: %[3]T)",
				ErrInvalidType, e[1], e[0],
			)
		}
		return getType(et.reflectType().Field(e[1].(int)).Type), nil
	}
	return et.eeAnyKindType.checkType(e, t2)
}

func (et *eeReflectType) div(vs *eeValues) { et.call(et.divOrd, vs) }

func (et *eeReflectType) eq(vs *eeValues) bool {
	if et.eqOrd == 0 {
		return et.eeAnyKindType.eq(vs)
	}
	et.call(et.eqOrd, vs)
	_ = vs.popType()
	return vs.popBool()
}

func (et *eeReflectType) get(vs *eeValues, i int) interface{} {
	type TArg struct {
		rt reflect.Type
		vs *eeValues
		i  int
	}
	return eeReflectTypeFn(
		et, TArg{et.reflectType(), vs, i},
		func(args TArg) interface{} { return args.vs.anys[args.i] },
		func(args TArg) interface{} {
			v := args.vs.nums[args.i] != 0
			if args.rt == boolType {
				return v
			}
			rv := reflect.New(args.rt).Elem()
			rv.SetBool(v)
			return rv.Interface()
		},
		func(args TArg) interface{} {
			v := (*args.vs.floats())[args.i]
			if args.rt == float64Type {
				return v
			}
			rv := reflect.New(args.rt).Elem()
			rv.SetFloat(v)
			return rv.Interface()
		},
		func(args TArg) interface{} {
			v := args.vs.nums[args.i]
			if args.rt == int64Type {
				return v
			}
			rv := reflect.New(args.rt).Elem()
			rv.SetInt(v)
			return rv.Interface()
		},
		func(args TArg) interface{} {
			v := args.vs.anys[args.i]
			if args.rt == reflect.TypeOf(v) {
				return v
			}
			return reflect.ValueOf(v).Convert(args.rt).Interface()
		},
		func(args TArg) interface{} {
			v := args.vs.strs[args.i]
			if args.rt == stringType {
				return v
			}
			rv := reflect.New(args.rt).Elem()
			rv.SetString(v)
			return rv.Interface()
		},
	)
}

func (et *eeReflectType) kind() opKind {
	return eeReflectTypeFn(
		et, struct{}{},
		func(_ struct{}) opKind { return opAny },
		func(_ struct{}) opKind { return opBool },
		func(_ struct{}) opKind { return opFloat },
		func(_ struct{}) opKind { return opInt },
		func(_ struct{}) opKind { return opRat },
		func(_ struct{}) opKind { return opStr },
	)
}

func (et *eeReflectType) mul(vs *eeValues) { et.call(et.mulOrd, vs) }

func (et *eeReflectType) pop(vs *eeValues) interface{} {
	type TArg struct {
		//rt reflect.Type
		vs *eeValues
	}
	return eeReflectTypeFn(
		et, TArg{vs},
		func(arg TArg) interface{} { return arg.vs.popAny() },
		func(arg TArg) interface{} { return eeBoolType.pop(arg.vs) },
		func(arg TArg) interface{} { return eeFloat64Type.pop(arg.vs) },
		func(arg TArg) interface{} { return eeInt64Type.pop(arg.vs) },
		func(arg TArg) interface{} { return eeRatType.pop(arg.vs) },
		func(arg TArg) interface{} { return eeStringType.pop(arg.vs) },
	)
}

func (et *eeReflectType) push(vs *eeValues, v interface{}) int {
	type TArg struct {
		//rt reflect.Type
		vs *eeValues
		v  interface{}
	}
	return eeReflectTypeFn(
		et, TArg{vs, v},
		func(arg TArg) int { return arg.vs.pushAny(arg.v) },
		func(arg TArg) int { return eeBoolType.push(arg.vs, arg.v) },
		func(arg TArg) int { return eeFloat64Type.push(arg.vs, arg.v) },
		func(arg TArg) int { return eeInt64Type.push(arg.vs, arg.v) },
		func(arg TArg) int { return eeRatType.push(arg.vs, arg.v) },
		func(arg TArg) int { return eeStringType.push(arg.vs, arg.v) },
	)
}

func (et *eeReflectType) pushElem(vs *eeValues, ptr interface{}) int {
	type TArg struct {
		vs  *eeValues
		ptr interface{}
	}
	return eeReflectTypeFn(
		et, TArg{vs, ptr},
		func(arg TArg) int { return arg.vs.pushAny(arg.vs) },
		func(arg TArg) int { return eeBoolType.pushElem(arg.vs, arg.ptr) },
		func(arg TArg) int { return eeFloat64Type.pushElem(arg.vs, arg.ptr) },
		func(arg TArg) int { return eeInt64Type.pushElem(arg.vs, arg.ptr) },
		func(arg TArg) int { return eeRatType.pushElem(arg.vs, arg.ptr) },
		func(arg TArg) int { return eeStringType.pushElem(arg.vs, arg.ptr) },
	)
}

func (et *eeReflectType) reflectType() reflect.Type { return et.rt }

func (et *eeReflectType) sub(vs *eeValues) { et.call(et.subOrd, vs) }

type eeBoolKindType[T ~bool] struct{ rt reflect.Type }

func (et eeBoolKindType[T]) checkType(e Expr, t2 eeType) (eeType, error) {
	if _, ok := e.(Not); ok {
		if t2 != nil {
			return nil, ErrInvalidType
		}
		return et, nil
	}
	if t2 != et {
		return nil, ErrInvalidType
	}
	switch e.(type) {
	case Eq:
	case Ne:
	case And:
	case Or:
	default:
		return nil, ErrInvalidType
	}
	return et, nil
}

func (et eeBoolKindType[T]) cmp(vs *eeValues) int {
	if et.eq(vs) {
		return 0
	}
	return 1
}

func (et eeBoolKindType[T]) eq(vs *eeValues) bool {
	a, _ := vs.popBool(), vs.popType()
	b, _ := vs.popBool(), vs.popType()
	return a == b
}

func (et eeBoolKindType[T]) get(vs *eeValues, i int) interface{} {
	return T(vs.nums[i] != 0)
}

func (et eeBoolKindType[T]) kind() opKind { return opBool }

func (et eeBoolKindType[T]) pop(vs *eeValues) interface{} {
	return T(vs.popBool())
}

func (et eeBoolKindType[T]) push(vs *eeValues, v interface{}) int {
	return vs.pushBool(bool(v.(T)))
}

func (et eeBoolKindType[T]) pushElem(vs *eeValues, ptr interface{}) int {
	return vs.pushBool(bool(*(ptr.(*T))))
}

func (et eeBoolKindType[T]) reflectType() reflect.Type { return et.rt }

type eeIntKindType[T ~int] struct{ rt reflect.Type }

var _ interface {
	eeType
	eeNumType
} = eeIntKindType[int]{}

func (et eeIntKindType[T]) add(vs *eeValues) {
	a, _ := vs.popInt(), vs.popType()
	b := vs.popInt()
	vs.pushInt(a + b)
}

func (et eeIntKindType[T]) checkType(e Expr, t2 eeType) (eeType, error) {
	if _, ok := e.(Not); !ok && et != t2 {
		return nil, ErrInvalidType
	}
	switch e.(type) {
	case Not, Lt, Le, Eq, Ne, Ge, Gt:
		return eeBoolType, nil
	case Add, Sub, Mul, Div, And, Or:
		return et, nil
	}
	return nil, ErrInvalidType
}

func (et eeIntKindType[T]) div(vs *eeValues) {
	a, _ := vs.popInt(), vs.popType()
	b := vs.popInt()
	vs.pushInt(a / b)
}

func (et eeIntKindType[T]) eq(vs *eeValues) bool {
	a, _ := vs.popInt(), vs.popType()
	b, _ := vs.popInt(), vs.popType()
	return a == b
}

func (et eeIntKindType[T]) get(vs *eeValues, i int) interface{} { return T(vs.nums[i]) }
func (et eeIntKindType[T]) kind() opKind                        { return opInt }

func (et eeIntKindType[T]) mul(vs *eeValues) {
	a, _ := vs.popInt(), vs.popType()
	b := vs.popInt()
	vs.pushInt(a * b)
}

func (et eeIntKindType[T]) pop(vs *eeValues) interface{}         { return T(vs.popInt()) }
func (et eeIntKindType[T]) push(vs *eeValues, v interface{}) int { return vs.pushInt(int64(v.(T))) }
func (et eeIntKindType[T]) pushElem(vs *eeValues, ptr interface{}) int {
	return vs.pushInt(int64(*(ptr.(*T))))
}
func (et eeIntKindType[T]) reflectType() reflect.Type { return et.rt }

func (et eeIntKindType[T]) sub(vs *eeValues) {
	a, _ := vs.popInt(), vs.popType()
	b := vs.popInt()
	vs.pushInt(a - b)
}

type eeInt64KindType[T ~int64] struct{ rt reflect.Type }

func (et eeInt64KindType[T]) checkType(e Expr, t2 eeType) (eeType, error) {
	if _, ok := e.(Not); ok {
		if t2 != nil {
			return nil, ErrInvalidType
		}
		return et, nil
	}
	switch e.(type) {
	case Eq:
	case Ne:
	case Gt:
	case Ge:
	case Lt:
	case Le:
	default:
		switch e.(type) {
		case And:
		case Or:
		case Add:
		case Sub:
		case Mul:
		case Div:
		default:
			return nil, ErrInvalidType
		}
		return et, nil
	}
	return eeBoolType, nil
}

func (et eeInt64KindType[T]) cmp(vs *eeValues) int {
	a, _ := vs.popInt(), vs.popType()
	b, _ := vs.popInt(), vs.popType()
	switch {
	case a == b:
		return 0
	case a > b:
		return 1
	}
	return -1
}

func (et eeInt64KindType[T]) eq(vs *eeValues) bool {
	a, _ := vs.popInt(), vs.popType()
	b, _ := vs.popInt(), vs.popType()
	return a == b
}

func (et eeInt64KindType[T]) get(vs *eeValues, i int) interface{}  { return T(vs.nums[i]) }
func (et eeInt64KindType[T]) kind() opKind                         { return opInt }
func (et eeInt64KindType[T]) pop(vs *eeValues) interface{}         { return T(vs.popInt()) }
func (et eeInt64KindType[T]) push(vs *eeValues, v interface{}) int { return vs.pushInt(int64(v.(T))) }
func (et eeInt64KindType[T]) pushElem(vs *eeValues, ptr interface{}) int {
	return vs.pushInt(int64(*(ptr.(*T))))
}
func (et eeInt64KindType[T]) reflectType() reflect.Type { return et.rt }

type eeFloatKindType[T ~float64] struct{ rt reflect.Type }

func (et eeFloatKindType[T]) add(vs *eeValues) {
	a, _ := vs.popFloat(), vs.popType()
	b := vs.popFloat()
	vs.pushFloat(a + b)
}

func (et eeFloatKindType[T]) checkType(e Expr, t2 eeType) (eeType, error) {
	if _, ok := e.(Not); ok {
		if t2 != nil {
			return nil, ErrInvalidType
		}
		return et, nil
	}
	switch e.(type) {
	case Eq:
	case Ne:
	case Gt:
	case Ge:
	case Lt:
	case Le:
	default:
		switch e.(type) {
		case Add:
		case Sub:
		case Mul:
		case Div:
		default:
			return nil, ErrInvalidType
		}
		return et, nil
	}
	return eeBoolType, nil
}

func (et eeFloatKindType[T]) cmp(vs *eeValues) int {
	a, _ := vs.popFloat(), vs.popType()
	b, _ := vs.popFloat(), vs.popType()
	switch {
	// TODO: Epsilon?
	case a == b:
		return 0
	case a > b:
		return 1
	}
	return -1
}

func (et eeFloatKindType[T]) div(vs *eeValues) {
	a, _ := vs.popFloat(), vs.popType()
	b := vs.popFloat()
	vs.pushFloat(a / b)
}

func (et eeFloatKindType[T]) eq(vs *eeValues) bool {
	a, _ := vs.popFloat(), vs.popType()
	b, _ := vs.popFloat(), vs.popType()
	return a == b
}

func (et eeFloatKindType[T]) get(vs *eeValues, i int) interface{} {
	return *((*T)(unsafe.Pointer(&vs.nums[i])))
}

func (et eeFloatKindType[T]) kind() opKind { return opFloat }

func (et eeFloatKindType[T]) mul(vs *eeValues) {
	a, _ := vs.popFloat(), vs.popType()
	b := vs.popFloat()
	vs.pushFloat(a * b)
}

func (et eeFloatKindType[T]) pop(vs *eeValues) interface{} {
	fs := (*[]T)(unsafe.Pointer(&vs.nums))
	v := (*fs)[len(*fs)-1]
	*fs = (*fs)[:len(*fs)-1]
	return v
}

func (et eeFloatKindType[T]) push(vs *eeValues, v interface{}) int {
	return vs.pushFloat(float64(v.(T)))
}

func (et eeFloatKindType[T]) pushElem(vs *eeValues, ptr interface{}) int {
	return vs.pushFloat(float64(*(ptr.(*T))))
}

func (et eeFloatKindType[T]) reflectType() reflect.Type { return et.rt }

func (et eeFloatKindType[T]) sub(vs *eeValues) {
	a, _ := vs.popFloat(), vs.popType()
	b := vs.popFloat()
	vs.pushFloat(a - b)
}

type eeRatKindType struct{ rt reflect.Type }

func (et eeRatKindType) add(vs *eeValues) {
	a, _ := vs.popRat(), vs.popType()
	b := vs.popRat()
	vs.pushRat(a.Add(a, b))
}

func (et eeRatKindType) checkType(e Expr, t2 eeType) (eeType, error) {
	if _, ok := e.(Not); ok {
		if t2 != nil {
			return nil, ErrInvalidType
		}
		return et, nil
	}
	if t2 != et {
		return nil, ErrInvalidType
	}
	switch e.(type) {
	case Eq:
	case Ne:
	case Gt:
	case Ge:
	case Lt:
	case Le:
	default:
		switch e.(type) {
		case And:
		case Or:
		case Add:
		case Sub:
		case Mul:
		case Div:
		default:
			return nil, ErrInvalidType
		}
		return et, nil
	}
	return eeBoolType, nil
}

func (et eeRatKindType) cmp(vs *eeValues) int {
	a, _ := vs.popRat(), vs.popType()
	b, _ := vs.popRat(), vs.popType()
	return a.Cmp(b)
}

func (et eeRatKindType) div(vs *eeValues) {
	a, _ := vs.popRat(), vs.popType()
	b := vs.popRat()
	vs.pushRat(a.Quo(a, b))
}

func (et eeRatKindType) eq(vs *eeValues) bool {
	return et.cmp(vs) == 0
}

func (et eeRatKindType) get(vs *eeValues, i int) interface{} { return vs.anys[i] }

func (et eeRatKindType) kind() opKind { return opRat }

func (et eeRatKindType) mul(vs *eeValues) {
	a, _ := vs.popRat(), vs.popType()
	b := vs.popRat()
	vs.pushRat(a.Mul(a, b))
}

func (et eeRatKindType) pop(vs *eeValues) interface{} {
	v := vs.popRat()
	if et.rt == bigRatType {
		return v
	}
	rv := reflect.New(et.rt.Elem())
	rv.Set(reflect.ValueOf(v))
	return rv.Interface()
}

func (et eeRatKindType) push(vs *eeValues, v interface{}) int {
	if et.reflectType() == bigRatType {
		return vs.pushRat(v.(*big.Rat))
	}
	return vs.pushRat(reflect.ValueOf(v).Convert(bigRatType).Interface().(*big.Rat))
}

func (et eeRatKindType) pushElem(vs *eeValues, ptr interface{}) int {
	if et.reflectType() == bigRatType {
		return vs.pushRat(*(ptr.(**big.Rat)))
	}
	return vs.pushRat(reflect.ValueOf(ptr).Elem().Convert(bigRatType).Interface().(*big.Rat))
}

func (et eeRatKindType) reflectType() reflect.Type { return et.rt }

func (et eeRatKindType) sub(vs *eeValues) {
	a, _ := vs.popRat(), vs.popType()
	b := vs.popRat()
	vs.pushRat(a.Sub(a, b))
}

type eeStringKindType[T ~string] struct{ rt reflect.Type }

var _ interface {
	eeType
	eeAddType
	eeCmpType
} = eeStringKindType[string]{}

func (et eeStringKindType[T]) add(vs *eeValues) {
	a, _ := vs.popStr(), vs.popType()
	b := vs.popStr() // leave 2nd type
	vs.pushStr(a + b)
}

func (et eeStringKindType[T]) checkType(e Expr, t2 eeType) (eeType, error) {
	if t2 != et {
		return nil, ErrInvalidType
	}
	switch e.(type) {
	case Eq:
	case Ne:
	case Gt:
	case Ge:
	case Lt:
	case Le:
	default:
		switch e.(type) {
		case Add:
		default:
			return nil, ErrInvalidType
		}
		return et, nil
	}
	return eeBoolType, nil
}

func (et eeStringKindType[T]) cmp(vs *eeValues) int {
	a, _ := vs.popStr(), vs.popType()
	b, _ := vs.popStr(), vs.popType()
	return strings.Compare(a, b)
}

func (et eeStringKindType[T]) eq(vs *eeValues) bool {
	a, _ := vs.popStr(), vs.popType()
	b, _ := vs.popStr(), vs.popType()
	return a == b
}

func (et eeStringKindType[T]) get(vs *eeValues, i int) interface{}  { return T(vs.strs[i]) }
func (et eeStringKindType[T]) kind() opKind                         { return opStr }
func (et eeStringKindType[T]) pop(vs *eeValues) interface{}         { return T(vs.popStr()) }
func (et eeStringKindType[T]) push(vs *eeValues, v interface{}) int { return vs.pushStr(string(v.(T))) }
func (et eeStringKindType[T]) pushElem(vs *eeValues, ptr interface{}) int {
	return vs.pushStr(string(*(ptr.(*T))))
}
func (et eeStringKindType[T]) reflectType() reflect.Type { return et.rt }

type eeMapStrAnyKindType[T ~string] struct {
	eeReflectType
	keyType eeStringKindType[T]
}

var _ interface {
	eeType
	eeMemType
} = (*eeMapStrAnyKindType[string])(nil)

func (et *eeMapStrAnyKindType[T]) checkType(e Expr, t2 eeType) (eeType, error) {
	switch e.(type) {
	case Mem:
		if et.keyType != t2 {
			return nil, ErrInvalidType
		}
		return eeAnyType, nil
	}
	return et.eeReflectType.checkType(e, t2)
}

func (et *eeMapStrAnyKindType[T]) getMem(vs *eeValues) {
	m, _ := et.popMap(vs), vs.popType()
	k, _ := T(vs.popStr()), vs.popType()
	v := m[k]
	vt := typeOf(v)
	vt.push(vs, v)
	vs.pushType(vt)
}

func (et *eeMapStrAnyKindType[T]) kind() opKind { return opAny }

func (et *eeMapStrAnyKindType[T]) popMap(vs *eeValues) map[T]interface{} {
	return vs.popAny().(map[T]interface{})
}

func (et *eeMapStrAnyKindType[T]) setMem(vs *eeValues) {
	m, _ := et.popMap(vs), vs.popType()
	k, _ := T(vs.popStr()), vs.popType()
	v := vs.popType().pop(vs)
	m[k] = v
}

func eeCmp(vs *eeValues) int {
	et := vs.peekType()
	if ct, ok := et.(eeCmpType); ok {
		return ct.cmp(vs)
	}
	if len(vs.types) > 1 {
		et = vs.types[len(vs.types)-2]
		if ct, ok := et.(eeCmpType); ok {
			return -ct.cmp(vs)
		}
	}
	// TODO: if the value 2nd from the top implements eeCmpType,
	// swap the two operands to perform the comparison.
	// if len(vs.types) > 1 {
	// 	et = vs.types[len(vs.types)-2]
	// }
	if et.eq(vs) {
		return 0
	}
	return -1
}

type eeStructType struct {
	eeReflectType
}

var _ interface {
	eeType
	eeMemType
} = (*eeStructType)(nil)

func (et *eeStructType) getMem(vs *eeValues) {
	v := et.getField(vs).Interface()
	vt := typeOf(v)
	vs.pushType(vt)
	vt.push(vs, v)
}

func (et *eeStructType) setMem(vs *eeValues) {

}

func (et *eeStructType) getField(vs *eeValues) reflect.Value {
	s, _ := vs.popAny(), vs.popType()
	m, _ := vs.popInt(), vs.popType()
	return reflect.ValueOf(s).Field(int(m))
}

type eeVarKindType struct{ eeType }

var varTypes sync.Map

func getVarType(rt reflect.Type) eeType {
	newVarType := func(rt reflect.Type) eeType {
		return &eeVarKindType{getType(rt)}
	}
	var k interface{} = rt
	v, loaded := varTypes.Load(k)
	if loaded {
		return v.(eeType)
	}
	vt := newVarType(rt)
	v, loaded = varTypes.LoadOrStore(k, vt)
	if loaded {
		return v.(eeType)
	}
	return vt
}
