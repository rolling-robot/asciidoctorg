package bridge

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoContextAlreadyDone(t *testing.T) {
	b := newBridge(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	err := b.DoContext(ctx, func() error { ran = true; return nil })
	if !errors.Is(err, context.Canceled) || ran {
		t.Fatalf("got %v (ran %v), want context.Canceled without running fn", err, ran)
	}
}

func TestDoContextStopsWaitingForTheInterpreter(t *testing.T) {
	b := newBridge(t)
	release, held := make(chan struct{}), make(chan struct{})
	go b.Do(func() error { close(held); <-release; return nil })
	<-held
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := b.DoContext(ctx, func() error { t.Error("fn ran while another call held the interpreter"); return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("waited %v", d)
	}
}

func TestCheckpointRaisesCanceledPastBareRescue(t *testing.T) {
	b := newBridge(t)
	ctx, cancel := context.WithCancel(context.Background())
	err := b.DoContext(ctx, func() error {
		if _, err := b.Interp().Eval("AsciidoctorGo::Bridge.checkpoint", "cancel_test.rb", 1); err != nil {
			t.Errorf("checkpoint before cancel: %v", err)
		}
		if b.Context() != ctx {
			t.Error("Context() is not the context of the call in progress")
		}
		cancel()
		_, err := b.Interp().Eval("begin; AsciidoctorGo::Bridge.checkpoint; rescue; :rescued; end", "cancel_test.rb", 1)
		return err
	})
	var ce *CallbackError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &ce) {
		t.Fatalf("got %v, want a CallbackError wrapping context.Canceled", err)
	}
	if b.Context() != context.Background() {
		t.Fatal("Context() between calls is not context.Background()")
	}
	// The interpreter stays usable.
	if got := eval(t, b, "1 + 1"); got != "2" {
		t.Fatalf("after cancel: 1 + 1 = %s", got)
	}
}
