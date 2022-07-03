package stream_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
	"github.com/skillian/expr/stream"
)

type localStreamTest struct {
	factory func(context.Context) stream.AnyStreamer
	expect  []any
	errstr  string
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

var (
	intEq = expr.MakeEqer(func(ctx context.Context, a, b int) (bool, error) {
		return a == b, nil
	})
	intAdder = expr.MakeAdder(func(ctx context.Context, a, b int) (int, error) {
		return a + b, nil
	})
	localStreamTests = []localStreamTest{
		{factory: func(ctx context.Context) stream.AnyStreamer {
			st := stream.SliceOf(0, 1, 2, 3, 4, 5, 6, 7, 8, 9).AsStreamer()
			return must(stream.Filter(ctx, st, expr.NewFunc1x1(func(v expr.Var[int]) expr.Expr[bool] {
				return expr.Eq(
					expr.Add(
						v.AsExpr(),
						expr.Const(1).AsExpr(),
						intAdder,
					).AsExpr(),
					expr.Const(2).AsExpr(),
				)
			})))
		}, expect: []any{1}},
	}
)

func TestLocalStream(t *testing.T) {
	for _, tc := range localStreamTests {
		t.Run(fmt.Sprint(rand.Int()), func(t *testing.T) {
			startCtx := ctxutil.Flatten(
				ctxutil.WithValue(
					ctxutil.WithValue(
						ctxutil.Background(),
						(*expr.Eqer[int])(nil),
						intEq,
					),
					(*expr.Adder[int, int, int])(nil),
					intAdder,
				),
			)
			st, err := tc.factory(startCtx).AnyStream(startCtx)
			if err != nil {
				t.Fatal(err)
			}
			vs := make([]interface{}, 0, 8)
			for {
				ctx, err := st.Next(startCtx)
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				v := ctxutil.Value(ctx, st.AnyVar())
				vs = append(vs, v)
			}
			if len(tc.expect) != len(vs) {
				t.Fatal("expected", len(tc.expect), "actually got", len(vs), "values")
			}
			for i, v := range vs {
				if tc.expect[i] != v {
					t.Fatal("expected index", i, "(", v, ") =", tc.expect[i])
				}
			}
		})
	}
}
