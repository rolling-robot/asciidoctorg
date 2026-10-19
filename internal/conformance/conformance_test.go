// Package conformance checks that Asciidoctor on rbgo reproduces, byte for
// byte, the output and log messages of Asciidoctor on MRI Ruby for every
// document selected by testdata/corpus/convert.rb. Regenerate the goldens with
// scripts/gen-goldens.rb.
package conformance

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor/internal/runtime"
)

type entry struct{ path, backend string }

func readManifest(t *testing.T, golden string) []entry {
	t.Helper()
	f, err := os.Open(filepath.Join(golden, "MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var entries []entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		path, backend, ok := strings.Cut(sc.Text(), " ")
		if !ok {
			t.Fatalf("malformed manifest line %q", sc.Text())
		}
		entries = append(entries, entry{path, backend})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

// firstDifference describes where got and want first diverge.
func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl || i >= len(g) || i >= len(w) {
			return "line " + strconv.Itoa(i+1) + ":\n  got:  " + quote(gl, i < len(g)) + "\n  want: " + quote(wl, i < len(w))
		}
	}
	return "identical"
}

func quote(s string, present bool) string {
	if !present {
		return "<end of text>"
	}
	return "\"" + s + "\""
}

func TestCorpusMatchesMRI(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join(repo, "testdata", "golden")
	rt, err := runtime.Boot(runtime.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	in := rt.Interp
	if err := in.Set("__corpus_driver", filepath.Join(repo, "testdata", "corpus", "convert.rb")); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Eval("load __corpus_driver", "conformance_test.rb", 1); err != nil {
		t.Fatal(err)
	}
	if err := in.Set("__corpus_root", repo); err != nil {
		t.Fatal(err)
	}
	entries := readManifest(t, golden)
	if len(entries) == 0 {
		t.Fatal("empty manifest")
	}
	for _, e := range entries {
		t.Run(e.path+"/"+e.backend, func(t *testing.T) {
			if err := in.Set("__corpus_rel", e.path); err != nil {
				t.Fatal(err)
			}
			if err := in.Set("__corpus_backend", e.backend); err != nil {
				t.Fatal(err)
			}
			v, err := in.Eval("CorpusConvert.convert __corpus_root, __corpus_rel, __corpus_backend", "conformance_test.rb", 1)
			if err != nil {
				t.Fatal(err)
			}
			pair, ok := v.Go().([]any)
			if !ok || len(pair) != 2 {
				t.Fatalf("unexpected result %s", v.Inspect())
			}
			gotOut, _ := pair[0].(string)
			gotLog, _ := pair[1].(string)
			base := filepath.Join(golden, filepath.FromSlash(e.path)+"."+e.backend)
			wantOut, err := os.ReadFile(base + ".out")
			if err != nil {
				t.Fatal(err)
			}
			wantLog, err := os.ReadFile(base + ".log")
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if gotOut != string(wantOut) {
				t.Errorf("output differs from MRI at %s", firstDifference(gotOut, string(wantOut)))
			}
			if gotLog != string(wantLog) {
				t.Errorf("log differs from MRI at %s", firstDifference(gotLog, string(wantLog)))
			}
		})
	}
}
