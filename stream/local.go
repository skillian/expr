package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/skillian/expr"
)

type localStreamer struct {
	source Streamer
	e      expr.Expr
}

type localFilterer localStreamer

func NewLocalFilterer(ctx context.Context, sr Streamer, e expr.Expr) (Streamer, error) {
	if srcFr, ok := sr.(localFilterer); ok {
		return localFilterer{
			source: srcFr.source,
			e:      expr.And{srcFr.e, e},
		}, nil
	}
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
	return &localFilter{
		localStream: localStream{
			source: source,
			e:      fr.e,
			f:      nil,
		},
		nexter: (*localFilter).initNext,
	}, nil
}

func (fr localFilterer) String() string {
	return fmt.Sprintf("%[1]T{source: %[2]v (type: %[2]T)}", fr, fr.source)
}

func (fr localFilterer) Var() expr.Var { return fr.source.Var() }

type localStream struct {
	source Stream
	e      expr.Expr
	f      expr.Func
}

type localFilter struct {
	localStream
	nexter func(*localFilter, context.Context) error
}

func (f *localFilter) Next(ctx context.Context) error { return f.nexter(f, ctx) }

func (f *localFilter) initNext(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	if err = f.source.Next(ctx); err != nil {
		return fmt.Errorf(
			"error from local filter %v source stream %v: %w",
			f, f.source, err,
		)
	}
	// compilation must happen here because only here
	// do the required variables have values of the proper
	// type.
	f.f, err = expr.FuncOfExpr(ctx, f.e, vs)
	if err != nil {
		return fmt.Errorf(
			"failed to compile local filter %v's filter "+
				"expression: %v: %w",
			f, f.e, err,
		)
	}
	f.nexter = (*localFilter).normalNext
	ok, err := f.evalFilter(ctx, vs)
	if ok || (err != nil) {
		return err
	}
	return f.nexter(f, ctx)
}

func (f *localFilter) normalNext(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	for {
		if err = f.source.Next(ctx); err != nil {
			return err
		}
		ok, err := f.evalFilter(ctx, vs)
		if ok || (err != nil) {
			return err
		}
	}
}

func (f *localFilter) evalFilter(ctx context.Context, vs expr.Values) (bool, error) {
	v, err := f.f.Call(ctx, vs)
	if err != nil {
		return false, fmt.Errorf(
			"error evaluating filter expression "+
				"%v: %w",
			f.e, err,
		)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf(
			"%w: filter expression must evaluate "+
				"to a boolean value, not "+
				"%#[2]v (type: %[2]T)",
			expr.ErrInvalidType, v,
		)
	}
	return b, nil
}

func (f *localFilter) String() string {
	return fmt.Sprintf("(%[1]T)(%[1]p){ %v }", f, f.e)
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
			"failed to create local mapping source stream: %w",
			err,
		)
	}
	return &localMap{
		localStream{source, mr.e, nil},
		mr.Var(),
		(*localMap).initNext,
	}, nil
}

func (mr *localMapper) Var() expr.Var { return mr }

type localMap struct {
	localStream
	va     expr.Var
	nexter func(*localMap, context.Context) error
}

func (m *localMap) Next(ctx context.Context) error { return m.nexter(m, ctx) }

func (m *localMap) initNext(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	if err := m.source.Next(ctx); err != nil {
		return fmt.Errorf(
			"failed to retrieve first value from local mapping %v: "+
				"source %v: %w",
			m, m.source, err,
		)
	}
	// compilation must happen here because only here
	// do the required variables have values of the proper
	// type.
	m.f, err = expr.FuncOfExpr(ctx, m.e, vs)
	if err != nil {
		return fmt.Errorf(
			"failed to compile map expression %v: %w",
			m.e, err,
		)
	}
	if err = m.evalMap(ctx, vs); err != nil {
		return err
	}
	m.nexter = (*localMap).normalNext
	return nil
}

func (m *localMap) normalNext(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	if err := m.source.Next(ctx); err != nil {
		return err
	}
	return m.evalMap(ctx, vs)
}

func (m *localMap) evalMap(ctx context.Context, vs expr.Values) error {
	v2, err := expr.Eval(ctx, m.e, vs)
	if err != nil {
		return fmt.Errorf(
			"error evaluating map %v: %w", m, err,
		)
	}
	return vs.Set(ctx, m.Var(), v2)
}

func (m *localMap) String() string {
	return fmt.Sprintf(
		"(%[1]T)(%[1]p){ %v }",
		m, m.e,
	)
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
	nexter  func(*localJoin, context.Context) error
	joiner  *localJoiner
	bigger  Stream
	smaller Stream
	when    expr.Func
	then    expr.Func
	va      expr.Var
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
		nexter:  (*localJoin).initNext,
		bigger:  bigger,
		smaller: smaller,
		va:      j,
		joiner:  j,
	}, nil
}

func (j localJoiner) Var() expr.Var { return j }

func (j *localJoin) Next(ctx context.Context) error {
	return j.nexter(j, ctx)
}

func (j *localJoin) initNext(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	if err = j.smaller.Next(ctx); err != nil {
		return fmt.Errorf("failed to get first element from smaller stream: %w", err)
	}
	if err = j.bigger.Next(ctx); err != nil {
		return fmt.Errorf("failed to get first element from bigger stream: %w", err)
	}
	// compilation must happen here because only here
	// do the required variables have values of the proper
	// type.
	what, e := "when", j.joiner.when
	if j.when, err = expr.FuncOfExpr(ctx, e, vs); err == nil {
		what, e = "then", j.joiner.then
		j.then, err = expr.FuncOfExpr(ctx, e, vs)
	}
	if err != nil {
		return fmt.Errorf(
			"failed to compile %v expression %v: %w",
			what, e, err,
		)
	}
	j.nexter = (*localJoin).normalNext
	ok, err := j.evalJoin(ctx, vs)
	if ok || (err != nil) {
		return err
	}
	return j.nexter(j, ctx)
}

func (j *localJoin) normalNext(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	// TODO: "Unrefactor" this back into
	//	if err = j.smaller.Next(ctx); err != nil { ... }
	resetAndFirstNext := func(
		ctx context.Context, j *localJoin, vs expr.Values,
		sr Streamer, st *Stream, whichStream string,
	) error {
		err := resetStream(ctx, *st)
		if errors.Is(err, expr.ErrInvalidType) {
			if err = closeStream(ctx, *st); err != nil {
				return fmt.Errorf(
					"failed to close %v.%v stream: %v: %w",
					j, whichStream, st, err,
				)
			}
			*st, err = sr.Stream(ctx)
		}
		if err != nil {
			return fmt.Errorf(
				"failed to reset/recreate %v.%v stream from %v: %w",
				j, whichStream, sr, err,
			)
		}
		if err = (*st).Next(ctx); err != nil {
			return fmt.Errorf(
				"failed to retrieve first "+
					"element from smaller "+
					"stream (%v) after "+
					"reset: %w",
				j.smaller, err,
			)
		}
		return nil
	}
	for {
		if err = j.smaller.Next(ctx); err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf(
					"error getting next element from %v smaller %v: %w",
					j, j.smaller, err,
				)
			}
			if err = resetAndFirstNext(
				ctx, j, vs,
				j.joiner.smaller, &j.smaller, "smaller",
			); err != nil {
				return err
			}
			if err = j.bigger.Next(ctx); err != nil {
				return err
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

func (j *localJoin) evalJoin(ctx context.Context, vs expr.Values) (bool, error) {
	res, err := j.when.Call(ctx, vs)
	if err != nil {
		return false, fmt.Errorf(
			"error evaluation %v when condition %v: %w",
			j, j.joiner.when, err,
		)
	}
	b, ok := res.(bool)
	if !ok {
		return false, fmt.Errorf(
			"join when condition result must be %T, not "+
				"%[2]v (type: %[2]T)",
			b, res,
		)
	}
	if !b {
		return false, nil
	}
	res, err = j.then.Call(ctx, vs)
	if err != nil {
		return false, fmt.Errorf(
			"error evaluating %v then condition %v: %w",
			j, j.joiner.then, err,
		)
	}
	return true, vs.Set(ctx, j.va, res)
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

// slice wraps a slice of anything.
type slice struct {
	rv reflect.Value
}

// FromSlice creates a streamer from a slice of anything.
func FromSlice(vs interface{}) Streamer {
	rv := reflect.ValueOf(vs)
	if rv.Kind() != reflect.Slice {
		panic(fmt.Sprintf(
			"%[1]v (type: %[1]T) is not a slice",
			vs,
		))
	}
	return &slice{rv}
}

var _ interface {
	Streamer
} = (*slice)(nil)

func (sl *slice) Stream(ctx context.Context) (Stream, error) {
	return &sliceStream{sl.rv, sl.Var()}, nil
}

func (sl *slice) Var() expr.Var { return sl }

type sliceStream struct {
	rv reflect.Value
	va expr.Var
}

var _ interface {
	Stream
} = (*sliceStream)(nil)

func (ss *sliceStream) Next(ctx context.Context) error {
	length := ss.rv.Len()
	if length == 0 {
		return io.EOF
	}
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	v := ss.rv.Index(0).Interface()
	ss.rv = ss.rv.Slice(1, length)
	return vs.Set(ctx, ss.va, v)
}

func (ss *sliceStream) Var() expr.Var { return ss.va }
