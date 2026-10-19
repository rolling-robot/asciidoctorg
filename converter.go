package asciidoctor

import (
	"fmt"
	"io"
	"os"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/asciidoctor-go/asciidoctor/internal/bridge"
)

// Converter converts document nodes to an output format. Asciidoctor calls
// Convert for the document and, as the converter asks for their content, for
// each node; transform is the node's name ("document", "section",
// "paragraph", "inline_quoted", ...) unless the caller chose another.
//
// One Converter value serves every document; per-document state can be kept
// against node.Document(). A Converter may also implement ConverterHandles
// and OutputWriter.
type Converter interface {
	Convert(node Node, transform string, opts map[string]any) (string, error)
}

// ConverterHandles reports whether a converter handles a transform. Without
// it, a converter handles every transform.
type ConverterHandles interface {
	Handles(transform string) bool
}

// OutputWriter writes the converted output itself, for formats that need more
// than writing the string out (binary or multi-file output). Asciidoctor calls
// it instead of writing the output to the target file or stream.
type OutputWriter interface {
	WriteOutput(output string, w io.Writer) error
}

// ConverterFunc adapts a function to a Converter.
type ConverterFunc func(node Node, transform string, opts map[string]any) (string, error)

func (f ConverterFunc) Convert(node Node, transform string, opts map[string]any) (string, error) {
	return f(node, transform, opts)
}

// BackendTraits describe the output of a converter. Empty fields are derived
// from the backend name as Asciidoctor does (backend "html5" has basebackend
// "html", filetype "html" and suffix ".html").
type BackendTraits struct {
	Basebackend   string
	Filetype      string
	OutfileSuffix string
	// HTMLSyntax is "html" or "xml" for HTML-based output.
	HTMLSyntax string
	// SupportsTemplates lets template directories override nodes.
	SupportsTemplates bool
}

func (t *BackendTraits) toRuby() map[ruby.Symbol]any {
	m := map[ruby.Symbol]any{}
	if t == nil {
		return m
	}
	set := func(k ruby.Symbol, v string) {
		if v != "" {
			m[k] = v
		}
	}
	set("basebackend", t.Basebackend)
	set("filetype", t.Filetype)
	set("outfilesuffix", t.OutfileSuffix)
	set("htmlsyntax", t.HTMLSyntax)
	if t.SupportsTemplates {
		m["supports_templates"] = true
	}
	return m
}

// RegisterConverter makes c the converter for the given backends, replacing
// built-in or earlier converters of the same names ("*" registers it for
// every backend without a converter of its own).
func (a *Asciidoctor) RegisterConverter(c Converter, traits *BackendTraits, backends ...string) error {
	if len(backends) == 0 {
		return fmt.Errorf("asciidoctor: RegisterConverter needs at least one backend")
	}
	h := &converterHandler{a: a, c: c}
	_, writes := c.(OutputWriter)
	return a.do(func() error {
		g, err := a.glue()
		if err != nil {
			return err
		}
		id := a.b.Register(h)
		names := make([]any, len(backends))
		for i, b := range backends {
			names[i] = b
		}
		if _, err := g.Call("register_converter", id, traits.toRuby(), writes, names); err != nil {
			a.b.Unregister(id)
			return err
		}
		return nil
	})
}

type converterHandler struct {
	a *Asciidoctor
	c Converter
}

func (h *converterHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	switch method {
	case "convert":
		var opts map[string]any
		if len(args) > 2 && !args[2].IsNil() {
			opts = attributesMap(args[2])
		}
		out, err := h.c.Convert(h.a.wrapNode(args[0]), rubyString(args[1]), opts)
		if err != nil {
			return nil, rubyError(err)
		}
		return out, nil
	case "handles":
		if hc, ok := h.c.(ConverterHandles); ok {
			return hc.Handles(rubyString(args[0])), nil
		}
		return true, nil
	case "write":
		return nil, rubyError(h.write(rubyString(args[0]), args[1]))
	}
	return nil, unknownMethod(method)
}

// write implements Writer#write for an OutputWriter: the target is a file
// path or an IO-like object.
func (h *converterHandler) write(output string, target ruby.Value) error {
	ow := h.c.(OutputWriter)
	if s, ok := target.Go().(string); ok {
		f, err := os.Create(s)
		if err != nil {
			return err
		}
		if err := ow.WriteOutput(output, f); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	if w, ok := target.Go().(*bridge.Writer); ok {
		return ow.WriteOutput(output, w.W)
	}
	return ow.WriteOutput(output, rubyIO{a: h.a, v: target})
}

// rubyIO writes to a Ruby IO-like object.
type rubyIO struct {
	a *Asciidoctor
	v ruby.Value
}

func (w rubyIO) Write(p []byte) (int, error) {
	if _, err := w.a.callValue(w.v, "write", string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// BuiltinConverter is one of Asciidoctor's own converters, for a Go converter
// that handles some nodes itself and delegates the others.
type BuiltinConverter struct {
	a *Asciidoctor
	v ruby.Value
}

// BuiltinConverter returns a new instance of Asciidoctor's own converter for
// backend ("html5", "docbook5" or "manpage"), even when RegisterConverter has
// replaced it for that backend.
func (a *Asciidoctor) BuiltinConverter(backend string) (*BuiltinConverter, error) {
	var v ruby.Value
	err := a.do(func() error {
		g, err := a.glue()
		if err != nil {
			return err
		}
		v, err = g.Call("builtin_converter", backend)
		return err
	})
	if err != nil {
		return nil, err
	}
	if v.IsNil() {
		return nil, fmt.Errorf("asciidoctor: no converter for backend %q", backend)
	}
	return &BuiltinConverter{a: a, v: v}, nil
}

// Convert converts node with the built-in converter. opts are the options
// the Go converter received (their names become Ruby symbols again).
func (c *BuiltinConverter) Convert(node Node, transform string, opts map[string]any) (string, error) {
	var o any
	if opts != nil {
		m := make(map[ruby.Symbol]any, len(opts))
		for k, v := range opts {
			m[ruby.Symbol(k)] = v
		}
		o = m
	}
	v, err := c.a.callValue(c.v, "convert", node, transform, o)
	if err != nil {
		return "", err
	}
	return rubyString(v), nil
}
