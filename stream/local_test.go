package stream_test

import (
	"context"
	"path"
	"reflect"
	"testing"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
	"github.com/skillian/expr/stream"
	"github.com/skillian/logging"
)

type streamTest struct {
	name    string
	factory func(ctx context.Context, t *testing.T) stream.Streamer
	expect  []interface{}
	errstr  string
}

var streamTests = []streamTest{
	{"filterMap", func(ctx context.Context, t *testing.T) stream.Streamer {
		t.Helper()
		sr := stream.Streamer(stream.FromSlice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}))
		sr = stream.Filter(ctx, sr, expr.Gt{sr.Var(), 5})
		return stream.Map(ctx, sr, expr.Mul{sr.Var(), 2})
	}, []interface{}{12, 14, 16, 18}, ""},
	{"filterMapJoin", func(ctx context.Context, t *testing.T) stream.Streamer {
		t.Helper()
		sr := stream.Streamer(stream.FromSlice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}))
		sr = stream.Filter(ctx, sr, expr.Gt{sr.Var(), 5})
		sr = stream.Map(ctx, sr, expr.Mul{sr.Var(), 2})
		sr2 := stream.Streamer(stream.FromSlice([]string{"a", "b"}))
		return stream.Join(ctx, sr, sr2, true, expr.Tuple{sr.Var(), sr2.Var()})
	}, []interface{}{
		expr.Tuple{12, "a"},
		expr.Tuple{12, "b"},
		expr.Tuple{14, "a"},
		expr.Tuple{14, "b"},
		expr.Tuple{16, "a"},
		expr.Tuple{16, "b"},
		expr.Tuple{18, "a"},
		expr.Tuple{18, "b"},
	}, ""},
}

func TestStream(t *testing.T) {
	//t.Parallel()
	for i := range streamTests {
		tc := &streamTests[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer logging.TestingHandler(
				logging.GetLogger(
					path.Join(expr.PkgName, "stream"),
					logging.LoggerTemporary(),
					logging.LoggerPropagate(true),
					logging.LoggerLevel(logging.EverythingLevel),
				),
				t, logging.HandlerFormatter(logging.GoFormatter{}),
			)()
			ctx := ctxutil.Background()
			vs := new([]interface{})
			*vs = make([]interface{}, 0, 8)
			if err := stream.Each(ctx, tc.factory(ctx, t), vs, func(
				ctx context.Context, _ stream.Stream, state, v interface{},
			) error {
				vs := state.(*[]interface{})
				*vs = append(*vs, v)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*vs, tc.expect) {
				t.Fatalf(
					"expected\n\t%#v\nactual:\n\t%#v",
					tc.expect, *vs,
				)
			}
		})
	}
}
