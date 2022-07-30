package gen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/skillian/expr"
)

func GenGoFunc(f expr.AnyFunc) (string, error) {
	g := goFuncGen{}
	g.initVars(f.AnyBody())
	if err := g.genFuncInto(f); err != nil {
		return "", err
	}
	return g.sb.String(), nil
}

type goFuncGen struct {
	sb     strings.Builder
	varmap map[expr.AnyVar]int
	vars   []expr.AnyVar
}

func (g *goFuncGen) initVars(e expr.AnyExpr) {
	g.varmap = make(map[expr.AnyVar]int, 8)
	g.vars = make([]expr.AnyVar, 0, 8)
	_ = expr.Walk(e, func(e expr.AnyExpr) bool {
		va, ok := e.(expr.AnyVar)
		if !ok {
			return true
		}
		if _, ok := g.varmap[va]; ok {
			return true
		}
		g.varmap[va] = len(g.vars)
		g.vars = append(g.vars, va)
		return true
	})
}

func (g *goFuncGen) genFuncInto(f expr.AnyFunc) error {
	g.sb.WriteString("func(")
	if len(g.vars) > 0 {
		g.sb.WriteString("v0 ")
		g.sb.WriteString(fmt.Sprint(g.vars[0].Type()))
	}
	for i := 1; i < len(g.vars); i++ {
		g.sb.WriteString(", v")
		g.sb.WriteString(strconv.FormatInt(int64(i), 10))
		g.sb.WriteByte(' ')
		g.sb.WriteString(fmt.Sprint(g.vars[i].Type()))
	}
	g.sb.WriteString(") (")
	g.sb.WriteString(fmt.Sprint(f.AnyBody().Type()))
	g.sb.WriteString(", error) { return ")
	g.genExpr(f.AnyBody())
	g.sb.WriteString(" }")
	return nil
}

func (g *goFuncGen) genExpr(e expr.AnyExpr) {
	type exprTmpl struct {
		e expr.AnyExpr
		// ops    []*exprTmpl
		prefix string
		infix  string
		suffix string
	}
	stack := make([]exprTmpl, 1, 8)
	// stack[0] = &exprTmpl{}
	_ = expr.Walk(e, func(e expr.AnyExpr) bool {
		// top := stack[len(stack)-1]
		if e != nil {
			et := exprTmpl{e: e}
			switch e.(type) {
			case expr.NotExpr:
				et.prefix = "!"
			default:
				switch e.Kind() {
				case expr.EqKind:
					et.infix = "=="
				case expr.NeKind:
					et.infix = "!="
				case expr.GtKind:
					et.infix = ">"
				case expr.GeKind:
					et.infix = ">="
				case expr.LtKind:
					et.infix = "<"
				case expr.LeKind:
					et.infix = "<="
				case expr.AndKind:
					et.infix = "&&"
				case expr.OrKind:
					et.infix = "||"
				case expr.AddKind:
					et.infix = "+"
				case expr.CallKind:
					et.infix = ", "
				}
			}
			// top.ops = append(top.ops, et)
			stack = append(stack, et)
			return true
		}
		et := stack[len(stack)-1]
		e = et.e
		stack = stack[:len(stack)-1]
		et2 := stack[len(stack)-1]
		if et2.infix != "" {
			// Check to see if we are the 2nd or later
			// operand.  If so, the first operand has been
			// written and we have to write the infix before
			// we write the next operand.
			writeInfix := false
			switch e2e := et2.e.(type) {
			case expr.AnyBinaryExpr:
				writeInfix = e2e.AnyOperands()[0] != e
			}
			if writeInfix {
				g.sb.WriteString(et2.infix)
			}
		}
		g.sb.WriteString(et2.prefix)
		switch e := e.(type) {
		case expr.AnyConstExpr:
			g.sb.WriteString(fmt.Sprintf("%#v", e.AnyValue()))
		case expr.AnyVar:
			g.sb.WriteString(fmt.Sprintf("v%d", g.varmap[e]))
		}
		g.sb.WriteString(et2.suffix)
		return true
	})
}
