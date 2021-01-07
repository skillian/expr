package stream

import (
	"context"
	"io"

	"github.com/skillian/expr/errors"
	"github.com/skillian/logging"
)

var logger = logging.GetLogger("expr/stream")

// Each executes f on the stream created from s until that stream returns
// io.EOF.
func Each(ctx context.Context, s Streamer, f func(s Stream) error) error {
	st, err := s.Stream(ctx)
	if err != nil {
		return errors.Errorf1From(
			err, "failed to create stream from %v", s)
	}
	for {
		if err = st.Next(ctx); err != nil {
			if errors.Any(err, func(e error) bool {
				return e == io.EOF
			}) {
				return nil
			}
			return err
		}
		if err = f(st); err != nil {
			return err
		}
	}
}
