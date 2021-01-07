package expr

import (
	"reflect"

	"github.com/skillian/expr/errors"
)

// Mem selects a member (a field, key, etc.) from a source expression
type Mem [2]Expr

// Operands implements the Binary interface.
func (e Mem) Operands() [2]Expr { return [2]Expr(e) }

// MemOf is a clunky way to work around Go's lack of having an expression type
// like C# has.  Source is the actual source expression that the the member
// will be retrieved from.  struc is used to compute a base offset.
//
// This in C#:
//
//	x => x.Member
//
// Looks like this here:
//
//	MemOf(x, &x, &x.Field)
//
func MemOf(source, struc, field interface{}) Mem {
	rv := reflect.ValueOf(struc)
	if rv.Kind() != reflect.Ptr {
		panic(errors.Errorf1("%v must be a pointer to struct", struc))
	}
	rf := reflect.ValueOf(field)
	if rf.Kind() != reflect.Ptr {
		panic(errors.Errorf1("%v must be a pointer to a field of struc", field))
	}
	offset := rf.Pointer() - rv.Pointer()
	rt := rv.Elem().Type()
	if rt.Kind() != reflect.Struct {
		panic(errors.Errorf2("%v must be a struct, not %v", struc, rt))
	}
	nf := rt.NumField()
	for i := 0; i < nf; i++ {
		sf := rt.Field(i)
		if sf.Offset == offset {
			return Mem{source, i}
		}
	}
	panic(errors.Errorf2("unable to find field %v of %v", field, struc))
}
