package vm

import (
	"github.com/skillian/expr/errors"
)

// Values is a collection of VM values.
//
// In my previous VM implementations, the stack was essentially a
// wrapper around an []interface{} slice which would require all values
// be boxed into interface{}s and require some implicit conversions.
//
// This way can avoid some boxing and unboxing to/from interface{}s
// which can keep memory accesses more localized and potentially improve
// performance.
type Values struct {
	ints []int
	strs []string
	anys []interface{}
}

func (vs *Values) init(capacity int) {
	vs.ints = make([]int, 0, capacity)
	vs.strs = make([]string, 0, capacity)
	vs.anys = make([]interface{}, 0, capacity)
}

// MakeValues creates a collection of values.
func MakeValues(values ...interface{}) (vs Values) {
	for _, v := range values {
		_, _ = vs.addConst(v)
	}
	return
}

func (vs *Values) addConst(v interface{}) (t OpType, a OpArg) {
	switch v := v.(type) {
	case bool:
		t, a = Bool, OpArg(len(vs.ints))
		i := 0
		if v {
			i++
		}
		vs.pushInt(i)
	case int:
		t, a = Int, OpArg(len(vs.ints))
		vs.pushInt(v)
	// TODO:
	//case int64:
	case string:
		t, a = Str, OpArg(len(vs.strs))
		vs.pushStr(v)
	default:
		t, a = Any, OpArg(len(vs.anys))
		vs.pushAny(v)
	}
	return
}

func (vs *Values) pushByte(v byte) { vs.pushInt(int(v)) }
func (vs *Values) peekByte() (byte, bool) {
	i, ok := vs.peekInt()
	if !ok {
		return 0, false
	}
	return byte(i), true
}
func (vs *Values) popByte() (v byte, ok bool) {
	v, ok = vs.peekByte()
	if !ok {
		return
	}
	vs.ints = vs.ints[:len(vs.ints)-1]
	return
}

func (vs *Values) pushInt(v int) { vs.ints = append(vs.ints, v) }
func (vs *Values) peekInt() (v int, ok bool) {
	if len(vs.ints) == 0 {
		return
	}
	return vs.ints[len(vs.ints)-1], true
}
func (vs *Values) popInt() (v int, ok bool) {
	v, ok = vs.peekInt()
	if !ok {
		return
	}
	vs.ints = vs.ints[:len(vs.ints)-1]
	return
}

func (vs *Values) pushStr(v string) { vs.strs = append(vs.strs, v) }
func (vs *Values) peekStr() (v string, ok bool) {
	if len(vs.strs) == 0 {
		return
	}
	return vs.strs[len(vs.strs)-1], true
}
func (vs *Values) popStr() (v string, ok bool) {
	v, ok = vs.peekStr()
	if !ok {
		return
	}
	vs.strs = vs.strs[:len(vs.strs)-1]
	return
}

func (vs *Values) pushAny(v interface{}) { vs.anys = append(vs.anys, v) }
func (vs *Values) peekAny() (v interface{}, ok bool) {
	if len(vs.anys) == 0 {
		return
	}
	return vs.anys[len(vs.anys)-1], true
}
func (vs *Values) popAny() (v interface{}, ok bool) {
	v, ok = vs.peekAny()
	if !ok {
		return
	}
	vs.anys = vs.anys[:len(vs.anys)-1]
	return
}

// Stack is the execution stack of a virtual machine.  It bundles
// together both the values of the stack and the StackFrames of
// VM functions
type Stack struct {
	frames []StackFrame
	Values
}

func (s *Stack) init(capacity int) {
	s.frames = make([]StackFrame, 0, capacity)
	s.Values.init(capacity)
}

// StackFrame keeps track of each function's frame's ranges of
// values on the stack.  The first int in each pair is the offset
// into the Stack's field that this frame begins ownership and the
// second int in each pair is the number of elements after that start
// offset.
//
// For functions with return values, the start offsets can be slightly
// lower than the end of the previous frame because returns are stored
// directly into their right locations in the caller's stack.
type StackFrame struct {
	ints [2]int
	strs [2]int
	anys [2]int
}

// enterFrame creates a new  stack frame and returns its index.
// That index can be used as a parameter to the frame function to get
// temporary access to a StackFrame.  That StackFrame pointer is
// invalidated after calling enterFrame again because the underlying
// slice could have been extended.
func (s *Stack) enterFrame() int {
	i := len(s.frames)
	s.frames = append(s.frames, StackFrame{})
	if i > 0 {
		old, cur := &s.frames[i-1], &s.frames[i]
		cur.ints[0] = old.ints[0] + old.ints[1]
		cur.strs[0] = old.strs[0] + old.strs[1]
		cur.anys[0] = old.anys[0] + old.anys[1]
	}
	return i
}

type stackFrameRef struct {
	stack *Stack
	frame *StackFrame
}

// frame gets a temporary reference to a StackFrame.  The reference
// is no longer valid after calling either enterFrame or exitFrame.
func (s *Stack) frame(i int) stackFrameRef {
	return stackFrameRef{s, &s.frames[i]}
}

// exitFrame exits the last frame during a function return.
func (s *Stack) exitFrame(i int) {
	s.frames = s.frames[:len(s.frames)-1]
}

func (f stackFrameRef) pushBool(v bool) {
	i := 0
	if v {
		i++
	}
	f.pushInt(i)
}

func (f stackFrameRef) peekBool() (bool, bool) {
	i, ok := f.peekInt()
	if !ok {
		return false, false
	}
	return i != 0, true
}

func (f stackFrameRef) popBool() (bool, bool) {
	i, ok := f.popInt()
	if !ok {
		return false, false
	}
	return i != 0, true
}

func (f stackFrameRef) pushInt(v int) {
	f.stack.ints = append(f.stack.ints, v)
	f.frame.ints[1]++
}

func (f stackFrameRef) peekInt() (int, bool) {
	if f.frame.ints[1] == 0 {
		return 0, false
	}
	return f.stack.ints[f.frame.ints[0]+f.frame.ints[1]-1], true
}

func (f stackFrameRef) popInt() (int, bool) {
	v, ok := f.peekInt()
	if !ok {
		return 0, false
	}
	f.frame.ints[1]--
	f.stack.ints = f.stack.ints[:len(f.stack.ints)-1]
	return v, true
}

func (f stackFrameRef) pushStr(v string) {
	f.stack.strs = append(f.stack.strs, v)
	f.frame.strs[1]++
}

func (f stackFrameRef) peekStr() (string, bool) {
	if f.frame.strs[1] == 0 {
		return "", false
	}
	return f.stack.strs[f.frame.strs[0]+f.frame.strs[1]-1], true
}

func (f stackFrameRef) popStr() (string, bool) {
	v, ok := f.peekStr()
	if !ok {
		return "", false
	}
	f.frame.strs[1]--
	f.stack.strs = f.stack.strs[:len(f.stack.strs)-1]
	return v, true
}

func (f stackFrameRef) pushAny(v interface{}) {
	f.stack.anys = append(f.stack.anys, v)
	f.frame.anys[1]++
}

func (f stackFrameRef) peekAny() (interface{}, bool) {
	if f.frame.anys[1] == 0 {
		return nil, false
	}
	return f.stack.anys[f.frame.anys[0]+f.frame.anys[1]-1], true
}

func (f stackFrameRef) popAny() (interface{}, bool) {
	v, ok := f.peekAny()
	if !ok {
		return nil, false
	}
	f.frame.anys[1]--
	f.stack.anys = f.stack.anys[:len(f.stack.anys)-1]
	return v, true
}

func (f stackFrameRef) popOp(t OpType) (v interface{}, ok bool) {
	switch t {
	case Any:
		return f.popAny()
	case Bool:
		return f.popBool()
	case Str:
		return f.popStr()
	case Int:
		return f.popInt()
	case Int64:
		return f.popInt64()
	}
	panic(errors.Errorf1("unknown %[1]T: %[1]v", t))
}

func (f stackFrameRef) pushOp(t OpType, v interface{}) bool {
	switch t {
	case Any:
		f.pushAny(v)
		return true
	case Bool:
		b, ok := v.(bool)
		if !ok {
			return false
		}
		f.pushBool(b)
		return true
	case Str:
		s, ok := v.(string)
		if !ok {
			return false
		}
		f.pushStr(s)
		return true
	case Int:
		i, ok := v.(int)
		if !ok {
			return false
		}
		f.pushInt(i)
		return true
	case Int64:
		i64, ok := v.(int64)
		if !ok {
			return false
		}
		f.pushInt64(i64)
		return true
	}
	panic(errors.Errorf1("unknown %[1]T: %[1]v", t))
}

// pushOpZero pushes a zero value based on the OpType.
func (f stackFrameRef) pushOpZero(t OpType) {
	switch t {
	case Any:
		f.pushAny(nil)
	case Bool:
		f.pushBool(false)
	case Str:
		f.pushStr("")
	case Int:
		f.pushInt(0)
	case Int64:
		f.pushInt64(0)
	default:
		panic(errors.Errorf1("unknown %[1]T: %[1]v", t))
	}
	return
}

// pushReturns allocates space on the caller's frame for fn's return values.
func (f stackFrameRef) pushReturns(fn *Func) {
	for _, p := range fn.params {
		f.pushOpZero(p.OpType)
	}
}
