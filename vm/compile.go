package vm

import (
	"math"
	"math/bits"

	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
)

// FuncOption configures a function
type FuncOption interface {
	Apply(c *funcCompiler) error
}

// FuncPreCompileOption configures a function before compilation
type FuncPreCompileOption func(c *funcCompiler) error

// Apply implements the FuncOption interface
func (f FuncPreCompileOption) Apply(c *funcCompiler) error { return f(c) }

// FuncPostCompileOption configures a function after compilation
type FuncPostCompileOption func(f *funcCompiler) error

// Apply implements the FuncOption interface
func (f FuncPostCompileOption) Apply(c *funcCompiler) error { return f(c) }

// FuncFromExpr creates a Func from the given expression.
//
// Any vars encountered in the
func FuncFromExpr(vars []expr.Var, e expr.Expr, options ...FuncOption) (*Func, error) {
	var fc funcCompiler
	fc.init(vars, e)
	for _, o := range options {
		if pre, ok := o.(FuncPreCompileOption); ok {
			if err := pre(&fc); err != nil {
				return nil, err
			}
		}
	}
	err := fc.handle(e)
	if err != nil {
		return nil, err
	}
	for _, o := range options {
		if pre, ok := o.(FuncPostCompileOption); ok {
			if err := pre(&fc); err != nil {
				return nil, err
			}
		}
	}
	ops := 0
	for _, sub := range fc.root.subs {
		ops += len(sub.code)
	}
	fc.fn.opCodes = make(OpCodes, 0, ops)
	for _, sub := range fc.root.subs {
		fc.fn.opCodes.append(sub.code...)
	}
	if op, ok := fc.fn.opCodes.top(0); ok {
		t := op.retType()
		if t != Any {
			fc.fn.opCodes.append(EncodeOp(Conv, t, OpArg(Any)))
		}
	}
	fc.fn.opCodes.append(
		EncodeOp(StStack, Any, 0),
		EncodeOp(Ret, Any, 0))
	return fc.fn, nil
}

// FuncName configures a function's name
func FuncName(name string) FuncOption {
	return FuncPreCompileOption(func(c *funcCompiler) error {
		c.fn.funcName = name
		return nil
	})
}

type funcCompiler struct {
	fn        *Func
	stack     funcCompExprStack
	root      funcCompExprTree
	free      []*funcCompExprTree
	exprstack []expr.Expr
	// paramops holds pre-build op codes for {load, store} each param
	paramops map[expr.Var]struct {
		OpType
		OpArg
	}
}

func (c *funcCompiler) init(vars []expr.Var, e expr.Expr) {
	const arbitraryCapacity = 8
	c.fn = &Func{
		opCodes: make(OpCodes, 0, arbitraryCapacity),
		consts:  MakeValues(),
		params:  make([]Var, len(vars)+1),
		vars:    append(make([]expr.Var, 0, len(vars)), vars...),
	}
	c.root.subs = make([]*funcCompExprTree, 0, arbitraryCapacity)
	c.stack = make([]*funcCompExprTree, 1, arbitraryCapacity)
	c.stack[0] = &c.root
	c.exprstack = make([]expr.Expr, 1, arbitraryCapacity)
	c.exprstack[0] = e
	c.paramops = make(map[expr.Var]struct {
		OpType
		OpArg
	})
	var anys, strs, ints int
	anys++ // return code
	for i, v := range vars {
		t := Any
		if va, ok := v.(*Var); ok {
			t = va.OpType
		}
		a := 0
		switch t {
		case Any:
			a = anys
			anys++
		case Str:
			a = strs
			strs++
		case Int64:
			// TODO:
			const is64bit = bits.UintSize == 64
			if is64bit {
				a = ints
				ints++
			} else {
				a = anys
				anys++
			}
		case Int:
			a = ints
			ints++
		case Bool:
			a = ints
			ints++
		}
		// +1 because we're skipping the return value's param
		c.fn.params[i+1].OpType = t
		c.fn.vars[i] = v
		logger.Verbose2("fn.vars[%d] = %p", i, v)
		c.paramops[v] = struct {
			OpType
			OpArg
		}{t, OpArg(a)}
	}
	c.fn.params = c.fn.params[:1]
	c.fn.rets.anys++
}

func (c *funcCompiler) handle(e expr.Expr) (err error) {
	tree, operands := c.enter(e)
	var op OpCode
	i, t, _ := op.decode()
	for _, operand := range operands {
		if err = c.handle(operand); err != nil {
			return
		}
	}
	switch e := e.(type) {
	case expr.Not:
		c.convertSubs(Bool)
		op = EncodeOp(Not, Bool, 0)
	case expr.Eq:
		i = Eq
	case expr.Ne:
		i = Ne
	case expr.Gt:
		i = Gt
	case expr.Ge:
		i = Ge
	case expr.Lt:
		i = Lt
	case expr.Le:
		i = Le
	case expr.Add:
		i = Add
	case expr.Sub:
		i = Sub
	case expr.Mul:
		i = Mul
	case expr.Div:
		i = Div
	case expr.And:
		i = And
	case expr.Or:
		i = Or
	case expr.Mem:
		i = LdMember
		sourceOp, ok := tree.subs[0].code.top(0)
		if !ok {
			return errors.Errorf0("invalid source")
		}
		memberOp, ok := tree.subs[1].code.top(0)
		if !ok {
			return errors.Errorf0("invalid member")
		}
		// mem needs the lhs to pop first:
		tree.subs[0], tree.subs[1] = tree.subs[1], tree.subs[0]
		op = EncodeOp(i, sourceOp.retType(), OpArg(memberOp.retType()))
	case expr.Set:
		i = Pack
		t = c.implicitConvertSubs()
		op = EncodeOp(i, t, OpArg(len(tree.subs)))
	default:
		_ = e
	}
	switch {
	case i.isBinary():
		t = c.implicitConvertSubs()
		op = EncodeOp(i, t, OpArg(t))
	case i == Nop:
		// assume we have a value?
		op = c.valueOp(e)
	}
	for _, sub := range tree.subs {
		tree.code.append(sub.code...)
	}
	tree.code.append(op)
	return c.exit(tree)
}

func (c *funcCompiler) valueOp(e expr.Expr) (op OpCode) {
	va, ok := e.(expr.Var)
	if ok {
		p, ok := c.paramops[va]
		if ok {
			op = EncodeOp(LdStack, p.OpType, p.OpArg)
		} else {
			// Otherwise, this is a closed-over variable:
			top := c.stack[len(c.stack)-1]
			t, a := c.fn.consts.addConst(va)
			top.code.append(EncodeOp(LdConst, t, a))
			t = Any
			if vmv, ok := va.(*Var); ok {
				t = vmv.OpType
			}
			op = EncodeOp(LdVar, t, 0)
		}
		return
	}
	t, a := c.fn.consts.addConst(e)
	return EncodeOp(LdConst, t, a)
}

// enter a frame, both appending that frame to the stack and to the old top
// of the stack's subs.
func (c *funcCompiler) enter(e expr.Expr) (t *funcCompExprTree, operands []expr.Expr) {
	top := c.stack[len(c.stack)-1]
	t = c.newTree()
	c.stack = append(c.stack, t)
	top.subs = append(top.subs, t)
	topexpr := len(c.exprstack)
	c.exprstack = appendOperands(c.exprstack, e)
	return t, c.exprstack[topexpr:]
}

// exit a frame but don't return it to the free store; the parent frame probably
// still wants it.
func (c *funcCompiler) exit(t *funcCompExprTree) error {
	if t != c.stack[len(c.stack)-1] {
		return errors.Errorf0(
			"cannot exit a frame that's not the top of the stack")
	}
	c.stack = c.stack[:len(c.stack)-1]
	return nil
}

func (c *funcCompiler) newTree() *funcCompExprTree {
	if len(c.free) == 0 {
		c.free = append(c.free, nil)
		c.free = c.free[0:cap(c.free)]
		frees := make([]funcCompExprTree, len(c.free))
		const arbitraryCap = 8
		freeptrs := make([]*funcCompExprTree, len(c.free)*arbitraryCap)
		for i := range c.free {
			f := &frees[i]
			start := i * arbitraryCap
			end := start + arbitraryCap
			f.subs = freeptrs[start:start:end]
			c.free[i] = f
		}
	}
	t := c.free[len(c.free)-1]
	c.free = c.free[:len(c.free)-1]
	return t
}

func (c *funcCompiler) freeTree(t *funcCompExprTree) {
	for _, sub := range t.subs {
		c.freeTree(sub)
	}
	t.subs = t.subs[:0]
	c.free = append(c.free, t)
}

func (c *funcCompiler) convertSubs(t OpType) {
	top := c.stack[len(c.stack)-1]
	for _, sub := range top.subs {
		op, ok := sub.code.top(0)
		if !ok {
			return
		}
		opt := op.retType()
		if opt != t {
			sub.code.append(EncodeOp(Conv, opt, OpArg(t)))
		}
	}
	return
}

func (c *funcCompiler) implicitConvertSubs() OpType {
	top := c.stack[len(c.stack)-1]
	// in ascending order
	minTypes := [2]OpType{math.MaxUint16, math.MaxUint16}
	for _, sub := range top.subs {
		last, ok := sub.code.top(0)
		if !ok {
			continue
		}
		t := last.retType()
		if t < minTypes[0] {
			minTypes[0], minTypes[1] = t, minTypes[0]
			continue
		}
		if t < minTypes[1] {
			minTypes[1] = t
		}
	}
	minType := minTypes[0]
	if minType == Any && minTypes[1] < math.MaxUint16 {
		minType = minTypes[1]
	}
	c.convertSubs(minType)
	return minType
}

type funcCompExprTree struct {
	//expr expr.Expr
	code OpCodes
	subs []*funcCompExprTree
}

type funcCompExprStack []*funcCompExprTree

func appendOperands(es []expr.Expr, e expr.Expr) []expr.Expr {
	switch e := e.(type) {
	case expr.Unary:
		return append(es, e.Operand())
	case expr.Binary:
		ops := e.Operands()
		return append(es, ops[:]...)
	case expr.Multary:
		return append(es, e.Operands()...)
	}
	return es
}
