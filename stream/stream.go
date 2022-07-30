package stream

import (
	"context"
	"fmt"
	"reflect"

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

// StaticMapper maps from one form to another, but due to Go 1.18's
// implementation of generics, can only map to a single target type.
// To implement projections to different types, see the MapperBuilder
// type.
type StaticMapper[TIn, TOut any] interface {
	Streamer[TIn]

	Map(ctx context.Context, e expr.Func1x1[TIn, TOut]) (Streamer[TOut], error)
}

// MapperBuilder builds a Mapper into b with the given function
type MapperBuilder[TIn any] interface {
	BuildMapper(b Mapping, f expr.Func1xAny1[TIn]) error
}

type Mapping interface {
	SetMapper(sr AnyStreamer) error
}

type mapping[TOut any] struct {
	sr Streamer[TOut]
}

func (b *mapping[TOut]) SetMapper(sr AnyStreamer) error {
	srOut, ok := sr.(Streamer[TOut])
	if !ok {
		return fmt.Errorf("mapped streamer must be %v", reflect.TypeOf(&srOut).Elem())
	}
	b.sr = srOut
	return nil
}

// Map returns a Streamer that applies a projection, e, to sr's
// elements.
func Map[TIn, TOut any](ctx context.Context, sr Streamer[TIn], f expr.Func1x1[TIn, TOut]) (Streamer[TOut], error) {
	switch sr := sr.(type) {
	case StaticMapper[TIn, TOut]:
		return sr.Map(ctx, f)
	case MapperBuilder[TIn]:
		m := &mapping[TOut]{}
		if err := sr.BuildMapper(m, f); err != nil {
			var in TIn
			var out TOut
			return nil, fmt.Errorf(
				"failed to build mapper from %v to %v: %w",
				reflect.TypeOf(&in).Elem(),
				reflect.TypeOf(&out).Elem(),
				err,
			)
		}
		return m.sr, nil
	}
	return &localMapper[TIn, TOut]{source: sr, f: f}, nil
}

type Joiner[TOuter, TInner, TResult any] interface {
	Streamer[TOuter]

	Join(ctx context.Context)
}
