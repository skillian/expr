package expr

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

//go:generate stringer -type opCode -trimprefix op
type opCode byte

func appendIntToOpCodesInto(ops *[]opCode, i int64) {
	*ops = appendIntToOpCodes(*ops, i)
}

func appendIntToOpCodes(ops []opCode, i int64) []opCode {
	var bs [9]byte
	n := binary.PutUvarint(bs[:], uint64(i))
	return append(ops, asOpCodeSlice(bs[:n])...)
}

func getIntFromOpCodes(ops []opCode) (int64, int) {
	_, n, _, _ := minMaxBy("", len(ops), "", 9)
	u, n := binary.Uvarint(asByteSlice(ops[:n]))
	return int64(u), n
}

func asByteSlice[T ~byte](sl []T) []byte {
	return *((*[]byte)(unsafe.Pointer(&sl)))
}

func asOpCodeSlice[T ~byte](sl []T) []opCode {
	return *((*[]opCode)(unsafe.Pointer(&sl)))
}

// mustBeByte makes sure its operand's underlying type is a byte.
// if there's a better way to do this, please let me know!
func mustBeByte[T ~byte](v T) {}

func _() {
	mustBeByte(opCode(0))
}

const (
	// opNop does nothing
	opNop opCode = iota

	// opNot negates its operand
	//
	//	x := pop()
	//	push(!x)
	//
	opNot

	// opAnd performs a boolean AND operation of its operands
	//
	//	a := pop()
	//	b := pop()
	//	push(a && b)
	//
	opAnd

	// opOr performs a boolean OR operation of its operands
	//
	//	a := pop()
	//	b := pop()
	//	push(a || b)
	//
	opOr

	// opEq compares its operands for equality
	//
	//	a := pop()
	//	b := pop()
	//	push(a == b)
	//
	opEq

	// opNe compares its operands for inequality
	//
	//	a := pop()
	//	b := pop()
	//	push(a != b)
	//
	opNe

	// opGt checks if the top operand is greater than the second.
	//
	//	a := pop()
	//	b := pop()
	//	push(a > b)
	//
	opGt

	// opGe checks if the top operand is greater than or equal to
	// the second.
	//
	//	a := pop()
	//	b := pop()
	//	push(a >= b)
	//
	opGe

	// opLt checks if the top operand is less than the second.
	//
	//	a := pop()
	//	b := pop()
	//	push(a < b)
	//
	opLt

	// opLe checks if the top operand is less than or equal to
	// the second.
	//
	//	a := pop()
	//	b := pop()
	//	push(a <= b)
	//
	opLe

	// opAdd adds the top two operands together.
	//
	//	a := pop()
	//	b := pop()
	//	push(a + b)
	//
	opAdd

	// opSub subtracts the second operand from the top operand
	//
	//	a := pop()
	//	b := pop()
	//	push(a - b)
	//
	opSub

	// opMul multipies the top two operands
	//
	//	a := pop()
	//	b := pop()
	//	push(a * b)
	//
	opMul

	// opDiv divides the top operand by the second.
	//
	//	a := pop()
	//	b := pop()
	//	push(a / b)
	//
	opDiv

	// opConv1 is followed by a single variadic int which is an
	// index into the function's const types that the top operand
	// should be converted to.  That type needs to know how to
	// convert from the source type.
	//
	//	op, index := _decode_op_and_int()
	//	v := pop()
	//	t1 := func.types[index]
	//	t1.conv(v, t1)
	//
	opConv1

	// opConv2 works the same as opConv1 except there's a second
	// index to a type that actually performs the conversion.
	//
	//	op, index1, index2 := _decode_op_and_2ints()
	//	v := pop()
	//	t1 := func.types[index]
	//	t2 := func.values[index].(eeConvType)
	//	t2.conv(v, t1)
	//
	opConv2

	// opSwap2 swaps the top two operands on the stack
	opSwap2

	// opDup duplicates the top operand on the stack.
	opDup

	// opLdc loads a constant from the function's constants.  It is
	// followed by a variadic int in the actual byte code which
	// indexes into a func's constkeys which is then used to
	// retrieve a value from the consts.
	opLdc

	// opLdv loads a variable value.  It works the same as opLdc,
	// but expects its constant value to be a Var whose value is
	// then loaded onto the stack.
	opLdv

	// opLdm loads a member value onto the stack.
	//
	//	a := pop()
	//	b := pop()
	//	push(a.b)
	//
	opLdm

	// opLdm loads a member address onto the stack.
	//
	//	a := pop()
	//	b := pop()
	//	push(&a.b)
	//
	opLdma

	// opLdk loads a value from a map associated with a key
	//
	//	m := pop()
	//	k := pop()
	//	push(m[k])
	//
	opLdk

	// opPackSlice hast two variadic-integer arguments:  The first
	// is an index to the type of the
	opPackSlice
)

func (op opCode) arity() int {
	switch op {
	case opNop:
		return 0
	case opNot:
		return 1
	}
	return 2
}

// nargs gets the number of variadic integers following the op code
// instruction
func (op opCode) nargs() int {
	switch op {
	case opConv1, opLdc:
		return 1
	case opConv2, opPackSlice:
		return 2
	}
	return 0
}

func opDasmAppendTo(sb *strings.Builder, f *opFunc, indent string) error {
	funcOpArgString := func(f *opFunc, op opCode, i int, arg int64) string {
		switch op {
		case opLdc:
			ck := f.constkeys[int(arg)]
			v := f.consts.types[ck.typeIndex].get(&f.consts, ck.valIndex)
			return fmt.Sprintf("%[1]v (type: %[1]T)", v)
		case opPackSlice:
			if i == 1 {
				return "" // fine as just numeric value.
			}
			fallthrough // else, it's 0: the type param:
		case opConv1, opConv2:
			t := f.consts.types[int(arg)]
			return fmt.Sprintf("%[1]v (metatype: %[1]T)", t)
		}
		panic(fmt.Errorf("unexpected op: %v", op))
	}
	var indexCache [2]int64
	for i := 0; i < len(f.ops); i++ {
		op := f.ops[i]
		indexes := indexCache[:op.nargs()]
		for j := range indexes {
			i64, n := getIntFromOpCodes(f.ops[i+1:])
			i += n
			indexes[j] = i64
		}
		sb.WriteString(indent)
		sb.WriteString(strconv.Itoa(i))
		sb.WriteByte('\t')
		sb.WriteString(op.String())
		if len(indexes) > 0 {
			sb.WriteByte('\t')
			for j, x := range indexes {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(strconv.FormatInt(x, 16))
				sb.WriteString(" (")
				sb.WriteString(strconv.FormatInt(x, 10))
				if s := funcOpArgString(f, op, j, x); s != "" {
					sb.WriteString(": ")
					sb.WriteString(s)
				}
				sb.WriteByte(')')
			}
		}
		sb.WriteByte('\n')
	}
	return nil
}
