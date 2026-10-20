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
		{"get_byte scans multibyte characters whole", `
s = StringScanner.new "éa,b"
r = []
r << s.get_byte << s.pos
s.unscan; r << s.pos
r << s.get_byte << s.get_byte << s.get_byte << s.matched << s.pos
s.unscan; r << s.pos
r << s.scan(/a/) << s.get_byte << s.get_byte << s.get_byte
r`, `["é", 2, 0, "é", "a", ",", ",", 4, 3, nil, ",", "b", nil]`},
		{"get_byte consumes an invalid byte alone", `
s = StringScanner.new "\xFFa".dup.force_encoding('UTF-8')
r = [s.get_byte.bytes, s.pos]
s.unscan
r << s.pos << s.get_byte.bytes << s.get_byte << s.pos`, "[[255], 1, 0, [255], \"a\", 2]"},
		{"scan after an invalid byte clears its unscan", `
s = StringScanner.new "\xFF,".dup.force_encoding('UTF-8')
s.get_byte
s.scan(/,/)
s.unscan
s.pos`, "1"},
		{"get_byte keeps later scans working", `
s = StringScanner.new "¦x,y"
[s.get_byte, s.scan(/[^,]+/), s.get_byte, s.rest]`, `["¦", "x", ",", "y"]`},
		{"StringScanner subclasses scan", `
class AgoxScanner < StringScanner
  def initialize source, extra
    super source
    @extra = extra
  end
  def extra = @extra
end
class AgoxSubScanner < AgoxScanner; end
s = AgoxSubScanner.new "abc", 7
r = [s.class, s.is_a?(StringScanner), s.extra, s.scan(/a/), s.pos, s.check(/b/), s.rest]
s.string = "zz"
r << s.scan(/z+/) << s.eos? << StringScanner.new("q").class
r`, `[AgoxSubScanner, true, 7, "a", 1, "b", "bc", "zz", true, StringScanner]`},
		{"Hash#dup and #clone keep the default", `
h = Hash.new(5)
def h.zz = :zz
pr = Hash.new {|_, k| k * 2 }
[h.dup[:x], h.clone[:x], h.clone.zz, h.freeze.clone.frozen?, h.clone[:x], pr.dup['ab'], pr.clone['ab'], Hash.new(1).dup.default_proc]`,
			`[5, 5, :zz, true, 5, "abab", "abab", nil]`},
		{"Hash and Array subclasses dup and clone", `
class AgoxHash < Hash
  def initialize(default = false) = super(default)
end
class AgoxArray < Array; end
w = AgoxHash.new(:none)
w['a'] = 1
w.instance_variable_set :@iv, 2
d = w.dup
d['b'] = 3
c = w.clone
c['c'] = 4
a = AgoxArray.new([1]).dup
a << 2
[d.class, d['a'], d['b'], d['zz'], d.instance_variable_get(:@iv), w.key?('b'), c.class, c['c'], a.class, a, AgoxArray.new([1]).clone.class]`,
			`[AgoxHash, 1, 3, :none, 2, false, AgoxHash, 4, AgoxArray, [1, 2], AgoxArray]`},
		{"StringScanner derived methods", `
s = StringScanner.new "abc def"
[s.rest?, s.restsize, s.peep(2), s.peek_byte, s.exist?(/d/), s.pos, s.search_full(/c/, true, true), s.pos,
 s.scan_full(/ d/, false, false), s.pos, s.getbyte, s.scan(/.*/), s.rest?]`,
			`[true, 7, "ab", 97, 5, 0, "abc", 3, 2, 3, " ", "def", false]`},
		{"file times are local", `
f = Asciidoctor::LIB_DIR + '/asciidoctor.rb'
times = [File.mtime(f), File.atime(f), File.ctime(f), File.stat(f).mtime, File.open(f, &:mtime)]
[times.map {|t| t.utc_offset == Time.now.utc_offset }.uniq, File.mtime(f) == File.stat(f).mtime]`, "[[true], true]"},
		{"OptionParser help after a banner ending with a newline", `
o = OptionParser.new {|opts| opts.banner = %(Usage: x\n\n); opts.on('-b', 'set') {} }
[o.help, o.to_s, o.banner]`, `["Usage: x\n\n    -b                               set\n", "Usage: x\n\n    -b                               set\n", "Usage: x\n\n"]`},
		{"File#mtime", `File.open(Asciidoctor::LIB_DIR + '/asciidoctor.rb') {|f| f.mtime == File.mtime(f.path) && Time === f.mtime }`, "true"},
		{"Module#=== with extend", `
m = Module.new
s = String.new('+').extend m
[m === s, (case s when m then :match else :none end), m === String.new('+')]`, "[true, :match, false]"},
		{"NotImplementedError is a ScriptError", `
r = [NotImplementedError.superclass]
begin; raise NotImplementedError, 'x'; rescue => e; r << :standard; rescue ScriptError => e; r << :script; end
begin; File.birthtime(Asciidoctor::LIB_DIR + '/asciidoctor.rb'); rescue => e; r << e.class; rescue NotImplementedError => e; r << :native_script; end
r`, "[ScriptError, :script, :native_script]"},
		{"Class.new calls inherited before the body", `
base = Class.new
seen = []
base.define_singleton_method(:inherited) {|sub| seen << sub.instance_variable_get(:@body) }
sub = Class.new(base) { @body = :ran; def hi = :hi }
named = Class.new(base)
[seen, sub.new.hi, sub.superclass == base, Class.new.superclass, (Class.new {|k| @arg = k }).instance_variable_get(:@arg).class]`, "[[nil, nil], :hi, true, Object, Class]"},
		{"full_message includes causes", `
begin
  begin
    raise ArgumentError, 'inner cause'
  rescue ArgumentError
    raise IOError, 'outer'
  end
rescue IOError => e
  m = e.full_message(highlight: false)
  [m.include?('outer (IOError)'), m.include?('inner cause (ArgumentError)'), (m.index 'outer') < (m.index 'inner cause')]
end`, "[true, true, true]"},
		{"rstrip rejects invalid byte sequences", `
s = "a\xFF ".dup.force_encoding('UTF-8')
[(begin; s.rstrip; rescue Encoding::CompatibilityError => e; e.message; end), "ok  ".rstrip, " x ".strip, "\xFF ".b.rstrip]`, `["invalid byte sequence in UTF-8", "ok", "x", "\xFF"]`},
		{"open-uri via autoload and require", `
module AgoxAutoloadProbe; autoload :OpenURI, 'open-uri'; end
[(require 'open-uri'), ::OpenURI.respond_to?(:open_uri), (begin; ::OpenURI.open_uri('ftp://x'); rescue ArgumentError => e; e.message; end)]`, `[true, true, "open-uri: unsupported scheme for ftp://x"]`},
		{"Asciidoctor's top-level autoloads resolve", `Asciidoctor::Helpers.require_open_uri; (Module.new { def self.get = [::OpenURI, ::Pathname, ::StringScanner, ::URI] }).get.map(&:name)`, `["OpenURI", "Pathname", "StringScanner", "URI"]`},
		{"URI with a replaced path", `[AsciidoctorGoURI.with_path(URI('https://u:p@example.org:8443/a/b.adoc?x=1#f'), '/a').to_s, AsciidoctorGoURI.with_path(URI('http://example.org/b.adoc'), '').to_s]`, `["https://u:p@example.org:8443/a?x=1#f", "http://example.org"]`},
		{"OptionParser skips nil and reports the invalid value", `
r = []
o = OptionParser.new {|op| op.on('-o FILE') {|v| r << v }; op.on('-d TYPE', %w(article book)) {|v| r << v } }
a = ['-o', 'x', nil, 'y']
r << o.parse!(a) << o.parse(['-o', 'z', nil])
begin; o.parse! %w(-d chapter in.adoc); rescue OptionParser::InvalidArgument => e; r << e.message; end
r`, `["x", ["y"], "z", [], "invalid argument: -d chapter"]`},
		{"FileUtils.rmdir and Pathname", `
require 'tmpdir'
Dir.mktmpdir do |d|
  FileUtils.mkdir_p File.join(d, 'a', 'b')
  FileUtils.rmdir File.join(d, 'a', 'b')
  p1 = Pathname(File.join d, 'a')
  [Dir.exist?(File.join d, 'a', 'b'), p1.directory?, p1.empty?, Pathname(p1).equal?(p1), (p1 + 'x').expand_path.basename.to_s, p1.children]
end`, "[false, true, true, true, \"x\", []]"},
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

// The logger shim replaces rbgo's native Logger, which cannot be subclassed.
// Expected values come from MRI Ruby 3.4 with logger 1.6.
func TestLoggerShim(t *testing.T) {
	rt := bootShared(t)
	tests := []struct{ name, src, want string }{
		{"memory logger collects messages", `
logger = Asciidoctor::MemoryLogger.new
logger.warn 'careful'
logger.error { 'broken' }
[logger.class, logger.messages, logger.max_severity]`, `[Asciidoctor::MemoryLogger, [{severity: :WARN, message: "careful"}, {severity: :ERROR, message: "broken"}], 3]`},
		{"asciidoctor logger format", `
io = StringIO.new
logger = Asciidoctor::Logger.new io
logger.info 'hidden'
logger.warn 'shown'
logger.fatal 'stop'
[io.string, logger.max_severity, logger.level]`, `["asciidoctor: WARNING: shown\nasciidoctor: FAILED: stop\n", 4, 2]`},
		{"null logger", `l = Asciidoctor::NullLogger.new; l.error 'x'; [l.class, l.max_severity]`, "[Asciidoctor::NullLogger, 3]"},
		{"default formatter", `Logger::Formatter.new.call('WARN', Time.at(1600000000, 123456, :usec).utc, 'prog', 'msg').sub(/#\d+/, '#PID')`, `"W, [2020-09-13T12:26:40.123456 #PID]  WARN -- prog: msg\n"`},
		{"level coercion", `l = Logger.new nil; l.level = :error; r = [l.level]; begin; l.level = 'bogus'; rescue ArgumentError; r << $!.message; end; l.with_level(:debug) { r << l.level }; r << l.level`, `[3, "invalid log level: bogus", 0, 3]`},
		{"severity constants", `Logger::Severity.constants.sort`, "[:DEBUG, :ERROR, :FATAL, :INFO, :UNKNOWN, :WARN]"},
		{"require logger is the shim", `require 'logger'; [Logger::VERSION, Logger::ProgName]`, `["1.6.4", "logger.rb/v1.6.4"]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := eval(t, rt, tc.src); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// rbgo mis-parses escapes in character literals; the patched constants must
// hold the control characters Asciidoctor uses as placeholders.
func TestCharLiteralConstants(t *testing.T) {
	rt := bootShared(t)
	got := eval(t, rt, `[Asciidoctor::Substitutors::CAN, Asciidoctor::Substitutors::DEL, Asciidoctor::Substitutors::PASS_START, Asciidoctor::Substitutors::PASS_END, (require 'asciidoctor/converter/manpage'; Asciidoctor::Converter::ManPageConverter::ESC)].map(&:bytes)`)
	if want := "[[24], [127], [194, 150], [194, 151], [27]]"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	// The set directive drops its placeholder with String#delete DEL.
	html := convert(t, rt, "{set:name:value} uses {name} but")
	if want := "<div class=\"paragraph\">\n<p> uses value but</p>\n</div>"; html != want {
		t.Fatalf("got %q, want %q", html, want)
	}
}

// Rouge is the go-rouge-backed shim: Asciidoctor's Rouge adapter highlights
// with it, including RougeExt's line numbering.
func TestRougeShim(t *testing.T) {
	rt := bootShared(t)
	html := eval(t, rt, `Asciidoctor.convert %([source%linenums,ruby]\n----\nputs 'hi'\n----), attributes: { 'source-highlighter' => 'rouge', 'rouge-css' => 'class' }`)
	for _, want := range []string{`<span class=\"nb\">puts</span>`, `<span class=\"s1\">'hi'</span>`, `<table class=\"linenotable\">`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in %s", want, html)
		}
	}
	if got := eval(t, rt, `[Rouge::Lexer.find('rb').tag, Rouge::Lexer.find_fancy('php?start_inline=true').options, Rouge::Lexer.find('nope'), Rouge::Theme.find('github').base_style.keys.sort, (Rouge::Theme.find('github').render scope: 'pre.rouge').start_with?('pre.rouge table td { padding: 5px; }')]`); got != `["ruby", {"start_inline" => "true"}, nil, [:bg, :fg], true]` {
		t.Errorf("api: %s", got)
	}
	if got := eval(t, rt, `Rouge::Lexer.find_fancy('console?prompt=$>').lex("$> ls\nout\n").map {|t, v| [t.qualname, v] }`); !strings.HasPrefix(got, `[["Generic.Prompt", "$>"]`) || !strings.Contains(got, `["Generic.Output", "out\n"]`) {
		t.Errorf("console: %s", got)
	}
}

// Two of Asciidoctor's regexes use a backreference to a group that may not
// participate, which rbgo's engine matches as empty; the patched regexes must
// give MRI's results. Expected output was produced by Asciidoctor on MRI.
func TestRegexBackreferencePatches(t *testing.T) {
	rt := bootShared(t)
	for src, want := range map[string]string{
		"https://github.com/<org>/ and &lt;https://example.org&gt;": "<div class=\"paragraph\">\n<p><a href=\"https://github.com/&lt;org&gt;/\" class=\"bare\">https://github.com/&lt;org&gt;/</a> and <a href=\"https://example.org\" class=\"bare\">https://example.org</a></p>\n</div>",
		"+ x-]+y+ and [x-]+z+": "<div class=\"paragraph\">\n<p>+ x-]y and <code>z</code></p>\n</div>",
	} {
		if got := convert(t, rt, src); got != want {
			t.Errorf("%q:\ngot  %q\nwant %q", src, got, want)
		}
	}
}

// rbgo's $~ is not frame-local; Asciidoctor's methods are wrapped so a match
// made in a called method does not replace the caller's. In the manpage NAME
// section, $2 is read after sub_attributes (which matches) has run on $1.
func TestFrameLocalLastMatch(t *testing.T) {
	rt := bootShared(t)
	got := eval(t, rt, `
doc = Asciidoctor.load "= {cmdname}(1)\n:doctype: manpage\n\n== NAME\n\n{cmdname} - {cmdname} puts the foo in your bar", backend: :manpage, attributes: { 'cmdname' => 'foobar' }
[doc.attr('manname'), doc.attr('manpurpose'), (/(z)/ =~ 'z'; Asciidoctor::Helpers.basename('a/b.adoc', true); $1)]`)
	if want := `["foobar", "foobar puts the foo in your bar", "z"]`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// Only methods that can set $~ are wrapped, and which ones is cached in the
// runtime directory; a boot that reads the cache wraps the same methods as
// the one that scanned the sources.
func TestFrameLocalCache(t *testing.T) {
	dir := t.TempDir()
	const query = `[AsciidoctorGoFrameLocalMatch::WRAPPED_LOCATIONS.sort, AsciidoctorGoFrameLocalMatch.instance_variable_get(:@needed).nil?]`
	var results []string
	for range 2 {
		rt, err := Boot(Config{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, eval(t, rt, query))
	}
	scanned, cached := strings.TrimSuffix(results[0], ", true]"), strings.TrimSuffix(results[1], ", false]")
	if scanned == results[0] || cached == results[1] {
		t.Fatalf("first boot should scan and second read the cache:\n%.200s\n%.200s", results[0], results[1])
	}
	if scanned != cached {
		t.Error("cached boot wraps different methods")
	}
	if !strings.Contains(scanned, `"asciidoctor/substitutors.rb:`) || strings.Count(scanned, ",") < 100 {
		t.Errorf("unexpected wrapped methods: %.300s", scanned)
	}
}
