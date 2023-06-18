package expr

import (
	"reflect"
	"strconv"
	"strings"
	"unsafe"
)

type numberLike interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

func minMax[T numberLike](a, b T) (min, max T) {
	if a > b {
		return b, a
	}
	return a, b
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

func reflectTypeName(t reflect.Type) string {
	var buildName func(t reflect.Type, parts []string) []string
	buildName = func(t reflect.Type, parts []string) []string {
		n := t.Name()
		if n != "" {
			return append(parts, n)
		}
		switch t.Kind() {
		case reflect.Ptr:
			parts = append(parts, "*")
		case reflect.Slice:
			parts = append(parts, "[]")
		case reflect.Array:
			parts = append(parts, "[", strconv.Itoa(t.Len()), "]")
		}
		return buildName(t.Elem(), parts)
	}
	parts := buildName(t, make([]string, 0, 8))
	return strings.Join(parts, "")
}
