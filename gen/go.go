package gen

import (
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"

	"github.com/skillian/expr"
)

func GenGoFunc1x1[T0, TResult any](f expr.Func1x1[T0, TResult]) (string, error) {
	sb := strings.Builder{}
	sb.WriteString("func f")
	sb.WriteString(strconv.FormatUint(rand.Uint64(), 16))
	sb.WriteString("(v0 ")
	varmap := map[expr.AnyVar]string{
		f.V0: "v0",
	}
	var v0 T0
	sb.WriteString(fmt.Sprint(reflect.TypeOf(&v0).Elem()))
	sb.WriteString(") (")
	var r0 TResult
	sb.WriteString(fmt.Sprint(reflect.TypeOf(&r0).Elem()))
	sb.WriteString(", error) { return ")
	sb.WriteString()
}

func genGoExpr(varmap map[expr.AnyVar]string, e expr.AnyExpr, sb *strings.Builder) {
	type exprTmpl struct {
		e      expr.AnyExpr
		prefix string
		infix  string
		suffix string
	}
	stack := make([]exprTmpl, 1, 8)
	_ = expr.Walk(e, func(e expr.AnyExpr) bool {
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
					et.prefix = 
					et.infix = ", "
				}
			}
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
				sb.WriteString(et2.infix)
			}
		}
		sb.WriteString(et2.prefix)
		switch e := e.(type) {
		case expr.AnyConstExpr:
			sb.WriteString(fmt.Sprintf("%#v", e.AnyValue()))
		case expr.AnyVar:
			sb.WriteString(varmap[e])
		}
	})
}
