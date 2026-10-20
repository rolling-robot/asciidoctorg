package asciidoctor

import (
	"fmt"

	ruby "github.com/go-embedded-ruby/ruby"
)

// StructuralNode is a block-level node: the document, a section, a block, a
// list or list item, a table or table cell.
type StructuralNode interface {
	Node

	// Title is the converted title ("" if none).
	Title() string
	// SetTitle sets the title source; it is converted when read.
	SetTitle(title string)
	HasTitle() bool
	// Caption is the caption, such as "Table 1. " ("" if none).
	Caption() string
	SetCaption(caption string)
	// CaptionedTitle is the caption followed by the title.
	CaptionedTitle() string
	// Style is the block style, such as "source" or "NOTE" ("" if none).
	Style() string
	SetStyle(style string)
	// Level is the section level of the node, or of the enclosing section.
	Level() int
	SetLevel(level int)
	// Numeral is the number assigned to the node ("" if none).
	Numeral() string
	SetNumeral(numeral string)
	// ContentModel is how the content is parsed: "compound", "simple",
	// "verbatim", "raw", "empty" (or "skip" for comments).
	ContentModel() string
	SetContentModel(model string)
	// SourceLocation is where the node starts in the source; it is recorded
	// only when the SourceMap option is set (nil otherwise).
	SourceLocation() *Cursor

	// Blocks are the child blocks.
	Blocks() []StructuralNode
	HasBlocks() bool
	// Append adds a child block and makes this node its parent.
	Append(child StructuralNode)
	// SetBlock replaces the child block at index and makes this node its
	// parent. Like the methods below, it panics if index is out of range.
	SetBlock(index int, child StructuralNode)
	// InsertBlock inserts a child block before index (len(Blocks()) appends)
	// and makes this node its parent.
	InsertBlock(index int, child StructuralNode)
	// RemoveBlock removes the child block at index and returns it.
	RemoveBlock(index int) StructuralNode
	// Sections are the child sections.
	Sections() []*Section
	HasSections() bool

	// Content returns the converted content: a string for most blocks; for
	// lists, the items ([]StructuralNode); for table cells, the converted
	// paragraphs ([]string) or, for AsciiDoc cells, a string.
	Content() (any, error)
	// Convert converts the node with the document's converter.
	Convert() (string, error)
	// FindBy returns the block-level nodes below (and including) this node
	// that match the selector, in document order.
	FindBy(selector Selector) ([]StructuralNode, error)
	// Xreftext is the text used for a cross reference to the node in the
	// given xrefstyle ("full", "short", "basic", or "" for the default).
	Xreftext(xrefstyle string) string

	// Substitutions are the names of the substitutions applied to the content.
	Substitutions() []string
	SetSubstitutions(subs ...string)
	HasSubstitution(sub string) bool
	AddSubstitution(sub string)
	PrependSubstitution(sub string)
	RemoveSubstitution(sub string)
}

// Cursor is a position in a source file.
type Cursor struct {
	// LineNumber is the 1-based line number.
	LineNumber int
	// File is the absolute path of the file ("" for a string source).
	File string
	// Dir is the directory of the file.
	Dir string
	// Path is the path of the file as given, or "<stdin>".
	Path string
}

// FilterResult is the verdict of a Selector filter.
type FilterResult int

const (
	// FilterAccept accepts the node and continues into its children.
	FilterAccept FilterResult = iota
	// FilterSkip skips the node, but visits its children.
	FilterSkip
	// FilterReject rejects the node and its children.
	FilterReject
	// FilterPrune accepts the node, but not its children.
	FilterPrune
	// FilterStop ends the search; the node is not accepted.
	FilterStop
)

// Selector selects nodes for FindBy. Empty fields match any node.
type Selector struct {
	// Context matches the node context, such as "section" or "listing".
	Context string
	// Style matches the block style.
	Style string
	// Role matches a role of the node.
	Role string
	// ID matches the node id; the search stops at the first node with it.
	ID string
	// TraverseDocuments also searches the documents of AsciiDoc table cells.
	TraverseDocuments bool
	// Filter, if set, is applied to each node the other fields match.
	Filter func(StructuralNode) FilterResult
}

func (s Selector) toRuby() map[ruby.Symbol]any {
	m := map[ruby.Symbol]any{}
	if s.Context != "" {
		m["context"] = ruby.Symbol(s.Context)
	}
	if s.Style != "" {
		m["style"] = s.Style
	}
	if s.Role != "" {
		m["role"] = s.Role
	}
	if s.ID != "" {
		m["id"] = s.ID
	}
	if s.TraverseDocuments {
		m["traverse_documents"] = true
	}
	return m
}

// structural implements StructuralNode.
type structural struct {
	node
}

func (n *structural) Title() string         { return rubyString(n.call("title")) }
func (n *structural) SetTitle(title string) { n.call("title=", nilIfEmpty(title)) }
func (n *structural) HasTitle() bool        { return rubyBool(n.call("title?")) }
func (n *structural) Caption() string       { return rubyString(n.call("caption")) }
func (n *structural) SetCaption(c string)   { n.call("caption=", nilIfEmpty(c)) }
func (n *structural) CaptionedTitle() string {
	return rubyString(n.call("captioned_title"))
}
func (n *structural) Style() string         { return rubyString(n.call("style")) }
func (n *structural) SetStyle(style string) { n.call("style=", nilIfEmpty(style)) }
func (n *structural) Level() int            { return rubyInt(n.call("level")) }
func (n *structural) SetLevel(level int)    { n.call("level=", level) }
func (n *structural) Numeral() string       { return rubyString(n.call("numeral")) }
func (n *structural) SetNumeral(num string) { n.call("numeral=", nilIfEmpty(num)) }
func (n *structural) ContentModel() string  { return rubyString(n.call("content_model")) }
func (n *structural) SetContentModel(m string) {
	n.call("content_model=", ruby.Symbol(m))
}

func (n *structural) SourceLocation() *Cursor {
	return n.a.cursor(n.call("source_location"))
}

func (n *structural) Blocks() []StructuralNode { return n.a.wrapStructurals(n.call("blocks")) }
func (n *structural) HasBlocks() bool          { return rubyBool(n.call("blocks?")) }
func (n *structural) Append(child StructuralNode) {
	n.call("<<", child.Value())
}

func (n *structural) blockList(index, limit int) ruby.Value {
	blocks := n.call("blocks")
	if count := rubyInt(n.a.mustCall(blocks, "size")); index < 0 || index > count-1+limit {
		panic(fmt.Sprintf("asciidoctor: block index %d out of range [0:%d]", index, count+limit))
	}
	return blocks
}

func (n *structural) SetBlock(index int, child StructuralNode) {
	n.a.mustCall(n.blockList(index, 0), "[]=", index, child.Value())
	child.base().call("parent=", n.v)
}

func (n *structural) InsertBlock(index int, child StructuralNode) {
	n.a.mustCall(n.blockList(index, 1), "insert", index, child.Value())
	child.base().call("parent=", n.v)
}

func (n *structural) RemoveBlock(index int) StructuralNode {
	removed, _ := n.a.wrapNode(n.a.mustCall(n.blockList(index, 0), "delete_at", index)).(StructuralNode)
	return removed
}

func (n *structural) Sections() []*Section {
	var out []*Section
	for _, b := range n.a.wrapStructurals(n.call("sections")) {
		if s, ok := b.(*Section); ok {
			out = append(out, s)
		}
	}
	return out
}

func (n *structural) HasSections() bool { return rubyBool(n.call("sections?")) }

func (n *structural) Content() (any, error) {
	v, err := n.callErr("content")
	if err != nil {
		return nil, err
	}
	return n.a.contentValue(v), nil
}

func (n *structural) Convert() (string, error) {
	v, err := n.callErr("convert")
	if err != nil {
		return "", err
	}
	return rubyString(v), nil
}

func (n *structural) FindBy(selector Selector) ([]StructuralNode, error) {
	return n.a.findBy(n.v, selector)
}

func (n *structural) Xreftext(xrefstyle string) string {
	return rubyString(n.call("xreftext", nilIfEmpty(xrefstyle)))
}

func (n *structural) Substitutions() []string { return rubyStrings(n.call("subs")) }

func (n *structural) SetSubstitutions(subs ...string) {
	syms := make([]any, len(subs))
	for i, s := range subs {
		syms[i] = ruby.Symbol(s)
	}
	arr, err := n.a.callValue(n.v, "subs")
	if err != nil {
		panic(err)
	}
	if _, err := n.a.callValue(arr, "replace", syms); err != nil {
		panic(err)
	}
}

func (n *structural) HasSubstitution(sub string) bool {
	return rubyBool(n.call("sub?", ruby.Symbol(sub)))
}

func (n *structural) AddSubstitution(sub string) {
	if !n.HasSubstitution(sub) {
		if _, err := n.a.callValue(n.call("subs"), "push", ruby.Symbol(sub)); err != nil {
			panic(err)
		}
	}
}

func (n *structural) PrependSubstitution(sub string) {
	if !n.HasSubstitution(sub) {
		if _, err := n.a.callValue(n.call("subs"), "unshift", ruby.Symbol(sub)); err != nil {
			panic(err)
		}
	}
}

func (n *structural) RemoveSubstitution(sub string) { n.call("remove_sub", ruby.Symbol(sub)) }

// findBy runs find_by on a Ruby node, with selector.Filter as the block.
func (a *Asciidoctor) findBy(v ruby.Value, selector Selector) ([]StructuralNode, error) {
	var handle any
	if f := selector.Filter; f != nil {
		id := a.b.Register(filterHandler{a: a, f: f})
		defer a.b.Unregister(id)
		handle = id
	}
	g, err := a.glueValue()
	if err != nil {
		return nil, err
	}
	res, err := a.callValue(g, "find_by", v, selector.toRuby(), handle)
	if err != nil {
		return nil, err
	}
	return a.wrapStructurals(res), nil
}

type filterHandler struct {
	a *Asciidoctor
	f func(StructuralNode) FilterResult
}

func (h filterHandler) Dispatch(_ *ruby.Call, _ string, args []ruby.Value) (any, error) {
	n, _ := h.a.wrapNode(args[0]).(StructuralNode)
	if n == nil {
		return false, nil
	}
	switch h.f(n) {
	case FilterAccept:
		return true, nil
	case FilterReject:
		return ruby.Symbol("reject"), nil
	case FilterPrune:
		return ruby.Symbol("prune"), nil
	case FilterStop:
		return ruby.Symbol("stop"), nil
	default:
		return false, nil
	}
}
