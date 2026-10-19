package asciidoctor_test

import (
	"path/filepath"
	"strings"
	"testing"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/asciidoctor-go/asciidoctor"
)

func TestRubyProcessorByClassName(t *testing.T) {
	a := newInstance(t)
	path, _ := filepath.Abs("testdata/ruby/shout_macro.rb")
	check(t, a.RequireLibrary(path))
	g, err := a.Extensions().RubyProcessor(asciidoctor.KindBlockMacro, "ShoutBlockMacro", "")
	check(t, err)
	eq(t, "output", mustConvert(t, a, "shout::hello[role=big]", nil), "<div class=\"paragraph big\">\n<p>HELLO!</p>\n</div>")
	// Registered again under another name.
	_, err = a.Extensions().RubyProcessor(asciidoctor.KindBlockMacro, "ShoutBlockMacro", "yell")
	check(t, err)
	mustContain(t, mustConvert(t, a, "yell::hi[]", nil), "<p>HI!</p>")
	check(t, g.Unregister())
	mustContain(t, mustConvert(t, a, "shout::hello[]", nil), "shout::hello[]")
}

func TestRubyExtensionsRegisteringThemselves(t *testing.T) {
	a := newInstance(t)
	path, _ := filepath.Abs("testdata/ruby/registers_itself.rb")
	check(t, a.RequireLibrary(path))
	mustContain(t, mustConvert(t, a, "Total: sum:1+2+39[]", nil), "Total: <strong>42</strong>")
	check(t, a.Extensions().UnregisterAll())
	mustContain(t, mustConvert(t, a, "Total: sum:1+2+39[]", nil), "sum:1+2+39[]")
}

func TestLoadRubySource(t *testing.T) {
	a := newInstance(t)
	check(t, a.LoadRubySource(`
class UpcaseTitles < Asciidoctor::Extensions::TreeProcessor
  def process document
    document.find_by(context: :section).each {|s| s.title = s.title.upcase }
    nil
  end
end`, "upcase_titles.rb"))
	_, err := a.Extensions().RubyProcessor(asciidoctor.KindTreeprocessor, "UpcaseTitles", "ignored")
	check(t, err)
	mustContain(t, mustConvert(t, a, "== Quiet Title\n\ntext", nil), ">QUIET TITLE</h2>")
	err = a.LoadRubySource("def broken(", "broken.rb")
	var e *asciidoctor.Error
	if !asErr(err, &e) || e.Class != "SyntaxError" {
		t.Fatalf("got %#v", err)
	}
	if err := a.RequireLibrary("no/such/library"); err == nil || !strings.Contains(err.Error(), "LoadError") {
		t.Fatalf("got %v", err)
	}
	_, err = a.Extensions().RubyProcessor(asciidoctor.KindPostprocessor, "NoSuchClass", "")
	if err == nil {
		t.Fatal("expected an error for an unknown class")
	}
}

func TestDoGivesInterpreterAccess(t *testing.T) {
	a := instance(t)
	var got string
	check(t, a.Do(func(in *ruby.Interp) error {
		v, err := in.Eval("Asciidoctor::SafeMode::SERVER", "do_test.rb", 1)
		got = v.Inspect()
		return err
	}))
	eq(t, "value", got, "10")
	d, err := a.Load("[#x]\npara", nil)
	check(t, err)
	check(t, a.Do(func(in *ruby.Interp) error {
		v, err := d.Blocks()[0].Value().Call("id")
		got = v.String()
		return err
	}))
	eq(t, "node value", got, "x")
}
