package vm

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
	"github.com/skillian/logging"
)

// VM is a virtual machine that can execute bytecode-compiled functions
type VM struct {
	stack Stack
}

// Option configures a VM
type Option func(*VM) error

// Capacity defines the VM operand capacity
func Capacity(c int) Option {
	return func(vm *VM) error {
		if c < 0 {
			return errors.Errorf1(
				"invalid capacity: %d", c)
		}
		vm.stack.init(c)
		return nil
	}
}

// New creates a new VM that can evaluate dynamically-compiled
// expressions.
func New(options ...Option) (*VM, error) {
	vm := &VM{}
	for _, o := range options {
		if err := o(vm); err != nil {
			return nil, err
		}
	}
	if cap(vm.stack.Values.ints) == 0 {
		vm.stack.init(8)
	}
	return vm, nil
}

type vmContextKey struct{}

// FromContext attempts to retrieve a VM associated with the given context.
func FromContext(ctx context.Context) (vm *VM, ok bool) {
	vm, ok = ctx.Value(vmContextKey{}).(*VM)
	return
}

// AddToContext adds the VM to the context if it isn't already in the context.
func (vm *VM) AddToContext(ctx context.Context) (new context.Context, added bool) {
	vm2, ok := FromContext(ctx)
	if ok && vm == vm2 {
		return ctx, false
	}
	return context.WithValue(ctx, vmContextKey{}, vm), true
}

// Var implements the stream Var interface but includes the encoding
// of its VM OpType
type Var struct {
	OpType
}

// Var opts-in to the expr.Var interface.
func (v *Var) Var() expr.Var { return v }

var (
	_ expr.Var = (*Var)(nil)

	logger = logging.GetLogger(
		"expr/vm",
		logging.LoggerLevel(logging.VerboseLevel))
)

func (vm *VM) execFunc(ctx context.Context, f *Func, vs expr.Values) (err error) {
	fi := len(vm.stack.frames) - 1
	for pc := 0; pc < len(f.opCodes); pc++ {
		frame := vm.stack.frame(fi)
		op := f.opCodes[pc]
		logger.Verbose3("pc: %d\top: %v\tvm: %v", pc, op, vm)
		i, t, a := op.decode()
		switch i {
		case Nop:
			continue
		case Not:
			b, ok := frame.popBool()
			if !ok {
				return vm.errStackUnderflow()
			}
			frame.pushBool(!b)
			continue
		}
		if i.isBinary() {
			switch t {
			case Any:
				right, ok := frame.popAny()
				if !ok {
					return vm.errStackUnderflow()
				}
				var left interface{}
				switch OpType(a) {
				case Any:
					left, ok = frame.popAny()
				case Bool:
					left, ok = frame.popBool()
				case Str:
					left, ok = frame.popStr()
				case Int:
					left, ok = frame.popInt()
				case Int64:
					left, ok = frame.popInt64()
				default:
					return vm.errBadOp(op)
				}
				if !ok {
					return vm.errStackUnderflow()
				}
				switch i {
				case Eq:
					frame.pushBool(left == right)
				case Ne:
					frame.pushBool(left != right)
				default:
					return vm.errBadOp(op)
				}
				continue
			case Bool:
				b, ok := frame.popBool()
				if !ok {
					return vm.errStackUnderflow()
				}
				a, ok := frame.popBool()
				if !ok {
					return vm.errStackUnderflow()
				}
				switch i {
				case Eq:
					a = a == b
				case Ne:
					a = a != b
				case And:
					a = a && b
				case Or:
					a = a || b
				default:
					return vm.errBadOp(op)
				}
				frame.pushBool(a)
				continue
			case Str:
				b, ok := frame.popStr()
				if !ok {
					return vm.errStackUnderflow()
				}
				a, ok := frame.popStr()
				if !ok {
					return vm.errStackUnderflow()
				}
				switch i {
				case Add:
					frame.pushStr(a + b)
				case Eq:
					frame.pushBool(a == b)
				case Ne:
					frame.pushBool(a != b)
				default:
					return vm.errBadOp(op)
				}
				continue
			case Int:
				var right int
				var ok bool
				switch OpType(a) {
				case Any:
					x, popOk := frame.popAny()
					if !popOk {
						return vm.errStackUnderflow()
					}
					right, ok = x.(int)
				case Int:
					right, ok = frame.popInt()
				default:
					return vm.errBadOp(op)
				}
				if !ok {
					return vm.errStackUnderflow()
				}
				left, ok := frame.popInt()
				if !ok {
					return vm.errStackUnderflow()
				}
				if i >= Add {
					switch i {
					case Add:
						left += right
					case Sub:
						left -= right
					case Mul:
						left *= right
					case Div:
						left /= right
					default:
						return vm.errBadOp(op)
					}
					frame.pushInt(left)
					continue
				}

				switch i {
				case Eq:
					ok = left == right
				case Ne:
					ok = left != right
				case Gt:
					ok = left > right
				case Ge:
					ok = left >= right
				case Lt:
					ok = left < right
				case Le:
					ok = left <= right
				default:
					return vm.errBadOp(op)
				}
				frame.pushBool(ok)
				continue
			case Int64:
				b, ok := frame.popInt64()
				if !ok {
					return vm.errStackUnderflow()
				}
				a, ok := frame.popInt64()
				if !ok {
					return vm.errStackUnderflow()
				}
				if i >= Add {
					switch i {
					case Add:
						a += b
					case Sub:
						a -= b
					case Mul:
						a *= b
					case Div:
						a /= b
					default:
						return vm.errBadOp(op)
					}
					frame.pushInt64(a)
					continue
				}

				switch i {
				case Eq:
					ok = a == b
				case Ne:
					ok = a != b
				case Gt:
					ok = a > b
				case Ge:
					ok = a >= b
				case Lt:
					ok = a < b
				case Le:
					ok = a <= b
				default:
					return vm.errBadOp(op)
				}
				frame.pushBool(ok)
				continue
			default:
				return vm.errBadOp(op)
			}
		}
		ld := false
		switch i {
		case LdConst:
			switch t {
			case Any:
				frame.pushAny(f.consts.anys[int(a)])
			case Bool:
				frame.pushBool(a != 0)
			case Str:
				frame.pushStr(f.consts.strs[int(a)])
			case Int:
				frame.pushInt(f.consts.ints[int(a)])
			case Int64:
				frame.pushInt64(f.consts.getInt64(nil, int(a)))
			default:
				return vm.errBadOp(op)
			}
			continue
		case LdStack:
			ld = true
			fallthrough
		case StStack:
			switch t {
			case Any:
				b := frame.frame.anys
				x := &frame.stack.anys[b[0]+int(a)]
				if ld {
					frame.pushAny(*x)
				} else {
					*x, ld = frame.popAny()
				}
			case Bool:
				b := frame.frame.ints
				x := &frame.stack.ints[b[0]+int(a)]
				if ld {
					frame.pushBool(*x != 0)
				} else {
					*x, ld = frame.popInt()
				}
			case Str:
				b := frame.frame.strs
				x := &frame.stack.strs[b[0]+int(a)]
				if ld {
					frame.pushStr(*x)
				} else {
					*x, ld = frame.popStr()
				}
			case Int:
				b := frame.frame.ints
				x := &frame.stack.ints[b[0]+int(a)]
				if ld {
					frame.pushInt(*x)
				} else {
					*x, ld = frame.popInt()
				}
			case Int64:
				if ld {
					x := frame.stack.Values.getInt64(frame.frame, int(a))
					frame.pushInt64(x)
				} else {
					x, ok := frame.popInt64()
					if !ok {
						return vm.errStackUnderflow()
					}
					frame.stack.Values.setInt64(frame.frame, int(a), x)
				}
			default:
				return vm.errBadOp(op)
			}
			if !ld {
				return vm.errStackUnderflow()
			}
			continue
		case LdMember:
			ld = true
			fallthrough
		case StMember:
			// Member expressions are pushed onto the stack
			// backwards because we have to pop the source first
			// before we pop the member.  This might change to be
			// more consistent in the future.

			// member type
			mt := OpType(a) & opTypeMask
			// member arg
			ma := int((a >> opTypeBits) & ((1<<opArgBits - opTypeBits) - 1))
			switch t {
			case Any:
				v, ok := frame.popAny()
				if !ok {
					return vm.errStackUnderflow()
				}
				switch v := v.(type) {
				case Memberer:
					var m interface{}
					switch mt {
					case Any:
						x, ok := frame.popAny()
						if !ok {
							return vm.errStackUnderflow()
						}
						m = x
					case Str:
						x, ok := frame.popStr()
						if !ok {
							return vm.errStackUnderflow()
						}
						m = x
					case Int64:
						x, ok := frame.popInt64()
						if !ok {
							return vm.errStackUnderflow()
						}
						m = x
					case Int:
						x, ok := frame.popInt()
						if !ok {
							return vm.errStackUnderflow()
						}
						m = x
					case Bool:
						x, ok := frame.popBool()
						if !ok {
							return vm.errStackUnderflow()
						}
						m = x
					default:
						return vm.errBadOp(op)
					}
					x, err := v.Member(m)
					if err != nil {
						return errors.Errorf2From(
							err, "failed to get member %v from %v",
							m, v)
					}
					if !frame.pushOp(OpType(ma), x) {
						return errors.Errorf2(
							"failed to push %v as %v",
							x, OpType(ma))
					}
				// TODO: complex real + imag "members"
				case string:
					if !ld {
						return errors.Errorf0From(
							vm.errBadOp(op),
							"cannot store into a string")
					}
					if err = vm.ldMemberStr(op, frame, mt, ma, v); err != nil {
						return err
					}
				default:
					rv := reflect.ValueOf(v)
					if err = vm.memberReflect(ctx, ld, op, frame, mt, ma, rv); err != nil {
						return err
					}
				}
			case Str:
				if !ld {
					return errors.Errorf0From(
						vm.errBadOp(op),
						"cannot store into a string")
				}
				v, ok := frame.popStr()
				if !ok {
					return vm.errStackUnderflow()
				}
				if err := vm.ldMemberStr(op, frame, mt, ma, v); err != nil {
					return err
				}
			default:
				return vm.errBadOp(op)
			}
			continue
		case LdVar:
			ld = true
			fallthrough
		case StVar:
			x, ok := frame.popAny()
			if !ok {
				return vm.errStackUnderflow()
			}
			va, ok := x.(expr.Var)
			if !ok {
				return vm.errBadType(op, &va, &x)
			}
			if ld {
				v := vs.Get(va)
				if vmv, ok := va.(*Var); ok {
					switch vmv.OpType {
					case Any:
						frame.pushAny(v)
					case Str:
						s, ok := v.(string)
						if !ok {
							return vm.errBadType(op, &s, &v)
						}
						frame.pushStr(s)
					case Int64:
						i, ok := v.(int64)
						if !ok {
							return vm.errBadType(op, &i, &v)
						}
						frame.pushInt64(i)
					case Int:
						i, ok := v.(int)
						if !ok {
							return vm.errBadType(op, &i, &v)
						}
						frame.pushInt(i)
					case Bool:
						b, ok := v.(bool)
						if !ok {
							return vm.errBadType(op, &b, &v)
						}
						frame.pushBool(b)
					default:
						return vm.errBadOp(op)
					}
					continue
				}
				frame.pushAny(x)
				continue
			}
			switch t {
			case Any:
				x, ok = frame.popAny()
			case Str:
				x, ok = frame.popStr()
			case Int64:
				x, ok = frame.popInt64()
			case Int:
				x, ok = frame.popInt()
			case Bool:
				x, ok = frame.popBool()
			}
			if !ok {
				return vm.errStackUnderflow()
			}
			vs.Set(va, x)
			continue
		case Conv:
			src, trg := t, OpType(a)
			if src == trg {
				continue
			}
			switch src {
			case Any:
				x, ok := frame.popAny()
				if !ok {
					return vm.errStackUnderflow()
				}
				switch trg {
				case Any:
					frame.pushAny(x)
				case Bool:
					var b bool
					switch x := x.(type) {
					case bool:
						b = x
					case int:
						b = x != 0
					case string:
						b = !(x == "" || x == "false")
					default:
						b = x != nil
					}
					frame.pushBool(b)
				case Int:
					var i int
					switch x := x.(type) {
					case bool:
						if x {
							i++
						}
					case int:
						i = x
					case string:
						i2, err := strconv.Atoi(x)
						if err != nil {
							return err
						}
						i = i2
					default:
						i2, err := strconv.Atoi(fmt.Sprint(x))
						if err != nil {
							return err
						}
						i = i2
					}
					frame.pushInt(i)
				case Str:
					frame.pushStr(fmt.Sprint(x))
				default:
					return vm.errBadOp(op)
				}
			case Bool:
				b, ok := frame.popBool()
				if !ok {
					return vm.errStackUnderflow()
				}
				switch trg {
				case Any:
					frame.pushAny(b)
				case Bool:
					frame.pushBool(b)
				case Int:
					i := 0
					if b {
						i++
					}
					frame.pushInt(i)
				case Str:
					s := "false"
					if b {
						s = "true"
					}
					frame.pushStr(s)
				default:
					return vm.errBadOp(op)
				}
			case Int:
				i, ok := frame.popInt()
				if !ok {
					return vm.errStackUnderflow()
				}
				switch trg {
				case Any:
					frame.pushAny(i)
				case Bool:
					frame.pushBool(i != 0)
				case Int:
					frame.pushInt(i)
				case Str:
					frame.pushStr(strconv.Itoa(i))
				default:
					return vm.errBadOp(op)
				}
			case Str:
				s, ok := frame.popStr()
				if !ok {
					return vm.errStackUnderflow()
				}
				switch trg {
				case Any:
					frame.pushAny(s)
				case Bool:
					frame.pushBool(s != "" && s != "false")
				case Int:
					i, err := strconv.Atoi(s)
					if err != nil {
						return err
					}
					frame.pushInt(i)
				default:
					return vm.errBadOp(op)
				}
			default:
				return vm.errBadOp(op)
			}
		case Pack:
			n := int(a)
			var v interface{}
			switch t {
			case Any:
				vs := make([]interface{}, n)
				for i := 0; i < n; i++ {
					x, ok := frame.popAny()
					if !ok {
						return vm.errStackUnderflow()
					}
					vs[len(vs)-1-i] = x
				}
				v = vs
			case Str:
				vs := make([]string, n)
				for i := 0; i < n; i++ {
					x, ok := frame.popStr()
					if !ok {
						return vm.errStackUnderflow()
					}
					vs[len(vs)-1-i] = x
				}
				v = vs
			case Int64:
				vs := make([]int64, n)
				for i := 0; i < n; i++ {
					x, ok := frame.popInt64()
					if !ok {
						return vm.errStackUnderflow()
					}
					vs[len(vs)-1-i] = x
				}
				v = vs
			case Int:
				vs := make([]int, n)
				for i := 0; i < n; i++ {
					x, ok := frame.popInt()
					if !ok {
						return vm.errStackUnderflow()
					}
					vs[len(vs)-1-i] = x
				}
				v = vs
			case Bool:
				vs := make([]bool, n)
				for i := 0; i < n; i++ {
					x, ok := frame.popBool()
					if !ok {
						return vm.errStackUnderflow()
					}
					vs[len(vs)-1-i] = x
				}
				v = vs
			default:
				return vm.errBadOp(op)
			}
			frame.pushAny(v)
			continue
		case Unpack:
			v, ok := frame.popAny()
			if !ok {
				return vm.errStackUnderflow()
			}
			switch t {
			case Any:
				vs, ok := v.([]interface{})
				if !ok {
					return vm.errBadType(op, &vs, &v)
				}
				for _, v := range vs {
					frame.pushAny(v)
				}
			case Str:
				vs, ok := v.([]string)
				if !ok {
					return vm.errBadType(op, &vs, &v)
				}
				for _, v := range vs {
					frame.pushStr(v)
				}
			case Int64:
				vs, ok := v.([]int64)
				if !ok {
					return vm.errBadType(op, &vs, &v)
				}
				for _, v := range vs {
					frame.pushInt64(v)
				}
			case Int:
				vs, ok := v.([]int)
				if !ok {
					return vm.errBadType(op, &vs, &v)
				}
				for _, v := range vs {
					frame.pushInt(v)
				}
			case Bool:
				vs, ok := v.([]bool)
				if !ok {
					return vm.errBadType(op, &vs, &v)
				}
				for _, v := range vs {
					frame.pushBool(v)
				}
			default:
				return vm.errBadOp(op)
			}
			continue
		case Ret:
			// TODO: zero-out the sliced-out values from the frame
			// so they don't hold onto memory
			vm.stack.anys = vm.stack.anys[:frame.frame.anys[0]+f.rets.anys]
			vm.stack.strs = vm.stack.strs[:frame.frame.strs[0]+f.rets.strs]
			vm.stack.ints = vm.stack.ints[:frame.frame.ints[0]+f.rets.ints]
			frame.frame.anys[1] = f.rets.anys
			frame.frame.strs[1] = f.rets.strs
			frame.frame.ints[1] = f.rets.ints
			continue
		default:
			return vm.errBadOp(op)
		}
	}
	return nil
}

// Memberer allows members to be accessed from values.
type Memberer interface {
	Member(key interface{}) (interface{}, error)
	SetMember(key, value interface{}) error
}

func (vm *VM) memberReflect(
	ctx context.Context, ld bool, op OpCode,
	frame stackFrameRef, mt OpType, ma int, v reflect.Value) (err error) {
	if !v.IsValid() {
		return errors.Errorf("cannot access invalid value")
	}
	elemoptype := OpType(ma & opTypeMask)
	switch v.Kind() {
	// TODO:
	//	case reflect.Array:
	//	case reflect.Chan:
	case reflect.Map:
		keyreftyp, elemreftyp := v.Type().Key(), v.Type().Elem()
		keyoptype := mt
		key, err := vm.popOpReflect(keyoptype, keyreftyp, frame)
		if err != nil {
			return err
		}
		if ld {
			elem := v.MapIndex(key)
			if !elem.IsValid() {
				frame.pushOpZero(elemoptype)
				return nil
			}
			frame.pushOp(elemoptype, elem.Interface())
			return nil
		}
		elem, err := vm.popOpReflect(elemoptype, elemreftyp, frame)
		if err != nil {
			return err
		}
		v.SetMapIndex(key, elem)
		return nil
	case reflect.Ptr:
		if v.IsNil() {
			if ld {
				frame.pushAny(nil)
				return nil
			}
			return errors.Errorf0("cannot store into nil pointer.")
		}
		v = v.Elem()
		if !v.IsValid() {
			return errors.Errorf("cannot get/set member from/to invalid pointer element")
		}
		fallthrough
	case reflect.Struct:
		var fv reflect.Value
		switch mt {
		case Any:
			x, ok := frame.popAny()
			if !ok {
				return vm.errStackUnderflow()
			}
			switch x := x.(type) {
			case string:
				fv = v.FieldByName(x)
			case int:
				fv = v.Field(x)
			case []int:
				fv = v.FieldByIndex(x)
			default:
				return errors.Errorf2From(
					vm.errBadOp(op),
					"cannot get %[1]v (type: %[1]T) "+
						"field from %[2]v (type %[2]T)",
					x, v.Interface())
			}
		case Str:
			x, ok := frame.popStr()
			if !ok {
				return vm.errStackUnderflow()
			}
			fv = v.FieldByName(x)
		case Int:
			x, ok := frame.popInt()
			if !ok {
				return vm.errStackUnderflow()
			}
			fv = v.Field(x)
		default:
			return errors.Errorf2From(
				vm.errBadOp(op),
				"cannot get %[1]T %[1]v member of "+
					"%[2]v (type: %[2]T)",
				mt, v.Interface())
		}
		if fv.IsValid() {
			if ld {
				fv, err = reflectConvert(fv, reflectTypesByOp[elemoptype])
				if err != nil {
					return err
				}
				switch elemoptype {
				case Any:
					frame.pushAny(fv.Interface())
				case Bool:
					frame.pushBool(fv.Bool())
				case Str:
					frame.pushStr(fv.String())
				case Int:
					frame.pushInt(int(fv.Int()))
				case Int64:
					frame.pushInt64(fv.Int())
				default:
					panic(errors.Errorf1(
						"unknown %[1]T %[1]v",
						elemoptype))
				}
				return nil
			}
			rv, err := vm.popOpReflect(elemoptype, fv.Type(), frame)
			if err != nil {
				return err
			}
			fv.Set(rv)
		}
		// Turn the struct back into a pointer because many
		// structs have functions on pointers instead of the
		// structs themselves
		if v.CanAddr() {
			v = v.Addr()
		}
		fallthrough
	case reflect.Interface:
		var f reflect.Value
		switch mt {
		case Any:
			x, ok := frame.popAny()
			if !ok {
				return vm.errStackUnderflow()
			}
			switch x := x.(type) {
			case string:
				if !ld {
					x = "Set" + x
				}
				f = v.MethodByName(x)
			case int:
				f = v.Method(x)
			default:
				return errors.Errorf1From(
					vm.errBadOp(op),
					"unknown method member %[1]T "+
						"%[1]v",
					x)
			}
		case Str:
			x, ok := frame.popStr()
			if !ok {
				return vm.errStackUnderflow()
			}
			if !ld {
				x = "Set" + x
			}
			f = v.MethodByName(x)
		case Int:
			x, ok := frame.popInt()
			if !ok {
				return vm.errStackUnderflow()
			}
			f = v.Method(x)
		default:
			return errors.Errorf1From(
				vm.errBadOp(op),
				"unknown method member %[1]T "+
					"%[1]v",
				mt)

		}
		if !f.IsValid() {
			frame.pushOpZero(elemoptype)
			return nil
		}
		ft := f.Type()
		vs := make([]reflect.Value, 1, 3)
		vs[0] = v
		hasCtx, hasErr, err := vm.checkReflectMethod(ld, f)
		if err != nil {
			return err
		}
		if hasCtx {
			vs = append(vs, reflect.ValueOf(ctx))
		}
		if !ld {
			x, err := vm.popOpReflect(elemoptype, ft.In(ft.NumIn()-1), frame)
			if err != nil {
				return err
			}
			vs = append(vs, x)
		}
		vs = f.Call(vs)
		if hasErr {
			if err = vs[len(vs)-1].Interface().(error); err != nil {
				return err
			}
		}
		if ld {
			frame.pushOp(elemoptype, vs[0].Interface())
		}
	}
	return nil
}

func (vm *VM) ldMemberStr(op OpCode, frame stackFrameRef, mt OpType, ma int, v string) error {
	if mt != Int {
		return errors.Errorf0From(
			vm.errBadOp(op),
			"string member types can only be int")
	}
	var a, b int
	var ok bool
	switch ma {
	case 2:
		b, ok = frame.popInt()
		fallthrough
	case 1:
		a, ok = frame.popInt()
	default:
		return vm.errBadOp(op)
	}
	if !ok {
		return vm.errStackUnderflow()
	}
	switch ma {
	case 2:
		vm.stack.pushStr(v[a:b])
	case 1:
		vm.stack.pushByte(v[a])
	default:
		panic(vm.errBadOp(op))
	}
	return nil
}

func (vm *VM) popOpReflect(ot OpType, rt reflect.Type, frame stackFrameRef) (reflect.Value, error) {
	v, ok := frame.popOp(ot)
	if !ok {
		return reflect.Value{}, vm.errStackUnderflow()
	}
	return reflectConvert(reflect.ValueOf(v), rt)
}

func (vm *VM) checkReflectMethod(ld bool, f reflect.Value) (hasCtx, hasErr bool, err error) {
	const errPre = "%s cannot be used as a %s because "
	var fndir string
	ft := f.Type()
	inputs, outputs := ft.NumIn(), ft.NumOut()
	if ld {
		fndir = "getter"
		outputs--
	} else {
		fndir = "setter"
		inputs--
	}
	switch inputs {
	case 1:
		t := ft.In(0)
		hasCtx = t.AssignableTo(contextType)
		if !hasCtx {
			err = errors.Errorf(
				errPre+"first argument, %v, is not assignable to %v",
				f, fndir, t, contextType)
			return
		}
	case 0:
		// pass
	default:
		err = errors.Errorf3(
			errPre+"the only other argument must be a %v",
			f, fndir, contextType)
		return
	}
	switch outputs {
	case 1:
		t := ft.Out(outputs)
		hasErr = t.AssignableTo(errorType)
		if !hasErr {
			err = errors.Errorf(
				errPre+"last return value, %v, is not assignable to %v",
				f, fndir, t, contextType)
			return
		}
	case 0:
		// pass
	default:
		err = errors.Errorf3(
			errPre+"the only return value must be a %v",
			f, fndir, errorType)
		return
	}
	return
}

func (vm *VM) errBadOp(op OpCode) error {
	i, t, a := op.decode()
	return errors.Errorf3(
		"invalid opcode: %v(%v, %v)", i, t, a)
}

func (vm *VM) errBadType(op OpCode, expectPtr, actualPtr interface{}) error {
	expectTp := reflect.TypeOf(expectPtr).Elem()
	actual := reflect.ValueOf(actualPtr).Elem()
	return errors.ErrorfFrom(
		vm.errBadOp(op),
		"%v requires %v, not %v (type: %v)",
		op, expectTp, actual.Interface(), actual.Type())
}

func (vm *VM) errStackUnderflow() error {
	return errors.Errorf("stack underflow")
}

func reflectConvert(v reflect.Value, t reflect.Type) (reflect.Value, error) {
	if t == nil {
		return v, nil
	}
	if !v.Type().AssignableTo(t) {
		if !v.Type().ConvertibleTo(t) {
			return reflect.Value{}, errors.Errorf(
				"cannot convert %v (type %v) to type %v",
				v, v.Type(), t)
		}
		v = v.Convert(t)
	}
	return v, nil
}

// OpCode defines a VM operation code consisting of several bit fields.
// The OpCode implementation can change at any time, so don't depend
// on the internals.
//
// bits[0:5] encode the Instr.
// bits[5:8] encode the Type.
// bits[8:16] encode an 8-bit optional argument value.
type OpCode uint16

// EncodeOp encodes an op code
func EncodeOp(i Instr, t OpType, a OpArg) (op OpCode) {
	op.encode(i, t, a)
	return
}

func (c OpCode) decode() (i Instr, t OpType, a OpArg) {
	i = (Instr(c) >> opInstrShift) & opInstrMask
	t = (OpType(c) >> opTypeShift) & opTypeMask
	a = (OpArg(c) >> opArgShift) & opArgMask
	return
}

func (c *OpCode) encode(i Instr, t OpType, a OpArg) {
	*c = OpCode((i&opInstrMask)<<opInstrShift) |
		OpCode((t&opTypeMask)<<opTypeShift) |
		OpCode((a&opArgMask)<<opArgShift)
	return
}

func (c OpCode) retType() OpType {
	i, t, a := c.decode()
	switch {
	case i == Not:
		return Bool
	case Eq <= i && i <= Le:
		return Bool
	case i == Conv:
		return OpType(a)
	}
	return t
}

func (c OpCode) String() string {
	i, t, a := c.decode()
	args := [...]interface{}{i, t, nil}
	if i.isBinary() || i == Conv {
		args[2] = OpType(a)
	} else {
		args[2] = a
	}
	return fmt.Sprintf("%v(%v, %v)", args[:]...)
}

// Instr is a VM instruction.
type Instr uint16

func (i Instr) isBinary() bool {
	return Eq <= i && i <= Div
}

//go:generate stringer -type=Instr

const (
	// Nop is a "dummy" operation that does nothing.  It gets
	// inserted by the compiler during code generation when it
	// knows it needs to rewind and fix offsets or rewrite
	// instructions
	Nop Instr = iota

	// Not negates the top operand on the stack.
	//	x := pop()
	//	push(!x)
	Not

	// Eq checks if the top of the stack == the 2nd from top.
	//	b := pop()
	//	a := pop()
	//	push(a == b)
	Eq

	// Ne checks if the top of the stack != the 2nd from top.
	//	b := pop()
	//	a := pop()
	//	push(a != b)
	Ne

	// Gt checks if the top of the stack > the 2nd from top.
	//	b := pop()
	//	a := pop()
	//	push(a > b)
	Gt

	// Ge checks if the top of the stack >= the 2nd from top.
	//	b := pop()
	//	a := pop()
	//	push(a >= b)
	Ge

	// Lt checks if the top of the stack < the 2nd from top.
	//	b := pop()
	//	a := pop()
	//	push(a < b)
	Lt

	// Le checks if the top of the stack <= the 2nd from top.
	//	b := pop()
	//	a := pop()
	//	push(a <= b)
	Le

	// And checks if both operands at the top of the stack are true.
	//	b := pop()
	//	a := pop()
	//	push(a && b)
	And

	// Or checks if either of the operands at the top of the
	// stack are true.
	//	b := pop()
	//	a := pop()
	//	push(a || b)
	Or

	// Add evaluates top of the stack + 2nd from top
	//	b := pop()
	//	a := pop()
	//	push(a + b)
	Add

	// Sub evaluates top of the stack - 2nd from top
	//	b := pop()
	//	a := pop()
	//	push(a - b)
	Sub

	// Mul evaluates top of the stack * 2nd from top
	//	b := pop()
	//	a := pop()
	//	push(a * b)
	Mul

	// Div evaluates top of the stack / 2nd from top
	//	b := pop()
	//	a := pop()
	//	push(a / b)
	Div

	// LdConst loads a constant from the current function's
	// constant pool to the top of the stack
	//	x := consts[op.type][op.arg]
	//	push(x)
	LdConst

	// LdStack copies a value from somewhere in the stack relative
	// to the start of the current function frame.
	//	x := stack[frame.start+op.arg]
	//	push(x)
	//
	// It's supposed to be used to load function parameters,
	// but technically you could access values from elsewhere on
	// the stack.
	LdStack

	// StStack pops the top of the stack and writes it to another
	// location in the stack.
	//	x := pop()
	//	stack[frame.start+op.arg] = x
	//
	// It's supposed to be used to store function return parameters,
	// but technically you could access values from elsewhere on
	// the stack.
	StStack

	// LdMember accesses a member of a value.
	//	m := pop()
	//	x := pop()
	// If x is an array, map, slice, string:
	//	push(x[m])
	// Else if x is a struct or pointer to struct with an m field:
	//	push(x.m)
	// Else if x is a struct, pointer to struct, or interface:
	//	push(x.m())
	LdMember

	// StMember stores to a member of a value.
	//	v := pop()
	//	m := pop()
	//	x := pop()
	// If x is an array, map, slice, string:
	//	x[m] = v
	// Else if x is a struct or pointer to struct with an m field:
	//	x.m = v
	// Else if x is a struct, pointer to struct, or interface:
	//	x.Setm(v)
	StMember

	// LdVar loads a variable from the function's Values.  This should
	// only be used to load closed-over variables' values
	//	var := pop()
	//	push(vals.Get(var))
	LdVar

	// StVar stores a variable into the function's values.
	//	var := pop()
	//	value := pop()
	//	vals.Set(var, value)
	StVar

	// Call another function.
	//
	// Before the function is called, the caller does this:
	//	push(ret[n])
	//	...
	//	push(ret[0])
	//	push(arg[n])
	//	...
	//	push(arg[0])
	//	push(fn)
	// Execution of call then does:
	//	fn := pop()
	//	for i := 0; i < len(fn.args); i++ {
	//		args = append(args, pop())
	//	}
	//	ret[0], ..., ret[n] = fn(*args)
	Call

	// Conv converts the value at the top of the stack to a new type,
	// possibly moving it from one substack to another.
	Conv

	// Pack some values into a single Set
	Pack

	// Unpack some values onto the stack.
	Unpack

	// Ret returns execution to the caller after cleaning up the
	// callee's stack.  The callee's return values are left at the
	// top of the stack.
	Ret

	opInstrBits  = 5
	opInstrMask  = (1 << opInstrBits) - 1
	opInstrShift = 0
)

// OpType contains information about the operand's/operands' types.
type OpType uint16

//go:generate stringer -type=OpType

const (
	// Any is the default OpType to indicate that the type could
	// be anything.
	Any OpType = iota

	// Str indicates that the instruction acts upon a string.
	Str

	// Int64 operates on int64s
	Int64

	// Int operates on ints.
	Int

	// Bool operates on boolean instructions.
	Bool

	opTypeBits  = 3
	opTypeMask  = (1 << opTypeBits) - 1
	opTypeShift = opInstrShift + opInstrBits
)

var (
	reflectTypesByOp = [...]reflect.Type{
		reflect.TypeOf((*interface{})(nil)).Elem(),
		reflect.TypeOf(false),
		reflect.TypeOf(""),
		reflect.TypeOf(0),
		reflect.TypeOf(int64(0)),
	}

	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

/*
switch t {
case Any:
case Bool:
case Str:
case Int:
case Int64:
case default:
	return vm.errBadOp(op)
}
*/

// OpArg defines the argument bits of an OpCode.
type OpArg uint16

const (
	opArgBits  = /*sizeof(uint16)*/ 16 - opTypeBits - opTypeShift
	opArgMask  = (1 << opArgBits) - 1
	opArgShift = opTypeBits + opTypeShift
)

// OpCodes is a slice of op codes with some helper functions on it.
type OpCodes []OpCode

func (cs *OpCodes) append(ops ...OpCode) {
	*cs = append(*cs, ops...)
}

func (cs *OpCodes) top(i int) (OpCode, bool) {
	i = len(*cs) - i - 1
	if i < 0 || i >= len(*cs) {
		return 0, false
	}
	return (*cs)[i], true
}
