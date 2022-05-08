// You can edit this code!
// Click here and start typing.
package main

import (
	"fmt"
	"unsafe"
)

type S struct {
	I int
	F func(string, int) float32
}

//go:noinline
func (s *S) initAorB() {
	if uintptr(unsafe.Pointer(s))&0x20 == 0x20 {
		s.F = a
		return
	}
	s.F = b
}

func main() {
	s := &S{}
	s.initAorB()
	fmt.Println(s.F("test", 0xfeed))
}

func a(s string, i int) float32 {
	return 1.25
}

func b(s string, i int) float32 {
	return 2.125
}
