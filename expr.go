package expr

import (
	"strings"
)

// Expr defines an expression
type Expr interface{}

// Unary is a specialization of an Expr with a single inner operand
type Unary interface {
	Expr

	// Operand gets the unary expression's single operand
	Operand() Expr
}

// Not negates its expression
type Not [1]Expr

// Operand is the operand to the unary expression
func (e Not) Operand() Expr { return e[0] }

// Binary is an expression with two operands
type Binary interface {
	Expr

	// Operands gets the two operands to the binary expression.
	Operands() [2]Expr
}

// Eq is an equality expression
type Eq [2]Expr

// Operands gets the two operands to the expression
func (e Eq) Operands() [2]Expr { return ([2]Expr)(e) }

// Ne is an inequality expression
type Ne [2]Expr

// Operands gets the two operands to the expression
func (e Ne) Operands() [2]Expr { return ([2]Expr)(e) }

// Gt is a greater than expression
type Gt [2]Expr

// Operands gets the two operands to the expression
func (e Gt) Operands() [2]Expr { return ([2]Expr)(e) }

// Ge is a greater than or equal to expression
type Ge [2]Expr

// Operands gets the two operands to the expression
func (e Ge) Operands() [2]Expr { return ([2]Expr)(e) }

// Lt is a less than expression
type Lt [2]Expr

// Operands gets the two operands to the expression
func (e Lt) Operands() [2]Expr { return ([2]Expr)(e) }

// Le is a less than or equal to expression
type Le [2]Expr

// Operands gets the two operands to the expression
func (e Le) Operands() [2]Expr { return ([2]Expr)(e) }

// And is a boolean AND expression
type And [2]Expr

// Operands gets the two operands to the expression
func (e And) Operands() [2]Expr { return ([2]Expr)(e) }

// Or is a boolean OR expression
type Or [2]Expr

// Operands gets the two operands to the expression
func (e Or) Operands() [2]Expr { return ([2]Expr)(e) }

// Add adds e[0] + e[1]
type Add [2]Expr

// Operands gets the two operands to the expression
func (e Add) Operands() [2]Expr { return ([2]Expr)(e) }

// Sub subtracts e[0] - e[1]
type Sub [2]Expr

// Operands gets the two operands to the expression
func (e Sub) Operands() [2]Expr { return ([2]Expr)(e) }

// Mul multiplies e[0] * e[1]
type Mul [2]Expr

// Operands gets the two operands to the expression
func (e Mul) Operands() [2]Expr { return ([2]Expr)(e) }

// Div divides e[0] / e[1]
type Div [2]Expr

// Operands gets the two operands to the expression
func (e Div) Operands() [2]Expr { return ([2]Expr)(e) }

// Value is an Expr implementation that holds a value.
type Value interface {
	Expr

	// Interface gets the Go value, similar to how the reflect.Value's
	// Interface function.
	Interface() interface{}
}

// IsConst returns true if its value is a constant.
func IsConst(v interface{}) bool {
	switch v := v.(type) {
	case Unary:
		return false
	case Binary:
		return false
	case bool:
		return true
	case string:
		return true
	case int:
		return true
	case int8:
		return true
	case int16:
		return true
	case int32:
		return true
	case int64:
		return true
	case float32:
		return true
	case float64:
		return true
	case uint:
		return true
	case uint8:
		return true
	case uint16:
		return true
	case uint32:
		return true
	case uint64:
		return true
	case uintptr:
		return true
	default:
		_ = v
	}
	return false
}

// Inspect an expression tree by performing a depth-first search and calling
// f on every expression upon "entering" and then f(nil) when "exiting".
func Inspect(e Expr, f func(e Expr) bool) bool {
	if !f(e) {
		return false
	}
	switch e := e.(type) {
	case Unary:
		if !Inspect(e.Operand(), f) {
			return false
		}
	case Binary:
		for _, o := range e.Operands() {
			if !Inspect(o, f) {
				return false
			}
		}
	case Multary:
		for _, o := range e.Operands() {
			if !Inspect(o, f) {
				return false
			}
		}
	default:
	}
	return f(nil)
}

// Var describes a variable in an expression
type Var interface {
	Expr

	// Var just returns itself.  The Var interface is otherwise empty so
	// everything in expressions could erroneously be treated as a Var.
	// This is a simple (but maybe code-smelly?) way to opt-in to the "Var"
	// status.
	Var() Var
}

type varString struct{ s string }

func newVar(name string) Var { return &varString{s: name} }

func (v *varString) String() string {
	return strings.Join([]string{"var(", v.s, ")"}, "")
}

func (v *varString) Var() Var { return v }

var _ Var = (*varString)(nil)

// IsBool returns true if the expression evaluates to a boolean value
func IsBool(e Expr) bool {
	switch e := e.(type) {
	case Eq:
		return true
	case Ne:
		return true
	case Gt:
		return true
	case Ge:
		return true
	case Lt:
		return true
	case Le:
		return true
	case And:
		return true
	case Or:
		return true
	default:
		_ = e
	}
	return false
}

// Multary has 0 or more operands
type Multary interface {
	Operands() []Expr
}

// Set is an ordered set of values
type Set []Expr

// Operands implements the Multary interface.
func (e Set) Operands() []Expr { return ([]Expr)(e) }
