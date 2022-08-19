package stream

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
)

// Streamer is a source from which streams of values can be created.
// Stream may be called concurrently.
type Streamer interface {
	Stream(context.Context) (Stream, error)

	Var() expr.Var
}

type eachStateType[TState, TValue any] struct {
	sr    Streamer
	state TState
	f     func(ctx context.Context, s Stream, state TState, v TValue) error
}

// Each executes f on each element in the streamer.
func Each[TState, TValue any](ctx context.Context, sr Streamer, state TState, f func(ctx context.Context, s Stream, state TState, v TValue) error) error {
	return expr.WithEvalContext(ctx, eachStateType[TState, TValue]{sr, state, f}, func(
		ctx context.Context, state2 eachStateType[TState, TValue],
	) error {
		sr, state, f := state2.sr, state2.state, state2.f
		vs, ok := expr.ValuesFromContextOK(ctx)
		if !ok {
			vs = expr.NewValues()
			ctx = ctxutil.WithValue(ctx, expr.ValuesContextKey(), vs)
		}
		s, err := sr.Stream(ctx)
		if err != nil {
			return fmt.Errorf("failed to create stream from %v: %w", sr, err)
		}
		for {
			if err := s.Next(ctx); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return fmt.Errorf(
					"error from %v.Next: %w", s, err,
				)
			}
			v, err := vs.Get(ctx, s.Var())
			if err != nil {
				return fmt.Errorf(
					"failed to get value associated with stream %v: %w",
					s, err,
				)
			}
			v2, ok := v.(TValue)
			if !ok {
				return fmt.Errorf(
					"%w: expected %T, but actual: %#[2]v (type: %[2]T)",
					expr.ErrInvalidType, v2, v,
				)
			}
			if err = f(ctx, s, state, v2); err != nil {
				return err
			}
		}
	})
}

// Stream is a stream of values.  The first call to Next initializes
// the stream and subsequent calls advance the stream to the next value
// until Next returns an error that is EOF (i.e. errors.Is(err, io.EOF))
// returns true, but maybe err != io.EOF).
type Stream interface {
	Next(context.Context) error

	Var() expr.Var
}

// Filterer is implemented by Streamers that have their own
// implementation to filter out values based on expressions.
type Filterer interface {
	Filter(ctx context.Context, e expr.Expr) (Streamer, error)
}

// Filter creates a new Streamer that yields values from the source
// streamer only where the expression, e, evaluates to true.
func Filter(ctx context.Context, sr Streamer, e expr.Expr) (Streamer, error) {
	if fr, ok := sr.(Filterer); ok {
		return fr.Filter(ctx, e)
	}
	return NewLocalFilterer(ctx, sr, e)
}

// Mapper is implemented by Streamers that have their own
// implementations to project their elements to new forms.
type Mapper interface {
	Map(ctx context.Context, e expr.Expr) (Streamer, error)
}

// Map creates a new Streamer that projects the values from the first
// streamer into a new form.
func Map(ctx context.Context, sr Streamer, e expr.Expr) (Streamer, error) {
	if mr, ok := sr.(Mapper); ok {
		return mr.Map(ctx, e)
	}
	return NewLocalMapper(ctx, sr, e)
}

// Joiner is implemented by Streamers that have their own
// implementations to join into other streamers.  The joined-into
// Streamer should have fewer elements than the outer stream.  The
// when expression defines when values from both streams should be
// paired together and the then expression defines how they are paired.
type Joiner interface {
	Join(ctx context.Context, smaller Streamer, when, then expr.Expr) (Streamer, error)
}

// Join pairs together results from two streamers whenever the when
// expression evaluates to true.  The returned Streamer's elements are
// created from the then expression.
func Join(ctx context.Context, larger, smaller Streamer, when, then expr.Expr) (Streamer, error) {
	if jr, ok := larger.(Joiner); ok {
		return jr.Join(ctx, smaller, when, then)
	}
	return NewLocalJoiner(ctx, larger, smaller, when, then)
}
