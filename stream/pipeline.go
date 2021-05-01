package stream

import (
	"context"

	"github.com/skillian/expr"
)

// Line wraps around Streamer implementations to make the API more
// "streamlined"
type Line interface {
	Streamer
	Filter(e expr.Expr) Line
	Map(e expr.Expr) Line
	Join(other Streamer, when, then expr.Expr) Line
}

type line struct {
	source Streamer
	err    error
}

var _ Line = line{}

// LineOf creates a Line from a Streamer.
func LineOf(s Streamer, fs ...func(Line) Line) Line {
	var ln Line = line{source: s}
	for _, f := range fs {
		ln = f(ln)
	}
	return ln
}

func (li line) Filter(e expr.Expr) Line {
	if li.err != nil {
		return li
	}
	li.source, li.err = Filter(li.source, e)
	return li
}

func (li line) Join(other Streamer, when, then expr.Expr) Line {
	if li.err != nil {
		return li
	}
	li.source, li.err = Join(li.source, other, when, then)
	return li
}

func (li line) Map(e expr.Expr) Line {
	if li.err != nil {
		return li
	}
	li.source, li.err = Map(li.source, e)
	return li
}

func (li line) Stream(ctx context.Context) (Stream, error) {
	if li.err != nil {
		return nil, li.err
	}
	return li.source.Stream(ctx)
}

func (li line) Var() expr.Var { return li.source.Var() }
