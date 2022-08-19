package stream_test

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
	"github.com/skillian/expr/stream"
)

type streamTest struct {
	factory func(ctx context.Context, t *testing.T) stream.Streamer
	expect  []interface{}
	errstr  string
}

var streamTests = []streamTest{
	{func(ctx context.Context, t *testing.T) stream.Streamer {
		t.Helper()
		sr := stream.Streamer(stream.Slice[int]{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
		sr = must(stream.Filter(ctx, sr, expr.Gt{sr.Var(), 5}))
		sr = must(stream.Map(ctx, sr, expr.Mul{sr.Var(), 2}))
		sr2 := stream.Streamer(stream.Slice[string]{"a", "b"})
		return must(stream.Join(ctx, sr, sr2, true, expr.Tuple{sr.Var(), sr2.Var()}))
	}, []interface{}{}, ""},
}

func TestStream(t *testing.T) {
	t.Parallel()
	for i := range streamTests {
		tc := &streamTests[i]
		t.Run(fmt.Sprint(rand.Int31()), func(t *testing.T) {
			t.Parallel()
			ctx := ctxutil.Background()
			vs := new([]any)
			*vs = make([]any, 0, 8)
			if err := stream.Each(ctx, tc.factory(ctx, t), vs, func(
				ctx context.Context, _ stream.Stream, vs *[]any, v any,
			) error {
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

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
