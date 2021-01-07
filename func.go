package expr

import (
	"context"
)

// Func is a function.  It could be implemented by native Go code or the result
// of calling Compile on an expression.
type Func interface {
	Call(ctx context.Context, vs Values) (interface{}, error)
}
