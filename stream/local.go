package stream

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/skillian/expr"
)

type localStreamer struct {
	source Streamer
	e      expr.Expr
}

type localFilterer localStreamer

func NewLocalFilterer(ctx context.Context, sr Streamer, e expr.Expr) (Streamer, error) {
	return localFilterer{sr, e}, nil
}

func (fr localFilterer) Stream(ctx context.Context) (Stream, error) {
	source, err := fr.source.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create local filter of "+
				"%[1]v (type: %[1]T): %[2]w",
			fr.source, err,
		)
	}
	return &localFilter{source, fr.e, nil}, nil
}

func (fr localFilterer) Var() expr.Var { return fr.source.Var() }

type localStream struct {
	source Stream
	e      expr.Expr
	f      expr.Func
}

type localFilter localStream

func (f *localFilter) Next(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	if f.f == nil {
		// compilation must happen here because only here
		// do the required variables have values of the proper
		// type.
		f.f, err = expr.FuncOf(ctx, f.e, vs)
		if err != nil {
			return fmt.Errorf(
				"failed to compile %v expression %v: %w",
				f, f.e, err,
			)
		}
	}
	for {
		if err := f.source.Next(ctx); err != nil {
			return fmt.Errorf("error from filter source: %w", err)
		}
		v, err := f.f.Call(ctx, vs)
		if err != nil {
			return fmt.Errorf(
				"error evaluating filter expression "+
					"%v: %w",
				f.e, err,
			)
		}
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf(
				"%w: filter expression must evaluate "+
					"to a boolean value, not "+
					"%#[2]v (type: %[2]T)",
				expr.ErrInvalidType, v,
			)
		}
		if b {
			return nil
		}
	}
}

func (f localFilter) Var() expr.Var { return f.source.Var() }

type localMapper localStreamer

func NewLocalMapper(ctx context.Context, sr Streamer, e expr.Expr) (Streamer, error) {
	return &localMapper{sr, e}, nil
}

func (mr *localMapper) Stream(ctx context.Context) (Stream, error) {
	source, err := mr.source.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create mapper source stream: %w",
			err,
		)
	}
	return &localMap{
		localStream{source, mr.e, nil},
		mr.Var(),
	}, nil
}

func (mr *localMapper) Var() expr.Var { return mr }

type localMap struct {
	localStream
	va expr.Var
}

func (m *localMap) Next(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to get values from context: %w", err,
		)
	}
	if m.f == nil {
		// compilation must happen here because only here
		// do the required variables have values of the proper
		// type.
		m.f, err = expr.FuncOf(ctx, m.e, vs)
		if err != nil {
			return fmt.Errorf(
				"failed to compile %v expression %v: %w",
				m, m.e, err,
			)
		}
	}
	if err := m.source.Next(ctx); err != nil {
		return fmt.Errorf(
			"local map source stream next: %w", err,
		)
	}
	v1, err := vs.Get(ctx, m.Var())
	if err != nil {
		return fmt.Errorf(
			"failed to get value from %#[1]v "+
				"(type: %[1]T): %w", m.source, err,
		)
	}
	v2, err := expr.Eval(ctx, m.e, vs)
	if err != nil {
		return fmt.Errorf(
			"error evaluating mapping of %v: %w", v1, err,
		)
	}
	return vs.Set(ctx, m.Var(), v2)
}

func (m localMap) Var() expr.Var { return m.va }

type localJoiner struct {
	bigger  Streamer
	smaller Streamer
	when    expr.Expr
	then    expr.Expr
}

// NewLocalJoiner performs a join operation within the current process.
func NewLocalJoiner(ctx context.Context, bigger, smaller Streamer, when, then expr.Expr) (Streamer, error) {
	return &localJoiner{bigger, smaller, when, then}, nil
}

type localJoin struct {
	bigger  Stream
	smaller Stream
	when    expr.Func
	then    expr.Func
	va      expr.Var
	joiner  *localJoiner
}

func (j *localJoiner) Stream(ctx context.Context) (Stream, error) {
	createStream := func(ctx context.Context, sr Streamer, name string) (Stream, error) {
		s, err := sr.Stream(ctx)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create %v join stream from "+
					"streamer %#[2]v (type: %[2]T): %w",
				name, sr, err,
			)
		}
		return s, nil
	}
	bigger, err := createStream(ctx, j.bigger, "bigger")
	if err != nil {
		return nil, err
	}
	smaller, err := createStream(ctx, j.smaller, "smaller")
	if err != nil {
		return nil, err
	}
	return &localJoin{
		bigger:  bigger,
		smaller: smaller,
		va:      j,
		joiner:  j,
	}, nil
}

func (j localJoiner) Var() expr.Var { return j }

func (j *localJoin) Next(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	if j.when == nil {
		// compilation must happen here because only here
		// do the required variables have values of the proper
		// type.
		what, e := "when", j.joiner.when
		if j.when, err = expr.FuncOf(ctx, j.joiner.when, vs); err == nil {
			what, e = "then", j.joiner.then
			j.then, err = expr.FuncOf(ctx, j.joiner.then, vs)
		}
		if err != nil {
			return fmt.Errorf(
				"failed to compile %v expression %v: %w",
				what, e, err,
			)
		}
		// this is our first call into Next, so also call Next
		// on the bigger stream:
		if nextErr := j.bigger.Next(ctx); nextErr != nil {
			return fmt.Errorf(
				"error calling %v.bigger's %v.Next "+
					"the first time: %w",
				j, j.bigger, nextErr,
			)
		}
	}
	for {
		if nextErr := j.smaller.Next(ctx); nextErr != nil {
			if !errors.Is(nextErr, io.EOF) {
				return fmt.Errorf(
					"error getting next element from %v smaller %v: %w",
					j, j.smaller, nextErr,
				)
			}
			resetErr := resetStream(ctx, j.smaller)
			if errors.Is(resetErr, expr.ErrInvalidType) {
				if resetErr = closeStream(ctx, j.smaller); resetErr != nil {
					return fmt.Errorf(
						"failed to close %v.smaller stream: %v: %w",
						j, j.smaller, resetErr,
					)
				}
				j.smaller, resetErr = j.joiner.smaller.Stream(ctx)
			}
			if resetErr != nil {
				return fmt.Errorf(
					"failed to reset/recreate smaller %v stream from %v: %w",
					j, j.joiner.smaller, resetErr,
				)
			}
			// also advance bigger stream because we just
			// exhausted the smaller stream:
			if nextErr = j.bigger.Next(ctx); nextErr != nil {
				return fmt.Errorf(
					"failed to advance %v.bigger: %v: %w",
					j, j.bigger, err,
				)
			}
		}
		v, err := j.when.Call(ctx, vs)
		if err != nil {
			return fmt.Errorf(
				"failed to evaluate %v condition: %v: %w",
				j, j.joiner.when, err,
			)
		}
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf(
				"%v condition must return %T, not "+
					"%#[3]v (type: %[3]T)",
				j, b, v,
			)
		}
		if !b {
			continue
		}
		v, err = j.then.Call(ctx, vs)
		if err != nil {
			return fmt.Errorf(
				"failed to evaluate %v projection: %v: %w",
				j, j.joiner.then, err,
			)
		}
		if err = vs.Set(ctx, j.Var(), v); err != nil {
			return fmt.Errorf(
				"failed to assign %v value: %#[2]v (type: %[2]T)",
				j, v,
			)
		}
		return nil
	}
}

func (j *localJoin) Var() expr.Var { return j.va }

func (j *localJoin) String() string {
	return fmt.Sprintf(
		"(%[1]T)(%[1]p){bigger: %v, smaller: %v, when: %v, "+
			"then: %v, va: %v, joiner: %p}",
		j, j.bigger, j.smaller, j.when, j.then, j.va, j.joiner,
	)
}

func closeStream(ctx context.Context, s Stream) error {
	switch s := s.(type) {
	case io.Closer:
		return s.Close()
	case interface{ Close(context.Context) error }:
		return s.Close(ctx)
	}
	return nil
}

func resetStream(ctx context.Context, s Stream) error {
	switch s := s.(type) {
	case interface{ Reset() error }:
		return s.Reset()
	case interface{ Reset(context.Context) error }:
		return s.Reset(ctx)
	}
	return expr.ErrInvalidType
}

type Slice[T any] []T

var _ interface {
	Streamer
} = (Slice[int])(nil)

func (sl Slice[T]) Stream(ctx context.Context) (Stream, error) {
	return &sliceStream[T]{sl[0:0:len(sl)]}, nil
}

func (sl Slice[T]) Var() expr.Var { return sliceVar{&sl[0], len(sl)} }

type sliceVar struct {
	v interface{}
	n int
}

func (v sliceVar) Var() expr.Var { return v }

type sliceStream[T any] struct {
	sl Slice[T]
}

var _ interface {
	Stream
} = (*sliceStream[int])(nil)

func (ss *sliceStream[T]) Next(ctx context.Context) error {
	if len(ss.sl) == cap(ss.sl) {
		return io.EOF
	}
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	ss.sl = ss.sl[:len(ss.sl)+1]
	return vs.Set(ctx, ss, ss.sl[len(ss.sl)-1])
}

func (ss *sliceStream[T]) Var() expr.Var { return ss.sl[:1].Var() }

func (ss *sliceStream[T]) String() string {
	return fmt.Sprintf(
		"(%[1]T)(%[1]p){%p, len: %d}",
		ss, &((ss.sl[:1])[0]), cap(ss.sl),
	)
}
