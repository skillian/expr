package expr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/skillian/ctxutil"
	"github.com/skillian/errutil"
)

var (
	ErrInvalidArgc = errors.New("invalid argument count")
	ErrInvalidType = errors.New("invalid type")
)

// Call a function.  The first parameter should be the function itself
// and the remaining expressions are the arguments.
type Call []Expr

func (x Call) Operands() []Expr { return ([]Expr)(x) }

func (x Call) String() string { return buildString(x) }

func (x Call) appendString(sb *strings.Builder) {
	sb.WriteByte('(')
	appendString(sb, x[0])
	for _, sub := range x[1:] {
		sb.WriteByte(' ')
		appendString(sb, sub)
	}
	sb.WriteByte(')')
}

// Func is a function expression that can actually be called during
// evaluation.
type Func interface {
	Expr
	Call(context.Context, Values) (interface{}, error)
}

func funcFromExpr(ctx context.Context, e Expr, vs Values) (Func, error) {
	bv := exprFuncBuilders.Get()
	defer exprFuncBuilders.Put(bv)
	b := bv.(*exprFuncBuilder)
	defer b.reset()
	if err := b.initVarTypes(ctx, e, vs); err != nil {
		return nil, err
	}
	_ = Walk(e, b.walk, WalkOperandsBackwards)
	if b.err != nil {
		return nil, b.err
	}
	return b.build()
}

var exprFuncBuilders = sync.Pool{
	New: func() interface{} {
		efb := &exprFuncBuilder{}
		efb.init(8)
		return efb
	},
}

type exprFuncBuilder struct {
	stack      []*efbStackFrame
	vartypes   map[Var]eeType
	valkeys    []eeValueKey
	values     eeValues
	freeFrames []*efbStackFrame
	ops        []opCode
	err        error
}

type efbStackFrame struct {
	e     Expr
	t     eeType
	opers []*efbStackFrame
	codes []opCode
}

func (fr *efbStackFrame) walk(f func(fr *efbStackFrame)) {
	f(fr)
	for _, op := range fr.opers {
		op.walk(f)
	}
	f(nil)
}

func (b *exprFuncBuilder) init(capacity int) {
	b.stack = make([]*efbStackFrame, 0, capacity)
	b.stack = append(b.stack, b.getFrame())
	b.vartypes = make(map[Var]eeType, capacity)
	b.valkeys = make([]eeValueKey, 0, capacity)
	b.values.init(capacity)
	b.ops = make([]opCode, 0, capacity)
}

// initVarTypes initializes the function builder's mapping of variables to
// their values' types.
func (b *exprFuncBuilder) initVarTypes(ctx context.Context, e Expr, vs Values) (err error) {
	_ = Walk(e, func(e Expr) bool {
		va, ok := e.(Var)
		if !ok {
			return true
		}
		if _, ok := b.vartypes[va]; ok {
			return true
		}
		v, walkErr := vs.Get(ctx, va)
		if walkErr != nil {
			err = fmt.Errorf(
				"error getting var %[1]v (type: "+
					"%[1]T) from values %[2]v "+
					"(type: %[2]T): %[3]w",
				va, vs, walkErr,
			)
			return false
		}
		b.vartypes[va] = eeTypeOf(v)
		return true
	})
	return
}

// reset the function builder before it is put back into the pool.
func (b *exprFuncBuilder) reset() {
	if len(b.stack[0].opers) == 0 {
		return
	}
	b.putFrame(b.stack[0].opers[0])
	for i := range b.stack {
		b.stack[i] = nil
	}
	b.stack = b.stack[:0]
	b.stack = append(b.stack, b.getFrame())
	// TODO: Maybe just make a new map?
	for k := range b.vartypes {
		delete(b.vartypes, k)
	}
	b.valkeys = b.valkeys[:0]
	b.values.reset()
	b.ops = b.ops[:0]
	b.err = nil
}

// walk is passed into the Walk function in the funcFromExpr func to build up
// the function op codes and values.
func (b *exprFuncBuilder) walk(e Expr) bool {
	if e != nil {
		b.pushExprFrame(e)
		return true
	}
	top := b.peekFrame(0)
	e = top.e
	sec := b.peekFrame(1)
	sec.opers = append(sec.opers, top)
	op := b.genOp()
	switch op {
	case opNop:
		return b.walkVarOrValue(top, e)
	case opPackSlice:
		return b.walkTuple(top, e)
	}
	t, err := b.checkType(op, top)
	if errors.Is(err, ErrInvalidType) {
		if len(top.opers) == 2 {
			if left, ok := top.opers[1].t.(eeNumType); ok {
				if right, ok := top.opers[0].t.(eeNumType); ok {
					t, err = b.tryPromoteNumTypes(top, left, right)
				}
			}
		}
	}
	if err != nil {
		b.err = err
		return false
	}
	top.codes = append(top.codes, op)
	top.t = t
	_ = b.popFrame()
	return true
}

// walkVarOrValue is delegated to by walk to handle loading a variable
// or value.
func (b *exprFuncBuilder) walkVarOrValue(fr *efbStackFrame, e Expr) bool {
	isVar, t := b.varOrValueType(e)
	index := -1
	for i, k := range b.valkeys {
		vt := b.values.types[k.typeIndex]
		if t != vt {
			continue
		}
		v := vt.get(&b.values, k.valIndex)
		if v != e {
			continue
		}
		index = i
		break
	}
	if index == -1 {
		index = len(b.valkeys)
		vt := t
		if isVar {
			vt = varType
		}
		b.valkeys = append(
			b.valkeys,
			b.values.append(e, vt),
		)
	}
	if isVar {
		fr.codes = append(fr.codes, opLdv)
	} else {
		fr.codes = append(fr.codes, opLdc)
	}
	appendIntToOpCodesInto(&fr.codes, int64(index))
	fr.t = t
	_ = b.popFrame()
	return true
}

var (
	reflectEmptyInterfaceType             = reflect.TypeOf((*interface{})(nil)).Elem()
	emptyEmptyInterfaceSlice  interface{} = ([]interface{})(nil)
)

func (b *exprFuncBuilder) walkTuple(fr *efbStackFrame, e Expr) bool {
	// if all elements are the same eeType, create a slice of that.
	// else, []interface{}
	if len(fr.opers) == 0 {
		return b.walkVarOrValue(fr, emptyEmptyInterfaceSlice)
	}
	et := fr.opers[0].t
	for _, oper := range fr.opers[1:] {
		if et != oper.t {
			et = eeTypeFromReflectType(reflectEmptyInterfaceType)
			break
		}
	}
	i := b.getTypeIndex(et)
	fr.codes = append(fr.codes, opPackSlice)
	appendIntToOpCodesInto(&fr.codes, int64(i))
	appendIntToOpCodesInto(&fr.codes, int64(len(fr.opers)))
	fr.t = tupleType
	_ = b.popFrame()
	return true
}

// varOrValueType gets a variable or value's type.  If e is a variable,
// the type of the variable's value is returned.
func (b *exprFuncBuilder) varOrValueType(e Expr) (isVar bool, t eeType) {
	if va, ok := e.(Var); ok {
		t, ok := b.vartypes[va]
		if !ok {
			panic(fmt.Errorf(
				"all vars should be accounted for "+
					"before %T.%v is called",
				b, errutil.Caller(1).FuncName,
			))
		}
		return true, t
	}
	return false, eeTypeOf(e)
}

func (b *exprFuncBuilder) checkType(op opCode, fr *efbStackFrame) (returnType eeType, err error) {
	t := fr.opers[len(fr.opers)-1].t
	errFmt := "%[1]w: %[2]v %#[3]v"
	var t2 eeType
	if op.arity() == 2 {
		t2 = fr.opers[len(fr.opers)-2].t
		errFmt = "%[1]w: %#[3]v %[2]v %#[4]v"
	}
	returnType = t.typeCheck(fr.e, t2)
	if returnType == nil {
		err = fmt.Errorf(errFmt, ErrInvalidType, op, t, t2)
	}
	return
}

func (b *exprFuncBuilder) tryPromoteNumTypes(fr *efbStackFrame, leftNt, rightNt eeNumType) (returnType eeType, err error) {
	left, right := fr.opers[1], fr.opers[0] // backwards on purpose
	numTypeIndex := func(nt eeNumType) int {
		for i, t := range numTypePromotions {
			if t == nt {
				return i
			}
		}
		panic(fmt.Errorf(
			"unknown number type for numeric "+
				"promotion: %[1]v (type: %[1]T)", nt,
		))
	}
	leftNti := numTypeIndex(leftNt)
	rightNti := numTypeIndex(rightNt)
	promoteFr, _, _, nti := minMaxBy(left, leftNti, right, rightNti)
	target := numTypePromotions[nti].(eeType)
	promoteFr.codes = append(promoteFr.codes, opConv2)
	appendIntToOpCodesInto(
		&promoteFr.codes,
		int64(getIndexOrAppendInto(
			&b.values.types,
			target,
		)),
	)
	appendIntToOpCodesInto(
		&promoteFr.codes,
		int64(getIndexOrAppendInto(
			&b.values.anys,
			interface{}(eeNumConvType{}),
		)),
	)
	return target, nil
}

// genOp generates an opCode for the current builder frame
func (b *exprFuncBuilder) genOp() opCode {
	top := b.peekFrame(0)
	switch top.e.(type) {
	case Not:
		return opNot
	case And:
		return opAnd
	case Or:
		return opOr
	case Eq:
		return opEq
	case Ne:
		return opNe
	case Gt:
		return opGt
	case Ge:
		return opGe
	case Lt:
		return opLt
	case Le:
		return opLe
	case Add:
		return opAdd
	case Sub:
		return opSub
	case Mul:
		return opMul
	case Div:
		return opDiv
	case Mem:
		if _, ok := top.opers[1].t.(eeMapType); ok {
			return opLdk
		}
		if _, ok := b.peekFrame(1).e.(Mem); ok {
			return opLdma
		}
		return opLdm
	case Tuple:
		return opPackSlice
	default:
		// will turn into some sort of load value
		return opNop
	}
}

func (b *exprFuncBuilder) getTypeIndex(t eeType) int {
	for i, t2 := range b.values.types {
		if t == t2 {
			return i
		}
	}
	b.values.types = append(b.values.types, t)
	return len(b.values.types) - 1
}

// peekFrame peeks at the i'th frame from the top of the stack (0 for
// the top).
func (b *exprFuncBuilder) peekFrame(i int) *efbStackFrame {
	return b.stack[len(b.stack)-i-1]
}

func (b *exprFuncBuilder) popFrame() (fr *efbStackFrame) {
	fr = b.stack[len(b.stack)-1]
	b.stack[len(b.stack)-1] = nil
	b.stack = b.stack[:len(b.stack)-1]
	return
}

// pushExprFrame wraps the expression into a stack frame and pushes it
// onto the stack.
func (b *exprFuncBuilder) pushExprFrame(e Expr) {
	fr := b.getFrame()
	fr.e = e
	b.stack = append(b.stack, fr)
}

func (b *exprFuncBuilder) getFrame() *efbStackFrame {
	if len(b.freeFrames) == 0 {
		if cap(b.freeFrames) == 0 {
			b.freeFrames = make([]*efbStackFrame, cap(b.stack))
		} else {
			b.freeFrames = append(b.freeFrames[:cap(b.freeFrames)], nil)
			b.freeFrames = b.freeFrames[:cap(b.freeFrames)]
		}
		capacity := cap(b.freeFrames)
		frameCache := make([]efbStackFrame, capacity)
		const codeCap = 8
		codeCache := make([]opCode, capacity*codeCap)
		const operCap = 2
		operCache := make([]*efbStackFrame, capacity*operCap)
		for i := range frameCache {
			fr := &frameCache[i]
			b.freeFrames[i] = fr
			start := i * codeCap
			end := start + codeCap
			fr.codes = codeCache[start:start:end]
			start = i * operCap
			end = start + operCap
			fr.opers = operCache[start:start:end]
		}
	}
	fr := b.freeFrames[len(b.freeFrames)-1]
	b.freeFrames = b.freeFrames[:len(b.freeFrames)-1]
	return fr
}

func (b *exprFuncBuilder) putFrame(fr *efbStackFrame) {
	fr.e = nil
	fr.t = nil
	for i, op := range fr.opers {
		b.putFrame(op)
		fr.opers[i] = nil
	}
	fr.opers = fr.opers[:0]
	fr.codes = fr.codes[:0]
	b.freeFrames = append(b.freeFrames, fr)
}

func (b *exprFuncBuilder) build() (f *opFunc, err error) {
	if b.err != nil {
		return nil, b.err
	}
	ops := b.getAllOps()
	f = &opFunc{
		ops:       make([]opCode, len(ops)),
		constkeys: make([]eeValueKey, len(b.valkeys)),
		consts: eeValues{
			types: make([]eeType, len(b.values.types)),
			nums:  make([]eeNum, len(b.values.nums)),
			strs:  make([]string, len(b.values.strs)),
			anys:  make([]interface{}, len(b.values.anys)),
		},
	}
	copy(f.ops, ops)
	copy(f.constkeys, b.valkeys)
	copy(f.consts.types, b.values.types)
	copy(f.consts.nums, b.values.nums)
	copy(f.consts.strs, b.values.strs)
	copy(f.consts.anys, b.values.anys)
	return
}

func (b *exprFuncBuilder) getAllOps() []opCode {
	ops := make([]opCode, 0, 64)
	var lastOps []opCode
	stack := make([]*efbStackFrame, 0, 8)
	b.stack[0].walk(func(fr *efbStackFrame) {
		if fr != nil {
			stack = append(stack, fr)
			return
		}
		fr = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if bytes.Equal(
			asByteSlice(lastOps),
			asByteSlice(fr.codes),
		) {
			ops = append(ops, opDup)
		} else {
			ops = append(ops, fr.codes...)
			lastOps = fr.codes
		}
	})
	return ops
}

// opFunc is an implementation of the Func interface.
type opFunc struct {
	ops       []opCode
	constkeys []eeValueKey
	consts    eeValues
}

func (f *opFunc) String() string {
	ops := make([]string, len(f.ops))
	for i, op := range f.ops {
		ops[i] = fmt.Sprint(op)
	}
	return fmt.Sprintf("(%[1]T)(%[1]p){%s}", f, strings.Join(ops, ", "))
}

var _ interface {
	Func
} = (*opFunc)(nil)

func (f *opFunc) Call(ctx context.Context, vs Values) (res interface{}, err error) {
	err = WithEvalContext(ctx, &res, func(ctx context.Context, pres *any) error {
		ee := ctxutil.Value(ctx, (*exprEvaluator)(nil).ContextKey()).(*exprEvaluator)
		if err := ee.evalFunc(ctx, f, vs); err != nil {
			return err
		}
		*pres = ee.stack.popType().pop(&ee.stack)
		return nil
	})
	return
}

type funcCache struct {
	parent *funcCache
	funcs  sync.Map // funcKey -> funcData
}

// WithFuncCache creates a context with a function cache in it.
func WithFuncCache(ctx context.Context) context.Context {
	fc := newFuncCache(ctx)
	return ctxutil.WithValue(ctx, fc.ContextKey(), fc)
}

func newFuncCache(ctx context.Context) *funcCache {
	fc2, _ := ctxutil.Value(ctx, (*funcCache)(nil).ContextKey()).(*funcCache)
	return &funcCache{parent: fc2}
}

func (fc *funcCache) ContextKey() interface{} { return (*funcCache)(nil) }

func (fc *funcCache) load(k funcKey) (f Func, loaded bool) {
	var key interface{} = k
	for p := fc; p != nil; p = p.parent {
		v, ok := p.funcs.Load(key)
		if !ok {
			continue
		}
		if p != fc {
			v, _ = fc.funcs.LoadOrStore(key, v)
		}
		fd := v.(*funcData)
		atomic.AddUint64(&fd.count, 1)
		return fd.fn, true
	}
	return nil, false
}

func (fc *funcCache) loadOrStore(k funcKey, f Func) (actual Func, loaded bool) {
	var key interface{} = k
	fd := &funcData{fn: f}
	v, loaded := fc.funcs.LoadOrStore(key, fd)
	if !loaded {
		return f, false
	}
	fd = v.(*funcData)
	atomic.AddUint64(&fd.count, 1)
	return fd.fn, true
}

// funcKey combines an expression's parameter types and the expression
// as a string to create a value that can be used as a key to the
// funcCache to attempt to load an existing function implementation.
type funcKey string

func makeFuncKey(ctx context.Context, e Expr, vs Values) funcKey {
	var sb strings.Builder
	appendString(&sb, makeFuncTypeSigKeyFromValues(ctx, vs))
	appendString(&sb, e)
	return funcKey(sb.String())
}

func (k funcKey) ContextKey() interface{} { return k }

// funcData is the internal element type that a funcCache maps to.
// In addition to the function implementation, it also contains the
// number of times this function was loaded out.  We'll use that
// information at some point to know whether or not the function
// implementation should be hoisted into a parent scope.
type funcData struct {
	fn    Func
	count uint64
}

type funcTypeSigKey string

const (
	funcTypeSigBool     = 'b'
	funcTypeSigInt      = 'i'
	funcTypeSigInt8     = 'o'
	funcTypeSigInt16    = 'w'
	funcTypeSigInt32    = 'd'
	funcTypeSigInt64    = 'q'
	funcTypeSigUint     = 'I'
	funcTypeSigUint8    = 'O'
	funcTypeSigUint16   = 'W'
	funcTypeSigUint32   = 'D'
	funcTypeSigUint64   = 'Q'
	funcTypeSigFloat32  = 'f'
	funcTypeSigFloat64  = 'F'
	funcTypeSigString   = 's'
	funcTypeSigTypeName = '('
)

func makeFuncTypeSigKeyFromValues(ctx context.Context, vs Values) funcTypeSigKey {
	sb := strings.Builder{}
	vvi := VarValueIterOf(vs)
	for {
		if err := vvi.Next(ctx); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			panic(err)
		}
		vv := vvi.VarValue(ctx)
		switch v := vv.Value.(type) {
		case bool:
			sb.WriteByte(funcTypeSigBool)
		case int8:
			sb.WriteByte(funcTypeSigInt8)
		case int16:
			sb.WriteByte(funcTypeSigInt16)
		case int32:
			sb.WriteByte(funcTypeSigInt32)
		case int64:
			sb.WriteByte(funcTypeSigInt64)
		case uint8:
			sb.WriteByte(funcTypeSigUint8)
		case uint16:
			sb.WriteByte(funcTypeSigUint16)
		case uint32:
			sb.WriteByte(funcTypeSigUint32)
		case uint64:
			sb.WriteByte(funcTypeSigUint64)
		case float32:
			sb.WriteByte(funcTypeSigFloat32)
		case float64:
			sb.WriteByte(funcTypeSigFloat64)
		case string:
			sb.WriteByte(funcTypeSigString)
		default:
			name := fmt.Sprintf("%T", v)
			sb.WriteByte(funcTypeSigTypeName)
			sb.WriteByte(byte(len(name)))
			sb.WriteString(name)
		}
	}
	return funcTypeSigKey(sb.String())
}

func (k funcTypeSigKey) appendString(sb *strings.Builder) {
	sb.WriteByte('[')
	for i := 0; i < len(k); i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		if k[i] == funcTypeSigTypeName {
			i++
			j := int(k[i])
			i++
			sb.WriteString(string(k[i : i+j]))
			i += j - 1 // loop will increment 1
			continue
		}
		sb.WriteByte(k[i])
	}
	sb.WriteByte(']')
}

func (k funcTypeSigKey) String() string {
	var sb strings.Builder
	k.appendString(&sb)
	return sb.String()
}
