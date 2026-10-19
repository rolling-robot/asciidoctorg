package asciidoctor

import (
	"errors"
	"fmt"

	ruby "github.com/go-embedded-ruby/ruby"
)

// The functions in this file create nodes, typically inside extensions. They
// are Asciidoctor's processor helpers (create_block, parse_content, ...); the
// new node belongs to the document of its parent, but is not added to the
// parent: return it from the processor, or Append it.

var errNilParent = errors.New("asciidoctor: parent node is nil")

// helper calls a method of the shared Ruby processor helper on n's instance.
func helper(n Node, method string, args ...any) (ruby.Value, error) {
	if n == nil {
		return ruby.Value{}, errNilParent
	}
	a := n.base().a
	var v ruby.Value
	err := a.do(func() error {
		h, err := a.b.Const("AsciidoctorGo::HELPER")
		if err != nil {
			return err
		}
		for i, x := range args {
			args[i] = rubyArg(x)
		}
		v, err = h.Call(method, args...)
		return err
	})
	return v, err
}

// glueCall calls a function of the AsciidoctorGo module on n's instance.
func glueCall(n Node, method string, args ...any) (ruby.Value, error) {
	if n == nil {
		return ruby.Value{}, errNilParent
	}
	a := n.base().a
	g, err := a.glueValue()
	if err != nil {
		return ruby.Value{}, err
	}
	return a.callValue(g, method, args...)
}

// sourceArg converts block source: a string, lines, or nil.
func sourceArg(source any) (any, error) {
	switch s := source.(type) {
	case nil:
		return nil, nil
	case string:
		return s, nil
	case []string:
		return append([]string{}, s...), nil
	}
	return nil, fmt.Errorf("asciidoctor: block source must be a string or []string, not %T", source)
}

// blockOptions converts creation options; content_model and subs values
// become Symbols, as Asciidoctor expects.
func blockOptions(opts map[string]any) map[ruby.Symbol]any {
	out := map[ruby.Symbol]any{}
	for k, v := range opts {
		switch k {
		case "content_model":
			if s, ok := v.(string); ok {
				v = ruby.Symbol(s)
			}
		case "subs":
			switch s := v.(type) {
			case []string:
				syms := make([]any, len(s))
				for i, x := range s {
					syms[i] = ruby.Symbol(x)
				}
				v = syms
			case string:
				if s == "default" {
					v = ruby.Symbol(s)
				}
			}
		case "type":
			if s, ok := v.(string); ok {
				v = ruby.Symbol(s)
			}
		}
		out[ruby.Symbol(k)] = v
	}
	return out
}

func attrsArg(attrs Attributes) map[string]any {
	if attrs == nil {
		return map[string]any{}
	}
	return attrs.toRuby()
}

func wrapAs[T Node](n Node, v ruby.Value, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, wrapError(err)
	}
	t, ok := n.base().a.wrapNode(v).(T)
	if !ok {
		return zero, fmt.Errorf("asciidoctor: unexpected node type %s", v.ClassName())
	}
	return t, nil
}

// CreateBlock creates a block of the given context ("paragraph", "listing",
// "image", "open", ...) with source (a string, []string or nil) and
// attributes. opts are Block options such as "content_model" and "subs"
// (a substitution group name, "default", or []string of substitutions).
func CreateBlock(parent StructuralNode, context string, source any, attrs Attributes, opts map[string]any) (*Block, error) {
	src, err := sourceArg(source)
	if err != nil {
		return nil, err
	}
	v, err := helper(parent, "create_block", parent, ruby.Symbol(context), src, attrsArg(attrs), blockOptions(opts))
	return wrapAs[*Block](parent, v, err)
}

// CreateParagraph creates a paragraph; see CreateBlock.
func CreateParagraph(parent StructuralNode, source any, attrs Attributes, opts map[string]any) (*Block, error) {
	return CreateBlock(parent, "paragraph", source, attrs, opts)
}

// CreateOpenBlock creates an open block; see CreateBlock.
func CreateOpenBlock(parent StructuralNode, source any, attrs Attributes, opts map[string]any) (*Block, error) {
	return CreateBlock(parent, "open", source, attrs, opts)
}

// CreateExampleBlock creates an example block; see CreateBlock.
func CreateExampleBlock(parent StructuralNode, source any, attrs Attributes, opts map[string]any) (*Block, error) {
	return CreateBlock(parent, "example", source, attrs, opts)
}

// CreatePassBlock creates a passthrough block; see CreateBlock.
func CreatePassBlock(parent StructuralNode, source any, attrs Attributes, opts map[string]any) (*Block, error) {
	return CreateBlock(parent, "pass", source, attrs, opts)
}

// CreateListingBlock creates a listing block; see CreateBlock.
func CreateListingBlock(parent StructuralNode, source any, attrs Attributes, opts map[string]any) (*Block, error) {
	return CreateBlock(parent, "listing", source, attrs, opts)
}

// CreateLiteralBlock creates a literal block; see CreateBlock.
func CreateLiteralBlock(parent StructuralNode, source any, attrs Attributes, opts map[string]any) (*Block, error) {
	return CreateBlock(parent, "literal", source, attrs, opts)
}

// CreateImageBlock creates an image block. attrs must include "target";
// "alt" defaults to a readable form of the file name, and "title" becomes a
// figure caption.
func CreateImageBlock(parent StructuralNode, attrs Attributes, opts map[string]any) (*Block, error) {
	v, err := helper(parent, "create_image_block", parent, attrsArg(attrs), blockOptions(opts))
	return wrapAs[*Block](parent, v, err)
}

// SectionOptions configure CreateSection.
type SectionOptions struct {
	// Level is the section level (default: one below the parent).
	Level *int
	// Numbered, when set, overrides whether the section is numbered.
	Numbered *bool
}

// CreateSection creates a section titled title. A "style" attribute makes it
// a special section (such as "appendix"); an "id" of false (or no id with
// sectids set) controls the generated id.
func CreateSection(parent StructuralNode, title string, attrs Attributes, opts *SectionOptions) (*Section, error) {
	o := map[ruby.Symbol]any{}
	if opts != nil {
		if opts.Level != nil {
			o["level"] = *opts.Level
		}
		if opts.Numbered != nil {
			o["numbered"] = *opts.Numbered
		}
	}
	v, err := helper(parent, "create_section", parent, title, attrsArg(attrs), o)
	return wrapAs[*Section](parent, v, err)
}

// CreateList creates an unordered ("ulist"), ordered ("olist") or callout
// ("colist") list. Use CreateDescriptionList for description lists.
func CreateList(parent StructuralNode, context string, attrs Attributes) (*List, error) {
	if context == "dlist" {
		return nil, errors.New(`asciidoctor: use CreateDescriptionList for a "dlist"`)
	}
	v, err := helper(parent, "create_list", parent, ruby.Symbol(context), attrsArg(attrs))
	return wrapAs[*List](parent, v, err)
}

// CreateDescriptionList creates a description list.
func CreateDescriptionList(parent StructuralNode, attrs Attributes) (*DescriptionList, error) {
	v, err := helper(parent, "create_list", parent, ruby.Symbol("dlist"), attrsArg(attrs))
	return wrapAs[*DescriptionList](parent, v, err)
}

// CreateListItem creates an item of list (a *List or *DescriptionList) with
// text, which is converted when read.
func CreateListItem(list StructuralNode, text string) (*ListItem, error) {
	v, err := helper(list, "create_list_item", list, nilIfEmpty(text))
	return wrapAs[*ListItem](list, v, err)
}

// AddItem appends an item to the list.
func (l *List) AddItem(item *ListItem) { l.Append(item) }

// AddEntry appends an entry with the given terms and description (which may
// be nil) to the list.
func (l *DescriptionList) AddEntry(terms []*ListItem, description *ListItem) {
	ts := make([]any, len(terms))
	for i, t := range terms {
		ts[i] = t.Value()
	}
	items := l.call("blocks")
	if _, err := l.a.callValue(items, "push", []any{ts, nodeValue(description)}); err != nil {
		panic(err)
	}
}

// CreatePhraseNode creates an inline node of the given context ("quoted",
// "anchor", "image", "footnote", ...). opts may set "type" (such as
// "strong" for a quoted phrase, or "link" or "xref" for an anchor), "target"
// and "id". A quoted phrase without a type is unquoted text.
func CreatePhraseNode(parent Node, context, text string, attrs Attributes, opts map[string]any) (*PhraseNode, error) {
	o := blockOptions(opts)
	o["attributes"] = attrsArg(attrs)
	v, err := helper(parent, "create_inline", parent, ruby.Symbol(context), text, o)
	return wrapAs[*PhraseNode](parent, v, err)
}

// CreateAnchor creates an anchor (link, xref, ref or bibref, as opts "type"
// says); see CreatePhraseNode.
func CreateAnchor(parent Node, text string, attrs Attributes, opts map[string]any) (*PhraseNode, error) {
	return CreatePhraseNode(parent, "anchor", text, attrs, opts)
}

// ParseContent parses AsciiDoc content (a string, []string or *Reader) and
// appends the resulting blocks to parent. attrs are the attributes for the
// first block.
func ParseContent(parent StructuralNode, content any, attrs Attributes) error {
	var c any
	switch x := content.(type) {
	case *Reader:
		c = x.Value()
	default:
		var err error
		if c, err = sourceArg(content); err != nil {
			return err
		}
		if s, ok := c.(string); ok {
			c = splitLines(s)
		}
	}
	var a any
	if attrs != nil {
		a = attrs.toRuby()
	}
	_, err := helper(parent, "parse_content", parent, c, a)
	return wrapError(err)
}

// ParseAttributes parses an attribute list such as `role=x,title="T"`, naming
// the positional attributes as given. With subAttributes, attribute
// references in the list are replaced first.
func ParseAttributes(node Node, attrlist string, positional []string, subAttributes bool) (map[string]any, error) {
	o := map[ruby.Symbol]any{}
	if positional != nil {
		o["positional_attributes"] = append([]string{}, positional...)
	}
	if subAttributes {
		o["sub_attributes"] = true
	}
	v, err := helper(node, "parse_attributes", node, attrlist, o)
	if err != nil {
		return nil, wrapError(err)
	}
	return attributesMap(v), nil
}

// CreateDocument creates an empty nested document whose parent is parent,
// for example the inner document of an AsciiDoc table cell.
func CreateDocument(parent *Document) (*Document, error) {
	if parent == nil {
		return nil, errNilParent
	}
	v, err := glueCall(parent, "create_document", parent)
	return wrapAs[*Document](parent, v, err)
}

// CreateTable creates an empty table. Add columns with CreateTableColumn and
// Table.AddColumn, rows with Table.AddRow, then call AssignColumnWidths.
func CreateTable(parent StructuralNode, attrs Attributes) (*Table, error) {
	if parent == nil {
		return nil, errNilParent
	}
	a := parent.base().a
	var v ruby.Value
	err := a.do(func() error {
		cls, err := a.b.Const("Asciidoctor::Table")
		if err != nil {
			return err
		}
		v, err = cls.Call("new", parent.Value(), attrsArg(attrs))
		return err
	})
	t, err := wrapAs[*Table](parent, v, err)
	if err != nil {
		return nil, err
	}
	t.SetAttr("rowcount", 0, false)
	return t, nil
}

// CreateTableColumn creates column number index+1 of table.
func CreateTableColumn(table *Table, index int, attrs Attributes) (*Column, error) {
	if table == nil {
		return nil, errNilParent
	}
	a := table.a
	var v ruby.Value
	err := a.do(func() error {
		cls, err := a.b.Const("Asciidoctor::Table::Column")
		if err != nil {
			return err
		}
		v, err = cls.Call("new", table.Value(), index, attrsArg(attrs))
		return err
	})
	return wrapAs[*Column](table, v, err)
}

// CreateTableCell creates a cell of column with text.
func CreateTableCell(column *Column, text string, attrs Attributes) (*Cell, error) {
	if column == nil {
		return nil, errNilParent
	}
	v, err := glueCall(column, "create_table_cell", column, text, attrsArg(attrs), nil)
	return wrapAs[*Cell](column, v, err)
}

// CreateTableCellDocument creates an AsciiDoc cell of column whose content is
// inner (see CreateDocument).
func CreateTableCellDocument(column *Column, inner *Document, attrs Attributes) (*Cell, error) {
	if column == nil {
		return nil, errNilParent
	}
	v, err := glueCall(column, "create_table_cell", column, nil, attrsArg(attrs), nodeValue(inner))
	return wrapAs[*Cell](column, v, err)
}

// AddColumn appends a column to the table and updates its column count.
func (t *Table) AddColumn(c *Column) {
	cols := t.call("columns")
	if _, err := t.a.callValue(cols, "push", c.Value()); err != nil {
		panic(err)
	}
	t.SetAttr("colcount", len(t.a.arrayItems(cols)), true)
}

// TableSection names a part of a table.
type TableSection string

const (
	TableHeader TableSection = "head"
	TableBody   TableSection = "body"
	TableFooter TableSection = "foot"
)

// AddRow appends a row of cells to a part of the table and updates the
// table's row count.
func (t *Table) AddRow(section TableSection, cells ...*Cell) {
	rows := t.call("rows")
	part, err := t.a.callValue(rows, string(section))
	if err != nil {
		panic(err)
	}
	row := make([]any, len(cells))
	for i, c := range cells {
		row[i] = c.Value()
	}
	if _, err := t.a.callValue(part, "push", row); err != nil {
		panic(err)
	}
	t.SetAttr("rowcount", len(t.Header())+len(t.Body())+len(t.Footer()), true)
}

func splitLines(s string) []string {
	lines := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
