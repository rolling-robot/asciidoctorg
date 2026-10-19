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

	err *ruby.Error
}

func (e *Error) Error() string {
	if strings.HasPrefix(e.Message, "asciidoctor: ") {
		return e.Message
	}
	return "asciidoctor: " + e.Message + " (" + e.Class + ")"
}

// Unwrap returns the underlying interpreter error.
func (e *Error) Unwrap() error {
	if e.err == nil {
		return nil
	}
	return e.err
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
		return &Error{Class: re.Class, Message: re.Message, Backtrace: re.Backtrace, err: re}
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
