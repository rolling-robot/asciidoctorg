package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// fakeAsciidoctor writes an Asciidoctor tree whose lib/ holds the given files
// and returns its root and the path of a marker file that an at_exit handler,
// registered by lib/asciidoctor.rb before anything else runs, writes when the
// interpreter is closed.
func fakeAsciidoctor(t *testing.T, files map[string]string) (dir, marker string) {
	t.Helper()
	dir = t.TempDir()
	marker = filepath.Join(dir, "closed")
	for name, body := range files {
		if name == "asciidoctor.rb" {
			body = fmt.Sprintf("at_exit { File.write %q, 'closed' }\n", marker) + body
		}
		p := filepath.Join(dir, "lib", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, marker
}

// D5: Boot left the interpreter it had created running when loading
// Asciidoctor failed.
func TestBootClosesInterpreterOnError(t *testing.T) {
	dir, marker := fakeAsciidoctor(t, map[string]string{"asciidoctor.rb": "raise 'broken asciidoctor'\n"})
	if _, err := Boot(Config{Dir: t.TempDir(), AsciidoctorDir: dir}); err == nil {
		t.Fatal("Boot succeeded with a broken Asciidoctor")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the interpreter was not closed after the failed boot: %v", err)
	}
}
