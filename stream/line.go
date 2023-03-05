package stream

import (
	"context"

	"github.com/skillian/expr"
)

type Line struct{ Stager }

func LineOf(sr Streamer) Line { return Line{stager{sr}} }

func LineOf2(sr Streamer, err error) Line {
	if err != nil {
		return Line{errStager{err}}
	}
	return LineOf(sr)
}

func (l Line) Filter(ctx context.Context, e expr.Expr) Line {
	return Line{l.Stage(ctx, func(ctx context.Context, sr Streamer) (Streamer, error) {
		return Filter(ctx, sr, e)
	})}
}

func (l Line) Map(ctx context.Context, e expr.Expr) Line {
	return Line{l.Stage(ctx, func(ctx context.Context, sr Streamer) (Streamer, error) {
		return Map(ctx, sr, e)
	})}
}

func (l Line) Streamer() Streamer { return l.Stager }

type Stager interface {
	Streamer

	// Stage adds a stage to the pipeline
	Stage(context.Context, func(ctx context.Context, sr Streamer) (Streamer, error)) Stager
}

type errStager struct{ err error }

func (e errStager) Stage(ctx context.Context, f func(ctx context.Context, sr Streamer) (Streamer, error)) Stager {
	return e
}

func (e errStager) Stream(ctx context.Context) (Stream, error) {
	return nil, e.err
}

func (e errStager) Var() expr.Var { return e }

type stager struct {
	sr Streamer
}

func (p stager) Stage(ctx context.Context, f func(ctx context.Context, sr Streamer) (Streamer, error)) Stager {
	sr, err := f(ctx, p.sr)
	if err != nil {
		return errStager{err}
	}
	return stager{sr}
}

func (p stager) Stream(ctx context.Context) (Stream, error) {
	return p.sr.Stream(ctx)
}

func (p stager) Streamer() Streamer { return p.sr }

func (p stager) Var() expr.Var { return p.sr.Var() }
