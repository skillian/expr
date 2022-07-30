package stream

import (
	"context"
	"fmt"
	"reflect"

	"github.com/skillian/ctxutil"
	"github.com/skillian/expr"
	"github.com/skillian/logging"
)

const (
	PkgName = "github.com/skillian/expr/stream"
)

var (
	logger = logging.GetLogger(PkgName)
)

type localFilterer[T any] struct {
	source Streamer[T]
	f      expr.Func1x1[T, bool]
}

func (fr localFilterer[T]) AnyStream(ctx context.Context) (AnyStream, error) { return fr.Stream(ctx) }
func (fr localFilterer[T]) Stream(ctx context.Context) (Stream[T], error) {
	st, err := fr.source.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return &localFilter[T]{
		source: st,
		f:      fr.f,
	}, nil
}

func (fr localFilterer[T]) AnyVar() expr.AnyVar { return fr.Var() }

func (fr localFilterer[T]) Var() expr.Var[T] { return fr.source.Var() }

type localFilter[T any] struct {
	source Stream[T]
	f      expr.Func1x1[T, bool]
}

func (lf *localFilter[T]) Next(ctx context.Context) (context.Context, error) {
	inCtx := ctx
	for {
		ctx, err := lf.source.Next(inCtx)
		if err != nil {
			return nil, err
		}
		v := ctxutil.Value(ctx, lf.Var())
		t, ok := v.(T)
		if !ok {
			return nil, expr.ErrorPexpectActual(&t, v)
		}
		res, err := expr.Call1x1(ctx, lf.f, expr.Const(t).AsExpr()).Eval(ctx)
		if err != nil {
			return nil, fmt.Errorf("error from filter %v: %w", lf.f, err)
		}
		if res {
			return ctx, nil
		}
	}
}

func (fr localFilter[T]) AnyVar() expr.AnyVar { return fr.Var() }
func (lf *localFilter[T]) Var() expr.Var[T]   { return lf.source.Var() }

type localMapper[TIn, TOut any] struct {
	source Streamer[TIn]
	f      expr.Func1x1[TIn, TOut]
}

func (mr *localMapper[TIn, TOut]) AnyStream(ctx context.Context) (AnyStream, error) {
	return mr.Stream(ctx)
}
func (mr *localMapper[TIn, TOut]) Stream(ctx context.Context) (Stream[TOut], error) {
	st, err := mr.source.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return localMap[TIn, TOut]{
		source: st,
		mr:     mr,
	}, nil
}
func (mr *localMapper[TIn, TOut]) AnyVar() expr.AnyVar { return mr.Var() }

func (mr *localMapper[TIn, TOut]) Var() expr.Var[TOut] {
	return (*localMapperVar[TIn, TOut])(mr)
}

type localMapperVar[TIn, TOut any] localMapper[TIn, TOut]

func (v *localMapperVar[TIn, TOut]) AsExpr() expr.Expr[TOut] { return v }
func (v *localMapperVar[TIn, TOut]) AnyVar() expr.AnyVar     { return v }
func (v *localMapperVar[TIn, TOut]) Eval(ctx context.Context) (TOut, error) {
	return expr.EvalVar[TOut](ctx, v)
}
func (v *localMapperVar[TIn, TOut]) EvalAny(ctx context.Context) (any, error) { return v.Eval(ctx) }
func (v *localMapperVar[TIn, TOut]) Var() expr.Var[TOut]                      { return v }
func (v *localMapperVar[TIn, TOut]) Kind() expr.Kind                          { return expr.VarKind }
func (v *localMapperVar[TIn, TOut]) Type() reflect.Type {
	var out TOut
	return reflect.TypeOf(&out).Elem()
}

type localMap[TIn, TOut any] struct {
	source Stream[TIn]
	mr     *localMapper[TIn, TOut]
}

func (lm localMap[TIn, TOut]) Next(ctx context.Context) (context.Context, error) {
	ctx, err := lm.source.Next(ctx)
	if err != nil {
		return nil, err
	}
	v := ctxutil.Value(ctx, lm.source.Var())
	t, ok := v.(TIn)
	if !ok {
		return nil, expr.ErrorPexpectActual(&t, v)
	}
	res, err := expr.Call1x1(ctx, lm.mr.f, expr.Const(t).AsExpr()).Eval(ctx)
	if err != nil {
		return nil, fmt.Errorf("error from mapping %v: %w", lm.mr.f, err)
	}
	return ctxutil.WithValue(ctx, lm.Var(), res), nil
}

func (lm localMap[TIn, TOut]) AnyVar() expr.AnyVar { return lm.Var() }
func (lm localMap[TIn, TOut]) Var() expr.Var[TOut] { return lm.mr.Var() }
