package errors

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
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

var getUintptr, putUintptr = func() (get func() []uintptr, put func([]uintptr)) {
	pool := sync.Pool{
		New: func() interface{} {
			return make([]uintptr, 0, 32)
		},
	}
	return func() []uintptr {
			return pool.Get().([]uintptr)
		}, func(ps []uintptr) {
			pool.Put(ps)
		}
}()

func (ps *pcs) put(skip int) {
	pcs := getUintptr()
	for {
		length, limit := len(pcs), cap(pcs)
		n := runtime.Callers(length+skip+1, ps.pcs[length:limit])
		pcs = pcs[:length+n]
		if len(pcs) == cap(pcs) {
			pcs = append(pcs, 0)[:limit]
			continue
		}
		break
	}
	ps.pcs = make([]uintptr, len(pcs))
	copy(ps.pcs, pcs)
	putUintptr(pcs[:0])
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
