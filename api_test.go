package asciidoctor_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

var (
	runtimeDir string
	sharedOnce sync.Once
	shared     *asciidoctor.Asciidoctor
	sharedErr  error
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "asciidoctor-go-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runtimeDir = dir
	code := m.Run()
	if shared != nil {
		shared.Close()
	}
	os.RemoveAll(dir)
	os.Exit(code)
}

// instance returns an instance shared by the tests that do not change its
// configuration.
func instance(t *testing.T) *asciidoctor.Asciidoctor {
	t.Helper()
	sharedOnce.Do(func() { shared, sharedErr = asciidoctor.New(asciidoctor.WithRuntimeDir(runtimeDir)) })
	if sharedErr != nil {
		t.Fatal(sharedErr)
	}
	return shared
}

// newInstance returns a fresh instance, closed when the test ends.
func newInstance(t *testing.T, opts ...asciidoctor.Option) *asciidoctor.Asciidoctor {
	t.Helper()
	a, err := asciidoctor.New(append([]asciidoctor.Option{asciidoctor.WithRuntimeDir(runtimeDir)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func mustConvert(t *testing.T, a *asciidoctor.Asciidoctor, src string, o *asciidoctor.Options) string {
	t.Helper()
	out, err := a.Convert(src, o)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Expected values in this file were produced by Asciidoctor on MRI Ruby.

func TestConvertEmbedded(t *testing.T) {
	got := mustConvert(t, instance(t), "Hello {name}", &asciidoctor.Options{Attributes: asciidoctor.Attributes{"name": "World"}})
	if want := "<div class=\"paragraph\">\n<p>Hello World</p>\n</div>"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestConvertNilOptions(t *testing.T) {
	if got := mustConvert(t, instance(t), "*strong*", nil); got != "<div class=\"paragraph\">\n<p><strong>strong</strong></p>\n</div>" {
		t.Fatalf("got %q", got)
	}
}

func TestAttributePrecedence(t *testing.T) {
	a := instance(t)
	src := ":name: Doc\n\nHello {name}"
	for _, tc := range []struct {
		value any
		want  string
	}{
		{"API", "Hello API"},
		{"API@", "Hello Doc"}, // soft set: the document wins
		{false, "Hello Doc"},  // unset by the API, then set by the document
		{true, "Hello "},      // set to the empty string, overriding the document
	} {
		got := mustConvert(t, a, src, &asciidoctor.Options{Attributes: asciidoctor.Attributes{"name": tc.value}})
		if !strings.Contains(got, "<p>"+tc.want+"</p>") {
			t.Errorf("name=%v: got %q, want %q", tc.value, got, tc.want)
		}
	}
	got := mustConvert(t, a, "{answer} {flag}", &asciidoctor.Options{Attributes: asciidoctor.Attributes{"answer": 42, "flag": true}})
	if !strings.Contains(got, "<p>42 </p>") {
		t.Errorf("got %q", got)
	}
}

func TestConvertStandalone(t *testing.T) {
	got := mustConvert(t, instance(t), "= Title\n\ntext", &asciidoctor.Options{Standalone: asciidoctor.Bool(true)})
	if !strings.HasPrefix(got, "<!DOCTYPE html>\n<html lang=\"en\">") || !strings.Contains(got, "<title>Title</title>") {
		t.Fatalf("not a standalone document:\n%s", got)
	}
}

func TestBackendAndDoctype(t *testing.T) {
	a := instance(t)
	if got := mustConvert(t, a, "para", &asciidoctor.Options{Backend: "docbook5"}); got != "<simpara>para</simpara>" {
		t.Errorf("docbook5: got %q", got)
	}
	if got := mustConvert(t, a, "*strong*", &asciidoctor.Options{Doctype: "inline"}); got != "<strong>strong</strong>" {
		t.Errorf("inline: got %q", got)
	}
}

func TestSafeModes(t *testing.T) {
	a := instance(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "other.adoc"), "included text")
	src := "include::other.adoc[]"
	// The API default is secure: the include becomes a link.
	if got := mustConvert(t, a, src, &asciidoctor.Options{BaseDir: dir}); got != "<div class=\"paragraph\">\n<p><a href=\"other.adoc\" class=\"bare include\">other.adoc</a></p>\n</div>" {
		t.Errorf("secure: got %q", got)
	}
	if got := mustConvert(t, a, src, &asciidoctor.Options{Safe: asciidoctor.SafeModeSafe, BaseDir: dir}); got != "<div class=\"paragraph\">\n<p>included text</p>\n</div>" {
		t.Errorf("safe: got %q", got)
	}
	for mode, level := range map[asciidoctor.SafeMode]int{
		asciidoctor.SafeModeDefault: 20, asciidoctor.SafeModeUnsafe: 0, asciidoctor.SafeModeSafe: 1,
		asciidoctor.SafeModeServer: 10, asciidoctor.SafeModeSecure: 20,
	} {
		got := mustConvert(t, a, "{safe-mode-level} {safe-mode-name}", &asciidoctor.Options{Safe: mode})
		want := fmt.Sprintf("<p>%d %s</p>", level, mode)
		if !strings.Contains(got, want) {
			t.Errorf("%v: got %q, want %q", mode, got, want)
		}
	}
}

func TestConvertToWriter(t *testing.T) {
	var buf bytes.Buffer
	out, err := instance(t).Convert("text", &asciidoctor.Options{Output: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Convert returned %q while writing to Output", out)
	}
	if got := buf.String(); got != "<div class=\"paragraph\">\n<p>text</p>\n</div>\n" {
		t.Errorf("wrote %q", got)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestConvertToFailingWriter(t *testing.T) {
	_, err := instance(t).Convert("text", &asciidoctor.Options{Output: failingWriter{}})
	var e *asciidoctor.Error
	if !errors.As(err, &e) || e.Class != "IOError" || e.Message != "disk full" {
		t.Fatalf("got %#v", err)
	}
}

func TestConvertToFileAndDir(t *testing.T) {
	a := instance(t)
	dir := t.TempDir()
	out, err := a.Convert("= Doc\n\ntext", &asciidoctor.Options{ToFile: filepath.Join(dir, "out.html"), Safe: asciidoctor.SafeModeUnsafe})
	if err != nil || out != "" {
		t.Fatalf("got %q, %v", out, err)
	}
	if html := readFile(t, filepath.Join(dir, "out.html")); !strings.HasPrefix(html, "<!DOCTYPE html>") {
		t.Errorf("output file is not standalone: %q", html[:min(len(html), 80)])
	}
	nested := filepath.Join(dir, "a", "b")
	if _, err := a.Convert("text", &asciidoctor.Options{ToDir: nested, ToFile: "x.html", Safe: asciidoctor.SafeModeUnsafe}); err == nil {
		t.Error("expected an error for a missing target directory")
	} else if !strings.Contains(err.Error(), "target directory does not exist") {
		t.Errorf("unexpected error: %v", err)
	}
	if _, err := a.Convert("text", &asciidoctor.Options{ToDir: nested, ToFile: "x.html", MkDirs: true, Safe: asciidoctor.SafeModeUnsafe}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(nested, "x.html")); err != nil {
		t.Error(err)
	}
}

func TestConvertFile(t *testing.T) {
	a := instance(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.adoc")
	writeFile(t, in, "= Doc\n\ninclude::part.adoc[]")
	writeFile(t, filepath.Join(dir, "part.adoc"), "part text")

	// Default: written next to the input, standalone.
	out, err := a.ConvertFile(in, &asciidoctor.Options{Safe: asciidoctor.SafeModeSafe})
	if err != nil || out != "" {
		t.Fatalf("got %q, %v", out, err)
	}
	html := readFile(t, filepath.Join(dir, "doc.html"))
	if !strings.HasPrefix(html, "<!DOCTYPE html>") || !strings.Contains(html, "<p>part text</p>") {
		t.Errorf("unexpected output file:\n%s", html)
	}

	// NoOutputFile: returned, embeddable; includes resolve against the file.
	out, err = a.ConvertFile(in, &asciidoctor.Options{Safe: asciidoctor.SafeModeSafe, NoOutputFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if out != "<div class=\"paragraph\">\n<p>part text</p>\n</div>" {
		t.Errorf("got %q", out)
	}

	// ToDir with a different backend.
	outDir := filepath.Join(dir, "out")
	if _, err := a.ConvertFile(in, &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, Backend: "docbook5", ToDir: outDir, MkDirs: true}); err != nil {
		t.Fatal(err)
	}
	if xml := readFile(t, filepath.Join(outDir, "doc.xml")); !strings.HasPrefix(xml, "<?xml") {
		t.Errorf("unexpected docbook output: %q", xml[:min(len(xml), 80)])
	}

	if _, err := a.ConvertFile(filepath.Join(dir, "missing.adoc"), nil); err == nil {
		t.Error("expected an error for a missing input file")
	}
}

func TestConvertFilesAndDirectory(t *testing.T) {
	a := instance(t)
	dir := t.TempDir()
	for _, f := range []string{"a.adoc", "b.asciidoc", "sub/c.asc", "sub/d.ad", "_partial.adoc", "_skip/e.adoc", ".hidden/f.adoc", "notes.txt"} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(f)), "content of "+f)
	}
	files, err := asciidoctor.AsciiDocDirectoryWalker{BaseDir: dir}.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, f := range files {
		r, _ := filepath.Rel(dir, f)
		rel = append(rel, filepath.ToSlash(r))
	}
	if got, want := strings.Join(rel, " "), "a.adoc b.asciidoc sub/c.asc sub/d.ad"; got != want {
		t.Errorf("walker found %q, want %q", got, want)
	}
	outs, err := a.ConvertDirectory(asciidoctor.AsciiDocDirectoryWalker{BaseDir: dir}, &asciidoctor.Options{NoOutputFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 4 || outs[2] != "<div class=\"paragraph\">\n<p>content of sub/c.asc</p>\n</div>" {
		t.Errorf("got %q", outs)
	}
	if _, err := a.ConvertFiles([]string{files[0], filepath.Join(dir, "nope.adoc")}, &asciidoctor.Options{NoOutputFile: true}); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestGlobDirectoryWalker(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"docs/a.adoc", "docs/b.txt", "docs/guide/c.adoc", "docs/guide/deep/d.adoc", "other/e.adoc"} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(f)), "x")
	}
	root := filepath.ToSlash(dir)
	for _, tc := range []struct{ glob, want string }{
		{root + "/docs/*.adoc", "docs/a.adoc"},
		{root + "/docs/**/*.adoc", "docs/a.adoc docs/guide/c.adoc docs/guide/deep/d.adoc"},
		{root + "/**/*.adoc", "docs/a.adoc docs/guide/c.adoc docs/guide/deep/d.adoc other/e.adoc"},
		{root + "/docs/guide/?.adoc", "docs/guide/c.adoc"},
		{root + "/docs/**.adoc", "docs/a.adoc docs/guide/c.adoc docs/guide/deep/d.adoc"},
		{root + "/docs/a.adoc", "docs/a.adoc"},
	} {
		files, err := asciidoctor.GlobDirectoryWalker{Glob: tc.glob}.Scan()
		if err != nil {
			t.Fatalf("%s: %v", tc.glob, err)
		}
		var rel []string
		for _, f := range files {
			r, _ := filepath.Rel(dir, f)
			rel = append(rel, filepath.ToSlash(r))
		}
		if got := strings.Join(rel, " "); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.glob, got, tc.want)
		}
	}
	if _, err := (asciidoctor.GlobDirectoryWalker{Glob: root + "/missing/*.adoc"}).Scan(); err == nil {
		t.Error("expected an error for a missing root directory")
	}
	if _, err := (asciidoctor.GlobDirectoryWalker{Glob: root + "/docs/a.adoc/*.x"}).Scan(); err == nil {
		t.Error("expected an error for a root that is a file")
	}
}

func TestErrors(t *testing.T) {
	_, err := instance(t).Convert("x", &asciidoctor.Options{Backend: "nope"})
	var e *asciidoctor.Error
	if !errors.As(err, &e) {
		t.Fatalf("got %T %v", err, err)
	}
	if e.Class != "NotImplementedError" || e.Message != "asciidoctor: FAILED: missing converter for backend 'nope'. Processing aborted." {
		t.Errorf("got class %q message %q", e.Class, e.Message)
	}
	if e.Error() != e.Message || len(e.Backtrace) == 0 {
		t.Errorf("Error() = %q, backtrace %v", e.Error(), e.Backtrace)
	}
	// The instance stays usable after an error.
	mustConvert(t, instance(t), "ok", nil)
}

func TestClose(t *testing.T) {
	a := newInstance(t)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := a.Convert("x", nil); !errors.Is(err, asciidoctor.ErrClosed) {
		t.Fatalf("got %v, want ErrClosed", err)
	}
}

func TestVersions(t *testing.T) {
	v, err := instance(t).AsciidoctorVersion()
	if err != nil || v != "2.1.0.alpha.0" {
		t.Fatalf("got %q, %v", v, err)
	}
	if asciidoctor.Version == "" {
		t.Fatal("empty Version")
	}
}

func TestStderrReceivesWarnings(t *testing.T) {
	var stderr bytes.Buffer
	a := newInstance(t, asciidoctor.WithStderr(&stderr))
	mustConvert(t, a, "<<missing>>\n\nfootnote:x[]", nil)
	mustConvert(t, a, "[[a]]\n== A\n\n[[a]]\n== B", nil)
	want := "asciidoctor: WARNING: invalid footnote reference: x\n" +
		"asciidoctor: WARNING: <stdin>: line 5: id assigned to section already in use: a\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q", got)
	}
}

func TestConcurrentUse(t *testing.T) {
	a := instance(t)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := a.Convert(fmt.Sprintf("item %d", i), nil)
			if err == nil && !strings.Contains(out, fmt.Sprintf("<p>item %d</p>", i)) {
				err = fmt.Errorf("goroutine %d got %q", i, out)
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestPool(t *testing.T) {
	setups := 0
	p, err := asciidoctor.NewPool(3, func(a *asciidoctor.Asciidoctor) error { setups++; return nil }, asciidoctor.WithRuntimeDir(runtimeDir))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if setups != 3 {
		t.Errorf("setup ran %d times", setups)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := p.Convert(fmt.Sprintf("n %d", i), nil)
			if err != nil || !strings.Contains(out, fmt.Sprintf("<p>n %d</p>", i)) {
				t.Errorf("got %q, %v", out, err)
			}
		}(i)
	}
	wg.Wait()
	if _, err := asciidoctor.NewPool(0, nil); err == nil {
		t.Error("expected an error for size 0")
	}
	failing := errors.New("setup failed")
	if _, err := asciidoctor.NewPool(1, func(*asciidoctor.Asciidoctor) error { return failing }, asciidoctor.WithRuntimeDir(runtimeDir)); !errors.Is(err, failing) {
		t.Errorf("got %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Convert("x", nil); !errors.Is(err, asciidoctor.ErrClosed) {
		t.Errorf("got %v, want ErrClosed", err)
	}
}

func TestPackageLevelConvert(t *testing.T) {
	out, err := asciidoctor.Convert("_emphasis_", nil)
	if err != nil || out != "<div class=\"paragraph\">\n<p><em>emphasis</em></p>\n</div>" {
		t.Fatalf("got %q, %v", out, err)
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "p.adoc"), "file text")
	out, err = asciidoctor.ConvertFile(filepath.Join(dir, "p.adoc"), &asciidoctor.Options{NoOutputFile: true})
	if err != nil || out != "<div class=\"paragraph\">\n<p>file text</p>\n</div>" {
		t.Fatalf("got %q, %v", out, err)
	}
}
