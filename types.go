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
func RegisterIntType(v interface{}) {
	rt := reflect.TypeOf(v)
	eeTypes.LoadOrStore(rt, eeIntKindType{eeKindType{rt: rt}})
}

// RegisterInt64Type registers an int64-kind type
func RegisterInt64Type(v interface{}) {
	rt := reflect.TypeOf(v)
	eeTypes.LoadOrStore(rt, eeInt64KindType{eeKindType{rt: rt}})
}

// RegisterInt64Type registers an int64-kind type
func RegisterStringType(v interface{}) {
	rt := reflect.TypeOf(v)
	eeTypes.LoadOrStore(rt, eeStringKindType{eeKindType{rt: rt}})
}

func init() {
	RegisterInt64Type(time.Duration(0))
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
		rt := et.reflectType()
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
			case "Sub", "Subtract":
				et.subOrd = i + 1
			case "Mul", "Multiply" /* , "Product" */ :
				et.mulOrd = i + 1
			case "Div", "Divide", "Quo":
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
	tupleType     = reflect.TypeOf(Tuple(nil))

	defaultTypeInit = func() (ti eeTypeInit) {
		ti.fn = func() {}
		return
	}

	eeAnyType   eeType = eeAnyKindType{}
	eeBoolType  eeType = eeBoolKindType{eeKindType{rt: boolType}}
	eeErrorType eeType = &eeReflectType{
		rt:         errorType,
		eeTypeInit: defaultTypeInit(),
	}
	eeFloat64Type eeType = eeFloatKindType{eeKindType{rt: float64Type}}
	eeIntType     eeType = eeIntKindType{eeKindType{rt: intType}}
	eeInt64Type   eeType = eeInt64KindType{eeKindType{rt: int64Type}}
	eeRatType     eeType = eeRatKindType{rt: bigRatType}
	eeStringType  eeType = eeStringKindType{eeKindType{rt: stringType}}

	eeMapStrAnyType eeType = &eeMapKindType{
		eeReflectType: eeReflectType{
			rt:         mapStrAnyType,
			eeTypeInit: defaultTypeInit(),
		},
		keyType: eeStringType.(eeStringKindType),
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
			ad := *ifacePtrData(unsafe.Pointer(&a))
			bd := *ifacePtrData(unsafe.Pointer(&b))
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

type eeReflectTypeFnFunc func(arg interface{})

func eeReflectTypeFn(et *eeReflectType, arg interface{}, anyFn, boolFn, floatFn, intFn, ratFn, strFn eeReflectTypeFnFunc) {
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
		boolFn(arg)
	case reflect.Float32, reflect.Float64:
		floatFn(arg)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intFn(arg)
	case reflect.Pointer:
		if et.reflectType().ConvertibleTo(bigRatType) {
			ratFn(arg)
		}
	case reflect.String:
		strFn(arg)
	}
	anyFn(arg)
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
		sf, ok := e[1].(*reflect.StructField)
		if !ok {
			return nil, fmt.Errorf(
				"%w: Cannot get %[2]v (type: %[2]T) member of %[3]v (type: %[3]T)",
				ErrInvalidType, e[1], e[0],
			)
		}
		return getType(sf.Type), nil
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

func (et *eeReflectType) get(vs *eeValues, i int) (res interface{}) {
	type getStateType struct {
		rt  reflect.Type
		res *interface{}
		vs  *eeValues
		i   int
	}
	eeReflectTypeFn(
		et, getStateType{et.reflectType(), &res, vs, i},
		func(getState interface{}) {
			args := getState.(getStateType)
			*args.res = args.vs.anys[args.i]
		},
		func(getState interface{}) {
			args := getState.(getStateType)
			v := args.vs.nums[args.i] != 0
			if args.rt == boolType {
				*args.res = v
				return
			}
			*args.res = reflect.ValueOf(v).Convert(args.rt).Interface()
		},
		func(getState interface{}) {
			args := getState.(getStateType)
			v := (*args.vs.floats())[args.i]
			if args.rt == float64Type {
				*args.res = v
				return
			}
			*args.res = reflect.ValueOf(v).Convert(args.rt).Interface()
		},
		func(getState interface{}) {
			args := getState.(getStateType)
			v := args.vs.nums[args.i]
			if args.rt == int64Type {
				*args.res = v
				return
			}
			*args.res = reflect.ValueOf(v).Convert(args.rt).Interface()
		},
		func(getState interface{}) {
			args := getState.(getStateType)
			v := args.vs.anys[args.i]
			if args.rt == reflect.TypeOf(v) {
				*args.res = v
				return
			}
			*args.res = reflect.ValueOf(v).Convert(args.rt).Interface()
		},
		func(getState interface{}) {
			args := getState.(getStateType)
			v := args.vs.strs[args.i]
			if args.rt == stringType {
				*args.res = v
				return
			}
			*args.res = reflect.ValueOf(v).Convert(args.rt).Interface()
		},
	)
	return
}

func (et *eeReflectType) kind() (k opKind) {
	eeReflectTypeFn(
		et, &k,
		func(p interface{}) { *(p.(*opKind)) = opAny },
		func(p interface{}) { *(p.(*opKind)) = opBool },
		func(p interface{}) { *(p.(*opKind)) = opFloat },
		func(p interface{}) { *(p.(*opKind)) = opInt },
		func(p interface{}) { *(p.(*opKind)) = opRat },
		func(p interface{}) { *(p.(*opKind)) = opStr },
	)
	return
}

func (et *eeReflectType) mul(vs *eeValues) { et.call(et.mulOrd, vs) }

func (et *eeReflectType) pop(vs *eeValues) (res interface{}) {
	type popStateType struct {
		vs  *eeValues
		res *interface{}
	}
	eeReflectTypeFn(
		et, popStateType{vs, &res},
		func(popState interface{}) { arg := popState.(popStateType); *arg.res = arg.vs.popAny() },
		func(popState interface{}) { arg := popState.(popStateType); *arg.res = eeBoolType.pop(arg.vs) },
		func(popState interface{}) { arg := popState.(popStateType); *arg.res = eeFloat64Type.pop(arg.vs) },
		func(popState interface{}) { arg := popState.(popStateType); *arg.res = eeInt64Type.pop(arg.vs) },
		func(popState interface{}) { arg := popState.(popStateType); *arg.res = eeRatType.pop(arg.vs) },
		func(popState interface{}) { arg := popState.(popStateType); *arg.res = eeStringType.pop(arg.vs) },
	)
	return
}

func (et *eeReflectType) push(vs *eeValues, v interface{}) (i int) {
	type pushStateType struct {
		vs *eeValues
		v  interface{}
		i  *int
	}
	eeReflectTypeFn(
		et, pushStateType{vs, v, &i},
		func(pushState interface{}) {
			arg := pushState.(pushStateType)
			*arg.i = arg.vs.pushAny(arg.v)
		},
		func(pushState interface{}) {
			arg := pushState.(pushStateType)
			*arg.i = eeBoolType.push(arg.vs, arg.v)
		},
		func(pushState interface{}) {
			arg := pushState.(pushStateType)
			*arg.i = eeFloat64Type.push(arg.vs, arg.v)
		},
		func(pushState interface{}) {
			arg := pushState.(pushStateType)
			*arg.i = eeInt64Type.push(arg.vs, arg.v)
		},
		func(pushState interface{}) {
			arg := pushState.(pushStateType)
			*arg.i = eeRatType.push(arg.vs, arg.v)
		},
		func(pushState interface{}) {
			arg := pushState.(pushStateType)
			*arg.i = eeStringType.push(arg.vs, arg.v)
		},
	)
	return
}

func (et *eeReflectType) pushElem(vs *eeValues, ptr interface{}) (i int) {
	type pushElemStateType struct {
		vs  *eeValues
		ptr interface{}
		i   *int
	}
	eeReflectTypeFn(
		et, pushElemStateType{vs, ptr, &i},
		func(pushElemState interface{}) {
			arg := pushElemState.(pushElemStateType)
			*arg.i = arg.vs.pushAny(arg.vs)
		},
		func(pushElemState interface{}) {
			arg := pushElemState.(pushElemStateType)
			*arg.i = eeBoolType.pushElem(arg.vs, arg.ptr)
		},
		func(pushElemState interface{}) {
			arg := pushElemState.(pushElemStateType)
			*arg.i = eeFloat64Type.pushElem(arg.vs, arg.ptr)
		},
		func(pushElemState interface{}) {
			arg := pushElemState.(pushElemStateType)
			*arg.i = eeInt64Type.pushElem(arg.vs, arg.ptr)
		},
		func(pushElemState interface{}) {
			arg := pushElemState.(pushElemStateType)
			*arg.i = eeRatType.pushElem(arg.vs, arg.ptr)
		},
		func(pushElemState interface{}) {
			arg := pushElemState.(pushElemStateType)
			*arg.i = eeStringType.pushElem(arg.vs, arg.ptr)
		},
	)
	return
}

func (et *eeReflectType) reflectType() reflect.Type { return et.rt }

func (et *eeReflectType) sub(vs *eeValues) { et.call(et.subOrd, vs) }

type eeKindType struct{ rt reflect.Type }

func (et eeKindType) fixType(v *interface{}) {
	rv := reflect.ValueOf(*v)
	rt := et.reflectType()
	if rt == rv.Type() {
		return
	}
	*v = rv.Convert(rt).Interface()
}

func (et eeKindType) reflectType() reflect.Type { return et.rt }

type eeBoolKindType struct{ eeKindType }

func (et eeBoolKindType) checkType(e Expr, t2 eeType) (eeType, error) {
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

func (et eeBoolKindType) cmp(vs *eeValues) int {
	a, _ := vs.popBool(), vs.popType()
	b, _ := vs.popBool(), vs.popType()
	if a == b {
		return 0
	}
	if a {
		return 1
	}
	return -1
}

func (et eeBoolKindType) eq(vs *eeValues) bool {
	return et.cmp(vs) == 0
}

func (et eeBoolKindType) get(vs *eeValues, i int) (v interface{}) {
	v = vs.nums[i] != 0
	et.fixType(&v)
	return
}

func (et eeBoolKindType) kind() opKind { return opBool }

func (et eeBoolKindType) pop(vs *eeValues) (v interface{}) {
	v = vs.popBool()
	et.fixType(&v)
	return
}

func (et eeBoolKindType) push(vs *eeValues, v interface{}) int {
	return vs.pushBool(reflect.ValueOf(v).Bool())
}

func (et eeBoolKindType) pushElem(vs *eeValues, ptr interface{}) int {
	return vs.pushBool(reflect.ValueOf(ptr).Elem().Bool())
}

func (et eeBoolKindType) reflectType() reflect.Type { return et.rt }

type eeIntKindType struct{ eeKindType }

var _ interface {
	eeType
	eeNumType
} = eeIntKindType{}

func (et eeIntKindType) add(vs *eeValues) {
	a, _ := vs.popInt(), vs.popType()
	b := vs.popInt()
	vs.pushInt(a + b)
}

func (et eeIntKindType) checkType(e Expr, t2 eeType) (eeType, error) {
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

func (et eeIntKindType) div(vs *eeValues) {
	a, _ := vs.popInt(), vs.popType()
	b := vs.popInt()
	vs.pushInt(a / b)
}

func (et eeIntKindType) eq(vs *eeValues) bool {
	a, _ := vs.popInt(), vs.popType()
	b, _ := vs.popInt(), vs.popType()
	return a == b
}

func (et eeIntKindType) get(vs *eeValues, i int) (v interface{}) {
	v = int(vs.nums[i])
	et.fixType(&v)
	return
}

func (et eeIntKindType) kind() opKind { return opInt }

func (et eeIntKindType) mul(vs *eeValues) {
	a, _ := vs.popInt(), vs.popType()
	b := vs.popInt()
	vs.pushInt(a * b)
}

func (et eeIntKindType) pop(vs *eeValues) (v interface{}) {
	v = vs.popInt()
	et.fixType(&v)
	return
}

func (et eeIntKindType) push(vs *eeValues, v interface{}) int {
	return vs.pushInt(reflect.ValueOf(v).Int())
}

func (et eeIntKindType) pushElem(vs *eeValues, ptr interface{}) int {
	if MoreUnsafe {
		return vs.pushInt(*((*int64)(ifacePtrData(unsafe.Pointer(&ptr)).Data)))
	}
	return vs.pushInt(reflect.ValueOf(ptr).Elem().Int())
}

func (et eeIntKindType) sub(vs *eeValues) {
	a, _ := vs.popInt(), vs.popType()
	b := vs.popInt()
	vs.pushInt(a - b)
}

type eeInt64KindType struct{ eeKindType }

func (et eeInt64KindType) checkType(e Expr, t2 eeType) (eeType, error) {
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

func (et eeInt64KindType) cmp(vs *eeValues) int {
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

func (et eeInt64KindType) eq(vs *eeValues) bool {
	a, _ := vs.popInt(), vs.popType()
	b, _ := vs.popInt(), vs.popType()
	return a == b
}

func (et eeInt64KindType) get(vs *eeValues, i int) (v interface{}) {
	v = vs.nums[i]
	et.fixType(&v)
	return
}

func (et eeInt64KindType) kind() opKind { return opInt }

func (et eeInt64KindType) pop(vs *eeValues) (v interface{}) {
	v = vs.popInt()
	et.fixType(&v)
	return
}

func (et eeInt64KindType) push(vs *eeValues, v interface{}) int {
	return vs.pushInt(reflect.ValueOf(v).Int())
}

func (et eeInt64KindType) pushElem(vs *eeValues, ptr interface{}) int {
	if MoreUnsafe {
		return vs.pushInt(*((*int64)(ifacePtrData(unsafe.Pointer(&ptr)).Data)))
	}
	return vs.pushInt(reflect.ValueOf(ptr).Elem().Int())
}

type eeFloatKindType struct{ eeKindType }

func (et eeFloatKindType) add(vs *eeValues) {
	a, _ := vs.popFloat(), vs.popType()
	b := vs.popFloat()
	vs.pushFloat(a + b)
}

func (et eeFloatKindType) checkType(e Expr, t2 eeType) (eeType, error) {
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

func (et eeFloatKindType) cmp(vs *eeValues) int {
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

func (et eeFloatKindType) div(vs *eeValues) {
	a, _ := vs.popFloat(), vs.popType()
	b := vs.popFloat()
	vs.pushFloat(a / b)
}

func (et eeFloatKindType) eq(vs *eeValues) bool {
	a, _ := vs.popFloat(), vs.popType()
	b, _ := vs.popFloat(), vs.popType()
	return a == b
}

func (et eeFloatKindType) get(vs *eeValues, i int) (v interface{}) {
	v = *((*float64)(unsafe.Pointer(&vs.nums[i])))
	et.fixType(&v)
	return
}

func (et eeFloatKindType) kind() opKind { return opFloat }

func (et eeFloatKindType) mul(vs *eeValues) {
	a, _ := vs.popFloat(), vs.popType()
	b := vs.popFloat()
	vs.pushFloat(a * b)
}

func (et eeFloatKindType) pop(vs *eeValues) (v interface{}) {
	fs := (*[]float64)(unsafe.Pointer(&vs.nums))
	v = (*fs)[len(*fs)-1]
	*fs = (*fs)[:len(*fs)-1]
	et.fixType(&v)
	return
}

func (et eeFloatKindType) push(vs *eeValues, v interface{}) int {
	return vs.pushFloat(reflect.ValueOf(v).Float())
}

func (et eeFloatKindType) pushElem(vs *eeValues, ptr interface{}) int {
	if MoreUnsafe {
		return vs.pushFloat(*((*float64)(ifacePtrData(unsafe.Pointer(&ptr)).Data)))
	}
	return vs.pushFloat(reflect.ValueOf(ptr).Elem().Float())
}

func (et eeFloatKindType) reflectType() reflect.Type { return et.rt }

func (et eeFloatKindType) sub(vs *eeValues) {
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

type eeStringKindType struct{ eeKindType }

var _ interface {
	eeType
	eeAddType
	eeCmpType
} = eeStringKindType{}

func (et eeStringKindType) add(vs *eeValues) {
	a, _ := vs.popStr(), vs.popType()
	b := vs.popStr() // leave 2nd type
	vs.pushStr(a + b)
}

func (et eeStringKindType) checkType(e Expr, t2 eeType) (eeType, error) {
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

func (et eeStringKindType) cmp(vs *eeValues) int {
	a, _ := vs.popStr(), vs.popType()
	b, _ := vs.popStr(), vs.popType()
	return strings.Compare(a, b)
}

func (et eeStringKindType) eq(vs *eeValues) bool {
	a, _ := vs.popStr(), vs.popType()
	b, _ := vs.popStr(), vs.popType()
	return a == b
}

func (et eeStringKindType) get(vs *eeValues, i int) (v interface{}) {
	v = vs.strs[i]
	et.fixType(&v)
	return
}

func (et eeStringKindType) kind() opKind { return opStr }

func (et eeStringKindType) pop(vs *eeValues) (v interface{}) {
	v = vs.popStr()
	et.fixType(&v)
	return
}

func (et eeStringKindType) push(vs *eeValues, v interface{}) (i int) {
	if MoreUnsafe {
		return vs.pushStr(*((*string)(ifacePtrData(unsafe.Pointer(&v)).Data)))
	}
	return vs.pushStr(reflect.ValueOf(v).String())
}

func (et eeStringKindType) pushElem(vs *eeValues, ptr interface{}) int {
	if MoreUnsafe {
		return vs.pushStr(*((*string)(ifacePtrData(unsafe.Pointer(&ptr)).Data)))
	}
	return vs.pushStr(reflect.ValueOf(ptr).Elem().String())
}
func (et eeStringKindType) reflectType() reflect.Type { return et.rt }

type eeMapKindType struct {
	eeReflectType
	keyType eeStringKindType
}

var _ interface {
	eeType
	eeMemType
} = (*eeMapKindType)(nil)

func (et *eeMapKindType) checkType(e Expr, t2 eeType) (eeType, error) {
	switch e.(type) {
	case Mem:
		if et.keyType != t2 {
			return nil, ErrInvalidType
		}
		return eeAnyType, nil
	}
	return et.eeReflectType.checkType(e, t2)
}

func (et *eeMapKindType) getMem(vs *eeValues) {
	m := reflect.ValueOf(vs.popType().pop(vs))
	k := reflect.ValueOf(vs.popType().pop(vs))
	v := m.MapIndex(k).Interface()
	vt := typeOf(v)
	vt.push(vs, v)
	vs.pushType(vt)
}

func (et *eeMapKindType) kind() opKind { return opAny }

func (et *eeMapKindType) setMem(vs *eeValues) {
	m := reflect.ValueOf(vs.popType().pop(vs))
	k := reflect.ValueOf(vs.popType().pop(vs))
	v := reflect.ValueOf(vs.popType().pop(vs))
	m.SetMapIndex(k, v)
}

type eeSliceKindType struct {
	eeAnyKindType
	rt reflect.Type
	et eeType
}

var _ interface {
	eeType
	eeMemType
} = (*eeSliceKindType)(nil)

func (et *eeSliceKindType) checkType(e Expr, t2 eeType) (eeType, error) {
	switch e.(type) {
	case Mem:
		if t2.kind() != opInt {
			return nil, ErrInvalidType
		}
		return et.et, nil
	}
	return et.eeAnyKindType.checkType(e, t2)
}

func (et *eeSliceKindType) getMem(vs *eeValues) {
	sl := reflect.ValueOf(vs.popType().pop(vs))
	i, _ := vs.popInt(), vs.popType()
	et.et.push(vs, sl.Index(int(i)).Interface())
	vs.pushType(et.et)
}

func (et *eeSliceKindType) setMem(vs *eeValues) {
	sl := reflect.ValueOf(vs.popType().pop(vs))
	i, _ := vs.popInt(), vs.popType()
	v := reflect.ValueOf(vs.popType().pop(vs))
	sl.Index(int(i)).Set(v)
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
