package asciidoctor_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// runCLI runs the command line on a fresh instance and returns the exit code,
// standard output and standard error.
func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	a := newInstance(t, asciidoctor.WithStdout(&stdout), asciidoctor.WithStderr(&stderr))
	code, err := a.RunCLI(args, strings.NewReader(stdin))
	if err != nil {
		t.Fatal(err)
	}
	return code, stdout.String(), stderr.String()
}

func TestRunCLIVersion(t *testing.T) {
	code, out, _ := runCLI(t, "", "--version")
	eqv(t, "code", code, 0)
	if !strings.HasPrefix(out, "Asciidoctor 2.1.0") {
		t.Errorf("output = %q", out)
	}
}

func TestRunCLIStdin(t *testing.T) {
	code, out, errOut := runCLI(t, "= Hi\n\n*bold* text\n", "-s", "-o", "-", "-")
	eqv(t, "code", code, 0)
	eqv(t, "stderr", errOut, "")
	eqv(t, "stdout", out, "<div class=\"paragraph\">\n<p><strong>bold</strong> text</p>\n</div>\n")
}

func TestRunCLIFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.adoc")
	writeFile(t, src, "= Doc\n\nversion {revnumber}\n")
	code, _, _ := runCLI(t, "", "-b", "docbook", "-a", "revnumber=2", src)
	eqv(t, "code", code, 0)
	if out := readFile(t, filepath.Join(dir, "doc.xml")); !strings.Contains(out, "<simpara>version 2</simpara>") {
		t.Errorf("output = %s", out)
	}
}

func TestRunCLIFailures(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nosuch.adoc")
	code, _, errOut := runCLI(t, "", missing)
	eqv(t, "missing input code", code, 1)
	eqv(t, "missing input message", errOut, "asciidoctor: FAILED: input file "+missing+" is missing\n")

	src := filepath.Join(dir, "doc.adoc")
	writeFile(t, src, "include::nope.adoc[]\n")
	code, _, errOut = runCLI(t, "", src)
	eqv(t, "warning code", code, 0)
	if !strings.Contains(errOut, "include file not found") {
		t.Errorf("stderr = %q", errOut)
	}
	code, _, _ = runCLI(t, "", "--failure-level", "WARN", src)
	eqv(t, "failure level code", code, 1)

	code, _, errOut = runCLI(t, "", "--bogus")
	eqv(t, "bad option code", code, 1)
	if !strings.HasPrefix(errOut, "asciidoctor: invalid option: --bogus\n") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunCLIRequire(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "shout.rb")
	writeFile(t, ext, "Asciidoctor::Extensions.register { inline_macro(:shout) { process {|p, t| create_inline p, :quoted, t.upcase } } }\n")
	code, out, _ := runCLI(t, "say shout:hi[]\n", "-r", ext, "-s", "-o", "-", "-")
	eqv(t, "code", code, 0)
	if !strings.Contains(out, "say HI") {
		t.Errorf("output = %s", out)
	}
}

func TestRunCLIHelp(t *testing.T) {
	code, out, _ := runCLI(t, "", "-h")
	eqv(t, "code", code, 0)
	if !strings.HasPrefix(out, "Usage: asciidoctor [OPTION]... FILE...\n") || !strings.Contains(out, "Example: asciidoctor input.adoc\n\n    -b, --backend") {
		t.Errorf("help = %q", out)
	}
}
