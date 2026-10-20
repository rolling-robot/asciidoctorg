package asciidoctorj

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenRubyExtensionIsRegistered and
// WhenRubyExtensionGroupIsRegistered: extensions written in Ruby, from
// AsciidoctorJ's ruby-extensions test resources, registered by class name
// directly and through a group.

const ajYellSource = "= Block Yell Example\n\ncontent\n\n[%s]\nThe time is now. Get a move on."

// rubyExtensionCase is one Ruby extension scenario.
type rubyExtensionCase struct {
	name string
	// load is a file in ruby-extensions loaded with LoadRubySource, and
	// require one required from the load path (the instance gets the
	// directory on its load path).
	load, require string
	kind          asciidoctor.ExtensionKind
	class, as     string
	check         func(t *testing.T, a *asciidoctor.Asciidoctor)
}

func assertRubyYell(name string) func(t *testing.T, a *asciidoctor.Asciidoctor) {
	return func(t *testing.T, a *asciidoctor.Asciidoctor) {
		paras := parseHTML(t, mustConvert(t, a, strings.Replace(ajYellSource, "%s", name, 1), nil)).Select(".paragraph")
		if len(paras) != 2 {
			t.Fatalf("paragraphs = %d, want 2", len(paras))
		}
		eqv(t, "yelled", text(paras[1]), "THE TIME IS NOW! GET A MOVE ON!")
	}
}

func assertRubyGist(macro, id string) func(t *testing.T, a *asciidoctor.Asciidoctor) {
	return func(t *testing.T, a *asciidoctor.Asciidoctor) {
		scripts := parseHTML(t, mustConvert(t, a, ".My Gist\n"+macro+"::"+id+"[]", nil)).Select("script")
		if len(scripts) != 1 {
			t.Fatalf("scripts = %d, want 1", len(scripts))
		}
		eqv(t, "src", attr(scripts[0], "src"), "https://gist.github.com/"+id+".js")
	}
}

func assertRubyMan(macro, page string) func(t *testing.T, a *asciidoctor.Asciidoctor) {
	return func(t *testing.T, a *asciidoctor.Asciidoctor) {
		src := "= Man Inline Macro Extension\n\nSee " + macro + ":" + page + "[7] to get started."
		links := parseHTML(t, mustConvert(t, a, src, nil)).Select("a")
		if len(links) != 1 {
			t.Fatalf("links = %d, want 1", len(links))
		}
		eqv(t, "href", attr(links[0], "href"), page+".html")
	}
}

var rubyExtensionCases = []rubyExtensionCase{
	{name: "block with explicit name", load: "YellRubyBlock.rb", kind: asciidoctor.KindBlock, class: "YellRubyBlock", as: "rubyyell",
		check: assertRubyYell("rubyyell")},
	{name: "block with implicit name", load: "YellRubyBlock.rb", kind: asciidoctor.KindBlock, class: "YellRubyBlock",
		check: assertRubyYell("yell")},
	{name: "block macro with explicit name", load: "gist-block-macro.rb", kind: asciidoctor.KindBlockMacro, class: "GistBlockMacro", as: "mygist",
		check: assertRubyGist("mygist", "123456")},
	{name: "block macro with implicit name", load: "gist-block-macro.rb", kind: asciidoctor.KindBlockMacro, class: "GistBlockMacro",
		check: assertRubyGist("gist", "42")},
	{name: "inline macro with explicit name", load: "man-inline-macro.rb", kind: asciidoctor.KindInlineMacro, class: "ManInlineMacro", as: "myman",
		check: assertRubyMan("myman", "gittutorial")},
	{name: "inline macro with implicit name", load: "man-inline-macro.rb", kind: asciidoctor.KindInlineMacro, class: "ManInlineMacro",
		check: assertRubyMan("man", "dockertutorial")},
	{name: "treeprocessor", require: "shell-session-tree-processor.rb", kind: asciidoctor.KindTreeprocessor, class: "ShellSessionTreeProcessor",
		check: func(t *testing.T, a *asciidoctor.Asciidoctor) {
			out := mustConvert(t, a, " $ echo \"Hello, World!\"\n > Hello, World!\n\n $ gem install asciidoctor", nil)
			commands := parseHTML(t, out).Select(".command")
			if len(commands) < 2 {
				t.Fatalf("commands = %d", len(commands))
			}
			for i, want := range []string{`echo "Hello, World!"`, "gem install asciidoctor"} {
				first := commands[i].FirstChild()
				if first == nil || !first.IsText() {
					t.Fatalf("command %d has no leading text", i)
				}
				eqv(t, "command", first.Text(), want)
			}
		}},
	{name: "include processor", load: "response-include-processor.rb", kind: asciidoctor.KindIncludeProcessor, class: "ResponseIncludeProcessor",
		check: func(t *testing.T, a *asciidoctor.Asciidoctor) {
			out := mustConvert(t, a, "The response to everything is\n\ninclude::response[]", &asciidoctor.Options{Safe: asciidoctor.SafeModeSafe})
			paras := parseHTML(t, out).Select(".paragraph")
			if len(paras) != 2 {
				t.Fatalf("paragraphs = %d, want 2", len(paras))
			}
			eqv(t, "response", paras[1].ElementChildren().First().ToHTML(), "<p>42</p>")
		}},
	{name: "postprocessor", require: "xml-entity-postprocessor.rb", kind: asciidoctor.KindPostprocessor, class: "XmlEntityPostprocessor",
		check: func(t *testing.T, a *asciidoctor.Asciidoctor) {
			out := mustConvert(t, a, "Read &sect;2 and it&apos;ll all be clear.", nil)
			if !strings.Contains(out, "Read &#167;2 and it&#39;ll all be clear.") {
				t.Errorf("output = %s", out)
			}
		}},
	{name: "preprocessor", require: "front-matter-preprocessor.rb", kind: asciidoctor.KindPreprocessor, class: "FrontMatterPreprocessor",
		check: func(t *testing.T, a *asciidoctor.Asciidoctor) {
			src := "---\ntags: [announcement, website]\n---\n= Document Title\n\ncontent\n\n" +
				"[subs=\"attributes,specialcharacters\"]\n.Captured front matter\n....\n---\n{front-matter}\n---\n...."
			pre := parseHTML(t, mustConvert(t, a, src, nil)).Select(".content pre").First()
			if got := strings.ReplaceAll(text(pre), "\r", ""); !strings.Contains(got, "---\ntags: [announcement, website]\n---") {
				t.Errorf("captured front matter = %q", got)
			}
		}},
	{name: "docinfo processor", require: "view-result-docinfoprocessor.rb", kind: asciidoctor.KindDocinfoProcessor, class: "ViewResultDocinfoProcessor",
		check: func(t *testing.T, a *asciidoctor.Asciidoctor) {
			src := "= View Result Sample\n\n.This will have a link next to it\n----\n* always displayed\n* always displayed 2\n----\n\n" +
				"[.result]\n====\n* hidden till clicked\n* hidden till clicked 2\n===="
			out := mustConvert(t, a, src, &asciidoctor.Options{Safe: asciidoctor.SafeModeSafe, Standalone: asciidoctor.Bool(true)})
			found := false
			for _, style := range parseHTML(t, out).Select("style") {
				found = found || strings.Contains(style.ToHTML(), ".listingblock a.view-result")
			}
			if !found {
				t.Error("no style element from the docinfo processor")
			}
		}},
}

// rubyExtensionsInstance returns an instance with the ruby-extensions
// directory on its load path and the case's Ruby source loaded or required.
func rubyExtensionsInstance(t *testing.T, tc rubyExtensionCase) *asciidoctor.Asciidoctor {
	t.Helper()
	dir := filepath.Join(ajResources(t), "ruby-extensions")
	a := newInstance(t, asciidoctor.WithLoadPath(dir))
	if tc.load != "" {
		if err := a.LoadRubySource(readFile(t, filepath.Join(dir, tc.load)), tc.load); err != nil {
			t.Fatal(err)
		}
	}
	if tc.require != "" {
		if err := a.RequireLibrary(tc.require); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

func TestAJRubyExtension(t *testing.T) {
	for _, tc := range rubyExtensionCases {
		t.Run(tc.name, func(t *testing.T) {
			a := rubyExtensionsInstance(t, tc)
			if _, err := a.Extensions().RubyProcessor(tc.kind, tc.class, tc.as); err != nil {
				t.Fatal(err)
			}
			tc.check(t, a)
		})
	}
}

func TestAJRubyExtensionGroup(t *testing.T) {
	for _, tc := range rubyExtensionCases {
		t.Run(tc.name, func(t *testing.T) {
			a := rubyExtensionsInstance(t, tc)
			if err := a.Extensions().Group("").RubyProcessor(tc.kind, tc.class, tc.as).Register(); err != nil {
				t.Fatal(err)
			}
			tc.check(t, a)
		})
	}
}

func TestAJRubyExtensionGroupUnregister(t *testing.T) {
	a := rubyExtensionsInstance(t, rubyExtensionCase{load: "YellRubyBlock.rb"})
	g := a.Extensions().Group("").RubyProcessor(asciidoctor.KindBlock, "YellRubyBlock", "rubyyell")
	second := func() string {
		paras := parseHTML(t, mustConvert(t, a, strings.Replace(ajYellSource, "%s", "rubyyell", 1), nil)).Select(".paragraph")
		if len(paras) != 2 {
			t.Fatalf("paragraphs = %d, want 2", len(paras))
		}
		return text(paras[1])
	}
	eqv(t, "before register", second(), "The time is now. Get a move on.")
	if err := g.Register(); err != nil {
		t.Fatal(err)
	}
	eqv(t, "registered", second(), "THE TIME IS NOW! GET A MOVE ON!")
	if err := g.Unregister(); err != nil {
		t.Fatal(err)
	}
	eqv(t, "after unregister", second(), "The time is now. Get a move on.")
}
