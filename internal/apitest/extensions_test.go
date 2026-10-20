package apitest

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
}

func mustNotContain(t *testing.T, out string, nots ...string) {
	t.Helper()
	for _, w := range nots {
		if strings.Contains(out, w) {
			t.Errorf("output contains %q:\n%s", w, out)
		}
	}
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreprocessor(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Preprocessor(asciidoctor.PreprocessorFunc(func(doc *asciidoctor.Document, r *asciidoctor.PreprocessorReader) (*asciidoctor.Reader, error) {
		lines := r.ReadLines()
		for i, l := range lines {
			lines[i] = strings.ReplaceAll(l, "@@NAME@@", "Go")
		}
		r.RestoreLines(lines)
		if doc.Doctitle() != "" {
			t.Errorf("document already parsed: %q", doc.Doctitle())
		}
		return nil, nil
	}), nil)
	check(t, err)
	out := mustConvert(t, a, "Hello @@NAME@@!\n\n* from @@NAME@@", nil)
	mustContain(t, out, "<p>Hello Go!</p>", "<p>from Go</p>")
}

func TestPreprocessorReturnsNewReader(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Preprocessor(asciidoctor.PreprocessorFunc(func(doc *asciidoctor.Document, r *asciidoctor.PreprocessorReader) (*asciidoctor.Reader, error) {
		r.ReadLines()
		return a.NewReader([]string{"= Replaced", "", "replacement body"}, nil)
	}), nil)
	check(t, err)
	d, err := a.Load("original", nil)
	check(t, err)
	eq(t, "title", d.Doctitle(), "Replaced")
	out, err := d.Convert()
	check(t, err)
	mustContain(t, out, "replacement body")
	mustNotContain(t, out, "original")
}

func TestPreprocessorReaderAccess(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Preprocessor(asciidoctor.PreprocessorFunc(func(doc *asciidoctor.Document, r *asciidoctor.PreprocessorReader) (*asciidoctor.Reader, error) {
		eq(t, "line number", r.LineNumber(), 1)
		eq(t, "has more", r.HasMoreLines(), true)
		line, ok := r.PeekLine()
		eq(t, "peek", []any{line, ok}, []any{"first", true})
		eq(t, "peek lines", r.PeekLines(2), []string{"first", "second"})
		line, _ = r.ReadLine()
		eq(t, "read line", line, "first")
		eq(t, "next empty", r.IsNextLineEmpty(), false)
		r.RestoreLine("restored")
		eq(t, "lines", r.Lines(), []string{"restored", "second", "", "third"})
		eq(t, "include depth", r.IncludeDepth(), 0)
		eq(t, "document", r.Document().Value() == doc.Value(), true)
		r.PushInclude([]string{"pushed one", "pushed two", ""}, "/virtual/pushed.adoc", "pushed.adoc", 1, nil)
		eq(t, "include depth after push", r.IncludeDepth(), 1)
		return nil, nil
	}), nil)
	check(t, err)
	out := mustConvert(t, a, "first\nsecond\n\nthird", nil)
	mustContain(t, out, "<p>pushed one\npushed two</p>", "<p>restored\nsecond</p>", "<p>third</p>")
}

type virtualInclude struct {
	files map[string]string
	seen  []map[string]any
}

func (v *virtualInclude) Handles(_ *asciidoctor.Document, target string) bool {
	return strings.HasPrefix(target, "virtual:")
}

func (v *virtualInclude) Process(doc *asciidoctor.Document, r *asciidoctor.PreprocessorReader, target string, attrs map[string]any) error {
	v.seen = append(v.seen, attrs)
	name := strings.TrimPrefix(target, "virtual:")
	content, ok := v.files[name]
	if !ok {
		return fmt.Errorf("no virtual file %q", name)
	}
	r.PushInclude(strings.Split(content, "\n"), target, target, 1, attrs)
	return nil
}

func TestIncludeProcessor(t *testing.T) {
	a := newInstance(t)
	inc := &virtualInclude{files: map[string]string{"a": "virtual *A*", "b": "== Included section\n\ninclude::virtual:a[]"}}
	_, err := a.Extensions().IncludeProcessor(inc, nil)
	check(t, err)
	// Include processors run even in secure mode, where file includes become
	// links. As on MRI, an include directive inside content pushed by an
	// include processor is left as is.
	out := mustConvert(t, a, "include::virtual:b[leveloffset=+0,custom=yes]\n\ninclude::local.adoc[]", nil)
	mustContain(t, out, `<h2 id="_included_section">Included section</h2>`, "<p>include::virtual:a[]</p>", `<a href="local.adoc" class="bare include">local.adoc</a>`)
	eq(t, "attributes seen", inc.seen, []map[string]any{{"leveloffset": "+0", "custom": "yes"}})
	mustContain(t, mustConvert(t, a, "include::virtual:a[]", nil), "<p>virtual <strong>A</strong></p>")
	_, err = a.Convert("include::virtual:missing[]", nil)
	if err == nil || !strings.Contains(err.Error(), `no virtual file "missing"`) {
		t.Errorf("got %v", err)
	}
}

func TestTreeprocessor(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Treeprocessor(asciidoctor.TreeprocessorFunc(func(doc *asciidoctor.Document) (*asciidoctor.Document, error) {
		paras, err := doc.FindBy(asciidoctor.Selector{Context: "paragraph"})
		if err != nil {
			return nil, err
		}
		for _, p := range paras {
			b := p.(*asciidoctor.Block)
			b.SetLines([]string{strings.ToUpper(b.Source())})
		}
		note, err := asciidoctor.CreateBlock(doc, "admonition", "Added by a tree processor.", asciidoctor.Attributes{"name": "note", "textlabel": "Note"}, map[string]any{"content_model": "simple"})
		if err != nil {
			return nil, err
		}
		doc.Append(note)
		return nil, nil
	}), nil)
	check(t, err)
	out := mustConvert(t, a, "first para\n\nsecond para", nil)
	mustContain(t, out, "<p>FIRST PARA</p>", "<p>SECOND PARA</p>", `<div class="admonitionblock note">`, "Added by a tree processor.")
}

func TestTreeprocessorReplacesDocument(t *testing.T) {
	a := newInstance(t)
	other, err := a.Load("= Other\n\nfrom the other document", nil)
	check(t, err)
	_, err = a.Extensions().Treeprocessor(asciidoctor.TreeprocessorFunc(func(doc *asciidoctor.Document) (*asciidoctor.Document, error) {
		return other, nil
	}), nil)
	check(t, err)
	out := mustConvert(t, a, "ignored", nil)
	mustContain(t, out, "from the other document")
	mustNotContain(t, out, "ignored")
}

func TestPostprocessorsAndPreference(t *testing.T) {
	a := newInstance(t)
	tag := func(s string) asciidoctor.PostprocessorFunc {
		return func(doc *asciidoctor.Document, out string) (string, error) {
			return out + "\n" + s, nil
		}
	}
	_, err := a.Extensions().Postprocessor(tag("<!-- first registered -->"), nil)
	check(t, err)
	_, err = a.Extensions().Postprocessor(tag("<!-- preferred -->"), &asciidoctor.DocumentProcessorOptions{Prefer: true})
	check(t, err)
	out := mustConvert(t, a, "text", nil)
	if !strings.HasSuffix(out, "<!-- preferred -->\n<!-- first registered -->") {
		t.Errorf("postprocessors ran in the wrong order:\n%s", out)
	}
}

func TestDocinfoProcessor(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().DocinfoProcessor(asciidoctor.DocinfoProcessorFunc(func(doc *asciidoctor.Document) (string, error) {
		return `<meta name="robots" content="index,follow">`, nil
	}), nil)
	check(t, err)
	_, err = a.Extensions().DocinfoProcessor(asciidoctor.DocinfoProcessorFunc(func(doc *asciidoctor.Document) (string, error) {
		return `<script src="footer.js"></script>`, nil
	}), &asciidoctor.DocumentProcessorOptions{Location: "footer"})
	check(t, err)
	out := mustConvert(t, a, "= T\n\nbody", &asciidoctor.Options{Standalone: asciidoctor.Bool(true)})
	head, foot, _ := strings.Cut(out, "</head>")
	mustContain(t, head, `<meta name="robots" content="index,follow">`)
	mustContain(t, foot, `<script src="footer.js"></script>`)
	mustNotContain(t, mustConvert(t, a, "body", nil), "robots")
}

func TestBlockProcessor(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Block("yell", asciidoctor.BlockProcessorFunc(func(parent asciidoctor.StructuralNode, r *asciidoctor.Reader, attrs map[string]any) (asciidoctor.StructuralNode, error) {
		lines := r.ReadLines()
		for i := range lines {
			lines[i] = strings.ToUpper(lines[i])
		}
		return asciidoctor.CreateParagraph(parent, lines, asciidoctor.Attributes{"role": "loud", "volume": attrs["volume"]}, nil)
	}), &asciidoctor.BlockOptions{Contexts: []string{"paragraph", "example"}, ContentModel: asciidoctor.ContentModelSimple, PositionalAttributes: []string{"volume"}})
	check(t, err)
	out := mustConvert(t, a, "[yell,max]\nquiet words\n\n[yell]\n====\nexample words\n====\n\n[yell]\n----\nnot handled\n----", nil)
	mustContain(t, out, `<div class="paragraph loud">`, "<p>QUIET WORDS</p>", "<p>EXAMPLE WORDS</p>", "not handled")
	d, err := a.Load("[yell,max]\nwords", nil)
	check(t, err)
	eq(t, "positional attribute", d.Blocks()[0].Attr("volume"), "max")
}

func TestBlockProcessorDropsBlockAndReadsUntil(t *testing.T) {
	a := newInstance(t)
	var seen []string
	_, err := a.Extensions().Block("collect", asciidoctor.BlockProcessorFunc(func(parent asciidoctor.StructuralNode, r *asciidoctor.Reader, attrs map[string]any) (asciidoctor.StructuralNode, error) {
		seen = r.ReadLinesUntil(asciidoctor.ReadUntilOptions{}, func(line string) bool { return line == "STOP" })
		var nilBlock *asciidoctor.Block
		return nilBlock, nil // dropped
	}), &asciidoctor.BlockOptions{Contexts: []string{"open"}})
	check(t, err)
	out := mustConvert(t, a, "[collect]\n--\none\ntwo\nSTOP\nthree\n--\n\nafter", nil)
	eq(t, "lines read", seen, []string{"one", "two"})
	eq(t, "output", out, "<div class=\"paragraph\">\n<p>after</p>\n</div>")
}

func TestBlockMacroProcessor(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().BlockMacro("gist", asciidoctor.BlockMacroProcessorFunc(func(parent asciidoctor.StructuralNode, target string, attrs map[string]any) (asciidoctor.StructuralNode, error) {
		return asciidoctor.CreatePassBlock(parent, fmt.Sprintf(`<script src="https://gist.github.com/%s.js?file=%v"></script>`, target, attrs["file"]), nil, nil)
	}), &asciidoctor.MacroOptions{PositionalAttributes: []string{"file"}})
	check(t, err)
	out := mustConvert(t, a, "gist::12345[main.go]", nil)
	eq(t, "output", out, `<script src="https://gist.github.com/12345.js?file=main.go"></script>`)
}

func TestInlineMacroProcessor(t *testing.T) {
	a := newInstance(t)
	man := asciidoctor.InlineMacroProcessorFunc(func(parent asciidoctor.Node, target string, attrs map[string]any) (*asciidoctor.PhraseNode, error) {
		section := attrs["volnum"]
		if section == nil {
			section = "7"
		}
		return asciidoctor.CreateAnchor(parent, fmt.Sprintf("%s(%v)", target, section), nil, map[string]any{"type": "link", "target": fmt.Sprintf("%s.%v.html", target, section)})
	})
	_, err := a.Extensions().InlineMacro("man", man, &asciidoctor.MacroOptions{PositionalAttributes: []string{"volnum"}})
	check(t, err)
	_, err = a.Extensions().InlineMacro("emoji", asciidoctor.InlineMacroProcessorFunc(func(parent asciidoctor.Node, target string, attrs map[string]any) (*asciidoctor.PhraseNode, error) {
		return asciidoctor.CreatePhraseNode(parent, "quoted", attrs["text"].(string), nil, map[string]any{"type": "strong"})
	}), &asciidoctor.MacroOptions{Format: "short", ContentModel: asciidoctor.ContentModelText})
	check(t, err)
	_, err = a.Extensions().InlineMacro("ticket", asciidoctor.InlineMacroProcessorFunc(func(parent asciidoctor.Node, target string, attrs map[string]any) (*asciidoctor.PhraseNode, error) {
		return asciidoctor.CreatePhraseNode(parent, "quoted", "ticket "+target, nil, map[string]any{"type": "emphasis"})
	}), &asciidoctor.MacroOptions{Regexp: `#(\d+)()`})
	check(t, err)
	out := mustConvert(t, a, "See man:git[1], man:ls[] and emoji:[smile]. Fixes #42.", nil)
	mustContain(t, out, `<a href="git.1.html">git(1)</a>`, `<a href="ls.7.html">ls(7)</a>`, "<strong>smile</strong>", "<em>ticket 42</em>")
}

func TestInlineMacroReturningNil(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().InlineMacro("hide", asciidoctor.InlineMacroProcessorFunc(func(asciidoctor.Node, string, map[string]any) (*asciidoctor.PhraseNode, error) {
		return nil, nil
	}), nil)
	check(t, err)
	eq(t, "output", mustConvert(t, a, "a hide:x[] b", nil), "<div class=\"paragraph\">\n<p>a  b</p>\n</div>")
}

var errFromGo = errors.New("extension failure from Go")

func TestExtensionErrorPropagates(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Treeprocessor(asciidoctor.TreeprocessorFunc(func(*asciidoctor.Document) (*asciidoctor.Document, error) {
		return nil, fmt.Errorf("wrapped: %w", errFromGo)
	}), nil)
	check(t, err)
	_, err = a.Convert("text", nil)
	if !errors.Is(err, errFromGo) {
		t.Fatalf("errors.Is failed for %v", err)
	}
	var e *asciidoctor.Error
	if !errors.As(err, &e) || e.Class != "RuntimeError" || !strings.Contains(e.Message, "wrapped: extension failure from Go") {
		t.Fatalf("got %#v", err)
	}
	// The instance remains usable once the extension is gone.
	check(t, a.Extensions().UnregisterAll())
	mustConvert(t, a, "ok", nil)
}

func TestExtensionPanicBecomesError(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Postprocessor(asciidoctor.PostprocessorFunc(func(*asciidoctor.Document, string) (string, error) {
		panic("postprocessor exploded")
	}), nil)
	check(t, err)
	_, err = a.Convert("text", nil)
	if err == nil || !strings.Contains(err.Error(), "panic in Go callback: postprocessor exploded") {
		t.Fatalf("got %v", err)
	}
}

func TestGroupsAndUnregister(t *testing.T) {
	a := newInstance(t)
	reg := a.Extensions()
	upper := asciidoctor.PostprocessorFunc(func(_ *asciidoctor.Document, out string) (string, error) { return strings.ToUpper(out), nil })
	marker := asciidoctor.DocinfoProcessorFunc(func(*asciidoctor.Document) (string, error) { return "<!-- docinfo -->", nil })
	g := reg.Group("styling").Postprocessor(upper, nil).DocinfoProcessor(marker, nil)
	eq(t, "group name", g.Name(), "styling")
	check(t, g.Register())
	mustContain(t, mustConvert(t, a, "text", nil), "<P>TEXT</P>")
	check(t, reg.Unregister("styling"))
	mustContain(t, mustConvert(t, a, "text", nil), "<p>text</p>")
	if err := reg.Unregister("styling"); err == nil {
		t.Error("expected an error for an unknown group")
	}
	// Re-registering a group of the same name replaces it.
	check(t, g.Register())
	check(t, reg.Group("styling").Postprocessor(asciidoctor.PostprocessorFunc(func(_ *asciidoctor.Document, out string) (string, error) {
		return out + "!", nil
	}), nil).Register())
	eq(t, "replaced group", mustConvert(t, a, "text", nil), "<div class=\"paragraph\">\n<p>text</p>\n</div>!")
	single, err := reg.Postprocessor(upper, nil)
	check(t, err)
	check(t, single.Unregister())
	check(t, reg.UnregisterAll())
	eq(t, "all unregistered", mustConvert(t, a, "text", nil), "<div class=\"paragraph\">\n<p>text</p>\n</div>")
}

func TestExtensionsAreScopedToInstance(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Postprocessor(asciidoctor.PostprocessorFunc(func(_ *asciidoctor.Document, out string) (string, error) { return "X", nil }), nil)
	check(t, err)
	eq(t, "with extension", mustConvert(t, a, "text", nil), "X")
	eq(t, "other instance", mustConvert(t, instance(t), "text", nil), "<div class=\"paragraph\">\n<p>text</p>\n</div>")
}

func TestInvalidRegistration(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().BlockMacro("not a name", asciidoctor.BlockMacroProcessorFunc(func(asciidoctor.StructuralNode, string, map[string]any) (asciidoctor.StructuralNode, error) {
		return nil, nil
	}), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid name for block macro: not a name") {
		t.Fatalf("got %v", err)
	}
	_, err = a.Extensions().InlineMacro("bad name", asciidoctor.InlineMacroProcessorFunc(func(asciidoctor.Node, string, map[string]any) (*asciidoctor.PhraseNode, error) {
		return nil, nil
	}), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid name for inline macro: bad name") {
		t.Fatalf("got %v", err)
	}
	// A failed registration leaves the instance unchanged.
	mustConvert(t, a, "text", nil)
}

// TestCreateNodes builds content with every creation helper from a tree
// processor and checks the converted result.
func TestCreateNodes(t *testing.T) {
	a := newInstance(t)
	_, err := a.Extensions().Treeprocessor(asciidoctor.TreeprocessorFunc(func(doc *asciidoctor.Document) (*asciidoctor.Document, error) {
		sec, err := asciidoctor.CreateSection(doc, "Generated Section", asciidoctor.Attributes{"id": "gen"}, nil)
		if err != nil {
			return nil, err
		}
		doc.Append(sec)
		eq(t, "section level", sec.Level(), 1)
		eq(t, "section name", sec.SectionName(), "section")

		app, err := asciidoctor.CreateSection(doc, "Generated Appendix", asciidoctor.Attributes{"style": "appendix"}, nil)
		if err != nil {
			return nil, err
		}
		eq(t, "appendix", []any{app.SectionName(), app.IsSpecial(), app.IsNumbered()}, []any{"appendix", true, true})

		ul, err := asciidoctor.CreateList(sec, "ulist", nil)
		if err != nil {
			return nil, err
		}
		for _, text := range []string{"*bold* item", "plain item"} {
			it, err := asciidoctor.CreateListItem(ul, text)
			if err != nil {
				return nil, err
			}
			ul.AddItem(it)
		}
		sec.Append(ul)

		dl, err := asciidoctor.CreateDescriptionList(sec, nil)
		if err != nil {
			return nil, err
		}
		term, _ := asciidoctor.CreateListItem(dl, "Go")
		desc, _ := asciidoctor.CreateListItem(dl, "A language.")
		dl.AddEntry([]*asciidoctor.ListItem{term}, desc)
		sec.Append(dl)

		img, err := asciidoctor.CreateImageBlock(sec, asciidoctor.Attributes{"target": "my_diagram.png", "title": "Generated figure"}, nil)
		if err != nil {
			return nil, err
		}
		sec.Append(img)

		lst, err := asciidoctor.CreateListingBlock(sec, "x := 1", asciidoctor.Attributes{"language": "go"}, nil)
		if err != nil {
			return nil, err
		}
		lst.SetStyle("source")
		sec.Append(lst)

		if err := asciidoctor.ParseContent(sec, "Parsed _paragraph_.\n\nNOTE: Parsed note.", nil); err != nil {
			return nil, err
		}

		attrs, err := asciidoctor.ParseAttributes(doc, `first,role=x,title="A {product}"`, []string{"name"}, true)
		if err != nil {
			return nil, err
		}
		eq(t, "parsed attributes", attrs, map[string]any{"1": "first", "name": "first", "role": "x", "title": "A Widget"})

		tbl, err := asciidoctor.CreateTable(sec, nil)
		if err != nil {
			return nil, err
		}
		c1, _ := asciidoctor.CreateTableColumn(tbl, 0, nil)
		c2, _ := asciidoctor.CreateTableColumn(tbl, 1, nil)
		tbl.AddColumn(c1)
		tbl.AddColumn(c2)
		h1, _ := asciidoctor.CreateTableCell(c1, "Key", nil)
		h2, _ := asciidoctor.CreateTableCell(c2, "Value", nil)
		tbl.AddRow(asciidoctor.TableHeader, h1, h2)
		k, _ := asciidoctor.CreateTableCell(c1, "*answer*", nil)
		inner, err := asciidoctor.CreateDocument(doc)
		if err != nil {
			return nil, err
		}
		if err := asciidoctor.ParseContent(inner, "* forty-two", nil); err != nil {
			return nil, err
		}
		v, err := asciidoctor.CreateTableCellDocument(c2, inner, nil)
		if err != nil {
			return nil, err
		}
		tbl.AddRow(asciidoctor.TableBody, k, v)
		tbl.AssignColumnWidths()
		sec.Append(tbl)
		eq(t, "rowcount", tbl.Attr("rowcount"), int64(2))
		return nil, nil
	}), nil)
	check(t, err)
	out := mustConvert(t, a, ":product: Widget\n\nIntro.", nil)
	mustContain(t, out,
		`<h2 id="gen">Generated Section</h2>`,
		"<p><strong>bold</strong> item</p>", "<p>plain item</p>",
		`<dt class="hdlist1">Go</dt>`, "<p>A language.</p>",
		`<img src="my_diagram.png" alt="my diagram">`, `<div class="title">Figure 1. Generated figure</div>`,
		`<code class="language-go" data-lang="go">x := 1</code>`,
		"<p>Parsed <em>paragraph</em>.</p>", "Parsed note.",
		`<th class="tableblock halign-left valign-top">Key</th>`,
		`<td class="tableblock halign-left valign-top"><p class="tableblock"><strong>answer</strong></p></td>`,
		"<p>forty-two</p>",
		`<col width="50%">`,
	)
}

func TestCreateNodeErrors(t *testing.T) {
	if _, err := asciidoctor.CreateBlock(nil, "paragraph", "x", nil, nil); err == nil {
		t.Error("expected an error for a nil parent")
	}
	d, err := instance(t).Load("x", nil)
	check(t, err)
	if _, err := asciidoctor.CreateBlock(d, "paragraph", 42, nil, nil); err == nil {
		t.Error("expected an error for an invalid source")
	}
	if _, err := asciidoctor.CreateList(d, "dlist", nil); err == nil {
		t.Error("expected an error for a dlist from CreateList")
	}
	if _, err := asciidoctor.CreateImageBlock(d, asciidoctor.Attributes{}, nil); err == nil || !strings.Contains(err.Error(), "target attribute is required") {
		t.Errorf("got %v", err)
	}
	p, err := asciidoctor.CreatePhraseNode(d, "quoted", "text", nil, map[string]any{"type": "monospaced"})
	check(t, err)
	html, err := p.Convert()
	check(t, err)
	eq(t, "phrase", html, "<code>text</code>")
	r, err := instance(t).NewReader([]string{"a", "b"}, &asciidoctor.Cursor{File: "/x/virtual.adoc", Dir: "/x", Path: "virtual.adoc", LineNumber: 10})
	check(t, err)
	eq(t, "reader cursor", r.Cursor(), &asciidoctor.Cursor{LineNumber: 10, File: "/x/virtual.adoc", Dir: "/x", Path: "virtual.adoc"})
	eq(t, "reader read", r.Read(), "a\nb")
	eq(t, "reader done", r.HasMoreLines(), false)
}
