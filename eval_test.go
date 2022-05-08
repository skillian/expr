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
	e      expr.Expr
	expect interface{}
	err    string
}

var evalTests = []evalTest{
	{expr.Not{true}, false, ""},
	{expr.Eq{1, 1}, true, ""},
	{expr.Eq{big.NewRat(1, 1), big.NewRat(2, 1)}, false, ""},
	{expr.Eq{false, false}, true, ""},
	{expr.Eq{struct{}{}, struct{}{}}, true, ""},
	{expr.Ne{1, 1}, false, ""},
	{expr.Eq{"hello", "world"}, false, ""},
	{expr.Eq{"hello", "hello"}, true, ""},
	{expr.Ne{"hello", "world"}, true, ""},
	{expr.Gt{"a", "a"}, false, ""},
	{expr.Gt{"a", "b"}, false, ""},
	{expr.Gt{"b", "a"}, true, ""},
	{expr.Gt{1, 1}, false, ""},
	{expr.Ge{"a", "a"}, true, ""},
	{expr.Lt{"a", "a"}, false, ""},
	{expr.Lt{"a", "b"}, true, ""},
	{expr.Lt{"b", "a"}, false, ""},
	{expr.Lt{big.NewRat(1, 1), big.NewRat(2, 1)}, true, ""},
	{expr.Le{"a", "a"}, true, ""},
	{expr.And{false, false}, false, ""},
	{expr.And{false, true}, false, ""},
	{expr.And{true, false}, false, ""},
	{expr.And{true, true}, true, ""},
	{expr.Or{false, false}, false, ""},
	{expr.Or{false, true}, true, ""},
	{expr.Or{true, false}, true, ""},
	{expr.Or{true, true}, true, ""},
	{expr.Add{1, 1}, 2, ""},
	{expr.Add{1, -1}, 0, ""},
	{expr.Sub{1, 1}, 0, ""},
	{expr.Sub{1, -1}, 2, ""},
	{expr.Mul{1, 1}, 1, ""},
	{expr.Mul{1, -1}, -1, ""},
	{expr.Mul{10, 10}, 100, ""},
	{expr.Div{2, 1}, 2, ""},
	{expr.Div{2, 2}, 1, ""},
	{expr.Div{4, 2}, 2, ""},
	{expr.Div{22, 7}, 3, ""},

	// multi-step
	{expr.Add{expr.Mul{3, 2}, 1}, 7, ""},
	{expr.Sub{expr.Mul{3, 2}, 1}, 5, ""},

	// coalescing types:
	{expr.Add{big.NewRat(2, 1), 3}, big.NewRat(5, 1), ""},
	{expr.Add{3, big.NewRat(2, 1)}, big.NewRat(5, 1), ""},
	{expr.Add{expr.Add{3, 7}, big.NewRat(2, 1)}, big.NewRat(12, 1), ""},
	{expr.Mul{3, big.NewRat(2, 1)}, big.NewRat(6, 1), ""},
	{expr.Mul{big.NewRat(2, 1), 3}, big.NewRat(6, 1), ""},
	{expr.Mul{2, expr.Sub{big.NewRat(3, 1), big.NewRat(1, 1)}}, big.NewRat(4, 1), ""},
	{expr.Div{big.NewRat(1, 1), 1}, big.NewRat(1, 1), ""},

	// member access:
	{expr.Mem{map[string]string{"hello": "world"}, "hello"}, "world", ""},
	{expr.Mem{&(struct{ A string }{A: "helloWorld"}), 0}, "helloWorld", ""},
	// // {expr.Mem{expr.Mem{&TestOuter{Inner: TestInner{S: "asdf"}}, 0}, 0}, "asdf", ""},

	// negative tests:
	{expr.Not{2}, false, "invalid type"},
}

func TestEval(t *testing.T) {
	ctx := context.Background()
	for _, tc := range evalTests {
		t.Run(fmt.Sprint(tc.e), func(t *testing.T) {
			res, err := expr.Eval(ctx, tc.e)
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

type TestInner struct {
	S string
}
