package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	sharedOnce sync.Once
	shared     *Runtime
	sharedErr  error
	sharedDir  string
)

// bootShared boots one runtime for the package's tests.
func bootShared(t *testing.T) *Runtime {
	t.Helper()
	sharedOnce.Do(func() {
		if sharedDir, sharedErr = os.MkdirTemp("", "asciidoctor-go-runtime-test-"); sharedErr != nil {
			return
		}
		shared, sharedErr = Boot(Config{Dir: sharedDir})
	})
	if sharedErr != nil {
		t.Fatalf("boot: %v", sharedErr)
	}
	return shared
}

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedDir != "" {
		os.RemoveAll(sharedDir)
	}
	os.Exit(code)
}

// eval evaluates src and returns the inspected result.
func eval(t *testing.T, rt *Runtime, src string) string {
	t.Helper()
	v, err := rt.Interp.Eval(src, "runtime_test.rb", 1)
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return v.Inspect()
}

// convert converts AsciiDoc source with default (embedded) options.
func convert(t *testing.T, rt *Runtime, src string) string {
	t.Helper()
	if err := rt.Interp.Set("__src", src); err != nil {
		t.Fatal(err)
	}
	v, err := rt.Interp.Eval("Asciidoctor.convert __src, safe: :safe", "runtime_test.rb", 1)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	s, ok := v.Go().(string)
	if !ok {
		t.Fatalf("convert returned %s", v.Inspect())
	}
	return s
}

func TestBootLoadsAsciidoctor(t *testing.T) {
	rt := bootShared(t)
	if got := eval(t, rt, "Asciidoctor::VERSION"); got != `"2.1.0.alpha.0"` {
		t.Fatalf("VERSION = %s", got)
	}
	if got := eval(t, rt, "Asciidoctor::Extensions.respond_to? :register"); got != "true" {
		t.Fatalf("extensions not loaded: %s", got)
	}
	want := filepath.Join(rt.Root, "asciidoctor")
	if got := eval(t, rt, "Asciidoctor::ROOT_DIR"); got != `"`+want+`"` {
		t.Fatalf("ROOT_DIR = %s, want %q", got, want)
	}
}

func TestConvertParagraph(t *testing.T) {
	rt := bootShared(t)
	got := convert(t, rt, "A *bold* and _emphasized_ paragraph.")
	want := "<div class=\"paragraph\">\n<p>A <strong>bold</strong> and <em>emphasized</em> paragraph.</p>\n</div>"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestMaterializeIsIdempotentAndComplete(t *testing.T) {
	dir := t.TempDir()
	a, err := Materialize(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Materialize(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("roots differ: %s vs %s", a, b)
	}
	for _, p := range []string{completeMarker, "asciidoctor/lib/asciidoctor.rb", "asciidoctor/data/stylesheets/asciidoctor-default.css", "shims/asciidoctor_go_prelude.rb"} {
		if _, err := os.Stat(filepath.Join(a, filepath.FromSlash(p))); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only the tree in %s, found %d entries", dir, len(entries))
	}
}

func TestPatchesApplied(t *testing.T) {
	rt := bootShared(t)
	src, err := os.ReadFile(filepath.Join(rt.Root, "asciidoctor/lib/asciidoctor/parser.rb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(src), "\n") {
		if strings.TrimSpace(line) == "redo" {
			t.Fatal("parser.rb still contains redo")
		}
	}
}

func TestApplyPatchesDetectsDrift(t *testing.T) {
	if _, err := applyPatches("asciidoctor/lib/asciidoctor/parser.rb", []byte("no such snippet")); err == nil {
		t.Fatal("expected an error when a patch no longer matches")
	}
	if got, err := applyPatches("other.rb", []byte("redo")); err != nil || string(got) != "redo" {
		t.Fatalf("unpatched file changed: %q, %v", got, err)
	}
}

// The patched redo loops parse table cells that span a delimiter. Expected
// output was produced by Asciidoctor on MRI.
func TestTableDelimiterContinuation(t *testing.T) {
	rt := bootShared(t)
	cell := func(s string) string {
		return `<td class="tableblock halign-left valign-top"><p class="tableblock">` + s + `</p></td>`
	}
	tests := []struct {
		name, src string
		cells     []string
	}{
		{"csv quoted delimiter", ",===\n\"a,b\",c\n\"d\n,e\",f\n,===\n", []string{"a,b", "c", "d\n,e", "f"}},
		{"dsv escaped delimiter", "[format=dsv]\n|===\na\\:b:c\n|===\n", []string{"a:b", "c"}},
		{"psv escaped delimiter", "|===\n|a\\|b |c\n|===\n", []string{"a|b", "c"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convert(t, rt, tc.src)
			for _, c := range tc.cells {
				if !strings.Contains(got, cell(c)) {
					t.Errorf("missing cell %q in\n%s", c, got)
				}
			}
			if n := strings.Count(got, "<td "); n != len(tc.cells) {
				t.Errorf("got %d cells, want %d", n, len(tc.cells))
			}
		})
	}
}

// Each shim is checked against the behavior of MRI Ruby 3.4.
func TestShims(t *testing.T) {
	rt := bootShared(t)
	tests := []struct{ name, src, want string }{
		{"cgi/escape", `require 'cgi/escape'; CGI.escapeHTML %(<a href="x">&'</a>)`, `"&lt;a href=&quot;x&quot;&gt;&amp;&#39;&lt;/a&gt;"`},
		{"Gem.loaded_specs", `Gem.loaded_specs.key? 'logger'`, "true"},
		{"get_byte multibyte and unscan", `
s = StringScanner.new "éa,b"
r = []
r << s.get_byte << s.pos
s.unscan; r << s.pos
r << s.get_byte << s.get_byte << s.get_byte << s.matched << s.pos
s.unscan; r << s.pos
r << s.scan(/a/) << s.get_byte << s.get_byte << s.get_byte << s.get_byte
r`, `["\xC3", 1, 0, "\xC3", "\xA9", "a", "a", 3, 2, "a", ",", "b", nil, nil]`},
		{"get_byte then scan clears byte unscan", `
s = StringScanner.new "é,"
s.get_byte
s.pos = 2
s.scan(/,/)
s.unscan
s.pos`, "2"},
		{"File#mtime", `File.open(Asciidoctor::LIB_DIR + '/asciidoctor.rb') {|f| f.mtime == File.mtime(f.path) && Time === f.mtime }`, "true"},
		{"Module#=== with extend", `
m = Module.new
s = String.new('+').extend m
[m === s, (case s when m then :match else :none end), m === String.new('+')]`, "[true, :match, false]"},
		{"Module#=== without extend unchanged", `[Comparable === 1, Enumerable === [], Kernel === Object.new, Comparable === Object.new]`, "[true, true, true, false]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := eval(t, rt, tc.src); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestListContinuation(t *testing.T) {
	rt := bootShared(t)
	got := convert(t, rt, "* item\n+\nattached\n")
	want := "<div class=\"ulist\">\n<ul>\n<li>\n<p>item</p>\n<div class=\"paragraph\">\n<p>attached</p>\n</div>\n</li>\n</ul>\n</div>"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}
