package apitest

import (
	"errors"
	"testing"
	"time"

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
