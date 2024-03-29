package expr

import (
	"fmt"
	"math/big"
	"math/bits"
	"reflect"
	"unsafe"
)

// eeValues is a stack of values that tries to avoid "boxing" values
// into anys. Ints, floats, and strings are stored directly
// into the stack without boxing.
type eeValues struct {
	types []eeType
	anys  []interface{}
	nums  []int64
	strs  []string
}

// init initializes the eeValues' "substacks" with the given
// capacity.
func (vs *eeValues) init(capacity int) {
	vs.types = make([]eeType, 0, capacity)
	vs.anys = make([]interface{}, 0, capacity)
	vs.nums = make([]int64, 0, capacity)
	vs.strs = make([]string, 0, capacity)
}

// copyTo is like get but it copies the value from one eeValues to
// another without first boxing it into an interface{}
func (vs *eeValues) copyTo(k eeValueKey, dest *eeValues) eeValueKey {
	ti, vi := k.typeAndValueIndexes()
	et := vs.types[ti]
	ti = dest.pushType(et)
	ki := et.kind()
	switch ki {
	case opAny:
		vi = dest.pushAny(vs.anys[vi])
	case opBool, opFloat, opInt:
		vi = dest.pushInt(vs.nums[vi])
	case opRat:
		v := vs.anys[vi].(*big.Rat)
		vi = dest.pushRat((&big.Rat{}).Set(v))
	case opStr:
		vi = dest.pushStr(vs.strs[vi])
	default:
		panic(fmt.Errorf("unknown kind: %v", ki))
	}
	return makeEEValueKey(ti, vi)
}

func (vs *eeValues) floats() *[]float64 { return (*[]float64)(unsafe.Pointer(&vs.nums)) }

// peekType peeks the type at the top of the stack without popping it.
func (vs *eeValues) peekType() (v eeType) {
	return vs.types[len(vs.types)-1]
}

func (vs *eeValues) popAny() (v interface{}) {
	v = vs.anys[len(vs.anys)-1]
	vs.anys = vs.anys[:len(vs.anys)-1]
	return
}

func (vs *eeValues) popBool() (v bool) {
	v = vs.popInt() != 0
	return
}

func (vs *eeValues) popFloat() (v float64) {
	floats := vs.floats()
	v = (*floats)[len(*floats)-1]
	*floats = (*floats)[:len(*floats)-1]
	return
}

func (vs *eeValues) popInt() (v int64) {
	v = vs.nums[len(vs.nums)-1]
	vs.nums = vs.nums[:len(vs.nums)-1]
	return
}

func (vs *eeValues) popRat() (v *big.Rat) {
	v = vs.popAny().(*big.Rat)
	return
}

func (vs *eeValues) popStr() (v string) {
	v = vs.strs[len(vs.strs)-1]
	vs.strs = vs.strs[:len(vs.strs)-1]
	return
}

func (vs *eeValues) popType() (v eeType) {
	v = vs.peekType()
	vs.types = vs.types[:len(vs.types)-1]
	return
}

func (vs *eeValues) push(v interface{}) {
	et := typeOf(v)
	et.push(vs, v)
	vs.pushType(et)
}

func (vs *eeValues) pushAny(v interface{}) int {
	vs.anys = append(vs.anys, v)
	return len(vs.anys) - 1
}

func (vs *eeValues) pushBool(v bool) int {
	i := int64(0)
	if v {
		i = 1
	}
	return vs.pushInt(i)
}

func (vs *eeValues) pushFloat(v float64) int {
	floats := vs.floats()
	*floats = append(*floats, v)
	return len(*floats) - 1
}

func (vs *eeValues) pushInt(v int64) int {
	vs.nums = append(vs.nums, v)
	return len(vs.nums) - 1
}

func (vs *eeValues) pushRat(v *big.Rat) int {
	vs.anys = append(vs.anys, v)
	return len(vs.anys) - 1
}

func (vs *eeValues) pushStr(v string) int {
	vs.strs = append(vs.strs, v)
	return len(vs.strs) - 1
}

func (vs *eeValues) pushType(v eeType) int {
	vs.types = append(vs.types, v)
	return len(vs.types) - 1
}

func (vs *eeValues) reset() {
	for i := range vs.types {
		vs.types[i] = nil
	}
	vs.types = vs.types[:0]
	for i := range vs.anys {
		vs.anys[i] = nil
	}
	vs.anys = vs.anys[:0]
	// Clearing numbers too, even though they don't reference
	// memory to reduce the chance of leaking information to
	// subsequent users of the exprEvaluator.
	for i := range vs.nums {
		vs.nums[i] = 0
	}
	vs.nums = vs.nums[:0]
	for i := range vs.strs {
		vs.strs[i] = ""
	}
	vs.strs = vs.strs[:0]
}

type eeValueKey struct {
	data uint
}

const (
	eeValueKeyTypeBits = (bits.UintSize / 2) + 2
	eeValueKeyTypeMask = (1 << eeValueKeyTypeBits) - 1

	eeValueKeyValIndexBits = bits.UintSize - eeValueKeyTypeBits
	eeValueKeyValIndexMask = (1 << eeValueKeyValIndexBits) - 1
)

func makeEEValueKey(typeIndex, valueIndex int) eeValueKey {
	if typeIndex < 0 || typeIndex > eeValueKeyTypeMask {
		panic(fmt.Errorf(
			"type index %d is out of range [0, %d]",
			typeIndex, eeValueKeyTypeMask,
		))
	}
	if valueIndex < 0 || valueIndex > eeValueKeyValIndexMask {
		panic(fmt.Errorf(
			"value index %d is out of range [0, %d]",
			valueIndex, eeValueKeyValIndexMask,
		))
	}
	return eeValueKey{
		data: (uint(valueIndex&eeValueKeyValIndexMask) << eeValueKeyTypeBits) |
			uint(typeIndex&eeValueKeyTypeMask),
	}
}

func eeValueKeyFromOpCodes(ops []opCode) (k eeValueKey, n int) {
	bs := opCodesBytes(ops)
	k.data = *((*uint)(unsafe.Pointer(&bs[0])))
	return k, int(unsafe.Sizeof(uint(0)))
}

func (vk eeValueKey) appendToOpCodes(ops []opCode) []opCode {
	return opCodesFromBytes(appendValBitsToBytes(opCodesBytes(ops), vk.data))
}

func (vk eeValueKey) typeAndValueIndexes() (typeIndex, valueIndex int) {
	return int(vk.data) & eeValueKeyTypeMask,
		int(vk.data>>eeValueKeyTypeBits) & eeValueKeyValIndexMask
}

func appendValBitsToBytes(bs []byte, v interface{}) []byte {
	rv := reflect.ValueOf(v)
	rv2 := reflect.New(rv.Type())
	rv2.Elem().Set(rv)
	return appendPtrToValBitsToBytes(bs, rv2.Interface())
}

func valBytes(v interface{}) (vBytes []byte) {
	rv := reflect.ValueOf(v)
	rt := rv.Type()
	if rt.Kind() != reflect.Pointer {
		panic("must be pointer")
	}
	vbsh := (*reflect.SliceHeader)(unsafe.Pointer(&vBytes))
	vbsh.Data = rv.Pointer()
	vbsh.Cap = int(rt.Size())
	vbsh.Len = vbsh.Cap
	return
}

func appendPtrToValBitsToBytes(bs []byte, v interface{}) []byte {
	return append(bs, valBytes(v)...)
}
