package asciidoctor_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// corpusAttributes mirrors CorpusConvert::ATTRIBUTES in
// testdata/corpus/convert.rb, which produced the goldens.
var corpusAttributes = asciidoctor.Attributes{
	"linkcss":       "",
	"docdate":       "2020-01-01",
	"doctime":       "00:00:00 +0000",
	"docdatetime":   "2020-01-01 00:00:00 +0000",
	"docyear":       "2020",
	"localdate":     "2020-01-01",
	"localtime":     "00:00:00 +0000",
	"localdatetime": "2020-01-01 00:00:00 +0000",
	"localyear":     "2020",
}

// TestCorpusThroughAPI converts every corpus document through the public API
// and compares the output with the golden rendered by Asciidoctor on MRI.
// (internal/conformance also compares the log messages.)
func TestCorpusThroughAPI(t *testing.T) {
	repo, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join(repo, "testdata", "golden")
	f, err := os.Open(filepath.Join(golden, "MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	a := newInstance(t, asciidoctor.WithStderr(nil))
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		rel, backend, _ := strings.Cut(sc.Text(), " ")
		n++
		t.Run(rel+"/"+backend, func(t *testing.T) {
			out, err := a.ConvertFile(filepath.Join(repo, filepath.FromSlash(rel)), &asciidoctor.Options{
				Safe:         asciidoctor.SafeModeSafe,
				Backend:      backend,
				Standalone:   asciidoctor.Bool(true),
				NoOutputFile: true,
				Attributes:   corpusAttributes,
			})
			if err != nil {
				out = "ERROR " + err.Error()
			}
			out = strings.ReplaceAll(out, repo, "{repo}")
			want := readFile(t, filepath.Join(golden, filepath.FromSlash(rel)+"."+backend+".out"))
			if out != want {
				t.Errorf("output differs from MRI (%d vs %d bytes)", len(out), len(want))
			}
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("empty manifest")
	}
}
