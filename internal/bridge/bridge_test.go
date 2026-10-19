package bridge

import (
	"errors"
	"strings"
	"testing"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/asciidoctor-go/asciidoctor/internal/runtime"
)

type handlerFunc func(c *ruby.Call, method string, args []ruby.Value) (any, error)

func (f handlerFunc) Dispatch(c *ruby.Call, method string, args []ruby.Value) (any, error) {
	return f(c, method, args)
}

func newBridge(t *testing.T) *Bridge {
	t.Helper()
	b, err := New(runtime.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func eval(t *testing.T, b *Bridge, src string) string {
	t.Helper()
	var out string
	err := b.Do(func() error {
		v, err := b.Interp().Eval(src, "bridge_test.rb", 1)
		if err != nil {
			return err
		}
		out = v.Inspect()
		return nil
	})
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return out
}

func TestDispatch(t *testing.T) {
	b := newBridge(t)
	id := b.Register(handlerFunc(func(c *ruby.Call, method string, args []ruby.Value) (any, error) {
		switch method {
		case "add":
			return args[0].Go().(int64) + args[1].Go().(int64), nil
		case "fail":
			return nil, errors.New("plain go error")
		case "argerror":
			return nil, ruby.NewError("ArgumentError", "bad argument")
		case "panic":
			panic("boom")
		case "panic_error":
			_, err := c.Interp.Eval("raise KeyError, 'missing key'", "inner.rb", 1)
			panic(err)
		case "nil_deref":
			var m map[string]*int
			return *m["x"], nil
		case "reenter":
			// Code in a callback may use the bridge again without deadlocking.
			var r string
			err := b.Do(func() error {
				v, err := b.Interp().Eval("6 * 7", "inner.rb", 1)
				r = v.Inspect()
				return err
			})
			return r, err
		case "ruby_raise":
			_, err := c.Interp.Eval("raise IOError, 'from ruby'", "inner.rb", 1)
			return nil, err
		}
		return nil, errors.New("unknown method " + method)
	}))
	b.Do(func() error { return b.Interp().Set("h", id) })
	tests := []struct{ src, want string }{
		{"AsciidoctorGo::Bridge.dispatch h, :add, 2, 3", "5"},
		{"begin; AsciidoctorGo::Bridge.dispatch h, :fail; rescue RuntimeError => e; e.message; end", `"plain go error"`},
		{"begin; AsciidoctorGo::Bridge.dispatch h, :argerror; rescue ArgumentError => e; e.message; end", `"bad argument"`},
		{"begin; AsciidoctorGo::Bridge.dispatch h, :panic; rescue RuntimeError => e; e.message; end", `"panic in Go callback: boom"`},
		{"AsciidoctorGo::Bridge.dispatch h, :reenter", `"42"`},
		{"begin; AsciidoctorGo::Bridge.dispatch h, :panic_error; rescue KeyError => e; e.message; end", `"missing key"`},
		{"begin; AsciidoctorGo::Bridge.dispatch h, :nil_deref; rescue RuntimeError => e; e.message.start_with? 'panic in Go callback: runtime error'; end", "true"},
		{"begin; AsciidoctorGo::Bridge.dispatch h, :ruby_raise; rescue IOError => e; e.message; end", `"from ruby"`},
		{"begin; AsciidoctorGo::Bridge.dispatch 999, :x; rescue RuntimeError => e; e.message; end", `"asciidoctor-go: no Go object for handle 999"`},
		{"begin; AsciidoctorGo::Bridge.dispatch 'h', :x; rescue TypeError => e; e.message; end", `"dispatch handle must be an Integer"`},
		{"begin; AsciidoctorGo::Bridge.dispatch; rescue ArgumentError => e; e.message; end", `"dispatch requires a handle and a method name"`},
	}
	for _, tc := range tests {
		if got := eval(t, b, tc.src); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
	b.Unregister(id)
	if got := eval(t, b, "begin; AsciidoctorGo::Bridge.dispatch h, :add, 1, 1; rescue RuntimeError => e; e.message; end"); !strings.Contains(got, "no Go object") {
		t.Errorf("after Unregister: %s", got)
	}
}

func TestClosedBridge(t *testing.T) {
	b := newBridge(t)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Do(func() error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("got %v", err)
	}
}

func TestConstCaches(t *testing.T) {
	b := newBridge(t)
	err := b.Do(func() error {
		v1, err := b.Const("Asciidoctor::SafeMode::SECURE")
		if err != nil {
			return err
		}
		v2, _ := b.Const("Asciidoctor::SafeMode::SECURE")
		if v1 != v2 || v1.Inspect() != "20" {
			t.Errorf("got %s and %s", v1.Inspect(), v2.Inspect())
		}
		if _, err := b.Const("No::Such"); err == nil {
			t.Error("expected an error for a missing constant")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
