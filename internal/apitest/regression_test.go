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
