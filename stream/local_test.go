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
	"github.com/skillian/logging"
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
	logger = logging.GetLogger(
		stream.PkgName,
		logging.LoggerLevel(logging.VerboseLevel),
	)

	intEq            = expr.DefaultEqer[int]()
	intAdder         = expr.DefaultAdder[int]()
	localStreamTests = []localStreamTest{
		{factory: func(ctx context.Context) stream.AnyStreamer {
			st := stream.SliceOf(0, 1, 2, 3, 4, 5, 6, 7, 8, 9).AsStreamer()
			st = must(stream.Map(ctx, st, expr.NewFunc1x1(func(v expr.Var[int]) expr.Expr[int] {
				return expr.Add(v.AsExpr(), v.AsExpr(), intAdder).AsExpr()
			})))
			return must(stream.Filter(ctx, st, expr.NewFunc1x1(func(v expr.Var[int]) expr.Expr[bool] {
				oneExpr := expr.Const(1).AsExpr()
				addExpr := expr.Add(v.AsExpr(), oneExpr, intAdder).AsExpr()
				threeExpr := expr.Const(3).AsExpr()
				return expr.Eq(addExpr, threeExpr)
			})))
		}, expect: []any{2}},
	}
)

func TestLocalStream(t *testing.T) {
	for _, tc := range localStreamTests {
		t.Run(fmt.Sprint(rand.Int()), func(t *testing.T) {
			defer logging.TestingHandler(
				logger, t,
				logging.HandlerFormatter(
					logging.DefaultFormatter{},
				),
				logging.HandlerLevel(
					logging.VerboseLevel,
				),
			)()
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
