package expr

// SubStr checks if a string contains a substring
type SubStr struct {
	// Sub is the substring
	Sub Expr

	// Str is the string
	Str Expr
}
