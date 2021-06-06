package errors

import (
	goerrors "errors"
	"fmt"
	"strings"

	"github.com/skillian/logging"
)

var logger = logging.GetLogger("expr/errors")

// Aggregate zero or more possibly nil errors into one error
func Aggregate(errs ...error) error {
	for i := len(errs) - 1; i >= 0; i-- {
		if errs[i] == nil {
			if i == len(errs)-1 {
				errs = errs[:i]
			} else {
				errs = append(errs[:i], errs[i+1:]...)
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors(errs)
}

// Any returns true if f evaluates any of the errors contained in err to true.
func Any(err error, f func(e error) bool) bool {
	return First(err, f) != nil
}

// Catch an error.  This function is expected to be called from a defer.
func Catch(p *error, f func() error) {
	*p = Aggregate(*p, f())
}

// First gets the first error for which f(error) returns true.
func First(err error, f func(e error) bool) error {
	if errs, ok := err.(errors); ok {
		for _, err := range errs {
			if f(err) {
				return err
			}
		}
		return nil
	}
	if f(err) {
		return err
	}
	for {
		if uw, ok := err.(interface{ Unwrap() error }); ok {
			err = uw.Unwrap()
			if f(err) {
				return err
			}
			continue
		}
		break
	}
	return nil
}

// New passes through to the go's errors package's New function
func New(s string) error {
	return goerrors.New(s)
}

type errors []error

// ErrorfFrom creates an error message around another error.
func ErrorfFrom(err error, format string, args ...interface{}) error {
	msg := errorf(1, format, args...)
	if errs, ok := err.(errors); ok {
		return errs.appendedTo(msg)
	}
	return errors{msg, err}
}

// Errorf0From creates an error message around another error.
func Errorf0From(err error, format string) error {
	msg := errorf0(1, format)
	if errs, ok := err.(errors); ok {
		return errs.appendedTo(msg)
	}
	return errors{msg, err}
}

// Errorf1From creates an error message around another error.
func Errorf1From(err error, format string, arg interface{}) error {
	msg := errorf1(1, format, arg)
	if errs, ok := err.(errors); ok {
		return errs.appendedTo(msg)
	}
	return errors{msg, err}
}

// Errorf2From creates an error message around another error.
func Errorf2From(err error, format string, arg0, arg1 interface{}) error {
	msg := errorf2(1, format, arg0, arg1)
	if errs, ok := err.(errors); ok {
		return errs.appendedTo(msg)
	}
	return errors{msg, err}
}

// Errorf3From creates an error message around another error.
func Errorf3From(err error, format string, arg0, arg1, arg2 interface{}) error {
	msg := errorf3(1, format, arg0, arg1, arg2)
	if errs, ok := err.(errors); ok {
		return errs.appendedTo(msg)
	}
	return errors{msg, err}
}

func (errs errors) appendedTo(err ...error) errors {
	return append(err, errs...)
}

func (errs errors) Error() string {
	strs := make([]string, len(errs))
	for i, err := range errs {
		strs[i] = err.Error()
	}
	sep := ": "
	for _, s := range strs {
		if strings.Contains(s, "\n") {
			sep = "\n\n"
			break
		}
	}
	return strings.Join(strs, sep)
}

type message struct {
	format string
	args   []interface{}
	pcs
}

func newMessage(skip int) *message {
	m := &message{}
	m.pcs.put(skip + 1)
	return m
}

// Errorf creates a formatted error string
func Errorf(format string, args ...interface{}) error {
	return errorf(1, format, args...)
}

func errorf(skip int, format string, args ...interface{}) error {
	m := newMessage(skip + 2)
	m.format = format
	m.args = args
	return m
}

// Errorf0 creates an error message with 0 arguments
func Errorf0(format string) error {
	return errorf0(1, format)
}

func errorf0(skip int, format string) error {
	m := newMessage(skip + 2)
	m.format = format
	return m
}

// Errorf1 creates an error message with 1 argument
func Errorf1(format string, arg interface{}) error {
	return errorf1(1, format, arg)
}

func errorf1(skip int, format string, arg interface{}) error {
	m := newMessage(skip + 2)
	m.format = format
	m.args = make([]interface{}, 1)
	m.args[0] = arg
	return m
}

// Errorf2 creates an error message with 2 arguments
func Errorf2(format string, arg0, arg1 interface{}) error {
	return errorf2(1, format, arg0, arg1)
}

func errorf2(skip int, format string, arg0, arg1 interface{}) error {
	m := newMessage(skip + 2)
	m.format = format
	m.args = make([]interface{}, 2)
	m.args[0] = arg0
	m.args[1] = arg1
	return m
}

// Errorf3 creates an error message with 3 arguments
func Errorf3(format string, arg0, arg1, arg2 interface{}) error {
	return errorf3(1, format, arg0, arg1, arg2)
}

func errorf3(skip int, format string, arg0, arg1, arg2 interface{}) error {
	m := newMessage(skip + 2)
	m.format = format
	m.args = make([]interface{}, 3)
	m.args[0] = arg0
	m.args[1] = arg1
	m.args[2] = arg2
	return m
}

func (m *message) Error() string {
	return fmt.Sprintf(m.format, m.args...) + m.pcs.Error()
}

type sentinel struct{}

func (s *sentinel) Error() string { return "sentinel" }
