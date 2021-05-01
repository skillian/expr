package sqlstream

// interfacer is implemented by anything that wraps an arbitrary value.
//
// The method is called "Interface" instead of something like "Value"
// because that's what the reflect package's Value implements.
type interfacer interface {
	Interface() interface{}
}

// interfaceWrapper is a simple implementation of interfacer that is provided
// the value to wrap.
type interfacerWrapper struct {
	v interface{}
}

func (w interfacerWrapper) Interface() interface{} { return w.v }

// interfaceOf recursively unpacks v ultimately returning the innermost
// interface{}
func interfaceOf(v interface{}) interface{} {
	for {
		if ir, ok := v.(interfacer); ok {
			v = ir.Interface()
		} else {
			break
		}
	}
	return v
}

// interfaceOf2 is the same as interfaceOf, but when the second return
// parameter is true, then the caller needs to know that the returned value
// is not the same as the value passed in.
func interfaceOf2(v interface{}) (w interface{}, changed bool) {
	ir, ok := v.(interfacer)
	if !ok {
		return v, false
	}
	changed = true
	for {
		w = ir.Interface()
		ir, ok = w.(interfacer)
		if !ok {
			break
		}
	}
	return
}
