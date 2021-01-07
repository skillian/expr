package stream

import (
	"context"

	"github.com/skillian/expr"
)

// Line wraps around Streamer implementations to make the API more
// "streamlined"
type Line interface {
	Streamer
	Filter(e expr.Expr) (Line, expr.Var)
	Map(e expr.Expr) (Line, expr.Var)
	Join(other Streamer, when, then expr.Expr) (Line, expr.Var)
}

type line struct {
	source Streamer
	err    error
}

var _ Line = line{}

// LineOf creates a Line from a Streamer.
func LineOf(s Streamer) Line {
	return line{source: s}
}

func (li line) Filter(e expr.Expr) (Line, expr.Var) {
	if li.err != nil {
		return li, li.Var()
	}
	li.source, li.err = Filter(li.source, e)
	return li, li.Var()
}

func (li line) Join(other Streamer, when, then expr.Expr) (Line, expr.Var) {
	if li.err != nil {
		return li, li.Var()
	}
	li.source, li.err = Join(li.source, other, when, then)
	return li, li.Var()
}

func (li line) Map(e expr.Expr) (Line, expr.Var) {
	if li.err != nil {
		return li, li.Var()
	}
	li.source, li.err = Map(li.source, e)
	return li, li.Var()
}

func (li line) Stream(ctx context.Context) (Stream, error) {
	if li.err != nil {
		return nil, li.err
	}
	return li.source.Stream(ctx)
}

func (li line) Var() expr.Var { return li.source.Var() }
