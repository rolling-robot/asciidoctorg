// Package bridge connects Go to the Asciidoctor runtime: it serializes access
// to the interpreter, converts values, and dispatches calls from Ruby proxy
// objects to the Go objects behind them.
package bridge

import (
	"errors"
	"fmt"
	goruntime "runtime"
	"runtime/debug"
	"strings"
	"sync"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/rolling-robot/asciidoctorg/internal/runtime"
)

// ErrClosed is returned by every operation on a closed Bridge.
var ErrClosed = errors.New("asciidoctor: instance is closed")

// ErrCloseInCallback is returned by Close when it is called from inside the
// critical section (from an extension, converter or log handler callback, or
// inside Do): closing there would pull the interpreter out from under the call
// in progress.
var ErrCloseInCallback = errors.New("asciidoctor: Close called from a callback of the same instance")

// A Handler receives the calls a Ruby proxy forwards to Go.
type Handler interface {
	// Dispatch handles the proxy method name. The result is converted to Ruby;
	// an error raises (see ruby.Func).
	Dispatch(c *ruby.Call, method string, args []ruby.Value) (any, error)
}

// Bridge owns one interpreter with Asciidoctor loaded.
//
// An interpreter is not safe for concurrent use, so every entry point runs
// under mu. Ruby calls back into Go (extension processors, converters) while
// mu is held; code running in such a callback is already inside the critical
// section and must not lock again. The goroutines inside the critical section
// are recorded in owners: the one that locked mu, and any goroutine running a
// callback (a Ruby thread runs on a goroutine of its own). Do runs its function
// directly on those goroutines and locks mu on any other, so another goroutine
// waits even while a callback is in progress. This relies on the rule,
// documented on the public API, that objects passed to a callback are used only
// by the goroutine running it.
type Bridge struct {
	rt     *runtime.Runtime
	in     *ruby.Interp
	mu     sync.Mutex
	closed bool

	ownersMu sync.Mutex
	owners   map[uint64]int // goroutine ID -> nesting depth

	handlesMu sync.Mutex
	handles   map[int64]Handler
	nextID    int64

	// goErr is the last Go error a callback raised during the current
	// top-level call (see CallbackError).
	goErr error

	asciidoctor ruby.Value // the Asciidoctor module
	consts      map[string]ruby.Value
}

// New boots an interpreter and loads the Go glue. If loading the glue fails,
// the interpreter is closed before the error is returned.
func New(cfg runtime.Config) (_ *Bridge, err error) {
	rt, err := runtime.Boot(cfg)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			rt.Interp.Close()
		}
	}()
	b := &Bridge{rt: rt, in: rt.Interp, handles: map[int64]Handler{}, consts: map[string]ruby.Value{}, owners: map[uint64]int{}}
	mod, err := b.in.DefineModule("AsciidoctorGo::Bridge")
	if err != nil {
		return nil, err
	}
	mod.DefineSingletonMethod("dispatch", b.dispatch)
	if err := defineWriter(b.in); err != nil {
		return nil, err
	}
	if _, err := b.in.Eval("require 'asciidoctor_go'", "(asciidoctor-go glue)", 1); err != nil {
		return nil, err
	}
	if b.asciidoctor, err = b.in.Eval("::Asciidoctor", "(asciidoctor-go)", 1); err != nil {
		return nil, err
	}
	return b, nil
}

// Interp is the interpreter. Use it only inside Do.
func (b *Bridge) Interp() *ruby.Interp { return b.in }

// Root is the materialized Ruby tree.
func (b *Bridge) Root() string { return b.rt.Root }

// Asciidoctor is the Asciidoctor module. Use it only inside Do.
func (b *Bridge) Asciidoctor() ruby.Value { return b.asciidoctor }

// Do runs fn with exclusive access to the interpreter. When a top-level call
// fails because a Go callback returned an error, the result is a
// *CallbackError holding both that error and the Ruby exception.
func (b *Bridge) Do(fn func() error) error {
	g := goroutineID()
	if b.owns(g) {
		return fn()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	b.enterOwner(g)
	defer b.leaveOwner(g)
	b.goErr = nil
	err := fn()
	if goErr := b.goErr; err != nil && goErr != nil {
		b.goErr = nil
		var re *ruby.Error
		if errors.As(err, &re) && strings.Contains(re.Message, goErr.Error()) {
			return &CallbackError{Ruby: err, Go: goErr}
		}
	}
	return err
}

// Enter runs fn as Do does. Code inside fn may use the bridge again on the
// same goroutine: the critical section is re-entrant for its owner.
func (b *Bridge) Enter(fn func() error) error { return b.Do(fn) }

// owns reports whether goroutine g is inside the critical section.
func (b *Bridge) owns(g uint64) bool {
	b.ownersMu.Lock()
	defer b.ownersMu.Unlock()
	return b.owners[g] > 0
}

func (b *Bridge) enterOwner(g uint64) {
	b.ownersMu.Lock()
	b.owners[g]++
	b.ownersMu.Unlock()
}

func (b *Bridge) leaveOwner(g uint64) {
	b.ownersMu.Lock()
	if b.owners[g]--; b.owners[g] <= 0 {
		delete(b.owners, g)
	}
	b.ownersMu.Unlock()
}

// goroutineID returns the ID of the calling goroutine, read from the header
// of its stack trace ("goroutine 123 [running]:"). Go does not expose the ID
// otherwise; it identifies the owners of the critical section.
func goroutineID() uint64 {
	var buf [64]byte
	s := buf[:goruntime.Stack(buf[:], false)]
	s = s[len("goroutine "):]
	var id uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}

// CallbackError is a Ruby exception caused by an error a Go callback
// returned. errors.Is and errors.As see both.
type CallbackError struct {
	Ruby error
	Go   error
}

func (e *CallbackError) Error() string   { return e.Ruby.Error() }
func (e *CallbackError) Unwrap() []error { return []error{e.Ruby, e.Go} }

// Close runs the interpreter's at_exit handlers and releases it. Called from
// inside the critical section it returns ErrCloseInCallback instead of
// deadlocking on the lock its own caller holds.
func (b *Bridge) Close() error {
	if b.owns(goroutineID()) {
		return ErrCloseInCallback
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.handlesMu.Lock()
	b.handles = map[int64]Handler{}
	b.handlesMu.Unlock()
	return b.in.Close()
}

// Register makes h reachable from Ruby and returns its handle.
func (b *Bridge) Register(h Handler) int64 {
	b.handlesMu.Lock()
	defer b.handlesMu.Unlock()
	b.nextID++
	b.handles[b.nextID] = h
	return b.nextID
}

// Unregister forgets a handle.
func (b *Bridge) Unregister(id int64) {
	b.handlesMu.Lock()
	defer b.handlesMu.Unlock()
	delete(b.handles, id)
}

// HandleCount reports how many handles are registered (for leak tests).
func (b *Bridge) HandleCount() int {
	b.handlesMu.Lock()
	defer b.handlesMu.Unlock()
	return len(b.handles)
}

// Handler returns the handler registered under a handle.
func (b *Bridge) Handler(id int64) (Handler, bool) {
	b.handlesMu.Lock()
	defer b.handlesMu.Unlock()
	h, ok := b.handles[id]
	return h, ok
}

// dispatch implements AsciidoctorGo::Bridge.dispatch(handle, method, *args).
func (b *Bridge) dispatch(c *ruby.Call) (any, error) {
	if len(c.Args) < 2 {
		return nil, ruby.NewError("ArgumentError", "dispatch requires a handle and a method name")
	}
	id, ok := c.Args[0].Go().(int64)
	if !ok {
		return nil, ruby.NewError("TypeError", "dispatch handle must be an Integer")
	}
	h, ok := b.Handler(id)
	if !ok {
		return nil, ruby.NewError("RuntimeError", fmt.Sprintf("asciidoctor-go: no Go object for handle %d", id))
	}
	method := c.Args[1].String()
	// Ruby is running, so the critical section is held (by the goroutine that
	// called into Ruby); the goroutine of this callback joins its owners.
	g := goroutineID()
	b.enterOwner(g)
	defer b.leaveOwner(g)
	res, err := safeDispatch(h, c, method, c.Args[2:])
	if err != nil {
		var re *ruby.Error
		if !errors.As(err, &re) {
			b.goErr = err
		}
		return nil, toRubyError(err)
	}
	return res, nil
}

// safeDispatch converts a panic in Go code called from Ruby into an error, so
// that it raises in Ruby instead of tearing down the interpreter. A panic with
// an error value that is not a Go runtime error (node accessors panic with the
// interpreter's error) re-raises that error; anything else is a PanicError.
func safeDispatch(h Handler, c *ruby.Call, method string, args []ruby.Value) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				if _, isRuntime := e.(goruntime.Error); !isRuntime {
					err = e
					return
				}
			}
			err = &PanicError{Value: r, Stack: debug.Stack()}
		}
	}()
	return h.Dispatch(c, method, args)
}

// PanicError reports a panic in Go code called from Ruby.
type PanicError struct {
	// Value is the value passed to panic.
	Value any
	// Stack is the stack trace of the goroutine that panicked, taken where
	// the panic was recovered (runtime/debug.Stack).
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic in Go callback: %v", e.Value) }

// toRubyError keeps Ruby exceptions (so they re-raise unchanged) and turns
// any other error into a RuntimeError carrying its message.
func toRubyError(err error) error {
	var re *ruby.Error
	if errors.As(err, &re) {
		return re
	}
	return ruby.NewError("RuntimeError", err.Error())
}

// Const resolves a constant path such as "Asciidoctor::SafeMode::SECURE". Use
// it only inside Do.
func (b *Bridge) Const(path string) (ruby.Value, error) {
	if v, ok := b.consts[path]; ok {
		return v, nil
	}
	v, err := b.in.Eval("::"+path, "(asciidoctor-go)", 1)
	if err != nil {
		return ruby.Value{}, err
	}
	b.consts[path] = v
	return v, nil
}
