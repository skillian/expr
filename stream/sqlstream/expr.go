package sqlstream

import "github.com/skillian/expr"

// In is a SQL-specific expression.
type In struct {
	V   expr.Expr
	Set expr.Set
}

// Operands gets the In expression's operands and implements the expr.Multary
// interface.
func (in In) Operands() []expr.Expr {
	return append(
		append(
			make([]expr.Expr, 0, len(in.Set)+1),
			in.V),
		in.Set...)
}
