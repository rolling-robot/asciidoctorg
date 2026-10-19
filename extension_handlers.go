package asciidoctor

import (
	"fmt"
	"reflect"
	"strconv"

	ruby "github.com/go-embedded-ruby/ruby"
)

// extensionHandler is the Go side of a Ruby proxy processor.
type extensionHandler interface {
	Dispatch(c *ruby.Call, method string, args []ruby.Value) (any, error)
	bind(a *Asciidoctor)
	setID(id int64)
	id() int64
}

type handlerBase struct {
	a      *Asciidoctor
	handle int64
}

func (h *handlerBase) bind(a *Asciidoctor) { h.a = a }
func (h *handlerBase) setID(id int64)      { h.handle = id }
func (h *handlerBase) id() int64           { return h.handle }

func (h *handlerBase) argCount(method string, args []ruby.Value, n int) error {
	if len(args) != n {
		return ruby.NewError("ArgumentError", fmt.Sprintf("%s: wrong number of arguments (given %d, expected %d)", method, len(args), n))
	}
	return nil
}

// attributesMap converts the attributes Hash a processor receives.
// Positional attributes appear under their position ("1", "2", ...).
func attributesMap(v ruby.Value) map[string]any {
	m, _ := goValue(v).(map[any]any)
	out := make(map[string]any, len(m))
	for k, val := range m {
		switch key := k.(type) {
		case string:
			out[key] = val
		case int64:
			out[strconv.FormatInt(key, 10)] = val
		}
	}
	return out
}

func unknownMethod(method string) error {
	return ruby.NewError("NoMethodError", "asciidoctor-go: unsupported processor method "+method)
}

type preprocessorHandler struct {
	handlerBase
	p Preprocessor
}

func (h *preprocessorHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	if method != "process" {
		return nil, unknownMethod(method)
	}
	if err := h.argCount(method, args, 2); err != nil {
		return nil, err
	}
	r, err := h.p.Process(h.a.wrapDocument(args[0]), &PreprocessorReader{&Reader{a: h.a, v: args[1]}})
	if err != nil {
		return nil, rubyError(err)
	}
	if r == nil {
		return args[1], nil
	}
	return r.v, nil
}

type includeProcessorHandler struct {
	handlerBase
	p IncludeProcessor
}

func (h *includeProcessorHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	switch method {
	case "handles":
		if err := h.argCount(method, args, 2); err != nil {
			return nil, err
		}
		return h.p.Handles(h.a.wrapDocument(args[0]), rubyString(args[1])), nil
	case "process":
		if err := h.argCount(method, args, 4); err != nil {
			return nil, err
		}
		err := h.p.Process(h.a.wrapDocument(args[0]), &PreprocessorReader{&Reader{a: h.a, v: args[1]}}, rubyString(args[2]), attributesMap(args[3]))
		return nil, rubyError(err)
	}
	return nil, unknownMethod(method)
}

type treeprocessorHandler struct {
	handlerBase
	p Treeprocessor
}

func (h *treeprocessorHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	if method != "process" {
		return nil, unknownMethod(method)
	}
	if err := h.argCount(method, args, 1); err != nil {
		return nil, err
	}
	d, err := h.p.Process(h.a.wrapDocument(args[0]))
	if err != nil {
		return nil, rubyError(err)
	}
	if d == nil {
		return nil, nil
	}
	return d.Value(), nil
}

type postprocessorHandler struct {
	handlerBase
	p Postprocessor
}

func (h *postprocessorHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	if method != "process" {
		return nil, unknownMethod(method)
	}
	if err := h.argCount(method, args, 2); err != nil {
		return nil, err
	}
	out, err := h.p.Process(h.a.wrapDocument(args[0]), rubyString(args[1]))
	if err != nil {
		return nil, rubyError(err)
	}
	return out, nil
}

type docinfoProcessorHandler struct {
	handlerBase
	p DocinfoProcessor
}

func (h *docinfoProcessorHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	if method != "process" {
		return nil, unknownMethod(method)
	}
	if err := h.argCount(method, args, 1); err != nil {
		return nil, err
	}
	out, err := h.p.Process(h.a.wrapDocument(args[0]))
	if err != nil {
		return nil, rubyError(err)
	}
	return out, nil
}

type blockProcessorHandler struct {
	handlerBase
	p BlockProcessor
}

func (h *blockProcessorHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	if method != "process" {
		return nil, unknownMethod(method)
	}
	if err := h.argCount(method, args, 3); err != nil {
		return nil, err
	}
	parent, _ := h.a.wrapNode(args[0]).(StructuralNode)
	b, err := h.p.Process(parent, &Reader{a: h.a, v: args[1]}, attributesMap(args[2]))
	if err != nil {
		return nil, rubyError(err)
	}
	return nodeValue(b), nil
}

type blockMacroProcessorHandler struct {
	handlerBase
	p BlockMacroProcessor
}

func (h *blockMacroProcessorHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	if method != "process" {
		return nil, unknownMethod(method)
	}
	if err := h.argCount(method, args, 3); err != nil {
		return nil, err
	}
	parent, _ := h.a.wrapNode(args[0]).(StructuralNode)
	b, err := h.p.Process(parent, rubyString(args[1]), attributesMap(args[2]))
	if err != nil {
		return nil, rubyError(err)
	}
	return nodeValue(b), nil
}

type inlineMacroProcessorHandler struct {
	handlerBase
	p      InlineMacroProcessor
	regexp string
}

func (h *inlineMacroProcessorHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	if method != "process" {
		return nil, unknownMethod(method)
	}
	if err := h.argCount(method, args, 3); err != nil {
		return nil, err
	}
	n, err := h.p.Process(h.a.wrapNode(args[0]), rubyString(args[1]), attributesMap(args[2]))
	if err != nil {
		return nil, rubyError(err)
	}
	return nodeValue(n), nil
}

// nodeValue returns the Ruby object of a node, or nil (also for a typed nil
// pointer such as (*Block)(nil)).
func nodeValue(n Node) any {
	if n == nil {
		return nil
	}
	if rv := reflect.ValueOf(n); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil
	}
	return n.Value()
}

// setInlineRegexp sets the regular expression of an inline macro processor
// from Ruby regexp source. Call it inside a.do.
func setInlineRegexp(a *Asciidoctor, proc ruby.Value, source string) error {
	glue, err := a.glue()
	if err != nil {
		return err
	}
	_, err = glue.Call("set_regexp", proc, source)
	return err
}
