// Package bridge connects Go to the Asciidoctor runtime: it serializes access
// to the interpreter, converts values, and dispatches calls from Ruby proxy
// objects to the Go objects behind them.
package bridge

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/asciidoctor-go/asciidoctor/internal/runtime"
)

// ErrClosed is returned by every operation on a closed Bridge.
var ErrClosed = errors.New("asciidoctor: instance is closed")

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
// section and must not lock again. callbacks counts the callbacks in progress,
// and Do runs its function directly while it is non-zero. This relies on the
// rule, documented on the public API, that objects passed to a callback are
// used only by the goroutine running it.
type Bridge struct {
	rt        *runtime.Runtime
	in        *ruby.Interp
	mu        sync.Mutex
	callbacks atomic.Int32
	closed    bool

	handlesMu sync.Mutex
	handles   map[int64]Handler
	nextID    int64

	asciidoctor ruby.Value // the Asciidoctor module
	consts      map[string]ruby.Value
}

// New boots an interpreter and loads the Go glue.
func New(cfg runtime.Config) (*Bridge, error) {
	rt, err := runtime.Boot(cfg)
	if err != nil {
		return nil, err
	}
	b := &Bridge{rt: rt, in: rt.Interp, handles: map[int64]Handler{}, consts: map[string]ruby.Value{}}
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

// Do runs fn with exclusive access to the interpreter.
func (b *Bridge) Do(fn func() error) error {
	if b.callbacks.Load() > 0 {
		return fn()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	return fn()
}

// Close runs the interpreter's at_exit handlers and releases it.
func (b *Bridge) Close() error {
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

func (b *Bridge) handler(id int64) (Handler, bool) {
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
	h, ok := b.handler(id)
	if !ok {
		return nil, ruby.NewError("RuntimeError", fmt.Sprintf("asciidoctor-go: no Go object for handle %d", id))
	}
	method := c.Args[1].String()
	b.callbacks.Add(1)
	defer b.callbacks.Add(-1)
	res, err := safeDispatch(h, c, method, c.Args[2:])
	if err != nil {
		return nil, toRubyError(err)
	}
	return res, nil
}

// safeDispatch converts a panic in Go code called from Ruby into an error, so
// that it raises in Ruby instead of tearing down the interpreter.
func safeDispatch(h Handler, c *ruby.Call, method string, args []ruby.Value) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{Value: r}
		}
	}()
	return h.Dispatch(c, method, args)
}

// PanicError reports a panic in Go code called from Ruby.
type PanicError struct{ Value any }

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
