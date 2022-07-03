package expr_test

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
)

type exprTest struct {
	e      expr.AnyExpr
	expect any
	errstr string
}

var intAdder = expr.Adder[int, int, int](expr.AdderFunc[int, int, int](func(ctx context.Context, a, b int) (int, error) {
	return a + b, nil
}))

var exprTests = []exprTest{
	{e: expr.Add(
		expr.Const(1),
		expr.Const(2),
		intAdder,
	), expect: 3},
}

func TestExpr(t *testing.T) {
	ctx := ctxutil.WithValue(
		ctxutil.Background(),
		(*expr.Adder[int, int, int])(nil),
		intAdder,
	)
	for _, tc := range exprTests {
		t.Run(fmt.Sprint(rand.Int()), func(t *testing.T) {
			res, err := tc.e.EvalAny(ctx)
			if err != nil {
				if tc.errstr != "" {
					if strings.Contains(err.Error(), tc.errstr) {
						t.SkipNow()
					}
					t.Fatalf(
						"expected error containing %q, but actual error: %v",
						tc.errstr, err,
					)
				}
				t.Fatal(err)
			}
			if res != tc.expect {
				t.Fatalf(
					"%[1]v\n\texpect:\t%[2]v (type: %[2]T)\n\t"+
						"actual:\t%[3]v (type: %[3]T)",
					tc.e, tc.expect, res,
				)
			}
		})
	}
}
