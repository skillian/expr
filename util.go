package expr

import (
	"reflect"
	"strconv"
	"strings"
	"unsafe"
)

func minMaxInt(a, b int) (min, max int) {
	if a > b {
		return b, a
	}
	return a, b
}

func eq(a, b interface{}) (Eq bool) {
	defer func() {
		if recover() != nil {
			ad := *ifacePtrData(unsafe.Pointer(&a))
			bd := *ifacePtrData(unsafe.Pointer(&b))
			Eq = ad == bd
		}
	}()
	return a == b
}

type ifaceData struct {
	Type unsafe.Pointer
	Data unsafe.Pointer
}

func ifacePtrData(v unsafe.Pointer) *ifaceData {
	return (*ifaceData)(v)
}

type goFuncData unsafe.Pointer

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
