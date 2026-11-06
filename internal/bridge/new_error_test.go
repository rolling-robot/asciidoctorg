package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rolling-robot/asciidoctorg/internal/runtime"
)

// D5: New left the booted interpreter running when loading the Go glue
// failed. The Asciidoctor tree here boots (it defines just enough) but lacks
// the converters the glue requires; an at_exit handler records whether the
// interpreter was closed.
func TestNewClosesInterpreterOnError(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "closed")
	files := map[string]string{
		"asciidoctor.rb":            fmt.Sprintf("at_exit { File.write %q, 'closed' }\nmodule Asciidoctor; LIB_DIR = __dir__; end\n", marker),
		"asciidoctor/extensions.rb": "",
	}
	for name, body := range files {
		p := filepath.Join(dir, "lib", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if rt, err := runtime.Boot(runtime.Config{Dir: t.TempDir(), AsciidoctorDir: dir}); err != nil {
		t.Fatalf("the fake Asciidoctor does not boot, so this test cannot reach New's glue: %v", err)
	} else {
		rt.Interp.Close()
		os.Remove(marker)
	}
	if _, err := New(runtime.Config{Dir: t.TempDir(), AsciidoctorDir: dir}); err == nil {
		t.Fatal("New succeeded without Asciidoctor's converters")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the interpreter was not closed after New failed: %v", err)
	}
}

// D6: AsciidoctorGo::Writer's methods assume a Go writer behind the object.
// One created from Ruby has none; using it must raise, not crash.
func TestWriterCreatedFromRubyRaises(t *testing.T) {
	b := newBridge(t)
	got := eval(t, b, "begin; AsciidoctorGo::Writer.new.write('x'); 'no error'; rescue ::StandardError => e; e.class.name; end")
	if got == `"no error"` {
		t.Fatalf("writing to a Writer without a Go writer did not raise")
	}
}
