package asciidoctor_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenAsciiDocIsLoadedToDocument.

const ajDocument = `= Document Title

preamble

== Section A

paragraph

--
Exhibit A::
+
[#tiger.animal]
image::tiger.png[Tiger]
--

image::cat.png[Cat]

== Section B

paragraph

|===
|A
|B
|===`

const ajRole = `["quote", "author", "source", role="famous"]
____
A famous quote.
____`

const ajReftext = `[reftext="the first section"]
== Section One

content`

func mustLoad(t *testing.T, src string, o *asciidoctor.Options) *asciidoctor.Document {
	t.Helper()
	d, err := newInstance(t).Load(src, o)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mustLoadFile(t *testing.T, path string, o *asciidoctor.Options) *asciidoctor.Document {
	t.Helper()
	d, err := newInstance(t).LoadFile(path, o)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func eqv[T any](t *testing.T, what string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

func compact() *asciidoctor.Options {
	return &asciidoctor.Options{Extra: map[string]any{"compact": true}}
}

func TestAJLoadEmptySource(t *testing.T) {
	d := mustLoad(t, "", nil)
	eqv(t, "source", d.Source(), "")
	eqv(t, "source lines", len(d.SourceLines()), 0)
}

func TestAJLoadSectionBlocks(t *testing.T) {
	d := mustLoad(t, ajDocument, nil)
	s, ok := d.Blocks()[1].(*asciidoctor.Section)
	if !ok {
		t.Fatalf("block 1 is %T", d.Blocks()[1])
	}
	eqv(t, "index", s.Index(), 0)
	eqv(t, "section name", s.SectionName(), "section")
	eqv(t, "special", s.IsSpecial(), false)
}

func TestAJLoadBlocks(t *testing.T) {
	d := mustLoad(t, ajDocument, nil)
	eqv(t, "doctitle", d.Doctitle(), "Document Title")
	eqv(t, "blocks", len(d.Blocks()), 3)
}

func TestAJFindAllNodes(t *testing.T) {
	d := mustLoad(t, ajDocument, &asciidoctor.Options{SourceMap: true})
	all, err := d.FindBy(asciidoctor.Selector{})
	if err != nil {
		t.Fatal(err)
	}
	eqv(t, "nodes", len(all), 17)
	var cells []*asciidoctor.Cell
	for _, n := range all {
		if c, ok := n.(*asciidoctor.Cell); ok {
			cells = append(cells, c)
		}
	}
	if len(cells) != 2 {
		t.Fatalf("cells = %d, want 2", len(cells))
	}
	eqv(t, "cell 1 line", cells[0].SourceLocation().LineNumber, 23)
	eqv(t, "cell 2 line", cells[1].SourceLocation().LineNumber, 24)
}

func TestAJFindImages(t *testing.T) {
	d := mustLoad(t, ajDocument, nil)
	images, err := d.FindBy(asciidoctor.Selector{Context: "image"})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("images = %d, want 2", len(images))
	}
	eqv(t, "target", images[0].Attributes()["target"], any("tiger.png"))
	if images[0].Level() <= 0 {
		t.Errorf("level = %d, want > 0", images[0].Level())
	}
}

func TestAJDocumentOptions(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		eqv(t, "compact", mustLoad(t, ajDocument, compact()).Options()["compact"], any(true))
	})
	t.Run("file", func(t *testing.T) {
		d := mustLoadFile(t, ajResource(t, "sourcelocation.adoc"), compact())
		eqv(t, "compact", d.Options()["compact"], any(true))
	})
}

func TestAJDocumentNodeProperties(t *testing.T) {
	d := mustLoad(t, ajDocument, nil)
	eqv(t, "node name", d.NodeName(), "document")
	eqv(t, "inline", d.IsInline(), false)
	eqv(t, "block", d.IsBlock(), true)
}

func TestAJManipulateAttributes(t *testing.T) {
	o := compact()
	o.Attributes = asciidoctor.Attributes{}.DataURI(true)
	d := mustLoad(t, ajDocument, o)
	if _, ok := d.Attributes()["toc-placement"]; !ok {
		t.Error("toc-placement missing from attributes")
	}
	eqv(t, "has toc-placement", d.HasAttr("toc-placement"), true)
	eqv(t, "is toc-placement auto", d.IsAttr("toc-placement", "auto"), true)
	eqv(t, "toc-placement", d.AttrOr("toc-placement", ""), any("auto"))
}

func TestAJRoles(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		b := mustLoad(t, ajRole, nil).Blocks()[0]
		eqv(t, "role", b.Role(), "famous")
		eqv(t, "has role", b.HasRole("famous"), true)
		eqv(t, "is role", b.IsRole(), true)
		eqv(t, "roles", b.Roles(), []string{"famous"})
	})
	t.Run("add", func(t *testing.T) {
		b := mustLoad(t, ajRole, nil).Blocks()[0]
		eqv(t, "has tmpRole", b.HasRole("tmpRole"), false)
		b.AddRole("tmpRole")
		eqv(t, "has tmpRole after add", b.HasRole("tmpRole"), true)
	})
	t.Run("remove", func(t *testing.T) {
		b := mustLoad(t, ajRole, nil).Blocks()[0]
		eqv(t, "has famous", b.HasRole("famous"), true)
		b.RemoveRole("famous")
		eqv(t, "has famous after remove", b.HasRole("famous"), false)
	})
}

func TestAJReftext(t *testing.T) {
	b := mustLoad(t, ajReftext, nil).Blocks()[0]
	eqv(t, "reftext", b.Reftext(), "the first section")
	eqv(t, "has reftext", b.HasReftext(), true)
}

func TestAJAssetURIs(t *testing.T) {
	t.Run("icon", func(t *testing.T) {
		o := compact()
		o.Attributes = asciidoctor.Attributes{}.DataURI(false)
		eqv(t, "icon uri", mustLoad(t, ajDocument, o).IconURI("note"), "./images/icons/note.png")
	})
	t.Run("icon data uri", func(t *testing.T) {
		o := compact()
		o.Safe = asciidoctor.SafeModeSafe
		o.Attributes = asciidoctor.Attributes{}.DataURI(true).Icons(asciidoctor.IconsFont)
		eqv(t, "icon uri", mustLoad(t, ajDocument, o).IconURI("note"), "data:image/png;base64,")
	})
	t.Run("media", func(t *testing.T) {
		eqv(t, "media uri", mustLoad(t, ajDocument, nil).MediaURI("target"), "target")
	})
	t.Run("image", func(t *testing.T) {
		o := compact()
		o.Safe = asciidoctor.SafeModeSafe
		o.Attributes = asciidoctor.Attributes{}.DataURI(false)
		d := mustLoad(t, ajDocument, o)
		eqv(t, "image uri", d.ImageURI("target.jpg"), "target.jpg")
		eqv(t, "image uri from imagesdir", d.ImageURIFrom("target.jpg", "imagesdir"), "target.jpg")
	})
	t.Run("normalize web path", func(t *testing.T) {
		eqv(t, "path", mustLoad(t, ajDocument, nil).NormalizeWebPath("target", "", true), "target")
	})
}

func TestAJReadAsset(t *testing.T) {
	path := ajResource(t, "rendersample.asciidoc")
	o := compact()
	o.Safe = asciidoctor.SafeModeSafe
	o.Attributes = asciidoctor.Attributes{}.DataURI(false)
	got := mustLoad(t, ajDocument, o).ReadAsset(path, false)
	eqv(t, "asset", strings.ReplaceAll(got, "\r", ""), strings.ReplaceAll(readFile(t, path), "\r", ""))
}

func TestAJSetAttribute(t *testing.T) {
	d := mustLoad(t, ajDocument, nil)
	eqv(t, "set", d.SetAttr("testattribute", "testvalue", true), true)
	eqv(t, "attr", d.Attr("testattribute"), any("testvalue"))
	eqv(t, "attributes", d.Attributes()["testattribute"], any("testvalue"))
}

// AsciidoctorJ's variant puts the value into the live attribute map; in Go,
// Attributes returns a copy, so the attribute is set without overwrite.
func TestAJSetAttributeWithoutOverwrite(t *testing.T) {
	d := mustLoad(t, ajDocument, nil)
	d.Attributes()["testattribute"] = "ignored"
	eqv(t, "copy is detached", d.HasAttr("testattribute"), false)
	eqv(t, "set", d.SetAttr("testattribute", "testvalue", false), true)
	eqv(t, "attr", d.Attr("testattribute"), any("testvalue"))
	eqv(t, "keeps existing", d.SetAttr("testattribute", "other", false), false)
	eqv(t, "attributes", d.Attributes()["testattribute"], any("testvalue"))
}

func TestAJSourceLocation(t *testing.T) {
	path := ajResource(t, "sourcelocation.adoc")
	d := mustLoadFile(t, path, &asciidoctor.Options{SourceMap: true, Doctype: "book"})
	paras, err := d.FindBy(asciidoctor.Selector{Context: "paragraph"})
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range []int{3, 8} {
		c := paras[i].SourceLocation()
		eqv(t, "line", c.LineNumber, line)
		eqv(t, "path", c.Path, filepath.Base(path))
		eqv(t, "file", filepath.Base(c.File), filepath.Base(path))
		eqv(t, "dir", c.Dir, filepath.ToSlash(filepath.Dir(path)))
	}
}

func TestAJSectionAttributes(t *testing.T) {
	d := mustLoad(t, "= Document Title\n:docattr: docvalue\n\npreamble\n\n== Section A\n\nparagraph\n\n", nil)
	s := d.Blocks()[1].(*asciidoctor.Section)
	s.SetAttr("testattr", "testvalue", true)
	eqv(t, "document has testattr", d.HasAttr("testattr"), false)
	eqv(t, "section has testattr", s.HasAttr("testattr"), true)
	eqv(t, "section has inherited testattr", s.HasInheritedAttr("testattr"), true)
	eqv(t, "section is testattr", s.IsAttr("testattr", "testvalue"), true)
	eqv(t, "section has inherited docattr", s.HasInheritedAttr("docattr"), true)
	eqv(t, "section has own docattr", s.HasAttr("docattr"), false)
}

func TestAJContentModel(t *testing.T) {
	d := mustLoad(t, "= Document Title\n\nA test document with a preamble and a section.\n\n"+
		"The preamble contains multiple paragraphs to force the outer block to be compound.\n\n"+
		"== First Section\n\nAnd herein lies the problem.\n\n", nil)
	preamble := d.Blocks()[0]
	eqv(t, "preamble", preamble.ContentModel(), "compound")
	eqv(t, "preamble para 1", preamble.Blocks()[0].ContentModel(), "simple")
	eqv(t, "preamble para 2", preamble.Blocks()[1].ContentModel(), "simple")
	section := d.Blocks()[1].(*asciidoctor.Section)
	eqv(t, "section", section.ContentModel(), "compound")
	eqv(t, "section para", section.Blocks()[0].ContentModel(), "simple")
}

func TestAJDocumentParent(t *testing.T) {
	if p := mustLoad(t, "== A small Example\n\nLorem ipsum dolor sit amet:\n", nil).Parent(); p != nil {
		t.Errorf("parent = %v, want nil", p)
	}
}

func TestAJCaption(t *testing.T) {
	d := mustLoad(t, "[caption=\"Table A. \"]\n.A formal table\n|===\n|Cell in column 1, row 1\n|Cell in column 2, row 1\n|===", nil)
	if len(d.Blocks()) != 1 {
		t.Fatalf("blocks = %d", len(d.Blocks()))
	}
	eqv(t, "caption", d.Blocks()[0].Caption(), "Table A. ")
}

func TestAJAuthors(t *testing.T) {
	docWriter := asciidoctor.Author{Name: "Doc Writer", FirstName: "Doc", LastName: "Writer", Initials: "DW", Email: "doc.writer@asciidoc.org"}
	for _, tc := range []struct {
		name, src string
		want      []asciidoctor.Author
	}{
		{"none", "= Sample Document\n\nPreamble...", nil},
		{"author line", "= Sample Document\nDoc Writer <doc.writer@asciidoc.org>\nPreamble...", []asciidoctor.Author{docWriter}},
		{"attributes", "= Sample Document\n:author: Doc Writer\n:email: doc.writer@asciidoc.org\n\nPreamble...", []asciidoctor.Author{docWriter}},
		{"multiple", "= Sample Document\nDoc Writer <doc.writer@asciidoc.org>; John A. Smith <john.smith@asciidoc.org>\n\nPreamble...", []asciidoctor.Author{
			docWriter,
			{Name: "John A. Smith", FirstName: "John", MiddleName: "A.", LastName: "Smith", Initials: "JAS", Email: "john.smith@asciidoc.org"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mustLoad(t, tc.src, nil).Authors()
			if len(got) != len(tc.want) {
				t.Fatalf("authors = %#v, want %#v", got, tc.want)
			}
			for i := range got {
				eqv(t, "author", got[i], tc.want[i])
			}
		})
	}
}

func TestAJRevisionInfo(t *testing.T) {
	d := mustLoad(t, "= Sample Document\nDoc Writer <doc.writer@asciidoc.org>\nv1.0, 2013-05-20: First draft\n\nPreamble...", nil)
	eqv(t, "revision", d.RevisionInfo(), asciidoctor.RevisionInfo{Date: "2013-05-20", Number: "1.0", Remark: "First draft"})
}

func TestAJSourceAndSourceLines(t *testing.T) {
	for _, tc := range []struct {
		name, src, source string
	}{
		{"sections", "= Document Title\n\n== Section A\n\nSection A paragraph.\n\n=== Section A Subsection\n\nSection A 'subsection' paragraph.\n\n",
			"= Document Title\n\n== Section A\n\nSection A paragraph.\n\n=== Section A Subsection\n\nSection A 'subsection' paragraph."},
		{"trailing whitespace", "= Document Title\n\n== Section\n\nHello\t  \n", "= Document Title\n\n== Section\n\nHello"},
		{"unresolved attributes", "= Document Title\n:an-attribute: a-value\n\nThis is {an-attribute}",
			"= Document Title\n:an-attribute: a-value\n\nThis is {an-attribute}"},
		{"unresolved includes", "= Document Title\n\n== Section\n\ninclude::partial.adoc[]",
			"= Document Title\n\n== Section\n\ninclude::partial.adoc[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := mustLoad(t, tc.src, nil)
			eqv(t, "source", d.Source(), tc.source)
			eqv(t, "source lines", d.SourceLines(), strings.Split(tc.source, "\n"))
		})
	}
}
