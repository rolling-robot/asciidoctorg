package asciidoctor

import (
	ruby "github.com/go-embedded-ruby/ruby"
)

// Table is a table block.
type Table struct {
	structural
}

// Row is a row of table cells.
type Row struct {
	Cells []*Cell
}

// HasHeaderOption reports whether the table's first row is a header row.
func (t *Table) HasHeaderOption() bool { return rubyBool(t.call("has_header_option")) }

// Columns are the column specifications.
func (t *Table) Columns() []*Column {
	var out []*Column
	for _, c := range t.a.arrayItems(t.call("columns")) {
		if col, ok := t.a.wrapNode(c).(*Column); ok {
			out = append(out, col)
		}
	}
	return out
}

func (t *Table) rows(section string) []*Row {
	rows, err := t.a.callValue(t.call("rows"), section)
	if err != nil {
		panic(err)
	}
	var out []*Row
	for _, r := range t.a.arrayItems(rows) {
		row := &Row{}
		for _, c := range t.a.arrayItems(r) {
			if cell, ok := t.a.wrapNode(c).(*Cell); ok {
				row.Cells = append(row.Cells, cell)
			}
		}
		out = append(out, row)
	}
	return out
}

// Header are the header rows.
func (t *Table) Header() []*Row { return t.rows("head") }

// Body are the body rows.
func (t *Table) Body() []*Row { return t.rows("body") }

// Footer are the footer rows.
func (t *Table) Footer() []*Row { return t.rows("foot") }

// Frame is the frame style ("all", "ends", "sides" or "none").
func (t *Table) Frame() string {
	if s := t.AttrString("frame"); s != "" {
		return s
	}
	return "all"
}

// SetFrame sets the frame style.
func (t *Table) SetFrame(frame string) { t.SetAttr("frame", frame, true) }

// Grid is the grid style ("all", "cols", "rows" or "none").
func (t *Table) Grid() string {
	if s := t.AttrString("grid"); s != "" {
		return s
	}
	return "all"
}

// SetGrid sets the grid style.
func (t *Table) SetGrid(grid string) { t.SetAttr("grid", grid, true) }

// AssignColumnWidths recomputes the column widths from their relative
// widths (call it after changing columns).
func (t *Table) AssignColumnWidths() { t.call("assign_column_widths") }

// HorizontalAlignment is a cell or column alignment: "left", "center" or
// "right".
type HorizontalAlignment string

// VerticalAlignment is a cell or column alignment: "top", "middle" or
// "bottom".
type VerticalAlignment string

const (
	AlignLeft   HorizontalAlignment = "left"
	AlignCenter HorizontalAlignment = "center"
	AlignRight  HorizontalAlignment = "right"
	AlignTop    VerticalAlignment   = "top"
	AlignMiddle VerticalAlignment   = "middle"
	AlignBottom VerticalAlignment   = "bottom"
)

// Column is a table column specification.
type Column struct {
	node
}

// Style is the column style, such as "asciidoc", "literal" or "" for default.
func (c *Column) Style() string { return rubyString(c.call("style")) }

// SetStyle sets the column style.
func (c *Column) SetStyle(style string) { c.call("style=", symbolOrNil(style)) }

// Table is the table the column belongs to.
func (c *Column) Table() *Table {
	t, _ := c.a.wrapNode(c.call("table")).(*Table)
	return t
}

// ColumnNumber is the 1-based column number.
func (c *Column) ColumnNumber() int {
	n, _ := c.Attr("colnumber").(int64)
	return int(n)
}

// Width is the relative width of the column.
func (c *Column) Width() int {
	n, _ := c.Attr("width").(int64)
	return int(n)
}

// SetWidth sets the relative width; call Table.AssignColumnWidths afterwards.
func (c *Column) SetWidth(width int) { c.SetAttr("width", width, true) }

// HorizontalAlignment is the column's horizontal alignment.
func (c *Column) HorizontalAlignment() HorizontalAlignment {
	return HorizontalAlignment(stringOr(c.AttrString("halign"), "left"))
}

// SetHorizontalAlignment sets the column's horizontal alignment.
func (c *Column) SetHorizontalAlignment(a HorizontalAlignment) { c.SetAttr("halign", string(a), true) }

// VerticalAlignment is the column's vertical alignment.
func (c *Column) VerticalAlignment() VerticalAlignment {
	return VerticalAlignment(stringOr(c.AttrString("valign"), "top"))
}

// SetVerticalAlignment sets the column's vertical alignment.
func (c *Column) SetVerticalAlignment(a VerticalAlignment) { c.SetAttr("valign", string(a), true) }

// Cell is a table cell.
type Cell struct {
	structural
}

// Column is the column the cell belongs to.
func (c *Cell) Column() *Column {
	col, _ := c.a.wrapNode(c.call("column")).(*Column)
	return col
}

// Colspan is the number of columns the cell spans (1 when it does not).
func (c *Cell) Colspan() int { return max(rubyInt(c.call("colspan")), 1) }

// Rowspan is the number of rows the cell spans (1 when it does not).
func (c *Cell) Rowspan() int { return max(rubyInt(c.call("rowspan")), 1) }

// Text is the converted text of the cell.
func (c *Cell) Text() string { return rubyString(c.call("text")) }

// Source is the text of the cell before substitutions.
func (c *Cell) Source() string {
	return rubyString(c.call("instance_variable_get", ruby.Symbol("@text")))
}

// SetSource replaces the text of the cell; it is converted when read.
func (c *Cell) SetSource(source string) { c.call("text=", source) }

// Style is the cell style, such as "asciidoc" or "" for default.
func (c *Cell) Style() string { return rubyString(c.call("style")) }

// SetStyle sets the cell style.
func (c *Cell) SetStyle(style string) { c.call("style=", symbolOrNil(style)) }

// InnerDocument is the document of an AsciiDoc cell (nil otherwise).
func (c *Cell) InnerDocument() *Document { return c.a.wrapDocument(c.call("inner_document")) }

// SetInnerDocument replaces the document of an AsciiDoc cell.
func (c *Cell) SetInnerDocument(d *Document) {
	var v any
	if d != nil {
		v = d.Value()
	}
	c.call("instance_variable_set", ruby.Symbol("@inner_document"), v)
}

// HorizontalAlignment is the cell's horizontal alignment.
func (c *Cell) HorizontalAlignment() HorizontalAlignment {
	return HorizontalAlignment(stringOr(c.AttrString("halign"), "left"))
}

// SetHorizontalAlignment sets the cell's horizontal alignment.
func (c *Cell) SetHorizontalAlignment(a HorizontalAlignment) { c.SetAttr("halign", string(a), true) }

// VerticalAlignment is the cell's vertical alignment.
func (c *Cell) VerticalAlignment() VerticalAlignment {
	return VerticalAlignment(stringOr(c.AttrString("valign"), "top"))
}

// SetVerticalAlignment sets the cell's vertical alignment.
func (c *Cell) SetVerticalAlignment(a VerticalAlignment) { c.SetAttr("valign", string(a), true) }

func stringOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func symbolOrNil(s string) any {
	if s == "" {
		return nil
	}
	return ruby.Symbol(s)
}
