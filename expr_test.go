package expr_test

import (
	"fmt"
	"testing"

	"github.com/skillian/expr"
)

type exprKind uint8

const (
	invalidKind exprKind = iota
	unaryKind
	binaryKind
)

func (k exprKind) String() string {
	return ([...]string{"invalid", "unary", "binary"})[int(k)]
}

func exprKindOf(e expr.Expr) exprKind {
	switch e.(type) {
	case expr.Unary:
		return unaryKind
	case expr.Binary:
		return binaryKind
	}
	return invalidKind
}

type exprKindTest struct {
	e expr.Expr
	k exprKind
}

var exprKindTests = []exprKindTest{
	{expr.Not{}, unaryKind},
	{expr.Eq{}, binaryKind},
	{expr.Ne{}, binaryKind},
	{expr.Gt{}, binaryKind},
	{expr.Ge{}, binaryKind},
	{expr.Lt{}, binaryKind},
	{expr.Le{}, binaryKind},
	{expr.And{}, binaryKind},
	{expr.Or{}, binaryKind},
	{expr.Add{}, binaryKind},
	{expr.Sub{}, binaryKind},
	{expr.Mul{}, binaryKind},
	{expr.Div{}, binaryKind},
	{expr.Mem{}, binaryKind},
}

func TestExprKinds(t *testing.T) {
	for _, tc := range exprKindTests {
		t.Run(fmt.Sprintf("%T", tc.e), func(t *testing.T) {
			k := exprKindOf(tc.e)
			if k != tc.k {
				t.Fatalf(
					"expected %T == %v. Actual: %v",
					tc.e, tc.k, k,
				)
			}
		})
	}
}
