package asciidoctor

import (
	"errors"
	"strings"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/asciidoctor-go/asciidoctor/internal/bridge"
)

// ErrClosed is returned by the methods of a closed Asciidoctor instance.
var ErrClosed = bridge.ErrClosed

// Error is an exception raised by Asciidoctor (or by Ruby code it runs).
type Error struct {
	// Class is the Ruby exception class, such as "ArgumentError" or "IOError".
	Class string
	// Message is the exception message. Asciidoctor prefixes fatal processing
	// errors with "asciidoctor: FAILED: ".
	Message string
	// Backtrace is the Ruby backtrace, innermost frame first.
	Backtrace []string
	// Cause is the error returned by Go code (an extension, converter or
	// other callback) that raised the exception, if any.
	Cause error

	err *ruby.Error
}

func (e *Error) Error() string {
	if strings.HasPrefix(e.Message, "asciidoctor: ") {
		return e.Message
	}
	return "asciidoctor: " + e.Message + " (" + e.Class + ")"
}

// Unwrap returns the underlying interpreter error and the Go Cause.
func (e *Error) Unwrap() []error {
	var errs []error
	if e.err != nil {
		errs = append(errs, e.err)
	}
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	return errs
}

// wrapError converts an interpreter error into an *Error; other errors are
// returned unchanged.
func wrapError(err error) error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return err
	}
	var re *ruby.Error
	if errors.As(err, &re) {
		e = &Error{Class: re.Class, Message: re.Message, Backtrace: re.Backtrace, err: re}
		var ce *bridge.CallbackError
		if errors.As(err, &ce) {
			e.Cause = ce.Go
		}
		return e
	}
	return err
}

// rubyError returns the interpreter error behind err, so that an *Error that
// crosses back into Ruby re-raises the original exception.
func rubyError(err error) error {
	var e *Error
	if errors.As(err, &e) && e.err != nil {
		return e.err
	}
	if errors.As(err, &e) {
		return ruby.NewError(e.Class, e.Message)
	}
	return err
}
