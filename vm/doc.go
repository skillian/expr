// Package vm implements a virtual machine used to evaluate expressions
// locally.  This package should only be used when either:
//
//	- Expressions are provided via configuration/user input
// OR
//	- Expressions may or may not be passed to an external system.
//
// If you know the expression is going to be executed locally, use a
// normal function.  The vm package is a fallback for when a filter,
// map, etc. function cannot be implemented by some database engines
// (or, more likely, their expr drivers).
package vm
