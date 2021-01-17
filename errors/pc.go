package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// callerRuntimeFunc gets the runtime.Func of the caller of the caller func.
func callerRuntimeFunc(skip int) (f *runtime.Func, ok bool) {
	pc, _, _, ok := runtime.Caller(skip + 1)
	if !ok {
		return nil, false
	}
	f = runtime.FuncForPC(pc)
	return f, f != nil
}

// CallerName gets the name of the function calling CallerName.
func CallerName(skip int) string {
	f, ok := callerRuntimeFunc(skip + 1)
	if !ok {
		logger.Warn("%v", Errorf0("unknown caller"))
		return ""
	}
	return f.Name()
}

type pcs struct {
	pcs []uintptr
}

func (ps *pcs) put(skip int) {
	ps.pcs = make([]uintptr, 0, 8)
	for {
		length, limit := len(ps.pcs), cap(ps.pcs)
		n := runtime.Callers(length+skip+1, ps.pcs[length:limit])
		ps.pcs = ps.pcs[:length+n]
		if len(ps.pcs) == cap(ps.pcs) {
			ps.pcs = append(ps.pcs, 0)[:limit]
			continue
		}
		break
	}
}

func (ps *pcs) Error() string {
	frames := runtime.CallersFrames(ps.pcs)
	strs := make([]string, len(ps.pcs))
	for i := 0; i < len(strs); i++ {
		frame, _ := frames.Next()
		strs[i] = ps.formatFrame(frame)
	}
	return strings.Join(strs, "")
}

func (ps *pcs) formatFrame(f runtime.Frame) string {
	return fmt.Sprintf(
		"\n%s\n\t%s:%d +%x",
		f.Function, f.File, f.Line, f.PC-f.Entry)
}
