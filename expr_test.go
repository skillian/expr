package expr_test

import (
	"context"
	"errors"
	"testing"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
	"github.com/skillian/logging"
)

var (
	logger = logging.GetLogger(
		expr.PkgName,
		logging.LoggerLevel(logging.EverythingLevel),
	)
)

func TestMemOf(t *testing.T) {
	ctx := ctxutil.Background()
	type S0 struct {
		I0 int
	}
	s0 := &S0{I0: 123}
	vs := expr.NewValues(expr.VarValue{dummyVar, s0})
	ctx = expr.AddValuesToContext(ctx, vs)
	m := expr.MemOf(dummyVar, s0, &s0.I0)
	res, err := expr.Eval(ctx, m, vs)
	if err != nil {
		t.Fatal(err)
	}
	if res != 123 {
		t.Fatal("expected", m, "==", 123, "but got:", res)
	}
	type S1 struct {
		I0 int64
		I1 int64
		S0 string
		S1 string
	}
	const helloWorld = "hello, world!"
	s1 := &S1{S0: helloWorld}
	if err = vs.Set(ctx, dummyVar, s1); err != nil {
		t.Fatal(err)
	}
	m = expr.MemOf(dummyVar, s1, &s1.S0)
	res, err = expr.Eval(ctx, m, vs)
	if err != nil {
		t.Fatal(err)
	}
	if res != helloWorld {
		t.Fatalf("expected member 0 == 2, not %[1]#v (type: %[1]T)", m[1])
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
