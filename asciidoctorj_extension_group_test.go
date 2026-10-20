package asciidoctor_test

import (
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenJavaExtensionGroupIsRegistered: the
// scenarios of WhenJavaExtensionIsRegistered, with the extensions added to a
// group that is then registered. (Its arrows-and-boxes test is @Disabled
// upstream and has no counterpart.)

func TestAJExtensionGroup(t *testing.T) {
	assertMan := func(match bool) func(t *testing.T, a *asciidoctor.Asciidoctor, logs *logRecorder) {
		return func(t *testing.T, a *asciidoctor.Asciidoctor, logs *logRecorder) {
			link := parseHTML(t, ajConvertResource(t, a, "sample-with-man-link.ad", nil)).Select("a").First()
			if !match {
				if link != nil {
					t.Errorf("unexpected link %s", link.ToHTML())
				}
			} else if got := attr(link, "href"); got != "gittutorial.html" {
				t.Errorf("href = %q", got)
			}
			eqv(t, "log records", len(logs.Records()), 0)
		}
	}
	assertGist := func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
		out := ajConvertResource(t, a, "sample-with-gist-macro.ad", nil)
		eqv(t, "script src", attr(parseHTML(t, out).Select("script").First(), "src"), "https://gist.github.com/123456.js")
	}
	assertFooter := func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
		footer := ajConvertToFile(t, a, "simple.adoc").ByID("footer-text")
		if !strings.Contains(text(footer), ajCopyright) {
			t.Errorf("footer = %q", text(footer))
		}
	}
	assertYell := func(file string) func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
		return func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
			assertYelled(t, ajConvertResource(t, a, file, nil))
		}
	}
	server := asciidoctor.Bool(true)
	for _, tc := range []struct {
		name  string
		add   func(t *testing.T, g *asciidoctor.ExtensionGroup)
		check func(t *testing.T, a *asciidoctor.Asciidoctor, logs *logRecorder)
	}{
		{"include processor",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.IncludeProcessor(uriIncludeProcessor(ajRubyClassProxy(t)), nil)
			},
			func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
				assertRubyClassIncluded(t, ajConvertResource(t, a, "sample-with-uri-include.ad", nil))
			}},
		{"include processor only handles its targets",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.IncludeProcessor(uriIncludeProcessor(ajRubyClassProxy(t)), nil)
			},
			func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
				out := ajConvertResource(t, a, "sample-with-include.ad", nil)
				if got := text(parseHTML(t, out).Select(".bare").First()); !strings.HasPrefix(got, "sample-book.adoc") {
					t.Errorf("link = %q", got)
				}
			}},
		{"include processor positional attributes",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.IncludeProcessor(positionalAttrsIncludeProcessor, nil)
			},
			func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
				out := ajConvertResource(t, a, "sample-with-include-pos-attrs.ad", nil)
				if got := text(parseHTML(t, out).Select(`[class="paragraph IncludeBlock"]`).First()); !strings.HasPrefix(got, "My,Positional,Attribute List") {
					t.Errorf("paragraph = %q", got)
				}
			}},
		{"docinfo processor in head",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) { g.DocinfoProcessor(metaRobotsDocinfo, nil) },
			func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
				out := ajConvertResource(t, a, "simple.adoc", &asciidoctor.Options{Standalone: server, Safe: asciidoctor.SafeModeServer})
				if parseHTML(t, out).Select(`head meta[name*="robots"]`).First() == nil {
					t.Error("no robots meta in head")
				}
			}},
		{"docinfo processor in footer",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.DocinfoProcessor(metaRobotsDocinfo, &asciidoctor.DocumentProcessorOptions{Location: "footer"})
			},
			func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
				out := ajConvertResource(t, a, "simple.adoc", &asciidoctor.Options{Standalone: server, Safe: asciidoctor.SafeModeServer})
				eqv(t, "sibling name", attr(parseHTML(t, out).ByID("footer").NextElement(), "name"), "robots")
			}},
		{"preprocessor",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.Preprocessor(changeAttributeValuePreprocessor, nil)
			},
			func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
				out := ajConvertResource(t, a, "changeattribute.adoc", nil)
				eqv(t, "paragraph", text(parseHTML(t, out).Select("p").First()), "sample Alex")
			}},
		{"postprocessor",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) { g.Postprocessor(customFooterPostprocessor, nil) },
			assertFooter},
		{"treeprocessor",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) { g.Treeprocessor(terminalCommandTreeprocessor, nil) },
			func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
				assertCommands(t, parseHTML(t, ajConvertResource(t, a, "sample-with-terminal-command.ad", nil)))
			}},
		{"treeprocessor and block macro",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.Treeprocessor(terminalCommandTreeprocessor, nil).BlockMacro("gist", gistMacro, nil)
			},
			func(t *testing.T, a *asciidoctor.Asciidoctor, _ *logRecorder) {
				doc := parseHTML(t, ajConvertResource(t, a, "sample-with-terminal-command-and-gist-macro.ad", nil))
				eqv(t, "script src", attr(doc.Select("script").First(), "src"), "https://gist.github.com/42.js")
				assertCommands(t, doc)
			}},
		{"block macro",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) { g.BlockMacro("gist", gistMacro, nil) },
			assertGist},
		{"block macro raw content model",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.BlockMacro("gist", gistMacro, &asciidoctor.MacroOptions{ContentModel: asciidoctor.ContentModelRaw})
			},
			assertGist},
		{"inline macro",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) { g.InlineMacro("man", manpageMacro, nil) },
			assertMan(true)},
		{"inline macro regexp",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.InlineMacro("man", manpageMacro, &asciidoctor.MacroOptions{Regexp: `man(?:page)?:(\S+?)\[(.*?)\]`})
			},
			assertMan(true)},
		{"inline macro regexp not matching",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.InlineMacro("man", manpageMacro, &asciidoctor.MacroOptions{Regexp: `man(?:page)?:(ThisDoesNotMatch)\[(.*?)\]`})
			},
			assertMan(false)},
		{"paragraph block processor",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) {
				g.Block("yell", yellBlock, yellOptions("paragraph"))
			},
			assertYell("sample-with-yell-block.ad")},
		{"listing block processor",
			func(t *testing.T, g *asciidoctor.ExtensionGroup) { g.Block("yell", yellBlock, yellOptions("listing")) },
			assertYell("sample-with-yell-listing-block.ad")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newInstance(t)
			logs := recordLogs(t, a)
			g := a.Extensions().Group("")
			tc.add(t, g)
			if err := g.Register(); err != nil {
				t.Fatal(err)
			}
			tc.check(t, a, logs)
		})
	}
}

func TestAJExtensionGroupUnregisterAll(t *testing.T) {
	a := newInstance(t)
	if err := a.Extensions().Group("").Postprocessor(customFooterPostprocessor, nil).Register(); err != nil {
		t.Fatal(err)
	}
	if err := a.Extensions().UnregisterAll(); err != nil {
		t.Fatal(err)
	}
	if footer := text(ajConvertToFile(t, a, "rendersample.asciidoc").ByID("footer-text")); strings.Contains(footer, ajCopyright) {
		t.Errorf("footer = %q", footer)
	}
}

func TestAJExtensionGroupTreeprocessorWithTOC(t *testing.T) {
	a := newInstance(t)
	visited := 0
	g := a.Extensions().Group("").Treeprocessor(asciidoctor.TreeprocessorFunc(func(doc *asciidoctor.Document) (*asciidoctor.Document, error) {
		for _, b := range doc.Blocks() {
			visited += len(b.Blocks())
		}
		return doc, nil
	}), nil)
	if err := g.Register(); err != nil {
		t.Fatal(err)
	}
	out := ajConvertResource(t, a, "documentwithtoc.adoc", &asciidoctor.Options{Standalone: asciidoctor.Bool(true), Safe: asciidoctor.SafeModeUnsafe})
	if visited == 0 {
		t.Error("treeprocessor saw no blocks")
	}
	eqv(t, "toc links to TestId", len(parseHTML(t, out).Select(`#toc [href="#TestId"]`)), 1)
}

func TestAJExtensionGroupNamed(t *testing.T) {
	a := newInstance(t)
	g := a.Extensions().Group("my-group").Postprocessor(customFooterPostprocessor, nil)
	eqv(t, "name", g.Name(), "my-group")
	if err := g.Register(); err != nil {
		t.Fatal(err)
	}
	footer := func() string { return text(ajConvertToFile(t, a, "rendersample.asciidoc").ByID("footer-text")) }
	if !strings.Contains(footer(), ajCopyright) {
		t.Error("postprocessor did not run")
	}
	if err := a.Extensions().Unregister("my-group"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(footer(), ajCopyright) {
		t.Error("postprocessor ran after its group was unregistered by name")
	}
}
