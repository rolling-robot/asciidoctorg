package asciidoctorj

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenReaderIsManipulatedInExtension,
// PreprocessorTest and WhenTheInlineMacroProcessorRunsTwice.

func TestAJReaderInPreprocessor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		check func(t *testing.T, r *asciidoctor.PreprocessorReader)
	}{
		{"line number", func(t *testing.T, r *asciidoctor.PreprocessorReader) {
			eqv(t, "line number", r.LineNumber(), 1)
		}},
		{"has more lines", func(t *testing.T, r *asciidoctor.PreprocessorReader) {
			eqv(t, "has more lines", r.HasMoreLines(), true)
		}},
		{"next line empty", func(t *testing.T, r *asciidoctor.PreprocessorReader) {
			eqv(t, "next line empty before advance", r.IsNextLineEmpty(), false)
			r.Advance()
			eqv(t, "next line empty after advance", r.IsNextLineEmpty(), true)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newInstance(t)
			called := false
			mustRegister(t)(a.Extensions().Preprocessor(asciidoctor.PreprocessorFunc(func(_ *asciidoctor.Document, r *asciidoctor.PreprocessorReader) (*asciidoctor.Reader, error) {
				called = true
				tc.check(t, r)
				return nil, nil
			}), nil))
			src := ajResource(t, "rendersample.asciidoc")
			mustConvertFile(t, a, src, nil)
			eqv(t, "preprocessor called", called, true)
			eqv(t, "output written", fileExists(filepath.Join(filepath.Dir(src), "rendersample.html")), true)
		})
	}
}

// frontMatterPreprocessor moves a front matter block into the front-matter
// attribute, as AsciidoctorJ's FrontMatterPreprocessor does.
var frontMatterPreprocessor = asciidoctor.PreprocessorFunc(func(doc *asciidoctor.Document, r *asciidoctor.PreprocessorReader) (*asciidoctor.Reader, error) {
	lines := r.Lines()
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, nil
	}
	var frontMatter []string
	rest := lines[1:]
	for len(rest) > 0 && strings.TrimSpace(rest[0]) != "---" {
		frontMatter = append(frontMatter, rest[0])
		rest = rest[1:]
	}
	if len(rest) > 0 {
		doc.SetAttr("front-matter", frontMatter, true)
		for range len(frontMatter) + 2 {
			r.Advance()
		}
	}
	return nil, nil
})

func TestAJPreprocessorRemovesFrontMatter(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().Preprocessor(frontMatterPreprocessor, nil))
	src := "---\nlayout: info\npermalink: /sample/\n---\n= Sample Page\n:url-asciidoctor: https://asciidoctor.org\n\n" +
		"This is a sample page composed in AsciiDoc.\nJekyll converts it to HTML using {url-asciidoctor}[Asciidoctor].\n\n" +
		"[source,ruby]\nputs \"Hello, World!\""
	d, err := a.Load(src, &asciidoctor.Options{Standalone: asciidoctor.Bool(true), NoOutputFile: true})
	if err != nil {
		t.Fatal(err)
	}
	eqv(t, "front matter", d.Attr("front-matter"), any([]any{"layout: info", "permalink: /sample/"}))
	html, err := d.Convert()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"permalink", "layout"} {
		if strings.Contains(html, s) {
			t.Errorf("output contains %q", s)
		}
	}
	doc := parseHTML(t, html)
	eqv(t, "title", text(doc.Select("body h1").First()), "Sample Page")
	if p := text(doc.Select("body p").First()); !strings.HasPrefix(p, "This is a sample page") {
		t.Errorf("first paragraph = %q", p)
	}
}

func TestAJPreprocessorReturnsNewReader(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().Preprocessor(asciidoctor.PreprocessorFunc(func(_ *asciidoctor.Document, r *asciidoctor.PreprocessorReader) (*asciidoctor.Reader, error) {
		lines := r.Lines()
		for i, l := range lines {
			if l != "" && !strings.HasPrefix(l, "=") {
				lines[i] = l + " +"
			}
		}
		return a.NewReader(lines, nil)
	}), nil))
	out := mustConvert(t, a, "Ruby is red.\nJava is beige.", nil)
	eqv(t, "br count", len(parseHTML(t, out).Select("br")), 2)
}

// AsciidoctorJ issue 926: the attributes passed to an inline macro must not
// leak from one conversion into the next.
func TestAJInlineMacroRunsTwice(t *testing.T) {
	a := newInstance(t)
	mustRegister(t)(a.Extensions().InlineMacro("example", asciidoctor.InlineMacroProcessorFunc(func(parent asciidoctor.Node, _ string, attrs map[string]any) (*asciidoctor.PhraseNode, error) {
		keys := make([]string, 0, len(attrs))
		for k := range attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = fmt.Sprintf("%s=%v", k, attrs[k])
		}
		return asciidoctor.CreatePhraseNode(parent, "quoted", "{"+strings.Join(parts, ", ")+"}", attrs, nil)
	}), nil))
	for range 2 {
		if out := mustConvert(t, a, "example:alpha.bravo[format=test]", nil); !strings.Contains(out, "{format=test}") {
			t.Errorf("output = %s", out)
		}
	}
}
