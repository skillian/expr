package internal

import "unsafe"

// Iface encapsulates the data of an interface.
type Iface struct {
	t IfaceType
	v uintptr
}

// IfaceType holds an opaque type key
type IfaceType struct {
	uintptr
}

// IfaceOf creates an Iface from an interface{}
func IfaceOf(v interface{}) Iface { return *((*Iface)(unsafe.Pointer(&v))) }

// Type gets the IfaceType associated with the Iface.
func (i Iface) Type() IfaceType { return i.t }

// Uintptr gets the Iface's value as a uintptr
func (i Iface) Uintptr() uintptr { return i.v }
