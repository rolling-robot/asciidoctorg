package asciidoctorj

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenAnAsciidoctorClassIsInstantiated.

// assertRenderedSample checks the embeddable HTML of rendersample.asciidoc.
func assertRenderedSample(t *testing.T, out string) {
	t.Helper()
	doc := parseXML(t, out)
	for _, expr := range []string{"/div[@class='sect1']", "/div/h2[@id='_section_a']", "/div/div[@class='sectionbody']"} {
		if len(doc.XPath(expr)) == 0 {
			t.Errorf("no match for %s in %s", expr, out)
		}
	}
	eqv(t, "h2", doc.XPath("/div/h2").Text(), "Section A")
}

func TestAJConvertEmptySource(t *testing.T) {
	eqv(t, "output", mustConvert(t, instance(t), "", nil), "")
}

func TestAJConvertEmptySourceToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "expected_empty.html")
	out := mustConvert(t, instance(t), "", &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, ToFile: path})
	eqv(t, "output", out, "")
	eqv(t, "file exists", fileExists(path), true)
}

func TestAJConvertReaderNil(t *testing.T) {
	var w strings.Builder
	if err := instance(t).ConvertReader(nil, &w, nil); err == nil || !strings.Contains(err.Error(), "nil reader") {
		t.Errorf("err = %v", err)
	}
}

func TestAJConvertReaderToWriter(t *testing.T) {
	f, err := os.Open(ajResource(t, "rendersample.asciidoc"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var w strings.Builder
	if err := instance(t).ConvertReader(f, &w, nil); err != nil {
		t.Fatal(err)
	}
	assertRenderedSample(t, w.String())
}

func TestAJConvertFileToString(t *testing.T) {
	assertRenderedSample(t, mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), &asciidoctor.Options{NoOutputFile: true}))
}

func TestAJConvertFileInPlace(t *testing.T) {
	for _, tc := range []struct {
		name, ext string
		options   *asciidoctor.Options
	}{
		{"html", ".html", nil},
		{"docbook attribute", ".xml", &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.Backend("docbook")}},
		{"docbook option", ".xml", &asciidoctor.Options{Backend: "docbook"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := ajResource(t, "rendersample.asciidoc")
			eqv(t, "output", mustConvertFile(t, instance(t), src, tc.options), "")
			eqv(t, "file exists", fileExists(filepath.Join(filepath.Dir(src), "rendersample"+tc.ext)), true)
		})
	}
}

func TestAJConvertFileToDir(t *testing.T) {
	dir := t.TempDir()
	eqv(t, "output", mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), unsafeToDir(dir, nil)), "")
	eqv(t, "file exists", fileExists(filepath.Join(dir, "rendersample.html")), true)
}

func TestAJConvertFileFromBaseDir(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "asciidoc", "docs")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	got := mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"),
		&asciidoctor.Options{BaseDir: dir, ToFile: "asciidoc/docs/rendersample.html"})
	eqv(t, "output", got, "")
	eqv(t, "file exists", fileExists(filepath.Join(out, "rendersample.html")), true)
}

func TestAJCustomLocalDate(t *testing.T) {
	src := readFile(t, filepath.Join(ajResourceDir, "documentwithdate.asciidoc"))
	date := time.Date(2012, time.December, 5, 10, 0, 0, 0, time.Local)
	out := mustConvert(t, instance(t), src, &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.LocalDate(date)})
	eqv(t, "paragraph", parseXML(t, out).XPath("/div/div[@class='sectionbody']/div/p").Text(), "2012-12-05.")
}

func TestAJCustomLocalTime(t *testing.T) {
	src := readFile(t, filepath.Join(ajResourceDir, "documentwithtime.asciidoc"))
	now := time.Now()
	at := time.Date(now.Year(), now.Month(), now.Day(), 23, 15, 0, 0, time.Local)
	out := mustConvert(t, instance(t), src, &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.LocalTime(at)})
	eqv(t, "paragraph", parseXML(t, out).XPath("/div/div[@class='sectionbody']/div/p").Text(), at.Format("15:04:05 MST")+".")
}

func TestAJConvertStringSample(t *testing.T) {
	assertRenderedSample(t, mustConvert(t, instance(t), readFile(t, filepath.Join(ajResourceDir, "rendersample.asciidoc")), nil))
}

// AsciidoctorJ returns only the documents converted to strings; ConvertFiles
// returns one result per file, "" for each one written to a file.
func TestAJConvertFiles(t *testing.T) {
	src := ajResource(t, "rendersample.asciidoc")
	got, err := instance(t).ConvertFiles([]string{src}, &asciidoctor.Options{NoOutputFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] == "" {
		t.Errorf("results = %q", got)
	}
	got, err = instance(t).ConvertFiles([]string{src}, unsafeToDir(t.TempDir(), nil))
	if err != nil {
		t.Fatal(err)
	}
	eqv(t, "results written to files", got, []string{""})
}

func TestAJConvertDirectory(t *testing.T) {
	walker := asciidoctor.AsciiDocDirectoryWalker{BaseDir: filepath.Join(ajResources(t), "src")}
	got, err := instance(t).ConvertDirectory(walker, &asciidoctor.Options{NoOutputFile: true})
	if err != nil {
		t.Fatal(err)
	}
	eqv(t, "documents", len(got), 4)
	got, err = instance(t).ConvertDirectory(walker, unsafeToDir(t.TempDir(), nil))
	if err != nil {
		t.Fatal(err)
	}
	eqv(t, "results written to files", got, []string{"", "", "", ""})
}

func TestAJUnresolvableBackend(t *testing.T) {
	_, err := instance(t).ConvertFile(ajResource(t, "rendersample.asciidoc"), &asciidoctor.Options{Backend: "mybackend"})
	var e *asciidoctor.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *asciidoctor.Error", err)
	}
	if !strings.Contains(e.Message, "mybackend") {
		t.Errorf("message = %q", e.Message)
	}
}
