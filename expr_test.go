package expr_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/skillian/ctxutil"
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

func TestMemOf(t *testing.T) {
	type S0 struct {
		I0 int
	}
	m := expr.MemOf(dummyVar, func(x *S0) *int { return &x.I0 })
	if m[1] != 0 {
		t.Fatalf("expected member 0 == 0, not %[1]#v (type: %[1]T)", m[1])
	}
	type S1 struct {
		I0 int64
		I1 int64
		S0 string
		S1 string
	}
	m = expr.MemOf(dummyVar, func(x *S1) *string { return &x.S0 })
	if m[1] != 2 {
		t.Fatalf("expected member 0 == 2, not %[1]#v (type: %[1]T)", m[1])
	}
	m = expr.MemOf(dummyVar, func(x *S1) *string { return &x.S1 })
	if m[1] != 3 {
		t.Fatalf("expected member 0 == 3, not %[1]#v (type: %[1]T)", m[1])
	}
}

var dummyVar expr.Var = dummyVarType{}

type dummyVarType struct{}

func (dummyVarType) Var() expr.Var { return dummyVar }

func TestValuesFromContext(t *testing.T) {
	ctx := context.Background()
	if _, err := expr.ValuesFromContext(ctx); !errors.Is(err, expr.ErrNotFound) {
		t.Fatalf(
			"expected no values in context, but got: "+
				"%[1]v (type: %[1]T)", err,
		)
	}
	vs := expr.NoValues()
	ctx = ctxutil.WithValue(ctx, expr.ValuesContextKey(), vs)
	vs2, err := expr.ValuesFromContext(ctx)
	if err != nil {
		t.Fatalf(
			"expected nil err, instead got: "+
				"%[1]v (type: %[1]T)",
			err,
		)
	}
	if vs2 != vs {
		t.Fatalf(
			"expected %[1]v (type: %[1]T). Actual: "+
				"%[2]v (type: %[2]T)",
			vs, vs2,
		)
	}
}
