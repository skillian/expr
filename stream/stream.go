package stream

import (
	"context"
	"unsafe"

	"github.com/skillian/expr"
)

// Stream is a stream of values.  A stream might not be safe for
// concurrent use and might not be resetable.
type Stream[T any] interface {
	AnyStream

	// Var gets the variable that corresponds to the current value
	// associated with this stream.
	Var() expr.Var[T]
}

// AnyStream is a non-generic base implementation of Stream.
type AnyStream interface {
	Next(context.Context) (context.Context, error)
	AnyVar() expr.AnyVar
}

// Resetter might be implemented by some Stream implementations to
// reset themselves so that they can be reused.
type Resetter interface {
	Reset(context.Context) error
}

// Streamer is a source that can produce streams of values.  It is
// safe for concurrent use and multiple streams can be created from it
// and used concurrently.
type Streamer[T any] interface {
	AnyStreamer

	Stream(context.Context) (Stream[T], error)

	Var() expr.Var[T]
}

func AsStreamer[T any](st Streamer[T]) Streamer[T] { return st }

// AnyStreamer is a non-generic base implementation of Streamer
type AnyStreamer interface {
	AnyStream(context.Context) (AnyStream, error)
	AnyVar() expr.AnyVar
}

// Filterer takes in a stream and returns a new stream with values
// filtered out that don't match the given expression.
type Filterer[T any] interface {
	Streamer[T]

	// Filter filters sr and only keeps values that match the
	// expression e.
	Filter(ctx context.Context, e expr.Func1x1[T, bool]) (Streamer[T], error)
}

// Filter creates a new Streamer that only keeps values from sr that
// match the filter predicate, e.
func Filter[T any](ctx context.Context, sr Streamer[T], f expr.Func1x1[T, bool]) (Streamer[T], error) {
	if fr, ok := sr.(Filterer[T]); ok {
		return fr.Filter(ctx, f)
	}
	return localFilterer[T]{source: sr, f: f}, nil
}

// Mapper applies a projection to values from a stream to produce
// new values in a new stream.
type Mapper[TIn, TOut any] interface {
	Streamer[TIn]

	Map(ctx context.Context, e expr.Func1x1[TIn, TOut]) (Streamer[TOut], error)
}

// Map returns a Streamer that applies a projection, e, to sr's
// elements.
func Map[TIn, TOut any](ctx context.Context, sr Streamer[TIn], f expr.Func1x1[TIn, TOut]) (Streamer[TOut], error) {
	if mr, ok := sr.(Mapper[TIn, TOut]); ok {
		return mr.Map(ctx, f)
	}
	return &localMapper[TIn, TOut]{source: sr, f: f}, nil
}

func ifaceDataPtr(v interface{}) unsafe.Pointer {
	// interface is {type, data}
	d := *((*[2]unsafe.Pointer)(unsafe.Pointer(&v)))
	return d[1]
}
