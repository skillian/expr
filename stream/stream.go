package stream

import (
	"context"
	"io"

	"github.com/skillian/expr"
	"github.com/skillian/expr/errors"
)

// Streamer creates streams.  This way, streams can be described and then
// compiled into some efficient form for repeated reuse.  Streamers must be
// safe for concurrent use, preferably immutable.  It is most common to create
// pipelines of streamers from other streamers.  The same root streamer could
// be used by multiple other streamers at the same time, so you should not
// mutate root streamers.
type Streamer interface {
	// Stream creates a stream of values from the Streamer.
	Stream(ctx context.Context) (Stream, error)

	// Var represents the result of the streamer for each pull from the
	// stream.  For a Map stream, it is the result of the mapping.
	// For a Filter stream, it's the value that passed through the stream.
	Var() expr.Var
}

// Stream is an actual stream of values created from a Streamer.
type Stream interface {
	// Next advances the stream to its next element
	Next(ctx context.Context) error

	// Var gets the var of this stream's result.  The var should be the same
	// one returned by the the stream's Streamer's Var function.
	Var() expr.Var
}

// closeStream closes the stream if the stream implements io.Closer.
func closeStream(s Stream) error {
	cr, ok := s.(io.Closer)
	if !ok {
		return nil
	}
	return cr.Close()
}

// IsEOF returns true if err is an EOF or if it contains an EOF
func IsEOF(err error) bool {
	return errors.Any(err, func(e error) bool {
		return e == io.EOF
	})
}

// Filterer is a Streamer that can filter a source streamer's results so that
// only results matching the filter come through.
type Filterer interface {
	Streamer

	// Filter a source streamer to only return values where expression e
	// evaluates to true.
	Filter(e expr.Expr) (Streamer, error)
}

// Filter to keep values from a source stream that match expression e.
func Filter(s Streamer, e expr.Expr) (Streamer, error) {
	var err error
	if fr, ok := s.(Filterer); ok {
		s, err = fr.Filter(e)
	} else {
		s, err = NewLocalFilter(s, e)
	}
	if err != nil {
		return nil, errors.Errorf1From(
			err, "failed to create filter from stream %v", s)
	}
	return s, nil
}

// Mapper applies a projection to the source stream to transform its elements
// into another form.
type Mapper interface {
	Streamer

	// Map applies expression e to each value from the source streamer
	// and returns a new streamer with the results.
	Map(e expr.Expr) (Streamer, error)
}

// Map applies a projection to elements of a stream.
func Map(s Streamer, e expr.Expr) (Streamer, error) {
	var err error
	if mr, ok := s.(Mapper); ok {
		s, err = mr.Map(e)
	} else {
		s, err = NewLocalMapper(s, e)
	}
	if err != nil {
		return nil, errors.Errorf2From(
			err, "failed to create mapper from %v with "+
				"expression %v", s, e)
	}
	return s, nil
}

// JoinOption configures a join operation
type JoinOption func(j Joiner) error

// Joiner can join between two streams.
type Joiner interface {
	Streamer

	// Join a smaller stream and yield the result of the then expression
	// whenever the when expression evaluates to true.
	Join(smaller Streamer, when, then expr.Expr, options ...JoinOption) (Streamer, error)
}

// Join a larger stream to a smaller stream whenever the when expression
// evaluates the elements from both streams to true.
func Join(larger, smaller Streamer, when, then expr.Expr) (Streamer, error) {
	if jr, ok := larger.(Joiner); ok {
		return jr.Join(smaller, when, then)
	}
	return NewLocalJoiner(larger, smaller, when, then)
}
