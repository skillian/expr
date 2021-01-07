package stream

import (
	"context"
	"fmt"
	"io"

	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/vm"
)

// Slice is a Streamer that streams can be created from.
type Slice []interface{}

// Stream implements the Streamer interface.
func (s *Slice) Stream(ctx context.Context) (Stream, error) {
	return &sliceStream{s, 0}, nil
}

// Var implements the Streamer interface.
func (s *Slice) Var() expr.Var { return s }

type sliceStream struct {
	s *Slice
	i int
}

func (s *sliceStream) Next(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	s.i++
	if s.i > len(*s.s) {
		return io.EOF
	}
	vs.Set(s.s.Var(), (*s.s)[s.i-1])
	return nil
}

func (s *sliceStream) Var() expr.Var { return s.s.Var() }

type localStreamer struct {
	source Streamer
	fn     *vm.Func
	ex     expr.Expr
}

func (s *localStreamer) Var() expr.Var { return s }

type localFilterer struct {
	localStreamer
}

// NewLocalFilter creates a Filter implementation that executes locally.
func NewLocalFilter(s Streamer, e expr.Expr) (Streamer, error) {
	f, err := vm.FuncFromExpr([]expr.Var{s.Var()}, e, vm.FuncName(fmt.Sprintf("FilterOf(%v)", s)))
	if err != nil {
		return nil, err
	}
	return &localFilterer{localStreamer{
		source: s,
		fn:     f,
		ex:     e,
	}}, nil
}

func (f *localFilterer) Stream(ctx context.Context) (Stream, error) {
	s, err := f.source.Stream(ctx)
	if err != nil {
		return nil, err
	}
	lf := &localFilter{
		localFilterer: f,
		localStream: localStream{
			source: s,
		},
	}
	return lf, nil
}

type localStream struct {
	source Stream
}

type localFilter struct {
	*localFilterer
	localStream
}

func (f *localFilter) Next(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	for {
		if err := f.source.Next(ctx); err != nil {
			return errors.Errorf1From(
				err, "failed to get next from %v", f.source)
		}
		x, err := f.fn.Call(ctx, vs)
		if err != nil {
			return errors.Errorf1From(
				err, "error executing %v", f.fn)
		}
		b, ok := x.(bool)
		if !ok {
			return errors.Errorf2(
				"%[1]v was supposed to return a bool but "+
					"returned %[2]v (type: %[2]T) instead",
				f.fn, x)
		}
		if b {
			vs.Set(f.Var(), vs.Get(f.source.Var()))
			break
		}
	}
	return nil
}

// NewLocalMapper projects values from a Streamer into a new form.
func NewLocalMapper(s Streamer, e expr.Expr) (Streamer, error) {
	f, err := vm.FuncFromExpr([]expr.Var{s.Var()}, e, vm.FuncName(fmt.Sprintf("MapOf(%v)", s)))
	if err != nil {
		return nil, err
	}
	return &localMapper{localStreamer{
		source: s,
		fn:     f,
		ex:     e,
	}}, nil
}

type localMapper struct {
	localStreamer
}

func (m *localMapper) Stream(ctx context.Context) (Stream, error) {
	s, err := m.source.Stream(ctx)
	if err != nil {
		return nil, err
	}
	lm := &localMap{
		localMapper: m,
		source:      s,
	}
	return lm, nil
}

type localMap struct {
	*localMapper
	source Stream
}

func (m *localMap) Next(ctx context.Context) error {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
	if err = m.source.Next(ctx); err != nil {
		return err
	}
	x, err := m.fn.Call(ctx, vs)
	if err != nil {
		return errors.Errorf1From(err, "error executing %v", m.fn)
	}
	vs.Set(m.Var(), x)
	return nil
}

type localJoiner struct {
	left   Streamer
	right  Streamer
	whenFn *vm.Func
	thenFn *vm.Func
	whenEx expr.Expr
	thenEx expr.Expr
}

// NewLocalJoiner executes join operations locally.  This can be slow, so only
// join into small data sets locally.
func NewLocalJoiner(left, right Streamer, when, then expr.Expr) (Streamer, error) {
	var err error
	lj := &localJoiner{
		left:   left,
		right:  right,
		whenEx: when,
		thenEx: then,
	}
	lj.whenFn, err = vm.FuncFromExpr([]expr.Var{left.Var(), right.Var()}, when)
	if err != nil {
		return nil, err
	}
	lj.thenFn, err = vm.FuncFromExpr([]expr.Var{left.Var(), right.Var()}, then)
	if err != nil {
		return nil, err
	}
	return lj, nil
}

type localJoin struct {
	*localJoiner
	left   Stream
	leftOK bool
	right  Stream
}

func (j *localJoiner) Stream(ctx context.Context) (Stream, error) {
	left, err := j.left.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return &localJoin{
		localJoiner: j,
		left:        left,
		right:       nil,
	}, nil
}

func (j *localJoiner) Var() expr.Var { return j }

func (j *localJoin) Next(ctx context.Context) (errs error) {
	vs, err := expr.ValuesFromContext(ctx)
	if err != nil {
		return err
	}
outer:
	for {
		if !j.leftOK {
			logger.Verbose("getting next left from %v", j.left)
			if err = j.left.Next(ctx); err != nil {
				return err
			}
			if err = j.closeRight(); err != nil {
				return err
			}
			j.leftOK = true
		}
		for {
			if err = j.nextRight(ctx); err != nil {
				if IsEOF(err) {
					j.leftOK = false
					continue outer
				}
				return err
			}
			x, err := j.whenFn.Call(ctx, vs)
			if err != nil {
				return err
			}
			b, ok := x.(bool)
			if !ok {
				return errors.Errorf(
					"When expression must be %[1]T, not %[2]v "+
						"(type: %[2]T)", b, x)
			}
			if b {
				x, err := j.thenFn.Call(ctx, vs)
				if err != nil {
					return err
				}
				vs.Set(j.Var(), x)
				return nil
			}
		}
	}
}

func (j *localJoin) nextRight(ctx context.Context) (err error) {
	if j.right == nil {
		logger.Verbose("creating right stream from %v", j.localJoiner.right)
		if j.right, err = j.localJoiner.right.Stream(ctx); err != nil {
			return
		}
	}
	logger.Verbose("getting next right from %v", j.right)
	if err = j.right.Next(ctx); err != nil {
		if IsEOF(err) {
			logger.Verbose("right is eof")
			return errors.Aggregate(j.closeRight(), err)
		}
		return
	}
	return
}

func (j *localJoin) closeRight() (err error) {
	if j.right != nil {
		logger.Verbose("closing right %v", j.right)
		if err = closeStream(j.right); err != nil {
			return
		}
		j.right = nil
	}
	return
}
