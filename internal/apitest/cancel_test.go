package apitest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/rolling-robot/asciidoctorg"
	"github.com/rolling-robot/asciidoctorg/internal/testutil"
)

func TestConvertContextAlreadyCancelled(t *testing.T) {
	a := instance(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.ConvertContext(ctx, "hello", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

// A cancellation during parsing stops at the next block: the Go block macro
// that cancels runs once, although the document has three, and the instance
// stays usable.
func TestConvertContextCancelledWhileParsing(t *testing.T) {
	a := newInstance(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	if _, err := a.Extensions().BlockMacro("stop", asciidoctor.BlockMacroProcessorFunc(func(parent asciidoctor.StructuralNode, _ string, _ map[string]any) (asciidoctor.StructuralNode, error) {
		calls++
		cancel()
		return nil, nil
	}), nil); err != nil {
		t.Fatal(err)
	}
	_, err := a.ConvertContext(ctx, "stop::a[]\n\nstop::b[]\n\nstop::c[]\n", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	var e *asciidoctor.Error
	if !errors.As(err, &e) || e.Class != "AsciidoctorGo::Canceled" {
		t.Fatalf("got %#v, want an *Error of class AsciidoctorGo::Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("the macro ran %d times after the cancellation, want 1", calls)
	}
	if got := mustConvert(t, a, "*after*", nil); !strings.Contains(got, "<strong>after</strong>") {
		t.Fatalf("instance unusable after a cancelled call: %q", got)
	}
}

// A cancellation after parsing stops before conversion; later Go callbacks
// (here a postprocessor) do not run.
func TestConvertContextCancelledBeforeConversion(t *testing.T) {
	a := newInstance(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg := a.Extensions()
	if _, err := reg.Treeprocessor(asciidoctor.TreeprocessorFunc(func(*asciidoctor.Document) (*asciidoctor.Document, error) {
		cancel()
		return nil, nil
	}), nil); err != nil {
		t.Fatal(err)
	}
	post := false
	if _, err := reg.Postprocessor(asciidoctor.PostprocessorFunc(func(_ *asciidoctor.Document, out string) (string, error) {
		post = true
		return out, nil
	}), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConvertContext(ctx, "para one\n\npara two", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if post {
		t.Fatal("the postprocessor ran after the cancellation")
	}
}

func TestLoadContextCancelled(t *testing.T) {
	a := instance(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.LoadContext(ctx, "= Title", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

// A deadline bounds the wait for an instance that another call holds.
func TestConvertContextDeadlineWhileWaiting(t *testing.T) {
	a := newInstance(t)
	release, held := make(chan struct{}), make(chan struct{})
	go a.Do(func(*ruby.Interp) error { close(held); <-release; return nil })
	<-held
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := a.ConvertContext(ctx, "hello", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded", err)
	}
}

func TestPoolContextDeadlineWhileAllBusy(t *testing.T) {
	p, err := asciidoctor.NewPool(1, nil, asciidoctor.WithRuntimeDir(testutil.RuntimeDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	release, held := make(chan struct{}), make(chan struct{})
	go p.Do(func(*asciidoctor.Asciidoctor) error { close(held); <-release; return nil })
	<-held
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.ConvertContext(ctx, "hello", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded", err)
	}
}

// A deadline cuts a long conversion short, without any Go extension.
func TestConvertContextDeadlineOnLargeDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("converts a large document")
	}
	a := instance(t)
	src := strings.Repeat("A *paragraph* with _some_ `markup`.\n\n", 20000)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := a.ConvertContext(ctx, src, nil)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v after %v, want context.DeadlineExceeded", err, elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("the cancelled conversion took %v", elapsed)
	}
}
