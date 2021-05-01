package stream

import (
	"context"
	"io"

	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
	"github.com/skillian/logging"
)

var logger = logging.GetLogger("expr/stream")

// JustNext can be passed to Each or Single to do nothing.  This is useful
// if the stream's Var is bound to a model and you just need to call Next.
func JustNext(s Stream) error { return nil }

// Each executes f on the stream created from s until that stream returns
// io.EOF.
func Each(ctx context.Context, s Streamer, f func(s Stream) error) (Err error) {
	st, err := s.Stream(ctx)
	if err != nil {
		return errors.Errorf1From(
			err, "failed to create stream from %v", s)
	}
	defer errors.Catch(&Err, func() error { return closeStream(st) })
	for {
		if err = st.Next(ctx); err != nil {
			if errors.Any(err, func(e error) bool {
				return e == io.EOF
			}) {
				return nil
			}
			return err
		}
		if err = f(st); err != nil {
			return err
		}
	}
}

// ValuesOf pulls values out of the stream and calls f on each of them.
func ValuesOf(ctx context.Context, s Streamer, f func(v interface{}) error) error {
	ctx, vs := expr.ValuesFromContextOrNew(ctx)
	va := s.Var()
	return Each(ctx, s, func(_ Stream) error {
		return f(vs.Get(va))
	})
}

// Single expects a single result from the streamer and returns an error if
// more than one is returned.
func Single(ctx context.Context, s Streamer, f func(s Stream) error) error {
	first := false
	return Each(ctx, s, func(s Stream) error {
		if first {
			return errors.Errorf1(
				"more than one result from %v", s)
		}
		first = true
		return f(s)
	})
}
