package asciidoctor

import (
	ruby "github.com/go-embedded-ruby/ruby"
)

// Do runs fn with exclusive access to the interpreter, for direct use of
// Node.Value and other Ruby values. Inside a callback the access is already
// held and fn runs directly.
func (a *Asciidoctor) Do(fn func(in *ruby.Interp) error) error {
	return a.do(func() error { return fn(a.b.Interp()) })
}

// rubyArg converts Go API values (nodes, attributes) into what the
// interpreter accepts.
func rubyArg(x any) any {
	switch v := x.(type) {
	case Node:
		if v == nil {
			return nil
		}
		return v.Value()
	case Attributes:
		return v.toRuby()
	case []Node:
		out := make([]any, len(v))
		for i, n := range v {
			out[i] = rubyArg(n)
		}
		return out
	case []StructuralNode:
		out := make([]any, len(v))
		for i, n := range v {
			out[i] = rubyArg(n)
		}
		return out
	}
	return x
}

// callValue calls a Ruby method with exclusive access to the interpreter.
func (a *Asciidoctor) callValue(v ruby.Value, method string, args ...any) (ruby.Value, error) {
	for i, x := range args {
		args[i] = rubyArg(x)
	}
	var r ruby.Value
	err := a.do(func() error {
		var err error
		r, err = v.Call(method, args...)
		return err
	})
	return r, err
}

func (a *Asciidoctor) glueValue() (ruby.Value, error) {
	var g ruby.Value
	err := a.do(func() error {
		var err error
		g, err = a.glue()
		return err
	})
	return g, err
}

// wrapNode wraps a Ruby node in the matching Go type; nil and non-nodes
// become nil.
func (a *Asciidoctor) wrapNode(v ruby.Value) Node {
	if v.IsNil() {
		return nil
	}
	g, err := a.glueValue()
	if err != nil {
		panic(err)
	}
	kind, err := a.callValue(g, "node_kind", v)
	if err != nil {
		panic(err)
	}
	base := node{a: a, v: v}
	switch rubyString(kind) {
	case "document":
		return &Document{structural{base}}
	case "section":
		return &Section{structural{base}}
	case "list":
		return &List{structural{base}}
	case "dlist":
		return &DescriptionList{structural{base}}
	case "list_item":
		return &ListItem{structural{base}}
	case "table":
		return &Table{structural{base}}
	case "column":
		return &Column{base}
	case "cell":
		return &Cell{structural{base}}
	case "inline":
		return &PhraseNode{base}
	case "block":
		return &Block{structural{base}}
	}
	return nil
}

func (a *Asciidoctor) wrapDocument(v ruby.Value) *Document {
	d, _ := a.wrapNode(v).(*Document)
	return d
}

// wrapStructurals wraps a Ruby Array of blocks.
func (a *Asciidoctor) wrapStructurals(v ruby.Value) []StructuralNode {
	items := a.arrayItems(v)
	out := make([]StructuralNode, 0, len(items))
	for _, it := range items {
		if n, ok := a.wrapNode(it).(StructuralNode); ok {
			out = append(out, n)
		}
	}
	return out
}

// arrayItems returns the elements of a Ruby Array as Values.
func (a *Asciidoctor) arrayItems(v ruby.Value) []ruby.Value {
	if v.IsNil() {
		return nil
	}
	size, err := a.callValue(v, "size")
	if err != nil {
		panic(err)
	}
	n := rubyInt(size)
	out := make([]ruby.Value, n)
	for i := 0; i < n; i++ {
		e, err := a.callValue(v, "[]", i)
		if err != nil {
			panic(err)
		}
		out[i] = e
	}
	return out
}

// cursor converts a Reader::Cursor; nil becomes nil.
func (a *Asciidoctor) cursor(v ruby.Value) *Cursor {
	if v.IsNil() {
		return nil
	}
	get := func(m string) ruby.Value {
		r, err := a.callValue(v, m)
		if err != nil {
			panic(err)
		}
		return r
	}
	return &Cursor{
		LineNumber: rubyInt(get("lineno")),
		File:       rubyString(get("file")),
		Dir:        rubyString(get("dir")),
		Path:       rubyString(get("path")),
	}
}

// contentValue converts the result of #content: a String, an Array of nodes
// (lists) or an Array of Strings (table cells).
func (a *Asciidoctor) contentValue(v ruby.Value) any {
	if v.IsNil() {
		return ""
	}
	switch x := v.Go().(type) {
	case string:
		return x
	case []any:
		allStrings := true
		strs := make([]string, len(x))
		for i, e := range x {
			s, ok := e.(string)
			if !ok {
				allStrings = false
				break
			}
			strs[i] = s
		}
		if allStrings {
			return strs
		}
		return a.wrapStructurals(v)
	}
	return v.String()
}
