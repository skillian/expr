package expr

import (
	"strings"
)

// Call a function.  The first parameter should be the function itself
// and the remaining expressions are the arguments.
type Call []Expr

func (x Call) Operands() []Expr { return ([]Expr)(x) }

func (x Call) String() string { return buildString(x) }

func (x Call) appendString(sb *strings.Builder) {
	sb.WriteByte('(')
	appendString(sb, x[0])
	for _, sub := range x[1:] {
		sb.WriteByte(' ')
		appendString(sb, sub)
	}
	sb.WriteByte(')')
}
