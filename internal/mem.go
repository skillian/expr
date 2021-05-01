package internal

import "math/bits"

// CapForLen returns a power of 2 capacity that's large enough to hold the
// given length.  For example, if length is 5, this function returns 8.  It
// is meant to standardize slice capacities for slices that are expected to
// grow by less than 100%.
//
// If you know how many elements you're going to need, of course do not use
// this.
func CapForLen(length int) int {
	return 1 << bits.Len(uint(length+1))
}
