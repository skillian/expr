package vm

import (
	"context"
	"math/bits"

	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
)

// Func is a function implemented for the VM.  If Go ever provides a way to
// emit functions at runtime, the VM will probably go away.
type Func struct {
	// OpCodes are the op codes of the function that the VM executes.
	opCodes OpCodes

	// Consts is a per-function constant pool.
	consts Values

	// Params holds return value types in Params[:len(Params)] and
	// arguments in Params[len(Params):cap(Params)]
	params []Var

	// Vars are the variables from the original expression and are the
	// keys into the expr.Values passed in as the function's parameters.
	// They have to be mapped to the function's Params.
	vars []expr.Var

	// rets tracks the number of each type of parameter in its returns.
	rets struct {
		anys int
		strs int
		ints int
	}

	funcName string
}

// NewFunc creates a new function with the given data
func NewFunc(vars []Var, body OpCodes, rets []Var, options ...FuncOption) (*Func, error) {
	var fc funcCompiler
	ps := append(make([]Var, 0, len(rets)+len(vars)), rets...)
	copy(ps[len(rets):cap(ps)], vars)
	vs := make([]expr.Var, len(vars))
	for i := range vars {
		vs[i] = &vars[i]
	}
	fc.fn = &Func{
		opCodes: append(make(OpCodes, 0, len(body)), body...),
		params:  ps,
		vars:    vs,
	}
	for _, r := range rets {
		switch r.OpType {
		case Any:
			fc.fn.rets.anys++
		case Str:
			fc.fn.rets.strs++
		case Int64:
			if bits.UintSize == 64 {
				fc.fn.rets.ints++
			} else {
				fc.fn.rets.anys++
			}
		case Int:
			fc.fn.rets.ints++
		case Bool:
			fc.fn.rets.ints++
		}
	}
	for _, opt := range options {
		if err := opt.Apply(&fc); err != nil {
			return nil, err
		}
	}
	return fc.fn, nil
}

// MustNewFunc constructs a func and panics if construction fails
func MustNewFunc(vars []Var, body OpCodes, rets []Var, options ...FuncOption) *Func {
	f, err := NewFunc(vars, body, rets, options...)
	if err != nil {
		panic(err)
	}
	return f
}

// FuncConsts is an option to NewFunc to specify a function's constants
func FuncConsts(vs Values) FuncPostCompileOption {
	return func(fc *funcCompiler) error {
		if fc.fn.consts.anys != nil ||
			fc.fn.consts.strs != nil ||
			fc.fn.consts.ints != nil {
			return errors.Errorf0("cannot replace function contsts")
		}
		fc.fn.consts = vs
		return nil
	}
}

// Call implements the stream.Func interface.
func (f *Func) Call(ctx context.Context, vs expr.Values) (interface{}, error) {
	vm, ok := FromContext(ctx)
	if !ok {
		return nil, errors.Errorf0("Context has no VM")
	}
	nargs := cap(f.params) - len(f.params)
	if vs.Len() < nargs {
		return nil, errors.Errorf3(
			"Incorrect argument count to %v.  Expected at least "+
				"%d, got %d",
			f, nargs, vs.Len())
	}
	index := vm.stack.enterFrame()
	frame := vm.stack.frame(index)
	defer vm.stack.exitFrame(index)
	frame.pushReturns(f)
	params := f.params[len(f.params):cap(f.params)]
	for i := range params {
		p := &params[i]
		va := f.vars[i]
		v := vs.Get(va)
		logger.Verbose4("Call: %#v = (f.vars[%d] (%p) = %p)", p, i, va, v)
		if !frame.pushOp(p.OpType, v) {
			return nil, errors.Errorf3(
				"failed to push %[1]s parameter %[2]d value "+
					"%[3]v (type: %[3]T)",
				p.OpType, i, v)
		}
	}
	if err := vm.execFunc(ctx, f, vs); err != nil {
		return nil, err
	}
	logger.Verbose1("\t\t\treturned: %[1]v (type %[1]T)", vm.stack.anys[0])
	switch len(f.params) {
	case 0:
		return nil, nil
	case 1:
		x, ok := frame.popOp(f.params[0].OpType)
		if !ok {
			return nil, vm.errStackUnderflow()
		}
		return x, nil
	}
	xs := make([]interface{}, len(f.params))
	for i, p := range f.params {
		var ok bool
		xs[len(f.params)-i-1], ok = frame.popOp(p.OpType)
		if !ok {
			return nil, vm.errStackUnderflow()
		}
	}
	return xs, nil
}

// Name of the function
func (f *Func) Name() string { return f.funcName }
