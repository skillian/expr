package expr

import (
	"errors"
	"math/bits"
	"strings"
)

type multierror []error

var _ interface {
	error
	Unwrap() error
} = multierror{}

func multierrorOf(errs ...error) error {
	capacityGuess := 1 << bits.Len(uint(len(errs)))
	if capacityGuess < 2 {
		capacityGuess = 2
	}
	me := make(multierror, 0, capacityGuess)
	for _, err := range errs {
		if err == nil {
			continue
		}
		if me2, ok := err.(multierror); ok {
			me = append(me, me2...)
			continue
		}
		me = append(me, err)
	}
	switch len(me) {
	case 0:
		return nil
	case 1:
		return me[0]
	}
	return me
}

func (me multierror) As(target interface{}) bool {
	if pme, ok := target.(*multierror); ok {
		*pme = me
		return true
	}
	for _, err := range me {
		if errors.As(err, target) {
			return true
		}
	}
	return false
}

func (me multierror) Error() string {
	strs := make([]string, len(me)*2)
	for i, err := range me {
		strs[i*2] = "\t•"
		strs[i*2+1] = err.Error()
	}
	return strings.Join(strs, "\n")
}

func (me multierror) Is(target error) bool {
	if me2, ok := target.(multierror); ok {
		if len(me) != len(me2) {
			return false
		}
		for i, err := range me {
			if !errors.Is(err, me2[i]) {
				return false
			}
		}
		return true
	}
	for _, err := range me {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (me multierror) Unwrap() error {
	return me[1:]
}
