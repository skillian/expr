package vm

func (vs *Values) getInt64(f *StackFrame, i int) int64 {
	off := 0
	if f != nil {
		off = f.ints[0] + f.ints[1]
	}
	return int64(vs.ints[off+i])
}

func (vs *Values) setInt64(f *StackFrame, i int, v int64) {
	off := 0
	if f != nil {
		off = f.ints[0] + f.ints[1]
	}
	vs.ints[off+i] = int(v)
	return
}

func (f stackFrameRef) pushInt64(v int64) {
	f.pushInt(int(v))
}

func (f stackFrameRef) peekInt64() (int64, bool) {
	i, ok := f.peekInt()
	if !ok {
		return 0, false
	}
	return int64(i), true
}

func (f stackFrameRef) popInt64() (int64, bool) {
	i, ok := f.popInt64()
	if !ok {
		return 0, false
	}
	return int64(i), true
}
