package asciidoctor

import (
	ruby "github.com/go-embedded-ruby/ruby"
)

// Document is the root of a parsed document.
type Document struct {
	structural
}

// Title is a document title partitioned into main title and subtitle.
type Title struct {
	// Main is the main title (the part before the last ": ").
	Main string
	// Subtitle is the subtitle ("" if none).
	Subtitle string
	// Combined is the full title.
	Combined string
	// Sanitized reports whether markup was removed from the title.
	Sanitized bool
}

// Author is a document author from the header.
type Author struct {
	Name       string
	FirstName  string
	MiddleName string
	LastName   string
	Initials   string
	Email      string
}

// RevisionInfo is the revision line of the document header.
type RevisionInfo struct {
	Date   string
	Number string
	Remark string
}

// Doctitle is the converted document title: the header title, or else the
// title of the first section ("" if none).
func (d *Document) Doctitle() string { return rubyString(d.call("doctitle")) }

// StructuredDoctitle returns the document title partitioned into main title
// and subtitle; sanitize removes markup. It returns nil if there is no title.
func (d *Document) StructuredDoctitle(sanitize bool) *Title {
	v := d.call("doctitle", map[ruby.Symbol]any{"partition": true, "sanitize": sanitize})
	if v.IsNil() {
		return nil
	}
	get := func(m string) ruby.Value {
		r, err := d.a.callValue(v, m)
		if err != nil {
			panic(err)
		}
		return r
	}
	return &Title{
		Main:      rubyString(get("main")),
		Subtitle:  rubyString(get("subtitle")),
		Combined:  rubyString(get("combined")),
		Sanitized: rubyBool(get("sanitized?")),
	}
}

// Authors lists the authors from the header.
func (d *Document) Authors() []Author {
	var out []Author
	for _, v := range d.a.arrayItems(d.call("authors")) {
		get := func(m string) string {
			r, err := d.a.callValue(v, m)
			if err != nil {
				panic(err)
			}
			return rubyString(r)
		}
		out = append(out, Author{
			Name: get("name"), FirstName: get("firstname"), MiddleName: get("middlename"),
			LastName: get("lastname"), Initials: get("initials"), Email: get("email"),
		})
	}
	return out
}

// RevisionInfo is the revision from the header.
func (d *Document) RevisionInfo() RevisionInfo {
	return RevisionInfo{Date: d.AttrString("revdate"), Number: d.AttrString("revnumber"), Remark: d.AttrString("revremark")}
}

// HasHeader reports whether the document has a header.
func (d *Document) HasHeader() bool { return rubyBool(d.call("header?")) }

// Source is the document source, after preprocessing.
func (d *Document) Source() string { return rubyString(d.call("source")) }

// SourceLines are the lines of Source.
func (d *Document) SourceLines() []string { return rubyStrings(d.call("source_lines")) }

// IsBasebackend reports whether the backend's base is backend (for
// example "html" for "html5").
func (d *Document) IsBasebackend(backend string) bool {
	return rubyBool(d.call("basebackend?", backend))
}

// Backend is the backend in use, such as "html5".
func (d *Document) Backend() string { return rubyString(d.call("backend")) }

// Doctype is the document type, such as "article".
func (d *Document) Doctype() string { return rubyString(d.call("doctype")) }

// SafeMode is the safe mode the document was loaded with.
func (d *Document) SafeMode() SafeMode {
	level, _ := d.call("safe").Go().(int64)
	return safeModeFromLevel(level)
}

// BaseDir is the base directory for resolving relative paths.
func (d *Document) BaseDir() string { return rubyString(d.call("base_dir")) }

// OutfileSuffix is the extension of the output file, such as ".html".
func (d *Document) OutfileSuffix() string { return rubyString(d.call("outfilesuffix")) }

// Options are the API options the document was loaded with, by name.
func (d *Document) Options() map[string]any {
	m, _ := goValue(d.call("options")).(map[any]any)
	out := make(map[string]any, len(m))
	for k, v := range m {
		if s, ok := k.(string); ok {
			out[s] = v
		}
	}
	return out
}

// Counter increments the named counter and returns its new value (an int64,
// or a string for a letter sequence). seed, if non-nil, is the initial value
// of a new counter.
func (d *Document) Counter(name string, seed any) any {
	return goValue(d.call("counter", name, seed))
}

// SourceMap reports whether source locations are recorded.
func (d *Document) SourceMap() bool { return rubyBool(d.call("sourcemap")) }

// SetSourceMap enables or disables recording source locations (effective for
// blocks parsed afterwards).
func (d *Document) SetSourceMap(enabled bool) { d.call("sourcemap=", enabled) }

// IsNested reports whether the document is the inner document of a table
// cell.
func (d *Document) IsNested() bool { return rubyBool(d.call("nested?")) }

// ParentDocument is the document that contains this nested document (nil
// for a top-level document).
func (d *Document) ParentDocument() *Document {
	return d.a.wrapDocument(d.call("parent_document"))
}

// HasFootnotes reports whether the document has footnotes.
func (d *Document) HasFootnotes() bool { return rubyBool(d.call("footnotes?")) }

// Footnotes lists the document's footnotes in order.
func (d *Document) Footnotes() []Footnote { return d.a.footnotes(d.call("footnotes")) }

// Footnote is a footnote from the catalog.
type Footnote struct {
	Index int
	ID    string
	Text  string
}

// ImageReference is an image recorded in the catalog.
type ImageReference struct {
	Target    string
	Imagesdir string
}

// Catalog holds what the parser recorded about a document.
type Catalog struct {
	// Refs maps ids to the nodes that define them (blocks, sections or
	// inline anchors).
	Refs map[string]Node
	// Footnotes are the footnotes in order.
	Footnotes []Footnote
	// Images are the images referenced, when the CatalogAssets option is set.
	Images []ImageReference
	// Links are the link targets, when the CatalogAssets option is set.
	Links []string
	// Includes are the paths of included files (without extension), mapped
	// to whether all of the file was included.
	Includes map[string]bool
}

// Catalog returns the document catalog.
func (d *Document) Catalog() *Catalog {
	cat := d.call("catalog")
	get := func(key string) ruby.Value {
		r, err := d.a.callValue(cat, "[]", ruby.Symbol(key))
		if err != nil {
			panic(err)
		}
		return r
	}
	c := &Catalog{Refs: map[string]Node{}, Includes: map[string]bool{}}
	refs := get("refs")
	keys, err := d.a.callValue(refs, "keys")
	if err != nil {
		panic(err)
	}
	for _, k := range rubyStrings(keys) {
		r, err := d.a.callValue(refs, "[]", k)
		if err != nil {
			panic(err)
		}
		c.Refs[k] = d.a.wrapNode(r)
	}
	c.Footnotes = d.a.footnotes(get("footnotes"))
	for _, img := range d.a.arrayItems(get("images")) {
		target, err := d.a.callValue(img, "target")
		if err != nil {
			panic(err)
		}
		dir, err := d.a.callValue(img, "imagesdir")
		if err != nil {
			panic(err)
		}
		c.Images = append(c.Images, ImageReference{Target: rubyString(target), Imagesdir: rubyString(dir)})
	}
	links := get("links")
	if l, err := d.a.callValue(links, "to_a"); err == nil {
		c.Links = rubyStrings(l)
	} else {
		panic(err)
	}
	if m, ok := goValue(get("includes")).(map[any]any); ok {
		for k, v := range m {
			if s, ok := k.(string); ok {
				b, _ := v.(bool)
				c.Includes[s] = b
			}
		}
	}
	return c
}

func (a *Asciidoctor) footnotes(v ruby.Value) []Footnote {
	var out []Footnote
	for _, fn := range a.arrayItems(v) {
		get := func(m string) ruby.Value {
			r, err := a.callValue(fn, m)
			if err != nil {
				panic(err)
			}
			return r
		}
		out = append(out, Footnote{Index: rubyInt(get("index")), ID: rubyString(get("id")), Text: rubyString(get("text"))})
	}
	return out
}
