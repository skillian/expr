package expr

import (
	"reflect"
	"sync"

	"github.com/skillian/expr/errors"
)

// Mem selects a member (a field, key, etc.) from a source expression
type Mem [2]Expr

// Operands implements the Binary interface.
func (e Mem) Operands() [2]Expr { return [2]Expr(e) }

// memberCache maps (reflect.Type, offset int64) to a reflect.Type's Field
// index.
var memberCache sync.Map

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
//	MemOf(x, &x, &x.Member)
//
// struc must be a pointer to a struct and field must be a pointer
// to a field within that struct.  The only benefit this construct has
// over passing the member name as a string is that if the struct field
// is renamed, this will break at compile time instead of panicking at
// runtime.
func MemOf(source, struc, field interface{}) Mem {
	rv := reflect.ValueOf(struc)
	if rv.Kind() != reflect.Ptr {
		panic(errors.Errorf1("%v must be a pointer to struct", struc))
	}
	rf := reflect.ValueOf(field)
	if rf.Kind() != reflect.Ptr {
		panic(errors.Errorf1("%v must be a pointer to a field of struc", field))
	}
	type memberCacheKey struct {
		reflect.Type
		Offset uintptr
	}
	mck := memberCacheKey{
		Type: rv.Elem().Type(),
		Offset: rf.Pointer() - rv.Pointer(),
	}
	if mck.Type.Kind() != reflect.Struct {
		panic(errors.Errorf2("%v must be a struct, not %v", struc, mck.Type))
	}
	var key interface{} = mck
	x, ok := memberCache.Load(key)
	if ok {
		return Mem{source, x}
	}
	nf := mck.Type.NumField()
	for i := 0; i < nf; i++ {
		sf := mck.Type.Field(i)
		if sf.Offset == mck.Offset {
			x, _ = memberCache.LoadOrStore(key, i)
			return Mem{source, x}
		}
	}
	panic(errors.Errorf2("unable to find field %v of %v", field, struc))
}
