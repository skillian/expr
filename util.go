package expr

import "unsafe"

type numberLike interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// minAndMaxBy takes two pairs of parameters:  An a pair and a b pair.
// within each pair, there is first the pair's value and second, a key
// which is used to compare as a key.
//
//	arbA := newArbitraryTypeValue()
//	keyA := arbA.getCompareKey()	// e.g. int(5)
//	arbB := newArbitraryTypeValue()
//	keyB := arbB.getCompareKey()	// e.g. int(2)
//	arbMin, keyMin, arbMax, keyMax := minAndMaxBy(arbA, keyA, arbB, keyB)
//	// arbMin == arbB, arbMax == arbA
//
func minMaxBy[TValue any, TKey numberLike](aVal TValue, aKey TKey, bVal TValue, bKey TKey) (minVal TValue, minKey TKey, maxVal TValue, maxKey TKey) {
	if aKey > bKey {
		return bVal, bKey, aVal, aKey
	}
	return aVal, aKey, bVal, bKey
}

func minMax[T numberLike](a, b T) (min, max T) {
	if a > b {
		return b, a
	}
	return a, b
}

func getIndexOrAppendInto[T any](vs *[]T, v T) int {
	for i, x := range *vs {
		if eq(v, x) {
			return i
		}
	}
	i := len(*vs)
	*vs = append(*vs, v)
	return i
}

func eq(a, b interface{}) (Eq bool) {
	defer func() {
		if recover() != nil {
			ad := *((*[2]uintptr)(unsafe.Pointer(&a)))
			bd := *((*[2]uintptr)(unsafe.Pointer(&b)))
			Eq = ad == bd
		}
	}()
	return a == b
}

type Tuple2[T0, T1 any] struct {
	V0 T0
	V1 T1
}

func Tuple2Of[T0, T1 any](v0 T0, v1 T1) Tuple2[T0, T1] {
	return Tuple2[T0, T1]{V0: v0, V1: v1}
}

type Tuple3[T0, T1, T2 any] struct {
	V0 T0
	V1 T1
	V2 T2
}

func Tuple3Of[T0, T1, T2 any](v0 T0, v1 T1, v2 T2) Tuple3[T0, T1, T2] {
	return Tuple3[T0, T1, T2]{V0: v0, V1: v1, V2: v2}
}
