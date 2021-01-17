package internal

// StringsContainAll evaluates a predicate against every string in strs and
// every string in vs and returns true if pred evaluates to true with at least
// one strs and vs pair.
func StringsPredicateAll(strs []string, pred func(a, b string) bool, vs ...string) bool {
	track := make(map[string]struct{}, len(vs))
	for _, str := range strs {
		for _, v := range vs {
			if pred(str, v) {
				track[v] = struct{}{}
			}
		}
	}
	for _, v := range vs {
		if _, ok := track[v]; !ok {
			return false
		}
	}
	return true
}

// StringsContainAll returns true if every value in vs is contained in strs.
func StringsContainAll(strs []string, vs ...string) bool {
	return StringsPredicateAll(strs, func(a, b string) bool { return a == b }, vs...)
}
