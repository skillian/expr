package expr

import (
	"io"
)

type lang interface {
	// AssociateLeft checks if the given expression is left-associative
	// in order to know when parentheses must be used.
	AssociateLeft(e Expr) bool

	// Precedence returns an absolute precedence value for the given
	// expression so that formatting knows when to add parentheses.
	Precedence(e Expr) int

	Format(w io.Writer, e Expr, next func(io.Writer, Expr) error) error
}
