package asciidoctor

import (
	"strings"

	ruby "github.com/go-embedded-ruby/ruby"
)

// Section is a document section.
type Section struct {
	structural
}

// Index is the position of the section among its parent's sections.
func (s *Section) Index() int { return rubyInt(s.call("index")) }

// SectionName is the section type: "section", or for special sections
// "preface", "appendix", "glossary", "bibliography", "index", and so on.
func (s *Section) SectionName() string { return rubyString(s.call("sectname")) }

// SetSectionName sets the section type.
func (s *Section) SetSectionName(name string) { s.call("sectname=", name) }

// IsSpecial reports whether the section is a special section.
func (s *Section) IsSpecial() bool { return rubyBool(s.call("special")) }

// SetSpecial marks the section as special.
func (s *Section) SetSpecial(special bool) { s.call("special=", special) }

// IsNumbered reports whether the section is numbered.
func (s *Section) IsNumbered() bool { return rubyBool(s.call("numbered")) }

// SetNumbered sets whether the section is numbered.
func (s *Section) SetNumbered(numbered bool) { s.call("numbered=", numbered) }

// Sectnum is the section number with its parents', such as "2.1." (the
// trailing delimiter is included).
func (s *Section) Sectnum() string { return rubyString(s.call("sectnum")) }

// SectnumWith is Sectnum with the given delimiter; append, if not "", replaces
// the trailing delimiter.
func (s *Section) SectnumWith(delimiter, appendStr string) string {
	if appendStr == "" {
		return rubyString(s.call("sectnum", delimiter))
	}
	return rubyString(s.call("sectnum", delimiter, appendStr))
}

// Block is a block such as a paragraph, listing, image or admonition.
type Block struct {
	structural
}

// Lines are the source lines of the block content.
func (b *Block) Lines() []string { return rubyStrings(b.call("lines")) }

// SetLines replaces the source lines.
func (b *Block) SetLines(lines []string) { b.call("lines=", append([]string{}, lines...)) }

// Source is the source of the block content (Lines joined by newlines).
func (b *Block) Source() string { return rubyString(b.call("source")) }

// SetSource replaces the source; it is split into lines.
func (b *Block) SetSource(source string) {
	b.SetLines(strings.Split(source, "\n"))
}

// List is an unordered, ordered or callout list ("ulist", "olist", "colist").
type List struct {
	structural
}

// Items are the list items.
func (l *List) Items() []*ListItem {
	var out []*ListItem
	for _, b := range l.Blocks() {
		if it, ok := b.(*ListItem); ok {
			out = append(out, it)
		}
	}
	return out
}

// HasItems reports whether the list has items.
func (l *List) HasItems() bool { return rubyBool(l.call("items?")) }

// ListItem is an item of a list, or a term or description of a description
// list.
type ListItem struct {
	structural
}

// Marker is the list marker, such as "*", "." or "1.".
func (li *ListItem) Marker() string { return rubyString(li.call("marker")) }

// SetMarker sets the list marker.
func (li *ListItem) SetMarker(marker string) { li.call("marker=", marker) }

// Text is the converted text of the item.
func (li *ListItem) Text() string { return rubyString(li.call("text")) }

// HasText reports whether the item has text.
func (li *ListItem) HasText() bool { return rubyBool(li.call("text?")) }

// Source is the text of the item before substitutions.
func (li *ListItem) Source() string {
	return rubyString(li.call("instance_variable_get", ruby.Symbol("@text")))
}

// SetSource replaces the text of the item; it is converted when read.
func (li *ListItem) SetSource(source string) { li.call("text=", source) }

// DescriptionList is a description list ("dlist"): terms with descriptions.
type DescriptionList struct {
	structural
}

// HasItems reports whether the list has entries.
func (l *DescriptionList) HasItems() bool { return rubyBool(l.call("items?")) }

// Items are the entries of the list.
func (l *DescriptionList) Items() []*DescriptionListEntry {
	var out []*DescriptionListEntry
	for _, pair := range l.a.arrayItems(l.call("items")) {
		out = append(out, &DescriptionListEntry{a: l.a, v: pair})
	}
	return out
}

// DescriptionListEntry is one or more terms with a description.
type DescriptionListEntry struct {
	a *Asciidoctor
	v ruby.Value // [terms, description]
}

func (e *DescriptionListEntry) part(i int) ruby.Value {
	v, err := e.a.callValue(e.v, "[]", i)
	if err != nil {
		panic(err)
	}
	return v
}

// Terms are the terms of the entry.
func (e *DescriptionListEntry) Terms() []*ListItem {
	var out []*ListItem
	for _, t := range e.a.arrayItems(e.part(0)) {
		if it, ok := e.a.wrapNode(t).(*ListItem); ok {
			out = append(out, it)
		}
	}
	return out
}

// Description is the description, or nil if the terms have none.
func (e *DescriptionListEntry) Description() *ListItem {
	it, _ := e.a.wrapNode(e.part(1)).(*ListItem)
	return it
}

// SetDescription replaces the description (nil removes it).
func (e *DescriptionListEntry) SetDescription(description *ListItem) {
	var v any
	if description != nil {
		v = description.Value()
	}
	if _, err := e.a.callValue(e.v, "[]=", 1, v); err != nil {
		panic(err)
	}
}

// PhraseNode is an inline node: a quoted phrase, link, anchor, image,
// footnote, keyboard or menu macro, and so on.
type PhraseNode struct {
	node
}

// Type is the kind of inline node within its context, such as "strong" for
// a quoted phrase or "link" for an anchor.
func (p *PhraseNode) Type() string { return rubyString(p.call("type")) }

// Text is the text of the node.
func (p *PhraseNode) Text() string { return rubyString(p.call("text")) }

// Target is the target of the node, such as a URL, image path or ref id.
func (p *PhraseNode) Target() string { return rubyString(p.call("target")) }

// Alt is the alternative text of an inline image.
func (p *PhraseNode) Alt() string { return rubyString(p.call("alt")) }

// Convert converts the node with the document's converter.
func (p *PhraseNode) Convert() (string, error) {
	v, err := p.callErr("convert")
	if err != nil {
		return "", err
	}
	return rubyString(v), nil
}
