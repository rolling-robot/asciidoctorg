package asciidoctor

import (
	"errors"

	ruby "github.com/go-embedded-ruby/ruby"
)

// SyntaxHighlighterAdapter integrates a syntax highlighter, selected with the
// source-highlighter attribute. It provides docinfo (head or footer content
// such as stylesheets and scripts), and may also implement Highlighter (to
// highlight source on the server), Formatter (to control the <pre> wrapper)
// and StylesheetWriter (to write a stylesheet next to the output).
//
// One adapter value serves every document.
type SyntaxHighlighterAdapter interface {
	// HasDocinfo reports whether the adapter adds content at location
	// ("head" or "footer").
	HasDocinfo(location string) bool
	// Docinfo returns the content for location. opts include "linkcss",
	// "cdn_base_url" and "self_closing_tag_slash".
	Docinfo(location string, doc *Document, opts map[string]any) (string, error)
}

// HighlightResult is highlighted source.
type HighlightResult struct {
	// Highlighted is the highlighted source (HTML).
	Highlighted string
	// LineOffset, if positive, is the line number where the highlighted source
	// starts, for highlighters that add line numbers (callouts follow it).
	LineOffset int
}

// Highlighter highlights source on the server.
type Highlighter interface {
	// Highlight highlights source in language lang. opts include "callouts",
	// "css_mode", "highlight_lines", "number_lines", "start_line_number" and
	// "style".
	Highlight(node *Block, source, lang string, opts map[string]any) (HighlightResult, error)
}

// Formatter wraps source (highlighted or not) for output. Without it, the
// output is <pre class="NAME highlight"><code data-lang="LANG">...</code></pre>.
type Formatter interface {
	// Format returns the markup for node. Use node.Content() for the
	// (possibly highlighted) source. opts include "nowrap".
	Format(node *Block, lang string, opts map[string]any) (string, error)
}

// StylesheetWriter writes a stylesheet when the document links stylesheets.
type StylesheetWriter interface {
	// IsWriteStylesheet reports whether to write a stylesheet for doc.
	IsWriteStylesheet(doc *Document) bool
	// WriteStylesheet writes the stylesheet into toDir.
	WriteStylesheet(doc *Document, toDir string) error
}

// RegisterSyntaxHighlighter registers an adapter under the given names (the
// values of source-highlighter it handles), replacing built-in adapters of the
// same names.
func (a *Asciidoctor) RegisterSyntaxHighlighter(adapter SyntaxHighlighterAdapter, names ...string) error {
	if len(names) == 0 {
		return errors.New("asciidoctor: RegisterSyntaxHighlighter needs at least one name")
	}
	var caps []any
	if _, ok := adapter.(Highlighter); ok {
		caps = append(caps, ruby.Symbol("highlight"))
	}
	if _, ok := adapter.(Formatter); ok {
		caps = append(caps, ruby.Symbol("format"))
	}
	if _, ok := adapter.(StylesheetWriter); ok {
		caps = append(caps, ruby.Symbol("stylesheet"))
	}
	h := &highlighterHandler{a: a, s: adapter}
	return a.do(func() error {
		g, err := a.glue()
		if err != nil {
			return err
		}
		id := a.b.Register(h)
		ns := make([]any, len(names))
		for i, n := range names {
			ns[i] = n
		}
		if caps == nil {
			caps = []any{}
		}
		if _, err := g.Call("register_syntax_highlighter", id, caps, ns); err != nil {
			a.b.Unregister(id)
			return err
		}
		return nil
	})
}

type highlighterHandler struct {
	a *Asciidoctor
	s SyntaxHighlighterAdapter
}

func (h *highlighterHandler) block(v ruby.Value) *Block {
	b, _ := h.a.wrapNode(v).(*Block)
	return b
}

func optsMap(v ruby.Value) map[string]any {
	if v.IsNil() {
		return map[string]any{}
	}
	return attributesMap(v)
}

func (h *highlighterHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	switch method {
	case "has_docinfo":
		return h.s.HasDocinfo(rubyString(args[0])), nil
	case "docinfo":
		out, err := h.s.Docinfo(rubyString(args[0]), h.a.wrapDocument(args[1]), optsMap(args[2]))
		return out, rubyError(err)
	case "highlight":
		res, err := h.s.(Highlighter).Highlight(h.block(args[0]), rubyString(args[1]), rubyString(args[2]), optsMap(args[3]))
		if err != nil {
			return nil, rubyError(err)
		}
		if res.LineOffset > 0 {
			return []any{res.Highlighted, res.LineOffset}, nil
		}
		return res.Highlighted, nil
	case "format":
		out, err := h.s.(Formatter).Format(h.block(args[0]), rubyString(args[1]), optsMap(args[2]))
		return out, rubyError(err)
	case "write_stylesheet?":
		return h.s.(StylesheetWriter).IsWriteStylesheet(h.a.wrapDocument(args[0])), nil
	case "write_stylesheet":
		return nil, rubyError(h.s.(StylesheetWriter).WriteStylesheet(h.a.wrapDocument(args[0]), rubyString(args[1])))
	}
	return nil, unknownMethod(method)
}
