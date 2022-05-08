package expr

import (
	"fmt"
	"strings"
)

type Expr interface{}

type Unary interface {
	Operand() Expr
}

type Not [1]Expr

func (x Not) Operand() Expr { return x[0] }

func (x Not) String() string { return buildString(x) }

func (x Not) appendString(sb *strings.Builder) {
	sb.WriteString("(not ")
	appendString(sb, x[0])
	sb.WriteByte(')')
}

type Binary interface {
	Operands() [2]Expr
}

type Eq [2]Expr

func (x Eq) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Eq) String() string { return buildString(x) }

func (x Eq) appendString(sb *strings.Builder) { appendBinary(sb, "eq", x) }

type Ne [2]Expr

func (x Ne) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Ne) String() string { return buildString(x) }

func (x Ne) appendString(sb *strings.Builder) { appendBinary(sb, "ne", x) }

type Gt [2]Expr

func (x Gt) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Gt) String() string { return buildString(x) }

func (x Gt) appendString(sb *strings.Builder) { appendBinary(sb, "gt", x) }

type Ge [2]Expr

func (x Ge) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Ge) String() string { return buildString(x) }

func (x Ge) appendString(sb *strings.Builder) { appendBinary(sb, "ge", x) }

type Lt [2]Expr

func (x Lt) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Lt) String() string { return buildString(x) }

func (x Lt) appendString(sb *strings.Builder) { appendBinary(sb, "lt", x) }

type Le [2]Expr

func (x Le) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Le) String() string { return buildString(x) }

func (x Le) appendString(sb *strings.Builder) { appendBinary(sb, "le", x) }

type And [2]Expr

func (x And) Operands() [2]Expr { return ([2]Expr)(x) }

func (x And) String() string { return buildString(x) }

func (x And) appendString(sb *strings.Builder) { appendBinary(sb, "and", x) }

type Or [2]Expr

func (x Or) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Or) String() string { return buildString(x) }

func (x Or) appendString(sb *strings.Builder) { appendBinary(sb, "or", x) }

type Add [2]Expr

func (x Add) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Add) String() string { return buildString(x) }

func (x Add) appendString(sb *strings.Builder) { appendBinary(sb, "+", x) }

type Sub [2]Expr

func (x Sub) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Sub) String() string { return buildString(x) }

func (x Sub) appendString(sb *strings.Builder) { appendBinary(sb, "-", x) }

type Mul [2]Expr

func (x Mul) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Mul) String() string { return buildString(x) }

func (x Mul) appendString(sb *strings.Builder) { appendBinary(sb, "*", x) }

type Div [2]Expr

func (x Div) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Div) String() string { return buildString(x) }

func (x Div) appendString(sb *strings.Builder) { appendBinary(sb, "/", x) }

// Mem selects a member of a source expression.
type Mem [2]Expr

func (x Mem) Operands() [2]Expr { return ([2]Expr)(x) }

func (x Mem) String() string { return buildString(x) }

func (x Mem) appendString(sb *strings.Builder) { appendBinary(sb, ".", x) }

func (x Mem) diffTypesOK() {}

type Var interface {
	Expr

	// Var differentiates the Var from an Expr but could just
	// return itself
	Var() Var
}

func buildString(x interface{}) string {
	sb := strings.Builder{}
	appendString(&sb, x)
	return sb.String()
}

func appendString(sb *strings.Builder, x interface{}) {
	switch x := x.(type) {
	case interface{ appendString(sb *strings.Builder) }:
		x.appendString(sb)
	default:
		sb.WriteString(fmt.Sprint(x))
	}
}

func appendBinary(sb *strings.Builder, op string, b Binary) {
	ops := b.Operands()
	sb.WriteByte('(')
	sb.WriteString(op)
	sb.WriteByte(' ')
	appendString(sb, ops[0])
	sb.WriteByte(' ')
	appendString(sb, ops[1])
	sb.WriteByte(')')
}
