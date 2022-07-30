package gen_test

import (
	"testing"

	"github.com/skillian/expr"
	"github.com/skillian/expr/gen"
)

func TestGoFuncGen(t *testing.T) {
	adder := expr.DefaultAdder[int]()
	f := expr.NewFunc1x1(func(v expr.Var[int]) expr.Expr[int] {
		return expr.Add(
			expr.Add(v.AsExpr(), expr.Const(2).AsExpr(), adder).AsExpr(),
			expr.Const(3).AsExpr(),
			adder,
		).AsExpr()
	})
	s, err := gen.GenGoFunc(f)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(s)
}
