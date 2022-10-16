package expr_test

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/skillian/expr"
)

type evalTest struct {
	e       expr.Expr
	varVals []expr.VarValue
	expect  interface{}
	err     string
}

var evalTests = []evalTest{
	{expr.Not{true}, nil, false, ""},
	{expr.Eq{1, 1}, nil, true, ""},
	{expr.Eq{big.NewRat(1, 1), big.NewRat(2, 1)}, nil, false, ""},
	{expr.Eq{false, false}, nil, true, ""},
	{expr.Eq{0.125, 0.125}, nil, true, ""},
	{expr.Eq{struct{}{}, struct{}{}}, nil, true, ""},
	{expr.Ne{1, 1}, nil, false, ""},
	{expr.Ne{0.125, 0.25}, nil, true, ""},
	{expr.Eq{"hello", "world"}, nil, false, ""},
	{expr.Eq{"hello", "hello"}, nil, true, ""},
	{expr.Ne{"hello", "world"}, nil, true, ""},
	{expr.Gt{"a", "a"}, nil, false, ""},
	{expr.Gt{"a", "b"}, nil, false, ""},
	{expr.Gt{"b", "a"}, nil, true, ""},
	{expr.Gt{1, 1}, nil, false, ""},
	{expr.Ge{"a", "a"}, nil, true, ""},
	{expr.Lt{"a", "a"}, nil, false, ""},
	{expr.Lt{"a", "b"}, nil, true, ""},
	{expr.Lt{"b", "a"}, nil, false, ""},
	{expr.Lt{big.NewRat(1, 1), big.NewRat(2, 1)}, nil, true, ""},
	{expr.Le{"a", "a"}, nil, true, ""},
	{expr.And{false, false}, nil, false, ""},
	{expr.And{false, true}, nil, false, ""},
	{expr.And{true, false}, nil, false, ""},
	{expr.And{true, true}, nil, true, ""},
	{expr.Or{false, false}, nil, false, ""},
	{expr.Or{false, true}, nil, true, ""},
	{expr.Or{true, false}, nil, true, ""},
	{expr.Or{true, true}, nil, true, ""},
	{expr.Add{1, 1}, nil, 2, ""},
	{expr.Add{1, -1}, nil, 0, ""},
	{expr.Add{0.625, 1.5}, nil, 2.125, ""},
	{expr.Sub{1, 1}, nil, 0, ""},
	{expr.Sub{1, -1}, nil, 2, ""},
	{expr.Sub{1.5, 0.625}, nil, 0.875, ""},
	{expr.Mul{1, 1}, nil, 1, ""},
	{expr.Mul{1, -1}, nil, -1, ""},
	{expr.Mul{10, 10}, nil, 100, ""},
	{expr.Mul{1.5, 0.625}, nil, 0.9375, ""},
	{expr.Div{2, 1}, nil, 2, ""},
	{expr.Div{2, 2}, nil, 1, ""},
	{expr.Div{4, 2}, nil, 2, ""},
	{expr.Div{22, 7}, nil, 3, ""},
	{expr.Div{1.5, 0.625}, nil, 2.4, ""},

	// multi-step
	{expr.Add{expr.Mul{3, 2}, 1}, nil, 7, ""},
	{expr.Sub{expr.Mul{3, 2}, 1}, nil, 5, ""},

	// coalescing types:
	{expr.Add{big.NewRat(2, 1), 3}, nil, big.NewRat(5, 1), ""},
	{expr.Add{3, big.NewRat(2, 1)}, nil, big.NewRat(5, 1), ""},
	{expr.Add{expr.Add{3, 7}, big.NewRat(2, 1)}, nil, big.NewRat(12, 1), ""},
	{expr.Mul{3, big.NewRat(2, 1)}, nil, big.NewRat(6, 1), ""},
	{expr.Mul{big.NewRat(2, 1), 3}, nil, big.NewRat(6, 1), ""},
	{expr.Mul{2, expr.Sub{big.NewRat(3, 1), big.NewRat(1, 1)}}, nil, big.NewRat(4, 1), ""},
	{expr.Div{big.NewRat(1, 1), 1}, nil, big.NewRat(1, 1), ""},

	// member access:
	{expr.Mem{map[string]string{"hello": "world"}, "hello"}, nil, "world", ""},
	{expr.Mem{&(struct{ A string }{A: "helloWorld"}), 0}, nil, "helloWorld", ""},
	{expr.Mem{expr.Mem{&TestOuter{Inner: TestInner{S: "asdf"}}, 0}, 0}, nil, "asdf", ""},
	{expr.Mem{expr.Mem{&TestOuter2{Inner: &TestInner{S: "asdf"}}, 0}, 0}, nil, "asdf", ""},

	// variables:
	func() evalTest {
		v1, v2 := newVar(), newVar()
		return evalTest{
			e: expr.Add{v1, v2},
			varVals: []expr.VarValue{
				{Var: v1, Value: 123},
				{Var: v2, Value: 456},
			},
			expect: 579,
		}
	}(),

	// Tuples
	{expr.Eq{expr.Tuple{1, 2}, expr.Tuple{1, 2}}, nil, true, ""},

	// negative tests:
	// {expr.Not{2}, nil, false, "invalid type"},
}

func TestEval(t *testing.T) {
	ctx := expr.WithFuncCache(context.Background())
	for i := 0; i < 2; i++ {
		// run tests twice to test function caching
		for _, tc := range evalTests {
			t.Run(fmt.Sprint(tc.e), func(t *testing.T) {
				vs := expr.NoValues()
				if len(tc.varVals) > 0 {
					vs = expr.NewValues(tc.varVals...)
				}
				res, err := expr.Eval(ctx, tc.e, vs)
				handleErr(t, err, tc.err)
				if !eq(res, tc.expect) {
					t.Fatalf(
						"expected:\n\t%[1]v (type: %[1]T)\nbut actual value:\n\t%[2]v (type: %[2]T)",
						tc.expect, res,
					)
				}
			})
		}
	}
}

func eq(a, b interface{}) bool {
	if a == b {
		return true
	}
	if ar, ok := a.(*big.Rat); ok {
		if br, ok := b.(*big.Rat); ok {
			return ar.Cmp(br) == 0
		}
	}
	return false
}

func handleErr(t *testing.T, err error, str string) {
	t.Helper()
	if err == nil {
		if str != "" {
			t.Fatalf("expected error %q, but there was no error", str)
		}
		return
	}
	if str == "" {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), str) {
		t.SkipNow()
	}
	t.Fatalf("expected error %q but got error %v", str, err)
}

type TestOuter struct {
	Inner TestInner
}

type TestOuter2 struct {
	Inner *TestInner
}

type TestInner struct {
	S string
}

type variable struct {
	va expr.Var
}

func newVar() expr.Var {
	va := &variable{}
	ev := expr.Var(va)
	va.va = ev
	return ev
}

func (va *variable) Var() expr.Var { return va.va }
