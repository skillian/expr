package expr

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/skillian/ctxutil"
)

// Eval evaluates an expression with the given values for parameters.
func Eval(ctx context.Context, e Expr, vs Values) (interface{}, error) {
	f, err := FuncOfExpr(ctx, e, vs)
	if err != nil {
		return nil, err
	}
	return f.Call(ctx, vs)
}

// FuncOfExpr creates a Func from the given expression and values.
// The values must be set so their types can be determined and thefunc
// can be compiled with the proper op codes.
// This function should be preferred for evaluating expressions in
// loops so the function can be compiled once and re-executed with
// different values through each iteration.
func FuncOfExpr(ctx context.Context, e Expr, vs Values) (Func, error) {
	fc, ok := ctxutil.Value(ctx, (*funcCache)(nil)).(*funcCache)
	if !ok {
		return funcFromExpr(ctx, e, vs)
	}
	fk := makeFuncKey(ctx, e, vs)
	f, ok := fc.load(fk)
	if ok {
		return f, nil
	}
	f, err := funcFromExpr(ctx, e, vs)
	if err != nil {
		return nil, err
	}
	f, _ = fc.loadOrStore(fk, f)
	return f, nil
}

type WithEvalContextFunc func(ctx context.Context, state interface{}) error

// WithEvalContext adds an expression evaluator to the context
func WithEvalContext(ctx context.Context, state interface{}, f func(ctx context.Context, state interface{}) (err error)) (err error) {
	key := exprEvaluatorContextKey()
	eev := exprEvaluators.Get()
	ee := eev.(*exprEvaluator)
	ctx = ctxutil.WithValue(ctx, key, eev)
	err = f(ctx, state)
	if err == nil {
		ee.reset()
		exprEvaluators.Put(eev)
	}
	return
}

// exprEvaluator is a VM that evaluates expressions.  exprEvaluators
// are not safe for concurrent usage.
type exprEvaluator struct {
	// callStack is the function call stack.
	callStack []eeFuncFrame
	// eeValues is the VM's value stack.
	eeValues

	// streq compares strings for equality.
	streq func(a, b string) bool

	// strcmp compares strings' sort orders.
	strcmp func(a, b string) int
}

func exprEvaluatorContextKey() interface{} { return (*exprEvaluator)(nil) }

func (ee *exprEvaluator) ContextKey() interface{} { return exprEvaluatorContextKey() }

// eeFuncFrame is an exprEvaluator function frame.
type eeFuncFrame struct {
	// fn is the function being executed in the frame.
	fn *opFunc

	// pc is the index of the currently-executing op code.
	pc int
}

// exprEvaluators is a pool of pre-constructed exprEvaluators that can
// be efficiently retrieved, used, and returned without always
// paying the computational cost of constructing and initializing a VM.
var exprEvaluators = sync.Pool{
	New: func() interface{} {
		const arbCap = 8
		return &exprEvaluator{
			eeValues: eeValues{
				types: make([]eeType, 0, 2*arbCap),
				anys:  make([]interface{}, 0, arbCap),
				nums:  make([]int64, 0, arbCap),
				strs:  make([]string, 0, arbCap),
			},
			streq:  defaultStreq,
			strcmp: strings.Compare,
		}
	},
}

func (ee *exprEvaluator) cmpAny() error {
	ee.pushInt(int64(eeCmp(&ee.eeValues)))
	ee.pushType(eeIntType)
	return nil
}

func (ee *exprEvaluator) evalFunc(ctx context.Context, f *opFunc, vs Values) (interface{}, error) {
	ee.callStack = append(ee.callStack, eeFuncFrame{
		fn: f,
		pc: 0,
	})
	if err := ee.eval(ctx, vs); err != nil {
		return nil, err
	}
	return ee.popType().pop(&ee.eeValues), nil
}

func (ee *exprEvaluator) eval(ctx context.Context, vs Values) (evalErr error) {
	// TODO: Instead of returning ErrInvalidType for currently
	// unsupported built-in operands, dispatch to the eeType.
opCodeLoop:
	for {
		var op opCode
		var extraOps []opCode
		{
			ff := ee.ff(0)
			cmp := ff.pc - len(ff.fn.ops)
			switch {
			case cmp > 0:
				logger.Warn4(
					"%v at stack frame %v "+
						"PC is at %d, "+
						"but only has "+
						"%v opcodes",
					ee, len(ee.callStack),
					ff.pc, len(ff.fn.ops),
				)
				fallthrough
			case cmp == 0:
				break opCodeLoop
			}
			op = ff.fn.ops[ff.pc]
			ff.pc++
			extra := op.length() - 1
			if extra > 0 {
				extraOps = ff.fn.ops[ff.pc : ff.pc+extra]
				ff.pc += extra
			}
		}
		switch op {
		case opNop:
			continue
		case opRet:
			return nil
		case opDerefAny:
			v, _ := ee.popAny(), ee.popType()
			ee.push(reflect.ValueOf(v).Elem().Interface())
		case opDerefBool:
			ee.pushBool(*(ee.popType().pop(&ee.eeValues).(*bool)))
			ee.pushType(eeBoolType)
		case opDerefFloat:
			ee.pushFloat(*(ee.popType().pop(&ee.eeValues).(*float64)))
			ee.pushType(eeFloat64Type)
		case opDerefInt:
			ee.pushInt(*(ee.popType().pop(&ee.eeValues).(*int64)))
			ee.pushType(eeInt64Type)
		case opDerefStr:
			ee.pushStr(*(ee.popType().pop(&ee.eeValues).(*string)))
			ee.pushType(eeStringType)
		case opLdv:
			//v, _ := ee.popAny(), ee.popType()
			v := ee.popType().pop(&ee.eeValues)
			if vs == nil || vs == NoValues() {
				if vs, evalErr = ValuesFromContext(ctx); evalErr != nil {
					return ee.wrapError(
						"failed to get values "+
							"from context"+
							": %w",
						evalErr,
					)
				}
			}
			v, err := vs.Get(ctx, v.(Var))
			if err != nil {
				return fmt.Errorf(
					"failed to get %v from context: %v",
					v, err,
				)
			}
			ee.push(v)
		case opNotBool:
			ee.pushBool(!ee.popBool())
		case opNotInt:
			ee.pushInt(^ee.popInt())
		case opEqAny:
			et := ee.peekType()
			ee.pushBool(et.eq(&ee.eeValues))
			ee.pushType(eeBoolType)
		case opEqBool:
			a, _ := ee.popBool(), ee.popType()
			b, _ := ee.popBool(), ee.popType()
			ee.pushBool(a == b)
			ee.pushType(eeBoolType)
		case opEqFloat:
			a, _ := ee.popFloat(), ee.popType()
			b, _ := ee.popFloat(), ee.popType()
			ee.pushBool(a == b) // TODO: epsilon?
			ee.pushType(eeBoolType)
		case opEqInt:
			a, _ := ee.popInt(), ee.popType()
			b, _ := ee.popInt(), ee.popType()
			ee.pushBool(a == b)
			ee.pushType(eeBoolType)
		case opEqRat:
			a, _ := ee.popRat(), ee.popType()
			b, _ := ee.popRat(), ee.popType()
			ee.pushBool(a.Cmp(b) == 0)
			ee.pushType(eeBoolType)
		case opEqStr:
			a, _ := ee.popStr(), ee.popType()
			b, _ := ee.popStr(), ee.popType()
			ee.pushBool(ee.streq(a, b))
			ee.pushType(eeBoolType)
		case opNeAny:
			et := ee.peekType()
			ee.pushBool(!et.eq(&ee.eeValues))
			ee.pushType(eeBoolType)
		case opNeBool:
			a, _ := ee.popBool(), ee.popType()
			b, _ := ee.popBool(), ee.popType()
			ee.pushBool(a != b)
			ee.pushType(eeBoolType)
		case opNeFloat:
			a, _ := ee.popFloat(), ee.popType()
			b, _ := ee.popFloat(), ee.popType()
			ee.pushBool(a != b) // TODO: epsilon?
			ee.pushType(eeBoolType)
		case opNeInt:
			a, _ := ee.popInt(), ee.popType()
			b, _ := ee.popInt(), ee.popType()
			ee.pushBool(a != b)
			ee.pushType(eeBoolType)
		case opNeRat:
			a, _ := ee.popRat(), ee.popType()
			b, _ := ee.popRat(), ee.popType()
			ee.pushBool(a.Cmp(b) != 0)
			ee.pushType(eeBoolType)
		case opNeStr:
			a, _ := ee.popStr(), ee.popType()
			b, _ := ee.popStr(), ee.popType()
			ee.pushBool(!ee.streq(a, b))
			ee.pushType(eeBoolType)
		case opLtAny:
			if err := ee.cmpAny(); err != nil {
				return err
			}
		case opLtFloat:
			a, _ := ee.popFloat(), ee.popType()
			b, _ := ee.popFloat(), ee.popType()
			ee.pushBool(a < b) // TODO: epsilon?
			ee.pushType(eeBoolType)
		case opLtInt:
			a, _ := ee.popInt(), ee.popType()
			b, _ := ee.popInt(), ee.popType()
			ee.pushBool(a < b)
			ee.pushType(eeBoolType)
		case opLtRat:
			a, _ := ee.popRat(), ee.popType()
			b, _ := ee.popRat(), ee.popType()
			ee.pushBool(a.Cmp(b) < 0)
			ee.pushType(eeBoolType)
		case opLtStr:
			a, _ := ee.popStr(), ee.popType()
			b, _ := ee.popStr(), ee.popType()
			ee.pushBool(ee.strcmp(a, b) < 0)
			ee.pushType(eeBoolType)
		case opLeAny:
			if err := ee.cmpAny(); err != nil {
				return err
			}
		case opLeFloat:
			a, _ := ee.popFloat(), ee.popType()
			b, _ := ee.popFloat(), ee.popType()
			ee.pushBool(a <= b) // TODO: epsilon?
			ee.pushType(eeBoolType)
		case opLeInt:
			a, _ := ee.popInt(), ee.popType()
			b, _ := ee.popInt(), ee.popType()
			ee.pushBool(a <= b)
			ee.pushType(eeBoolType)
		case opLeRat:
			a, _ := ee.popRat(), ee.popType()
			b, _ := ee.popRat(), ee.popType()
			ee.pushBool(a.Cmp(b) <= 0)
			ee.pushType(eeBoolType)
		case opLeStr:
			a, _ := ee.popStr(), ee.popType()
			b, _ := ee.popStr(), ee.popType()
			ee.pushBool(ee.strcmp(a, b) <= 0)
			ee.pushType(eeBoolType)
		case opGeAny:
			if err := ee.cmpAny(); err != nil {
				return err
			}
		case opGeFloat:
			a, _ := ee.popFloat(), ee.popType()
			b, _ := ee.popFloat(), ee.popType()
			ee.pushBool(a >= b) // TODO: epsilon?
			ee.pushType(eeBoolType)
		case opGeInt:
			a, _ := ee.popInt(), ee.popType()
			b, _ := ee.popInt(), ee.popType()
			ee.pushBool(a >= b)
			ee.pushType(eeBoolType)
		case opGeRat:
			a, _ := ee.popRat(), ee.popType()
			b, _ := ee.popRat(), ee.popType()
			ee.pushBool(a.Cmp(b) >= 0)
			ee.pushType(eeBoolType)
		case opGeStr:
			a, _ := ee.popStr(), ee.popType()
			b, _ := ee.popStr(), ee.popType()
			ee.pushBool(ee.strcmp(a, b) >= 0)
			ee.pushType(eeBoolType)
		case opGtAny:
			if err := ee.cmpAny(); err != nil {
				return err
			}
		case opGtFloat:
			a, _ := ee.popFloat(), ee.popType()
			b, _ := ee.popFloat(), ee.popType()
			ee.pushBool(a > b) // TODO: epsilon?
			ee.pushType(eeBoolType)
		case opGtInt:
			a, _ := ee.popInt(), ee.popType()
			b, _ := ee.popInt(), ee.popType()
			ee.pushBool(a > b)
			ee.pushType(eeBoolType)
		case opGtRat:
			a, _ := ee.popRat(), ee.popType()
			b, _ := ee.popRat(), ee.popType()
			ee.pushBool(a.Cmp(b) > 0)
			ee.pushType(eeBoolType)
		case opGtStr:
			a, _ := ee.popStr(), ee.popType()
			b, _ := ee.popStr(), ee.popType()
			ee.pushBool(ee.strcmp(a, b) > 0)
			ee.pushType(eeBoolType)
		case opAndBool:
			a, _ := ee.popBool(), ee.popType()
			b, _ := ee.popBool(), ee.popType()
			ee.pushBool(a && b)
			ee.pushType(eeBoolType)
		case opAndInt:
			a, _ := ee.popInt(), ee.popType()
			b, _ := ee.popInt(), ee.popType()
			ee.pushInt(a & b)
			ee.pushType(eeIntType)
		case opAndRat:
			a, _ := ee.popRat(), ee.popType()
			b, _ := ee.popRat(), ee.popType()
			ee.pushRat((&big.Rat{}).SetFrac(
				(&big.Int{}).And(a.Num(), b.Num()),
				(&big.Int{}).And(a.Denom(), b.Denom()),
			))
			ee.pushType(eeRatType)
		case opOrBool:
			a, _ := ee.popBool(), ee.popType()
			b, _ := ee.popBool(), ee.popType()
			ee.pushBool(a || b)
			ee.pushType(eeBoolType)
		case opOrInt:
			a, _ := ee.popInt(), ee.popType()
			b, _ := ee.popInt(), ee.popType()
			ee.pushInt(a | b)
			ee.pushType(eeBoolType)
		case opOrRat:
			a, _ := ee.popRat(), ee.popType()
			b, _ := ee.popRat(), ee.popType()
			ee.pushRat((&big.Rat{}).SetFrac(
				(&big.Int{}).Or(a.Num(), b.Num()),
				(&big.Int{}).Or(a.Denom(), b.Denom()),
			))
			ee.pushType(eeRatType)
		case opAddFloat:
			a, _ := ee.popFloat(), ee.popType()
			b := ee.popFloat()
			ee.pushFloat(a + b)
		case opAddInt:
			a, _ := ee.popInt(), ee.popType()
			b := ee.popInt()
			ee.pushInt(a + b)
		case opAddRat:
			a, _ := ee.popRat(), ee.popType()
			b := ee.popRat()
			ee.pushRat((&big.Rat{}).Add(a, b))
		case opAddStr:
			a, _ := ee.popStr(), ee.popType()
			b := ee.popStr()
			ee.pushStr(a + b)
		case opSubFloat:
			a, _ := ee.popFloat(), ee.popType()
			b := ee.popFloat()
			ee.pushFloat(a - b)
		case opSubInt:
			a, _ := ee.popInt(), ee.popType()
			b := ee.popInt()
			ee.pushInt(a - b)
		case opSubRat:
			a, _ := ee.popRat(), ee.popType()
			b := ee.popRat()
			ee.pushRat((&big.Rat{}).Sub(a, b))
		case opMulFloat:
			a, _ := ee.popFloat(), ee.popType()
			b := ee.popFloat()
			ee.pushFloat(a * b)
		case opMulInt:
			a, _ := ee.popInt(), ee.popType()
			b := ee.popInt()
			ee.pushInt(a * b)
		case opMulRat:
			a, _ := ee.popRat(), ee.popType()
			b := ee.popRat()
			ee.pushRat((&big.Rat{}).Mul(a, b))
		case opDivFloat:
			a, _ := ee.popFloat(), ee.popType()
			b := ee.popFloat()
			ee.pushFloat(a / b)
		case opDivInt:
			a, _ := ee.popInt(), ee.popType()
			b := ee.popInt()
			ee.pushInt(a / b)
		case opDivRat:
			a, _ := ee.popRat(), ee.popType()
			b := ee.popRat()
			ee.pushRat((&big.Rat{}).Quo(a, b))
		case opLdmAnyAny:
			v, vt, _ := ee.popAny(), ee.popType(), ee.popType()
			switch m := ee.popAny().(type) {
			case *reflect.StructField:
				v = vt.unsafereflectType().FieldPointer(v, m.Index[0])
			default:
				return fmt.Errorf(
					"%[1]w: Unexpected member: %[2]v (type: %[2]T) of %[3]v (%[3]T)",
					ErrInvalidType, m, v,
				)
			}
			et := typeOf(v)
			ee.pushType(et)
			et.push(&ee.eeValues, v)
		case opLdmStrMapAny:
			m := reflect.ValueOf(
				ee.popType().pop(&ee.eeValues),
			).Convert(
				mapStrAnyType.ReflectType(),
			).Interface().(map[string]interface{})
			k, _ := ee.popStr(), ee.popType()
			v := m[k]
			et := typeOf(v)
			et.push(&ee.eeValues, v)
			ee.pushType(et)
		case opConvFloat64ToInt64:
			a, _ := ee.popFloat(), ee.popType()
			ee.pushInt(int64(a))
			ee.pushType(eeIntType)
		case opConvFloat64ToRat:
			a, _ := ee.popFloat(), ee.popType()
			ee.pushRat((&big.Rat{}).SetFloat64(a))
			ee.pushType(eeRatType)
		case opConvInt64ToFloat64:
			a, _ := ee.popInt(), ee.popType()
			ee.pushFloat(float64(a))
			ee.pushType(eeFloat64Type)
		case opConvInt64ToRat:
			a, _ := ee.popInt(), ee.popType()
			ee.pushRat((&big.Rat{}).SetInt64(a))
			ee.pushType(eeRatType)
		case opConvRatToFloat64:
			a, _ := ee.popRat(), ee.popType()
			f, _ := a.Float64()
			ee.pushFloat(f)
			ee.pushType(eeFloat64Type)
		case opConvRatToInt64:
			a, _ := ee.popRat(), ee.popType()
			num, denom := a.Num(), a.Denom()
			var i int64
			if !a.IsInt() {
				div, rem := (&big.Int{}).DivMod(num, denom, &big.Int{})
				rem = rem.Mul(rem, &bigInts[2])
				if rem.Cmp(denom) >= 0 {
					div = div.Add(div, &bigInts[1])
				}
				num = div
			}
			i = num.Int64()
			if !num.IsInt64() {
				// TODO: Is saturation the best thing
				// to do here?
				if num.Cmp(&bigInts[0]) < 0 {
					i = math.MinInt64
				} else {
					i = math.MaxInt64
				}
			}
			ee.pushInt(i)
			ee.pushType(eeInt64Type)
		case opLdc:
			vk, _ := eeValueKeyFromOpCodes(extraOps)
			ee.ff(0).fn.consts.copyTo(vk, &ee.eeValues)
		case opPackTuple:
			vk, _ := eeValueKeyFromOpCodes(extraOps)
			t := make(Tuple, 0, int(vk.data))
			for len(t) < cap(t) {
				t = append(t, ee.eeValues.popType().pop(&ee.eeValues))
			}
			ee.eeValues.push(t)
		default:
			return fmt.Errorf(
				"%w: %v",
				ErrBadOp, op,
			)
		}
	}
	return nil
}

// ff gets the eeFuncFrame from the top of the call stack
func (ee *exprEvaluator) ff(fromTop int) *eeFuncFrame {
	return &ee.callStack[len(ee.callStack)-fromTop-1]
}

func (ee *exprEvaluator) wrapError(format string, args ...interface{}) error {
	var err error
	if format != "" {
		err = fmt.Errorf(format, args...)
	} else if len(args) == 1 {
		var ok bool
		err, ok = args[0].(error)
		if !ok {
			logger.Warn1(
				"wrapError expects single-arg to be "+
					"an error, not %T",
				args[0],
			)
		}
	} else {
		err = fmt.Errorf("unknown error")
	}
	ff := ee.ff(0)
	return eeError{ff.fn, ff.pc, err}
}

func (ee *exprEvaluator) reset() {
	{
		anys := ee.anys[:cap(ee.anys)]
		for i := range anys {
			anys[i] = nil
		}
		ee.anys = ee.anys[:0]
	}
	// no pointers; so skip nums:
	/*{
		nums := ee.nums[:cap(ee.nums)]
		for i := range nums {
			nums[i] = 0
		}
		ee.nums = ee.nums[:0]
	}*/
	{
		strs := ee.strs[:cap(ee.strs)]
		for i := range strs {
			strs[i] = ""
		}
		ee.strs = ee.strs[:0]
	}
	ee.streq = defaultStreq
	ee.strcmp = strings.Compare
}

//go:generate stringer -type=opCode -trimprefix=op
type opCode uint8

func opCodesBytes(ops []opCode) (bs []byte) {
	_ = [1]struct{}{}[unsafe.Sizeof(ops[0])-unsafe.Sizeof(bs[0])]
	bs = *((*[]byte)(unsafe.Pointer(&ops)))
	return
}

func opCodesFromBytes(bs []byte) (ops []opCode) {
	_ = [1]struct{}{}[unsafe.Sizeof(ops[0])-unsafe.Sizeof(bs[0])]
	ops = *((*[]opCode)(unsafe.Pointer(&bs)))
	return
}

const (
	// opNop does nothing
	opNop opCode = iota

	// opRet returns from the function
	opRet

	// opDerefAny dereferences a pointer to anything.
	opDerefAny
	// opDerefBool dereferences a pointer to bool.
	opDerefBool
	// opDerefFloat dereferences a pointer to float64.
	opDerefFloat
	// opDerefFloat dereferences a pointer to int64.
	opDerefInt
	// opDerefStr dereferences a pointer to string.
	opDerefStr

	// opLdv loads the value of a variable at the top of the stack.
	opLdv

	// opNotBool inverts the top boolean result
	//
	//	push(!pop())
	//
	opNotBool

	// opNotInt performs a binary NOT operation on two ints
	//
	//	push(pop() ^ pop())
	//
	opNotInt

	// opEqAny checks if two Any values on top of the stack are
	// equal.
	opEqAny

	// opEqBool checks if two bool values on top of the stack are
	// equal.
	opEqBool

	// opEqFloat checks if two float values on top of the stack are
	// equal.
	opEqFloat

	// opEqInt checks if two int values on top of the stack are
	// equal.
	opEqInt

	// opEqRat checks if two (*math/big.Rat) values on top of the
	// stack are equal.
	opEqRat

	// opEqStr checks if two string values on top of the stack are
	// equal.
	opEqStr

	// opNeAny checks if two Any values on top of the stack are
	// not equal.
	opNeAny

	// opNeBool checks if two bool values on top of the stack are
	// not equal.
	opNeBool

	// opNeFloat checks if two float values on top of the stack are
	// not equal.
	opNeFloat

	// opNeInt checks if two int values on top of the stack are
	// not equal.
	opNeInt

	// opNeRat checks if two (*math/big.Rat) values on top of the
	// stack are not equal.
	opNeRat

	// opNeStr checks if two string values on top of the stack are
	// not equal.
	opNeStr

	// opLtAny evaluates if the first operand on the stack is
	// less than the second:
	//
	//	a := pop()
	//	b := pop()
	//	push(a.cmp(b) < 0)
	//
	// where a.cmp is determined by a's eeType.
	opLtAny

	// opLtFloat evaluates if the first operand on the stack is
	// less than the second:
	//
	//	a := pop()
	//	b := pop()
	//	push(a < b)
	//
	opLtFloat

	// opLtInt (see opLtFloat)
	opLtInt
	// opLtRat (see opLtFloat)
	opLtRat
	// opLtStr (see opLtFloat)
	opLtStr

	// opLeAny evaluates if the first operand on the stack is
	// less than or equal to the second:
	//
	//	a := pop()
	//	b := pop()
	//	push(a.cmp(b) <= 0)
	//
	// where a.cmp is determined by a's eeType.
	opLeAny

	// opLeFloat evaluates if the first operand on the stack is
	// less than or equal to the second:
	//
	//	a := pop()
	//	b := pop()
	//	push(a <= b)
	//
	opLeFloat

	// opLeInt (see opLeFloat)
	opLeInt
	// opLeRat (see opLeFloat)
	opLeRat
	// opLeStr (see opLeFloat)
	opLeStr

	// opGeAny evaluates if the first operand on the stack is
	// greater than or equal to the second:
	//
	//	a := pop()
	//	b := pop()
	//	push(a.cmp(b) >= 0)
	//
	// where a.cmp is determined by a's eeType.
	opGeAny

	// opGeFloat evaluates if the first operand on the stack is
	// greater than or equal to the second:
	//
	//	a := pop()
	//	b := pop()
	//	push(a >= b)
	//
	opGeFloat

	// opGeInt (see opGeFloat)
	opGeInt
	// opGeRat (see opGeFloat)
	opGeRat
	// opGeStr (see opGeFloat)
	opGeStr

	// opGtAny evaluates if the first operand on the stack is
	// greater than the second:
	//
	//	a := pop()
	//	b := pop()
	//	push(a.cmp(b) > 0)
	//
	// where a.cmp is determined by a's eeType.
	opGtAny

	// opGtFloat evaluates if the first operand on the stack is
	// greater than the second:
	//
	//	a := pop()
	//	b := pop()
	//	push(a > b)
	//
	opGtFloat

	// opGtInt (see opGtFloat)
	opGtInt
	// opGtRat (see opGtFloat)
	opGtRat
	// opGtStr (see opGtFloat)
	opGtStr

	// opAndBool pushes a logical AND of its operands
	//
	//	push(pop() && pop())
	//
	opAndBool
	// opAndInt pushes a binary AND of its operands
	//
	//	push(pop() & pop())
	//
	opAndInt
	// opAndRat performs a bitwise AND of both the numerator and
	// denominator.
	//
	//	push(pop() & pop())
	//
	opAndRat

	// opOrBool pushes a logical OR of its operands
	//
	//	push(pop() || pop())
	//
	opOrBool
	// opOrInt pushes a logical AND of its operands
	//
	//	push(pop() | pop())
	//
	opOrInt
	// opOrRat performs a bitwise OR of both the numerator and
	// denominator.
	//
	//	push(pop() | pop())
	//
	opOrRat

	// opAddFloat adds the top two operands on the stack
	//
	//	a := pop()
	//	b := pop()
	//	push(a + b)
	//
	opAddFloat

	// opAddInt (see opAddFloat)
	opAddInt
	// opAddRat (see opAddFloat)
	opAddRat
	// opAddStr appends two strings together
	//
	//	a := pop()
	//	b := pop()
	//	push(a + b)
	//
	opAddStr

	// opSubFloat adds the top two operands on the stack
	//
	//	a := pop()
	//	b := pop()
	//	push(a - b)
	//
	opSubFloat

	// opSubInt (see opSubFloat)
	opSubInt
	// opSubRat (see opSubFloat)
	opSubRat

	// opMulFloat adds the top two operands on the stack
	//
	//	a := pop()
	//	b := pop()
	//	push(a * b)
	//
	opMulFloat

	// opMulInt (see opMulFloat)
	opMulInt
	// opMulRat (see opMulFloat)
	opMulRat

	// opDivFloat adds the top two operands on the stack
	//
	//	a := pop()
	//	b := pop()
	//	push(a / b)
	//
	opDivFloat

	// opDivInt (see opDivFloat)
	opDivInt
	// opDivRat (see opDivFloat)
	opDivRat

	// opLdmAnyAny loads an any member from an any value.
	// Struct field members are stored as *reflect.StructFields.
	opLdmAnyAny

	// opLdmStrMapAny loads a value from a map[string]interface{}
	opLdmStrMapAny

	// opConvFloat64ToInt64 converts a float64 to an int64
	opConvFloat64ToInt64
	// opConvFloat64ToRat converts a float64 to a (*math/big).Rat
	opConvFloat64ToRat

	// opConvIntToFloat converts an int64 to a float64
	opConvInt64ToFloat64
	// opConvInt64ToRat converts an int64 to a (*math/big).Rat
	opConvInt64ToRat

	// opConvRatToFloat64 converts a (*math/big).Rat to a float64
	opConvRatToFloat64
	// opConvRatToInt64 converts a (*math/big).Rat to an int64.
	// If the value is fractional, if that fractional component
	// is >=0.5, the integer is rounted up.  If the value cannot
	// fit into an int64, the int64 is "saturated"
	// (values > math.MaxInt64 are converted to math.MaxInt64,
	// values < math.MinInt64 are converted to math.MinInt64).
	opConvRatToInt64

	// opLdc loads a constant value onto the stack.  It currently
	// doesn't need different opKind variants because of its
	// implementation, but maybe a future version will.
	opLdc

	// opPackTuple is followed by an integer that indicates the
	// number of values on the stack to be packed into a Tuple.
	opPackTuple

	// opBrt branches if the top value on the stack is true.
	// This opCode has an int following it with the opCode offset
	// to the branched code.
	opBrt
)

const opFirstMultiByteCode = opLdc

func (op opCode) length() int {
	if op < opFirstMultiByteCode {
		return 1
	}
	switch op {
	case opLdc, opPackTuple:
		return 1 + int(unsafe.Sizeof(eeValueKey{}))
	case opBrt:
		return 1 + bits.UintSize/8
	}
	panic(fmt.Errorf("%w: %v", ErrBadOp, op))
}

type opKind uint8

const (
	opAny opKind = iota
	opBool
	opFloat
	opInt
	opRat
	opStr
)

func defaultStreq(a, b string) bool { return a == b }

type eeError struct {
	Func Func
	PC   int
	Err  error
}

func (e eeError) Error() string {
	return fmt.Sprintf("%v +%d: %v", nameOfFunc(e.Func), e.PC, e.Err)
}

func (e eeError) Unwrap() error { return e.Err }
