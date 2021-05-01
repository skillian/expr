package vm

func (vs *Values) getInt64(f *StackFrame, i int) int64 {
	off := 0
	if f != nil {
		off = f.anys[0] + f.anys[1]
	}
	return vs.anys[off+i].(int64)
}

func (vs *Values) setInt64(f *StackFrame, i int, v int64) {
	off := 0
	if f != nil {
		off = f.anys[0] + f.anys[1]
	}
	vs.anys[off+i] = v
	return
}

func (f stackFrameRef) pushInt64(v int64) { f.pushAny(v) }

func (f stackFrameRef) peekInt64() (i64 int64, ok bool) {
	v, ok := f.peekAny()
	if !ok {
		return 0, false
	}
	i64, ok = v.(int64)
	return
}

func (f stackFrameRef) popInt64() (int64, bool) {
	v, ok := f.peekInt64()
	if !ok {
		return 0, false
	}
	f.frame.anys[1]--
	f.stack.anys = f.stack.anys[:len(f.stack.anys)-1]
	return v, true
}
