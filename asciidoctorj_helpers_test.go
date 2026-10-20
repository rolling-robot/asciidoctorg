package asciidoctor_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nokogiri "github.com/go-nokogiri/nokogiri"
)

// Helpers for the tests ported from AsciidoctorJ (asciidoctorj_*_test.go).
// The originals query the output with Jsoup; these parse it with go-nokogiri,
// which parses HTML by the HTML5 algorithm as Jsoup does.

// ajResourceDir is AsciidoctorJ's test resource directory.
const ajResourceDir = "third_party/asciidoctorj/asciidoctorj-core/src/test/resources"

// ajResources copies AsciidoctorJ's test resources to a temporary directory
// and returns it, so tests that write next to their input (as AsciidoctorJ's
// in-place conversions do) leave the submodule untouched.
func ajResources(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(ajResourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(ajResourceDir, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatalf("copying AsciidoctorJ resources (is the submodule checked out?): %v", err)
	}
	return dst
}

// ajResource returns the path of one resource in a fresh copy of the
// resource directory.
func ajResource(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(ajResources(t), filepath.FromSlash(name))
}

// htmlDoc is a parsed HTML document with Jsoup-style queries.
type htmlDoc struct {
	t   *testing.T
	doc *nokogiri.Document
}

func parseHTML(t *testing.T, src string) *htmlDoc {
	t.Helper()
	d, err := nokogiri.HTML(src)
	if err != nil {
		t.Fatalf("parsing HTML: %v", err)
	}
	return &htmlDoc{t, d}
}

func parseHTMLFile(t *testing.T, path string) *htmlDoc {
	t.Helper()
	return parseHTML(t, readFile(t, path))
}

// parseXML parses well-formed markup, such as the embeddable HTML of a
// document with a single top-level element (AsciidoctorJ parses those as XML).
func parseXML(t *testing.T, src string) *htmlDoc {
	t.Helper()
	d, err := nokogiri.XML(src)
	if err != nil {
		t.Fatalf("parsing XML: %v", err)
	}
	return &htmlDoc{t, d}
}

// Select returns the elements matching a CSS selector.
func (h *htmlDoc) Select(selector string) elements {
	h.t.Helper()
	s, err := h.doc.CSS(selector)
	if err != nil {
		h.t.Fatalf("selector %q: %v", selector, err)
	}
	return s.Nodes()
}

// XPath returns the nodes matching an XPath expression.
func (h *htmlDoc) XPath(expr string) elements {
	h.t.Helper()
	s, err := h.doc.XPath(expr)
	if err != nil {
		h.t.Fatalf("xpath %q: %v", expr, err)
	}
	return s.Nodes()
}

// ByID returns the element with an id, or nil.
func (h *htmlDoc) ByID(id string) *nokogiri.Node {
	h.t.Helper()
	return h.Select(`[id="` + id + `"]`).First()
}

// elements is a list of matched nodes, like Jsoup's Elements.
type elements []*nokogiri.Node

func (e elements) First() *nokogiri.Node {
	if len(e) == 0 {
		return nil
	}
	return e[0]
}

// Text is the normalized text of the elements, joined by spaces.
func (e elements) Text() string {
	parts := make([]string, 0, len(e))
	for _, n := range e {
		if s := text(n); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// HTML is the inner HTML of the elements, joined by newlines.
func (e elements) HTML() string {
	parts := make([]string, len(e))
	for i, n := range e {
		parts[i] = strings.TrimSpace(n.InnerHTML())
	}
	return strings.Join(parts, "\n")
}

// Attr is the attribute value of the first element that has it.
func (e elements) Attr(name string) string {
	for _, n := range e {
		if v, ok := n.Get(name); ok {
			return v
		}
	}
	return ""
}

// HasClass reports whether any element has a class.
func (e elements) HasClass(class string) bool {
	for _, n := range e {
		v, _ := n.Get("class")
		for _, c := range strings.Fields(v) {
			if c == class {
				return true
			}
		}
	}
	return false
}

// text is the text of a node with whitespace normalized, as Jsoup's text().
func text(n *nokogiri.Node) string {
	if n == nil {
		return ""
	}
	return strings.Join(strings.Fields(n.Text()), " ")
}

// attr is an attribute of a node, or "".
func attr(n *nokogiri.Node, name string) string {
	if n == nil {
		return ""
	}
	v, _ := n.Get(name)
	return v
}
