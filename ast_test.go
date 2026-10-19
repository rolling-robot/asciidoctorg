package asciidoctor_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %#v, want %#v", what, got, want)
	}
}

// TestDocumentTree walks testdata/ast/sample.adoc. Every expected value was
// produced by the same queries against Asciidoctor on MRI Ruby; the queries run
// in the same order, because converting content registers footnotes.
func TestDocumentTree(t *testing.T) {
	a := instance(t)
	path, _ := filepath.Abs("testdata/ast/sample.adoc")
	d, err := a.LoadFile(path, &asciidoctor.Options{Safe: asciidoctor.SafeModeSafe, SourceMap: true, CatalogAssets: true})
	if err != nil {
		t.Fatal(err)
	}

	eq(t, "doctitle", d.Doctitle(), "Main Title: The Subtitle")
	eq(t, "structured doctitle", d.StructuredDoctitle(true), &asciidoctor.Title{Main: "Main Title", Subtitle: "The Subtitle", Combined: "Main Title: The Subtitle", Sanitized: true})
	eq(t, "authors", d.Authors(), []asciidoctor.Author{
		{Name: "Jane Doe", FirstName: "Jane", LastName: "Doe", Initials: "JD", Email: "jane@example.org"},
		{Name: "John Q. Public", FirstName: "John", MiddleName: "Q.", LastName: "Public", Initials: "JQP"},
	})
	eq(t, "revision", d.RevisionInfo(), asciidoctor.RevisionInfo{Date: "2021-01-02", Number: "1.2", Remark: "Initial draft"})
	eq(t, "doctype", d.Doctype(), "book")
	eq(t, "backend", d.Backend(), "html5")
	eq(t, "safe", d.SafeMode(), asciidoctor.SafeModeSafe)
	eq(t, "header", d.HasHeader(), true)
	eq(t, "blocks", len(d.Blocks()), 4)
	eq(t, "basebackend", d.IsBasebackend("html"), true)
	eq(t, "outfilesuffix", d.OutfileSuffix(), ".html")
	eq(t, "custom attr", d.Attr("custom"), "value")
	eq(t, "sourcemap", d.SourceMap(), true)
	eq(t, "document parent", d.Parent(), nil)
	eq(t, "context", d.Context(), "document")
	eq(t, "options", d.Options()["sourcemap"], true)

	secs := d.Sections()
	type secInfo struct {
		Name                    string
		Special, Numbered       bool
		Level, Index            int
		Sectnum, ID, Num, Title string
		Xref                    string
	}
	var got []secInfo
	for _, s := range secs {
		got = append(got, secInfo{s.SectionName(), s.IsSpecial(), s.IsNumbered(), s.Level(), s.Index(), s.Sectnum(), s.ID(), s.Numeral(), s.Title(), s.Xreftext("")})
	}
	eq(t, "sections", got, []secInfo{
		{"preface", true, false, 1, 0, ".", "_preface", "", "Preface", "Preface"},
		{"chapter", false, true, 1, 1, "1.", "ch-one", "1", "First Chapter", "Chapter One"},
		{"chapter", false, true, 1, 2, "2.", "_second_chapter", "2", "Second Chapter", "Second Chapter"},
		{"appendix", true, true, 1, 3, "A.", "_extra", "A", "Extra", "Extra"},
	})

	ch := secs[1]
	blocks := ch.Blocks()
	para := blocks[0].(*asciidoctor.Block)
	eq(t, "para context", para.Context(), "paragraph")
	eq(t, "para node name", para.NodeName(), "paragraph")
	eq(t, "para id", para.ID(), "intro")
	eq(t, "para roles", para.Roles(), []string{"lead", "highlight"})
	eq(t, "para role", para.Role(), "lead highlight")
	eq(t, "para has role", para.HasRole("highlight"), true)
	eq(t, "para style", para.Style(), "")
	eq(t, "para title", para.Title(), "Intro title")
	eq(t, "para captioned title", para.CaptionedTitle(), "Intro title")
	eq(t, "para lines", para.Lines(), []string{"This is the *intro* paragraph.", "Second line.footnote:fn2[Second note.]"})
	eq(t, "para source", para.Source(), "This is the *intro* paragraph.\nSecond line.footnote:fn2[Second note.]")
	content, err := para.Content()
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "para content", content, "This is the <strong>intro</strong> paragraph.<br>\nSecond line.<sup class=\"footnote\" id=\"_footnote_fn2\">[<a id=\"_footnoteref_1\" class=\"footnote\" href=\"#_footnotedef_1\" title=\"View footnote.\">1</a>]</sup>")
	eq(t, "para option", para.HasOption("hardbreaks"), true)
	eq(t, "para enabled options", para.EnabledOptions(), []string{"hardbreaks"})
	eq(t, "para attr", para.Attr("source"), "docs")
	loc := para.SourceLocation()
	eq(t, "para location", loc, &asciidoctor.Cursor{LineNumber: 19, File: path, Dir: filepath.Dir(path), Path: "sample.adoc"})
	eq(t, "para level", para.Level(), 1)
	eq(t, "para content model", para.ContentModel(), "simple")
	eq(t, "para subs", para.Substitutions(), []string{"specialcharacters", "quotes", "attributes", "replacements", "macros", "post_replacements"})
	eq(t, "para parent", para.Parent().(*asciidoctor.Section).Title(), "First Chapter")
	eq(t, "para document", para.Document().Doctitle(), "Main Title: The Subtitle")
	var keys []string
	for k := range para.Attributes() {
		keys = append(keys, k)
	}
	sortStrings(keys)
	eq(t, "para attribute keys", keys, []string{"1", "hardbreaks-option", "id", "role", "source", "title"})

	listing := blocks[1].(*asciidoctor.Block)
	eq(t, "listing", []any{listing.Context(), listing.Style(), listing.Attr("language"), listing.HasOption("linenums"), listing.ContentModel(), listing.Substitutions()},
		[]any{"listing", "source", "ruby", true, "verbatim", []string{"specialcharacters"}})

	ul := blocks[2].(*asciidoctor.List)
	items := ul.Items()
	eq(t, "ulist", []any{ul.Context(), len(items), items[0].Text(), items[1].Text(), items[0].Marker(), items[1].Source(), ul.HasItems()},
		[]any{"ulist", 2, "item one", "item <em>two</em>", "*", "item _two_", true})
	nested := items[1].Blocks()[0].(*asciidoctor.List)
	eq(t, "nested list", []any{nested.Context(), nested.Items()[0].Text()}, []any{"ulist", "nested"})
	listContent, err := ul.Content()
	if err != nil {
		t.Fatal(err)
	}
	if nodes, ok := listContent.([]asciidoctor.StructuralNode); !ok || len(nodes) != 2 {
		t.Errorf("list content: got %#v", listContent)
	}

	dl := blocks[3].(*asciidoctor.DescriptionList)
	var entries [][]string
	for _, e := range dl.Items() {
		var row []string
		for _, term := range e.Terms() {
			row = append(row, term.Text())
		}
		row = append(row, "=", e.Description().Text())
		entries = append(entries, row)
	}
	eq(t, "dlist", entries, [][]string{{"term1", "=", "def one"}, {"term2", "term3", "=", "def two and three"}})

	tb := blocks[4].(*asciidoctor.Table)
	eq(t, "table", []any{tb.Context(), tb.Title(), tb.Caption(), tb.HasHeaderOption(), tb.Frame(), tb.Grid(), tb.Attr("rowcount")},
		[]any{"table", "A table", "Table 1. ", true, "all", "all", int64(2)})
	var cols [][]any
	for _, c := range tb.Columns() {
		cols = append(cols, []any{c.ColumnNumber(), c.Width(), c.Style(), c.HorizontalAlignment(), c.Table().Title()})
	}
	eq(t, "columns", cols, [][]any{{1, 1, "", asciidoctor.AlignLeft, "A table"}, {2, 2, "asciidoc", asciidoctor.AlignLeft, "A table"}})
	head := tb.Header()
	eq(t, "header", []string{head[0].Cells[0].Text(), head[0].Cells[1].Text()}, []string{"Name", "Doc"})
	body := tb.Body()
	c0, c1 := body[0].Cells[0], body[0].Cells[1]
	eq(t, "cells", []any{c0.Text(), c0.Style(), c0.Colspan(), c0.Rowspan(), c1.Source(), c1.Style(), c1.Column().ColumnNumber()},
		[]any{"cell <strong>one</strong>", "", 1, 1, "* inner list", "asciidoc", 2})
	eq(t, "inner document", c1.InnerDocument().Blocks()[0].Context(), "ulist")
	eq(t, "inner document nested", c1.InnerDocument().IsNested(), true)
	eq(t, "inner document parent", c1.InnerDocument().ParentDocument().Doctitle(), "Main Title: The Subtitle")
	eq(t, "footer", len(tb.Footer()), 0)
	cc0, _ := c0.Content()
	cc1, _ := c1.Content()
	eq(t, "cell content", []any{cc0, cc1}, []any{[]string{"cell <strong>one</strong>"}, "<div class=\"ulist\">\n<ul>\n<li>\n<p>inner list</p>\n</li>\n</ul>\n</div>"})

	sub := blocks[5].(*asciidoctor.Section)
	img := sub.Blocks()[0].(*asciidoctor.Block)
	eq(t, "subsection", []any{sub.Context(), sub.Title(), sub.Level(), sub.Sectnum(), img.Context(), img.Attr("target"), img.Attr("alt"), img.ImageURI("diagram.png")},
		[]any{"section", "Sub Section", 2, "1.1.", "image", "diagram.png", "A diagram", "diagram.png"})

	find := func(sel asciidoctor.Selector) []asciidoctor.StructuralNode {
		t.Helper()
		r, err := d.FindBy(sel)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	var prefixes []string
	for _, b := range find(asciidoctor.Selector{Context: "paragraph"}) {
		prefixes = append(prefixes, b.(*asciidoctor.Block).Source()[:12])
	}
	eq(t, "find by context", prefixes, []string{"Preface text", "This is the "})
	eq(t, "find by role", find(asciidoctor.Selector{Role: "lead"})[0].ID(), "intro")
	eq(t, "find by id", find(asciidoctor.Selector{ID: "ch-one"})[0].Title(), "First Chapter")
	eq(t, "find by style", len(find(asciidoctor.Selector{Style: "source"})), 1)
	titles := func(nodes []asciidoctor.StructuralNode) []string {
		var out []string
		for _, n := range nodes {
			out = append(out, n.Title())
		}
		return out
	}
	eq(t, "find by filter", titles(find(asciidoctor.Selector{Context: "section", Filter: func(n asciidoctor.StructuralNode) asciidoctor.FilterResult {
		if n.Level() == 2 {
			return asciidoctor.FilterAccept
		}
		return asciidoctor.FilterSkip
	}})), []string{"Sub Section"})
	eq(t, "find by prune", titles(find(asciidoctor.Selector{Filter: func(n asciidoctor.StructuralNode) asciidoctor.FilterResult {
		if n.Context() == "section" {
			return asciidoctor.FilterPrune
		}
		return asciidoctor.FilterSkip
	}})), []string{"Main Title: The Subtitle", "Preface", "First Chapter", "Second Chapter", "Extra"})
	eq(t, "find by reject", len(find(asciidoctor.Selector{Context: "list_item", Filter: func(n asciidoctor.StructuralNode) asciidoctor.FilterResult {
		return asciidoctor.FilterReject
	}})), 0)
	eq(t, "find by stop", len(find(asciidoctor.Selector{Filter: func(n asciidoctor.StructuralNode) asciidoctor.FilterResult {
		if n.Context() == "section" {
			return asciidoctor.FilterStop
		}
		return asciidoctor.FilterAccept
	}})), 1)
	eq(t, "find traverse", []int{len(find(asciidoctor.Selector{Context: "ulist"})), len(find(asciidoctor.Selector{Context: "ulist", TraverseDocuments: true}))}, []int{2, 3})

	cat := d.Catalog()
	var refs []string
	for k := range cat.Refs {
		refs = append(refs, k)
	}
	sortStrings(refs)
	eq(t, "catalog refs", refs, []string{"_extra", "_preface", "_second_chapter", "_sub_section", "ch-one", "intro"})
	eq(t, "catalog ref node", cat.Refs["intro"].(*asciidoctor.Block).Title(), "Intro title")
	eq(t, "catalog footnotes", cat.Footnotes, []asciidoctor.Footnote{{Index: 1, ID: "fn2", Text: "Second note."}})
	eq(t, "catalog images", cat.Images, []asciidoctor.ImageReference{{Target: "diagram.png"}})
	eq(t, "catalog links", len(cat.Links), 0)
	eq(t, "footnotes", []any{d.HasFootnotes(), len(d.Footnotes())}, []any{true, 1})
	eq(t, "counters", []any{d.Counter("x", nil), d.Counter("x", nil), d.Counter("y", "a"), d.Counter("y", nil)}, []any{int64(1), int64(2), "a", "b"})

	html, err := para.Convert()
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "para convert", html, "<div id=\"intro\" class=\"paragraph lead highlight\">\n<div class=\"title\">Intro title</div>\n<p>This is the <strong>intro</strong> paragraph.<br>\nSecond line.<sup class=\"footnoteref\">[<a class=\"footnote\" href=\"#_footnotedef_1\" title=\"View footnote.\">1</a>]</sup></p>\n</div>")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// TestNodeMutation changes nodes through the Go API and checks the effect on
// conversion.
func TestNodeMutation(t *testing.T) {
	a := instance(t)
	d, err := a.Load("[#p1]\nFirst paragraph.\n\n* one\n* two\n\n|===\n|a |b\n|===", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := d.Blocks()[0].(*asciidoctor.Block)
	p.SetID("renamed")
	p.SetTitle("A *title*")
	p.AddRole("extra")
	eq(t, "add role twice", p.AddRole("extra"), false)
	p.SetLines([]string{"Replaced _text_."})
	eq(t, "set attr no overwrite", p.SetAttr("id", "x", false), false)
	eq(t, "set attr", p.SetAttr("data-x", 1, true), true)
	eq(t, "attr int", p.Attr("data-x"), int64(1))
	eq(t, "remove attr", p.RemoveAttr("data-x"), int64(1))
	eq(t, "attr or", p.AttrOr("data-x", "dflt"), "dflt")
	html, err := p.Convert()
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "converted", html, "<div id=\"renamed\" class=\"paragraph extra\">\n<div class=\"title\">A <strong>title</strong></div>\n<p>Replaced <em>text</em>.</p>\n</div>")

	p.RemoveSubstitution("quotes")
	eq(t, "has quotes", p.HasSubstitution("quotes"), false)
	p.AddSubstitution("quotes")
	p.SetSubstitutions("specialcharacters")
	eq(t, "subs", p.Substitutions(), []string{"specialcharacters"})
	p.PrependSubstitution("attributes")
	eq(t, "subs prepended", p.Substitutions(), []string{"attributes", "specialcharacters"})
	c, _ := p.Content()
	eq(t, "content without quotes", c, "Replaced _text_.")

	ul := d.Blocks()[1].(*asciidoctor.List)
	ul.Items()[0].SetSource("*changed*")
	ul.Items()[1].SetMarker("-")
	eq(t, "item text", ul.Items()[0].Text(), "<strong>changed</strong>")
	eq(t, "item marker", ul.Items()[1].Marker(), "-")

	tb := d.Blocks()[2].(*asciidoctor.Table)
	tb.SetFrame("none")
	tb.SetGrid("rows")
	cell := tb.Body()[0].Cells[0]
	cell.SetSource("_cell_")
	cell.SetHorizontalAlignment(asciidoctor.AlignRight)
	cell.SetVerticalAlignment(asciidoctor.AlignBottom)
	out, err := tb.Convert()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`frame-none grid-rows`, `<td class="tableblock halign-right valign-bottom"><p class="tableblock"><em>cell</em></p></td>`} {
		if !strings.Contains(out, want) {
			t.Errorf("table output lacks %q:\n%s", want, out)
		}
	}

	// Moving a block: append to another parent.
	sec, err := a.Load("== S\n\ntext", nil)
	if err != nil {
		t.Fatal(err)
	}
	section := sec.Sections()[0]
	section.Append(p)
	eq(t, "appended", len(section.Blocks()), 2)
	eq(t, "new parent", p.Parent().(*asciidoctor.Section).Title(), "S")

	d.SetSourceMap(true)
	eq(t, "sourcemap", d.SourceMap(), true)
	d.SetAttr("toc", "left", true)
	eq(t, "doc attr", d.Attr("toc"), "left")
}

func TestPhraseNodesInCatalog(t *testing.T) {
	d, err := instance(t).Load("See [[anchor1]]text and https://example.org[the site].", &asciidoctor.Options{CatalogAssets: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Convert(); err != nil {
		t.Fatal(err)
	}
	cat := d.Catalog()
	anchor, ok := cat.Refs["anchor1"].(*asciidoctor.PhraseNode)
	if !ok {
		t.Fatalf("anchor1 is %T", cat.Refs["anchor1"])
	}
	eq(t, "anchor", []any{anchor.Context(), anchor.Type(), anchor.ID(), anchor.Target(), anchor.IsInline()}, []any{"anchor", "ref", "anchor1", "", true})
	html, err := anchor.Convert()
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "anchor html", html, `<a id="anchor1"></a>`)
	eq(t, "links", cat.Links, []string{"https://example.org"})
}

func TestLoadErrorsAndPackageLevel(t *testing.T) {
	if _, err := instance(t).LoadFile("testdata/does-not-exist.adoc", nil); err == nil {
		t.Error("expected an error")
	}
	d, err := asciidoctor.Load("= Package Level", nil)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "title", d.Doctitle(), "Package Level")
	path, _ := filepath.Abs("testdata/ast/sample.adoc")
	d, err = asciidoctor.LoadFile(path, &asciidoctor.Options{ParseHeaderOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "header only", []any{d.Doctitle(), len(d.Blocks())}, []any{"Main Title: The Subtitle", 0})
}
