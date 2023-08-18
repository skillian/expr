package expr

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/davecgh/go-spew/spew"
	"github.com/skillian/ctxutil"
	"github.com/skillian/logging"
)

var (
	// ErrBadOp is returned when an invalid bytecode op is
	// encountered during the execution of a func.
	//
	// It indicates a design flaw in the function compiler.
	ErrBadOp = errors.New("invalid opcode")

	// ErrInvalidType indicates that one or more objects' types are
	// incompatible with some operation.  This error should usually
	// be wrapped to include more specific information.
	ErrInvalidType = errors.New("invalid type")
)

// Call a function.  The first parameter should be the function itself
// and the remaining expressions are the arguments.
type Call []Expr

// Operands return the operands of the call expression; the first of
// which is the function to be called.
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

// Func is a function expression that can be called during
// evaluation.
//
// Function expressions are often compiled with operands with specific
// types
type Func interface {
	Expr
	Call(context.Context, Values) (interface{}, error)
}

// funcFromExpr compiles the expression, e, into a function.
func funcFromExpr(ctx context.Context, e Expr, vs Values) (Func, error) {
	// Currently, functions are "statically compiled:"  The
	// operands' types are inspected and type-specific op codes
	// are generated.
	bv := exprFuncBuilders.Get()
	defer exprFuncBuilders.Put(bv)
	b := bv.(*exprFuncBuilder)
	defer b.reset()
	if err := b.initVarTypes(ctx, e, vs); err != nil {
		return nil, err
	}
	Walk(e, b, WalkOperandsBackwards)
	if b.err != nil {
		return nil, b.err
	}
	f, err := b.build()
	if err != nil {
		return nil, err
	}
	logger.Debug2("expr: %v, func:\n%v", e, f)
	return f, nil
}

func nameOfFunc(f Func) string {
	switch f := f.(type) {
	case *opFunc:
		if f.name != "" {
			return f.name
		}
	case interface{ Name() string }:
		return f.Name()
	}
	return fmt.Sprintf("%[1]T@%[1]p", f)
}

var exprFuncBuilders = sync.Pool{
	New: func() interface{} {
		efb := &exprFuncBuilder{}
		efb.init(8)
		return efb
	},
}

type exprFuncBuilder struct {
	ee         *exprEvaluator
	stack      []*efbStackFrame
	vartypes   map[Var]*eeVarKindType
	valkeys    []eeValueKey
	values     eeValues
	freeFrames []*efbStackFrame
	ops        []opCode
	err        error
}

var _ interface {
	Visitor
} = (*exprFuncBuilder)(nil)

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
	b.ee = exprEvaluators.Get().(*exprEvaluator)
	b.stack = make([]*efbStackFrame, 0, capacity)
	b.stack = append(b.stack, b.getFrame())
	b.vartypes = make(map[Var]*eeVarKindType, capacity)
	b.valkeys = make([]eeValueKey, 0, capacity)
	b.values.init(capacity)
	b.ops = make([]opCode, 0, capacity)
}

// initVarTypes initializes the function builder's mapping of variables to
// their values' types.
func (b *exprFuncBuilder) initVarTypes(ctx context.Context, e Expr, vs Values) (walkErr error) {
	var vf Visitor
	vf = VisitFunc(func(e Expr) Visitor {
		va, ok := e.(Var)
		if !ok {
			return vf
		}
		if _, ok := b.vartypes[va]; ok {
			return vf
		}
		v, err := vs.Get(ctx, va)
		if err != nil {
			walkErr = fmt.Errorf(
				"error getting var %[1]v (type: "+
					"%[1]T) from values %[2]v "+
					"(type: %[2]T): %[3]w",
				va, vs, err,
			)
			return nil
		}
		b.vartypes[va] = getVarType(typeOf(v).reflectType()).(*eeVarKindType)
		return vf
	})
	Walk(e, vf)
	return
}

// reset the function builder before it is put back into the pool.
func (b *exprFuncBuilder) reset() {
	b.ee.reset()
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
func (b *exprFuncBuilder) Visit(e Expr) Visitor {
	if e != nil {
		b.pushExprFrame(e)
		return b
	}
	top := b.peekFrame(0)
	e = top.e
	sec := b.peekFrame(1)
	sec.opers = append(sec.opers, top)
	op, err := b.genOp()
	{
		if errors.Is(err, ErrInvalidType) {
			switch e.(type) {
			case Eq, Ne, Add, Sub, Mul, Div:
				if err = b.tryPromoteNumTypes(top); err == nil {
					op, err = b.genOp()
				}
			}
		}
		if err != nil {
			b.err = err
			return nil
		}
	}
	switch op {
	case opNop:
		return b.walkVarOrValue(top, e)
	case opPackTuple:
		top.codes = append(top.codes, opPackTuple)
		top.codes = appendValBitsToBytes(top.codes, len(top.opers))
		return b
	}
	t, err := b.checkType(op, top)
	if err != nil {
		b.err = err
		return b
	}
	top.codes = append(top.codes, op)
	_, topIsMem := e.(Mem)
	isMapMem := topIsMem && top.opers[1].t.reflectType().Kind() == reflect.Map
	_, secIsMem := sec.e.(Mem)
	switch {
	case topIsMem && secIsMem && t.reflectType().Kind() != reflect.Ptr:
		top.t = getType(reflect.PtrTo(t.reflectType()))
	case topIsMem && !isMapMem:
		// TODO: deref something other than Any?
		top.codes = append(top.codes, opDerefAny)
		fallthrough
	default:
		top.t = t
	}
	_ = b.popFrame()
	return b
}

// walkVarOrValue is delegated to by walk to handle loading a variable
// or value.
func (b *exprFuncBuilder) walkVarOrValue(fr *efbStackFrame, e Expr) Visitor {
	findOrCreateValKey := func(b *exprFuncBuilder, e Expr, et eeType) eeValueKey {
		eq := Expr(Eq{})
		for _, k := range b.valkeys {
			ti, _ := k.typeAndValueIndexes()
			vt := b.values.types[ti]
			_, err := vt.checkType(eq, et)
			if err != nil {
				continue
			}
			b.values.copyTo(k, &b.ee.eeValues)
			b.ee.eeValues.pushType(et)
			et.push(&b.ee.eeValues, e)
			if !et.eq(&b.ee.eeValues) {
				continue
			}
			return k
		}
		b.valkeys = append(b.valkeys, makeEEValueKey(
			b.values.pushType(et),
			et.push(&b.values, e),
		))
		return b.valkeys[len(b.valkeys)-1]
	}
	et := typeOf(e)
	vk := findOrCreateValKey(b, e, et)
	fr.codes = append(fr.codes, opLdc)
	fr.codes = vk.appendToOpCodes(fr.codes)
	fr.t = et
	if va, ok := e.(Var); ok {
		fr.codes = append(fr.codes, opLdv)
		fr.t = b.vartypes[va].eeType
	}
	_ = b.popFrame()
	return b
}

func (b *exprFuncBuilder) checkType(op opCode, fr *efbStackFrame) (returnType eeType, err error) {
	if len(fr.opers) < 1 {
		return nil, fmt.Errorf("cannot check type without an operand")
	}
	if len(fr.opers) > 2 {
		return nil, fmt.Errorf(
			"cannot check type of %d-ary operation",
			len(fr.opers),
		)
	}
	t := fr.opers[len(fr.opers)-1].t
	errFmt := "%[1]w: %[2]v %#[3]v"
	var t2 eeType
	if len(fr.opers) > 1 {
		t2 = fr.opers[len(fr.opers)-2].t
		errFmt = "%[1]w: %#[3]v %[2]v %#[4]v"
	}
	returnType, err = t.checkType(fr.e, t2)
	if err != nil {
		err = fmt.Errorf(errFmt, err, op, t, t2)
	}
	return
}

var numTypePromotions = []struct {
	src  eeType
	dest eeType
	op   opCode
}{
	{eeIntType, eeFloat64Type, opConvInt64ToFloat64},
	{eeIntType, eeRatType, opConvInt64ToRat},
	{eeInt64Type, eeFloat64Type, opConvInt64ToFloat64},
	{eeInt64Type, eeRatType, opConvInt64ToRat},
	{eeFloat64Type, eeRatType, opConvFloat64ToRat},
}

func (b *exprFuncBuilder) tryPromoteNumTypes(fr *efbStackFrame) error {
	getPromote := func(a, b eeType) (t eeType, op opCode) {
		for _, p := range numTypePromotions {
			if p.src == a && p.dest == b {
				return p.dest, p.op
			}
		}
		return nil, opNop
	}
	left, right := fr.opers[1], fr.opers[0] // backwards on purpose
	et, op := getPromote(left.t, right.t)
	operIndex := 1
	if op == opNop {
		et, op = getPromote(right.t, left.t)
		operIndex = 0
	}
	if op == opNop {
		return fmt.Errorf(
			"%w: Cannot promote %v to %v or vice-versa",
			ErrInvalidType, reflectTypeName(left.t.reflectType()),
			reflectTypeName(right.t.reflectType()),
		)
	}
	promoteFrame := fr.opers[operIndex]
	promoteFrame.codes = append(promoteFrame.codes, op)
	promoteFrame.t = et
	return nil
}

// genOp generates an opCode for the current builder frame
func (b *exprFuncBuilder) genOp() (opCode, error) {
	checkKinds := func(fr *efbStackFrame, kinds ...opKind) bool {
		if len(kinds) != len(fr.opers) {
			return false
		}
		for i, k := range kinds {
			if k != fr.opers[i].t.kind() {
				return false
			}
		}
		return true
	}
	top := b.peekFrame(0)
	switch top.e.(type) {
	case Tuple:
		return opPackTuple, nil
	case Not:
		switch {
		case checkKinds(top, opBool):
			return opNotBool, nil
		case checkKinds(top, opInt):
			return opNotInt, nil
		}
	case And:
		switch {
		case checkKinds(top, opBool, opBool):
			return opAndBool, nil
		case checkKinds(top, opInt, opInt):
			return opAndInt, nil
		case checkKinds(top, opRat, opRat):
			return opAndRat, nil
		}
	case Or:
		switch {
		case checkKinds(top, opBool, opBool):
			return opOrBool, nil
		case checkKinds(top, opInt, opInt):
			return opOrInt, nil
		case checkKinds(top, opRat, opRat):
			return opOrRat, nil
		}
	case Eq:
		switch {
		case checkKinds(top, opAny, opAny):
			return opEqAny, nil
		case checkKinds(top, opBool, opBool):
			return opEqBool, nil
		case checkKinds(top, opFloat, opFloat):
			return opEqFloat, nil
		case checkKinds(top, opInt, opInt):
			return opEqInt, nil
		case checkKinds(top, opRat, opRat):
			return opEqRat, nil
		case checkKinds(top, opStr, opStr):
			return opEqStr, nil
		}
	case Ne:
		switch {
		case checkKinds(top, opAny, opAny):
			return opNeAny, nil
		case checkKinds(top, opBool, opBool):
			return opNeBool, nil
		case checkKinds(top, opFloat, opFloat):
			return opNeFloat, nil
		case checkKinds(top, opInt, opInt):
			return opNeInt, nil
		case checkKinds(top, opRat, opRat):
			return opNeRat, nil
		case checkKinds(top, opStr, opStr):
			return opNeStr, nil
		}
	case Gt:
		switch {
		case checkKinds(top, opAny, opAny):
			return opGtAny, nil
		case checkKinds(top, opFloat, opFloat):
			return opGtFloat, nil
		case checkKinds(top, opInt, opInt):
			return opGtInt, nil
		case checkKinds(top, opRat, opRat):
			return opGtRat, nil
		case checkKinds(top, opStr, opStr):
			return opGtStr, nil
		}
	case Ge:
		switch {
		case checkKinds(top, opAny, opAny):
			return opGeAny, nil
		case checkKinds(top, opFloat, opFloat):
			return opGeFloat, nil
		case checkKinds(top, opInt, opInt):
			return opGeInt, nil
		case checkKinds(top, opRat, opRat):
			return opGeRat, nil
		case checkKinds(top, opStr, opStr):
			return opGeStr, nil
		}
	case Lt:
		switch {
		case checkKinds(top, opAny, opAny):
			return opLtAny, nil
		case checkKinds(top, opFloat, opFloat):
			return opLtFloat, nil
		case checkKinds(top, opInt, opInt):
			return opLtInt, nil
		case checkKinds(top, opRat, opRat):
			return opLtRat, nil
		case checkKinds(top, opStr, opStr):
			return opLtStr, nil
		}
	case Le:
		switch {
		case checkKinds(top, opAny, opAny):
			return opLeAny, nil
		case checkKinds(top, opFloat, opFloat):
			return opLeFloat, nil
		case checkKinds(top, opInt, opInt):
			return opLeInt, nil
		case checkKinds(top, opRat, opRat):
			return opLeRat, nil
		case checkKinds(top, opStr, opStr):
			return opLeStr, nil
		}
	case Add:
		switch {
		case checkKinds(top, opFloat, opFloat):
			return opAddFloat, nil
		case checkKinds(top, opInt, opInt):
			return opAddInt, nil
		case checkKinds(top, opRat, opRat):
			return opAddRat, nil
		case checkKinds(top, opStr, opStr):
			return opAddStr, nil
		}
	case Sub:
		switch {
		case checkKinds(top, opFloat, opFloat):
			return opSubFloat, nil
		case checkKinds(top, opInt, opInt):
			return opSubInt, nil
		case checkKinds(top, opRat, opRat):
			return opSubRat, nil
		}
	case Mul:
		switch {
		case checkKinds(top, opFloat, opFloat):
			return opMulFloat, nil
		case checkKinds(top, opInt, opInt):
			return opMulInt, nil
		case checkKinds(top, opRat, opRat):
			return opMulRat, nil
		}
	case Div:
		switch {
		case checkKinds(top, opFloat, opFloat):
			return opDivFloat, nil
		case checkKinds(top, opInt, opInt):
			return opDivInt, nil
		case checkKinds(top, opRat, opRat):
			return opDivRat, nil
		}
	case Mem:
		switch {
		case checkKinds(top, opAny, opAny):
			return opLdmAnyAny, nil
		case checkKinds(top, opStr, opAny):
			return opLdmStrMapAny, nil
		}
	default:
		// will turn into some sort of load value
		return opNop, nil
	}
	return 0, fmt.Errorf(
		"%w: Cannot determine op code for: %v",
		ErrInvalidType, top.e,
	)
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
	if logger.EffectiveLevel() <= logging.DebugLevel {
		logger.Debug2("%[1]T(%[1]p): %[2]v", b, spew.Sdump(b))
	}
	ops := b.getAllOps()
	if len(ops) == 0 {
		return nil, fmt.Errorf("Cannot build empty function")
	}
	f = &opFunc{
		ops: make([]opCode, len(ops)),
		consts: eeValues{
			types: make([]eeType, len(b.values.types)),
			nums:  make([]int64, len(b.values.nums)),
			strs:  make([]string, len(b.values.strs)),
			anys:  make([]interface{}, len(b.values.anys)),
		},
	}
	copy(f.ops, ops)
	copy(f.consts.types, b.values.types)
	copy(f.consts.nums, b.values.nums)
	copy(f.consts.strs, b.values.strs)
	copy(f.consts.anys, b.values.anys)
	return
}

func (b *exprFuncBuilder) getAllOps() []opCode {
	opsCap := 0
	b.stack[0].walk(func(fr *efbStackFrame) {
		if fr == nil {
			return
		}
		opsCap += len(fr.codes)
	})
	ops := make([]opCode, 0, opsCap)
	stack := make([]*efbStackFrame, 0, 8)
	b.stack[0].walk(func(fr *efbStackFrame) {
		if fr != nil {
			stack = append(stack, fr)
			return
		}
		fr = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		ops = append(ops, fr.codes...)
	})
	return ops
}

// opFunc is an implementation of the Func interface.
//
// opFunc contains a sequence of `opCode`s, and an optional collection
// of constant values with which those `opCode`s can execute as
// operands.
type opFunc struct {
	// ops is the sequence of instructions that this package's
	// virtual machine carries out to execute the function.
	ops []opCode

	// consts are a collection of constant values bound to and
	// referenced by this function.
	consts eeValues

	// name is an optional name that can be provided for the
	// function.  This can be helpful for diagnostics information.
	name string
}

// opDasmAppendTo disassembles a function into a string builder.
// If pc is greater than or equal to zero, an arrow is inserted at that
// index into f's op codes to show where an error occurred.
// An optional `indent` prefix can be inserted, for example, if this
// function is being called from another function to format a function.
func (f *opFunc) opDasmAppendTo(sb *strings.Builder, pc int, indent string) error {
	for i := 0; i < len(f.ops); {
		op := f.ops[i]
		sb.WriteString(indent)
		sb.WriteString(strconv.Itoa(i))
		opLength := op.length()
		i += opLength
		sb.WriteByte('\t')
		if i == pc {
			sb.WriteString("->")
		}
		sb.WriteByte('\t')
		sb.WriteString(op.String())
		switch op {
		case opLdc:
			sb.WriteByte(' ')
			vk := *((*eeValueKey)(unsafe.Pointer(&f.ops[i-(opLength-1)])))
			ti, vi := vk.typeAndValueIndexes()
			v := f.consts.types[ti].get(&f.consts, vi)
			sb.WriteString(fmt.Sprintf("%#[1]v", v))
		case opBrt:
			sb.WriteByte(' ')
			i := *((*int)(unsafe.Pointer(&f.ops[i-(opLength-1)])))
			sb.WriteString(strconv.Itoa(i))
		}
		sb.WriteByte('\n')
	}
	return nil
}

// String produces a string representation of the function
func (f *opFunc) String() string {
	sb := strings.Builder{}
	if err := f.opDasmAppendTo(&sb, -1, ""); err != nil {
		return strings.Join([]string{"<error: ", err.Error(), ">"}, "")
	}
	return sb.String()
}

var _ interface {
	Func
} = (*opFunc)(nil)

func (f *opFunc) Call(ctx context.Context, vs Values) (interface{}, error) {
	return WithEvalContext(ctx, any(nil), func(ctx context.Context, _ any) (res interface{}, err error) {
		ee := ctxutil.Value(ctx, (*exprEvaluator)(nil).ContextKey()).(*exprEvaluator)
		return ee.evalFunc(ctx, f, vs)
	})
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

// funcKey is an array of expr types
type funcKey interface{}

func makeFuncKey(ctx context.Context, e Expr, vs Values) funcKey {
	var appendExpr func(exprs []Expr, e Expr) (appended []Expr, enter bool)
	appendExpr = func(exprs []Expr, e Expr) ([]Expr, bool) {
		switch e := e.(type) {
		case Unary, Binary, interface{ Operands() []Expr }:
			return append(exprs, reflect.TypeOf(e)), true
		}
		rv := reflect.ValueOf(e)
		switch rv.Kind() {
		case reflect.Func:
			return append(exprs, goFuncData(ifacePtrData(unsafe.Pointer(&e)).Data)), false
		case reflect.Map:
			mr := rv.MapRange()
			for mr.Next() {
				exprs, _ = appendExpr(exprs, mr.Key().Interface())
				exprs, _ = appendExpr(exprs, mr.Value().Interface())
			}
			return exprs, false
		case reflect.Slice:
			length := rv.Len()
			for i := 0; i < length; i++ {
				exprs, _ = appendExpr(exprs, rv.Index(i).Interface())
			}
			return exprs, false
		}
		return append(exprs, e), true
	}
	const arbitraryCapacity = 8
	exprs := make([]Expr, 0, arbitraryCapacity)
	var f Visitor
	f = VisitFunc(func(e Expr) Visitor {
		if e != nil {
			var enter bool
			exprs, enter = appendExpr(exprs, e)
			if !enter {
				return nil
			}
		}
		return f
	})
	Walk(e, f)
	arrayType := reflect.ArrayOf(len(exprs), exprType)
	arrayPtr := reflect.NewAt(arrayType, unsafe.Pointer(&exprs[0]))
	return funcKey(arrayPtr.Elem().Interface())
}

// funcData is the internal element type that a funcCache maps to.
// In addition to the function implementation, it also contains the
// number of times this function was loaded out.  We'll use that
// information at some point to know whether or not the function
// implementation should be hoisted into a parent scope.
type funcData struct {
	fn    Func
	count uint64
}
