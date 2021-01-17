package internal

import "math/bits"

// CapForLen returns a power of 2 capacity that's large enough to hold the
// given length.  For example, if length is 5, this function returns 8.  It
// is meant to simplify
func CapForLen(length int) int {
	return 1 << bits.Len(uint(length+1))
}
