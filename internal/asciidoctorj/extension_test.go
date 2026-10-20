package asciidoctorj

import (
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenJavaExtensionIsRegistered. AsciidoctorJ
// registers each extension by class, class name and instance; a Go extension
// is a value, so each of those variants is one test here.

func assertRubyClassIncluded(t *testing.T, out string) {
	t.Helper()
	got := text(parseHTML(t, out).Select(`[class="language-ruby"]`).First())
	if !strings.Contains(got, ajRubyClassPrefix) {
		t.Errorf("included content = %q", got)
	}
}

func assertCommands(t *testing.T, doc *htmlDoc) {
	t.Helper()
	commands := doc.Select(`[class="command"]`)
	if len(commands) == 0 {
		t.Fatal("no commands")
	}
	eqv(t, "first command", text(commands[0]), `echo "Hello, World!"`)
	eqv(t, "last command", text(commands[len(commands)-1]), "gem install asciidoctor")
}

func assertYelled(t *testing.T, out string) {
	t.Helper()
	paras := parseHTML(t, out).Select(".paragraph")
	if len(paras) != 1 {
		t.Fatalf("paragraphs = %d, want 1", len(paras))
	}
	eqv(t, "paragraph", text(paras[0]), ajYelled)
}

func TestAJIncludeProcessor(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().IncludeProcessor(uriIncludeProcessor(ajRubyClassProxy(t)), nil))
	// AsciidoctorJ's *_should_be_executed_twice variants convert twice.
	for range 2 {
		assertRubyClassIncluded(t, ajConvertResource(t, a, "sample-with-uri-include.ad", nil))
	}
}

func TestAJDocinfoProcessorHead(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().DocinfoProcessor(metaRobotsDocinfo, nil))
	out := ajConvertResource(t, a, "simple.adoc", &asciidoctor.Options{Standalone: asciidoctor.Bool(true), Safe: asciidoctor.SafeModeServer})
	if parseHTML(t, out).Select(`head meta[name*="robots"]`).First() == nil {
		t.Error("no robots meta in head")
	}
}

func TestAJDocinfoProcessorFooter(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().DocinfoProcessor(metaRobotsDocinfo, &asciidoctor.DocumentProcessorOptions{Location: "footer"}))
	out := ajConvertResource(t, a, "simple.adoc", &asciidoctor.Options{Standalone: asciidoctor.Bool(true), Safe: asciidoctor.SafeModeServer})
	footer := parseHTML(t, out).ByID("footer")
	if footer == nil {
		t.Fatal("no footer")
	}
	eqv(t, "sibling name", attr(footer.NextElement(), "name"), "robots")
}

func TestAJPreprocessor(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().Preprocessor(changeAttributeValuePreprocessor, nil))
	out := ajConvertResource(t, a, "changeattribute.adoc", nil)
	eqv(t, "paragraph", text(parseHTML(t, out).Select("p").First()), "sample Alex")
}

func TestAJPostprocessor(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().Postprocessor(customFooterPostprocessor, nil))
	footer := ajConvertToFile(t, a, "rendersample.asciidoc").ByID("footer-text")
	if !strings.Contains(text(footer), ajCopyright) {
		t.Errorf("footer = %q", text(footer))
	}
}

func TestAJIncludeProcessorOnlyHandlesItsTargets(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().IncludeProcessor(uriIncludeProcessor(ajRubyClassProxy(t)), nil))
	out := ajConvertResource(t, a, "sample-with-include.ad", nil)
	if got := text(parseHTML(t, out).Select(".bare").First()); !strings.HasPrefix(got, "sample-book.adoc") {
		t.Errorf("link = %q", got)
	}
}

func TestAJIncludeProcessorPositionalAttributes(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().IncludeProcessor(positionalAttrsIncludeProcessor, nil))
	out := ajConvertResource(t, a, "sample-with-include-pos-attrs.ad", nil)
	if got := text(parseHTML(t, out).Select(`[class="paragraph IncludeBlock"]`).First()); !strings.HasPrefix(got, "My,Positional,Attribute List") {
		t.Errorf("paragraph = %q", got)
	}
}

func TestAJTreeprocessor(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().Treeprocessor(terminalCommandTreeprocessor, nil))
	assertCommands(t, parseHTML(t, ajConvertResource(t, a, "sample-with-terminal-command.ad", nil)))
}

func TestAJTreeprocessorAndBlockMacro(t *testing.T) {
	a := newInstance(t)
	g := a.Extensions().Group("").Treeprocessor(terminalCommandTreeprocessor, nil).BlockMacro("gist", gistMacro, nil)
	if err := g.Register(); err != nil {
		t.Fatal(err)
	}
	doc := parseHTML(t, ajConvertResource(t, a, "sample-with-terminal-command-and-gist-macro.ad", nil))
	assertCommands(t, doc)
	eqv(t, "script src", attr(doc.Select("script").First(), "src"), "https://gist.github.com/42.js")
}

func TestAJBlockMacro(t *testing.T) {
	for name, opts := range map[string]*asciidoctor.MacroOptions{
		"default":           nil,
		"raw content model": {ContentModel: asciidoctor.ContentModelRaw},
	} {
		t.Run(name, func(t *testing.T) {
			a := newInstance(t)
			mustRegister(t)(a.Extensions().BlockMacro("gist", gistMacro, opts))
			out := ajConvertResource(t, a, "sample-with-gist-macro.ad", nil)
			eqv(t, "script src", attr(parseHTML(t, out).Select("script").First(), "src"), "https://gist.github.com/123456.js")
		})
	}
}

func TestAJInlineMacro(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *asciidoctor.MacroOptions
		match bool
	}{
		{"default", nil, true},
		{"regexp", &asciidoctor.MacroOptions{Regexp: `man(?:page)?:(\S+?)\[(.*?)\]`}, true},
		{"regexp not matching", &asciidoctor.MacroOptions{Regexp: `man(?:page)?:(ThisDoesNotMatch)\[(.*?)\]`}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newInstance(t)
			logs := recordLogs(t, a)
			mustRegister(t)(a.Extensions().InlineMacro("man", manpageMacro, tc.opts))
			link := parseHTML(t, ajConvertResource(t, a, "sample-with-man-link.ad", nil)).Select("a").First()
			if !tc.match {
				if link != nil {
					t.Errorf("unexpected link %s", link.ToHTML())
				}
			} else if got := attr(link, "href"); got != "gittutorial.html" {
				t.Errorf("href = %q", got)
			}
			eqv(t, "log records", len(logs.Records()), 0)
		})
	}
}

func TestAJInlineMacroWithSubs(t *testing.T) {
	a := newInstance(t)
	logs := recordLogs(t, a)
	mustRegister(t)(a.Extensions().InlineMacro("say", sayMacro, nil))
	out := mustConvert(t, a, "Hello say:word[]!", nil)
	eqv(t, "strong", parseHTML(t, out).Select("p strong").Text(), "word")
	eqv(t, "log records", len(logs.Records()), 0)
}

func TestAJUnregisterAllExtensions(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().Postprocessor(customFooterPostprocessor, nil))
	if err := a.Extensions().UnregisterAll(); err != nil {
		t.Fatal(err)
	}
	footer := ajConvertToFile(t, a, "rendersample.asciidoc").ByID("footer-text")
	if strings.Contains(text(footer), ajCopyright) {
		t.Errorf("footer = %q", text(footer))
	}
}

func TestAJBlockProcessor(t *testing.T) {
	for _, tc := range []struct{ context, file string }{
		{"paragraph", "sample-with-yell-block.ad"},
		{"listing", "sample-with-yell-listing-block.ad"},
	} {
		t.Run(tc.context, func(t *testing.T) {
			a := newInstance(t)
			mustRegister(t)(a.Extensions().Block("yell", yellBlock, yellOptions(tc.context)))
			for range 2 {
				assertYelled(t, ajConvertResource(t, a, tc.file, nil))
			}
		})
	}
}

func TestAJTreeprocessorWithTOC(t *testing.T) {
	a := newInstance(t)
	var ids []string
	mustRegister(t)(a.Extensions().Treeprocessor(asciidoctor.TreeprocessorFunc(func(doc *asciidoctor.Document) (*asciidoctor.Document, error) {
		for _, b := range doc.Blocks() {
			for _, b2 := range b.Blocks() {
				if s, ok := b2.(*asciidoctor.Section); ok {
					ids = append(ids, s.ID())
				}
			}
		}
		return doc, nil
	}), nil))
	out := ajConvertResource(t, a, "documentwithtoc.adoc", &asciidoctor.Options{Standalone: asciidoctor.Bool(true), Safe: asciidoctor.SafeModeUnsafe})
	eqv(t, "subsection ids", ids, []string{"_subsection_1_1", "_subsection_2_1"})
	doc := parseHTML(t, out)
	if doc.ByID("toc") == nil {
		t.Fatal("no toc")
	}
	eqv(t, "toc links to TestId", len(doc.Select(`#toc [href="#TestId"]`)), 1)
}

func TestAJUnregisterPostprocessorGroup(t *testing.T) {
	a := newInstance(t)
	g := a.Extensions().Group("").Postprocessor(customFooterPostprocessor, nil)
	footer := func() string { return text(ajConvertToFile(t, a, "rendersample.asciidoc").ByID("footer-text")) }
	if strings.Contains(footer(), ajCopyright) {
		t.Error("postprocessor ran before the group was registered")
	}
	if err := g.Register(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(footer(), ajCopyright) {
		t.Error("postprocessor did not run")
	}
	if err := g.Unregister(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(footer(), ajCopyright) {
		t.Error("postprocessor ran after the group was unregistered")
	}
}

func TestAJUnregisterBlockProcessorGroup(t *testing.T) {
	a := newInstance(t)
	g := a.Extensions().Group("").Block("yell", yellBlock, yellOptions("paragraph"))
	paragraph := func() string {
		paras := parseHTML(t, ajConvertResource(t, a, "sample-with-yell-block.ad", nil)).Select(".paragraph")
		if len(paras) != 1 {
			t.Fatalf("paragraphs = %d, want 1", len(paras))
		}
		return text(paras[0])
	}
	if paragraph() == ajYelled {
		t.Error("block processor ran before the group was registered")
	}
	if err := g.Register(); err != nil {
		t.Fatal(err)
	}
	eqv(t, "registered", paragraph(), ajYelled)
	if err := g.Unregister(); err != nil {
		t.Fatal(err)
	}
	if paragraph() == ajYelled {
		t.Error("block processor ran after the group was unregistered")
	}
}

func TestAJExtensionGetsItsInstance(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().Block("quiet", asciidoctor.BlockProcessorFunc(func(parent asciidoctor.StructuralNode, reader *asciidoctor.Reader, attrs map[string]any) (asciidoctor.StructuralNode, error) {
		if parent.Asciidoctor() != a {
			t.Error("node belongs to another instance")
		}
		lines := reader.ReadLines()
		for i, l := range lines {
			lines[i] = strings.ToLower(l)
		}
		return asciidoctor.CreateParagraph(parent, lines, attrs, nil)
	}), nil))
	for _, word := range []string{"World", "Again"} {
		paras := parseHTML(t, mustConvert(t, a, "[quiet]\nHello "+word, nil)).Select(".paragraph")
		if len(paras) != 1 {
			t.Fatalf("paragraphs = %d", len(paras))
		}
		eqv(t, "paragraph", text(paras[0]), "hello "+strings.ToLower(word))
	}
}
