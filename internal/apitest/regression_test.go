package apitest

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/rolling-robot/asciidoctorg"
)

// Regression tests for the defects found in the code review of 2026-11-07
// (AGENTS.md, "Code review"). Each test names the defect it covers.

// within runs fn and fails the test if it does not return within d; a
// deadlock then fails instead of hanging the test binary.
func within(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("did not return within %v (deadlock?)", d)
	}
}

// D1: Close called from a callback deadlocked the instance for good.
func TestCloseInCallback(t *testing.T) {
	a := newInstance(t)
	var closeErr error
	if _, err := a.Extensions().Treeprocessor(asciidoctor.TreeprocessorFunc(func(*asciidoctor.Document) (*asciidoctor.Document, error) {
		closeErr = a.Close()
		return nil, nil
	}), nil); err != nil {
		t.Fatal(err)
	}
	within(t, 10*time.Second, func() {
		if _, err := a.Convert("hello", nil); err != nil {
			t.Errorf("Convert: %v", err)
		}
	})
	if !errors.Is(closeErr, asciidoctor.ErrCloseInCallback) {
		t.Fatalf("Close in a callback returned %v, want ErrCloseInCallback", closeErr)
	}
	// The instance is still usable, and a Close outside the callback works.
	mustConvert(t, a, "still open", nil)
	within(t, 10*time.Second, func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}

// D2: node accessors panic once the instance is closed. The panic is part of
// the documented contract; its value must be ErrClosed so that a recover can
// tell it apart from a bug.
func TestNodeAccessorAfterClosePanicsWithErrClosed(t *testing.T) {
	a := newInstance(t)
	doc, err := a.Load("= Title\n\nparagraph", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		r := recover()
		err, _ := r.(error)
		if !errors.Is(err, asciidoctor.ErrClosed) {
			t.Fatalf("recovered %v, want a panic with ErrClosed", r)
		}
	}()
	_ = doc.Doctitle()
	t.Fatal("Doctitle on a closed instance did not panic")
}

// D3: ExtensionRegistry took its own mutex before the interpreter, while a
// callback (which holds the interpreter) registering a group takes them in the
// opposite order, so the two deadlocked.
func TestRegisterFromCallbackWhileAnotherGoroutineRegisters(t *testing.T) {
	a := newInstance(t)
	reg := a.Extensions()
	noop := asciidoctor.PostprocessorFunc(func(_ *asciidoctor.Document, out string) (string, error) { return out, nil })
	var inner error
	concurrentDone := make(chan struct{})
	if _, err := reg.Treeprocessor(asciidoctor.TreeprocessorFunc(func(*asciidoctor.Document) (*asciidoctor.Document, error) {
		// Another goroutine registers now: it takes the registry's lock
		// and then waits for the interpreter, which this callback holds.
		started := make(chan struct{})
		go func() {
			defer close(concurrentDone)
			close(started)
			if _, err := reg.Postprocessor(noop, nil); err != nil {
				t.Errorf("concurrent Postprocessor: %v", err)
			}
		}()
		<-started
		time.Sleep(100 * time.Millisecond)
		_, inner = reg.Postprocessor(noop, nil)
		return nil, nil
	}), nil); err != nil {
		t.Fatal(err)
	}
	within(t, 10*time.Second, func() {
		if _, err := a.Convert("hello", nil); err != nil {
			t.Errorf("Convert: %v", err)
		}
	})
	within(t, 10*time.Second, func() { <-concurrentDone })
	if inner != nil {
		t.Fatalf("register from callback: %v", inner)
	}
}

// D4: RegisterLogHandler and the removal of the last handler decided whether
// to install or uninstall the Go logger under one lock and acted on it under
// another. A handler registered while the last one was being removed could
// end up registered with no Go logger installed, so its messages were lost.
func TestLogHandlerRegisteredWhileLastOneIsRemoved(t *testing.T) {
	a := newInstance(t)
	remove1, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(asciidoctor.LogRecord) {}))
	if err != nil {
		t.Fatal(err)
	}
	// Hold the interpreter so that the removal and the registration below
	// both have to wait for it, in this order.
	release, held := make(chan struct{}), make(chan struct{})
	go a.Do(func(*ruby.Interp) error { close(held); <-release; return nil })
	<-held
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := remove1(); err != nil {
			t.Error(err)
		}
	}()
	time.Sleep(100 * time.Millisecond) // let the removal reach the interpreter lock
	var got atomic.Int32
	var remove2 func() error
	go func() {
		defer wg.Done()
		var err error
		if remove2, err = a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(asciidoctor.LogRecord) { got.Add(1) })); err != nil {
			t.Error(err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	close(release)
	within(t, 10*time.Second, wg.Wait)
	if t.Failed() {
		return
	}
	if err := a.Log(asciidoctor.LogRecord{Severity: asciidoctor.SeverityWarn, Message: "probe"}); err != nil {
		t.Fatal(err)
	}
	if n := got.Load(); n != 1 {
		t.Fatalf("the registered handler received %d messages, want 1", n)
	}
	if err := remove2(); err != nil {
		t.Fatal(err)
	}
}

// D9: an error a Go callback returned was attached (as Error.Cause) to
// whatever exception ended the call if its message contained the Go error's
// text, so an empty message, or a callback error that Ruby had rescued,
// was blamed for an unrelated exception.
func TestCallbackErrorNotBlamedForUnrelatedException(t *testing.T) {
	for _, msg := range []string{"", "boom"} {
		t.Run("message "+msg, func(t *testing.T) {
			a := newInstance(t)
			goErr := errors.New(msg)
			if _, err := a.Extensions().InlineMacro("fail", asciidoctor.InlineMacroProcessorFunc(func(asciidoctor.Node, string, map[string]any) (*asciidoctor.PhraseNode, error) {
				return nil, goErr
			}), nil); err != nil {
				t.Fatal(err)
			}
			err := a.Do(func(in *ruby.Interp) error {
				_, err := in.Eval(`
begin
  Asciidoctor.convert 'fail:x[]'
rescue
end
raise ArgumentError, 'unrelated boom'`, "d9.rb", 1)
				return err
			})
			var e *asciidoctor.Error
			if !errors.As(err, &e) || e.Class != "ArgumentError" {
				t.Fatalf("got %v, want the ArgumentError", err)
			}
			if e.Cause != nil || errors.Is(err, goErr) {
				t.Fatalf("the rescued Go error %q was blamed for the ArgumentError", msg)
			}
		})
	}
}
