package asciidoctor

import (
	"strings"
	"testing"
)

// D8: registering a registered group again bound its handlers to new bridge
// handles without releasing the old ones.
func TestRegisterGroupAgainReleasesHandles(t *testing.T) {
	a, err := New(WithRuntimeDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	calls := 0
	g := a.Extensions().Group("again").Postprocessor(PostprocessorFunc(func(_ *Document, out string) (string, error) {
		calls++
		return out + "!", nil
	}), nil)
	if err := g.Register(); err != nil {
		t.Fatal(err)
	}
	before := a.b.HandleCount()
	if err := g.Register(); err != nil {
		t.Fatal(err)
	}
	if after := a.b.HandleCount(); after != before {
		t.Fatalf("handles after registering the group again: %d, want %d", after, before)
	}
	out, err := a.Convert("x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.HasSuffix(out, "!") || strings.HasSuffix(out, "!!") {
		t.Fatalf("processor ran %d times, output %q; want once", calls, out)
	}
}
