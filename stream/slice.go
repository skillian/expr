package stream

import (
	"context"
	"io"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
)

type Slice[T any] []T

func SliceOf[T any](vs ...T) Slice[T] { return Slice[T](vs) }

func (sl Slice[T]) AsStreamer() Streamer[T] { return sl }

var _ interface {
	Streamer[int]
} = Slice[int]{}

func (sl Slice[T]) AnyStream(ctx context.Context) (AnyStream, error) { return sl.Stream(ctx) }
func (sl Slice[T]) Stream(context.Context) (Stream[T], error) {
	return &sliceStream[T]{slice: sl, v: sl.Var()}, nil
}

func (sl Slice[T]) AnyVar() expr.AnyVar { return sl.Var() }
func (sl Slice[T]) Var() expr.Var[T]    { return sliceVar[T]{&sl[0], len(sl)} }

type sliceVar[T any] struct {
	first  *T
	length int
}

func (v sliceVar[T]) AsExpr() expr.Expr[T] { return v }
func (v sliceVar[T]) AnyVar() expr.AnyVar  { return v.Var() }
func (v sliceVar[T]) Eval(ctx context.Context) (T, error) {
	return expr.EvalVar[T](ctx, v)
}
func (v sliceVar[T]) EvalAny(ctx context.Context) (any, error) { return v.Eval(ctx) }
func (v sliceVar[T]) Var() expr.Var[T]                         { return v }

type sliceStream[T any] struct {
	slice Slice[T]
	v     expr.Var[T]
	index int
}

var _ interface {
	Stream[int]
} = (*sliceStream[int])(nil)

func (st *sliceStream[T]) Next(ctx context.Context) (context.Context, error) {
	if st.index >= len(st.slice) {
		return nil, io.EOF
	}
	st.index++
	return ctxutil.WithValue(ctx, st.Var(), st.slice[st.index-1]), nil
}

func (st *sliceStream[T]) AnyVar() expr.AnyVar { return st.Var() }
func (st *sliceStream[T]) Var() expr.Var[T]    { return st.v }
