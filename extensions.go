package asciidoctor

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	ruby "github.com/go-embedded-ruby/ruby"
)

// Preprocessor runs before the document is parsed and may rewrite its source
// through the reader.
type Preprocessor interface {
	// Process returns the reader to parse from, or nil to keep reader.
	Process(doc *Document, reader *PreprocessorReader) (*Reader, error)
}

// IncludeProcessor handles include directives whose target it claims.
type IncludeProcessor interface {
	// Handles reports whether the processor handles the include target.
	Handles(doc *Document, target string) bool
	// Process supplies the included content, usually with reader.PushInclude.
	// attributes are those of the include directive.
	Process(doc *Document, reader *PreprocessorReader, target string, attributes map[string]any) error
}

// Treeprocessor runs after parsing and may change the document tree.
type Treeprocessor interface {
	// Process returns a replacement document, or nil to keep doc.
	Process(doc *Document) (*Document, error)
}

// Postprocessor runs after conversion and may change the output.
type Postprocessor interface {
	// Process returns the (possibly changed) output.
	Process(doc *Document, output string) (string, error)
}

// DocinfoProcessor contributes content to the document head or footer.
type DocinfoProcessor interface {
	// Process returns the content to insert.
	Process(doc *Document) (string, error)
}

// BlockProcessor handles a delimited block or paragraph with a custom style
// ([name]).
type BlockProcessor interface {
	// Process returns the block to insert in place of the source block, or
	// nil to drop it. reader holds the lines of the block; attributes are the
	// block attributes.
	Process(parent StructuralNode, reader *Reader, attributes map[string]any) (StructuralNode, error)
}

// BlockMacroProcessor handles a block macro (name::target[attributes]).
type BlockMacroProcessor interface {
	// Process returns the block to insert, or nil to drop the macro.
	Process(parent StructuralNode, target string, attributes map[string]any) (StructuralNode, error)
}

// InlineMacroProcessor handles an inline macro (name:target[attributes]).
type InlineMacroProcessor interface {
	// Process returns the inline node that replaces the macro, or nil to
	// remove it.
	Process(parent Node, target string, attributes map[string]any) (*PhraseNode, error)
}

// PreprocessorFunc adapts a function to a Preprocessor.
type PreprocessorFunc func(doc *Document, reader *PreprocessorReader) (*Reader, error)

func (f PreprocessorFunc) Process(doc *Document, reader *PreprocessorReader) (*Reader, error) {
	return f(doc, reader)
}

// TreeprocessorFunc adapts a function to a Treeprocessor.
type TreeprocessorFunc func(doc *Document) (*Document, error)

func (f TreeprocessorFunc) Process(doc *Document) (*Document, error) { return f(doc) }

// PostprocessorFunc adapts a function to a Postprocessor.
type PostprocessorFunc func(doc *Document, output string) (string, error)

func (f PostprocessorFunc) Process(doc *Document, output string) (string, error) {
	return f(doc, output)
}

// DocinfoProcessorFunc adapts a function to a DocinfoProcessor.
type DocinfoProcessorFunc func(doc *Document) (string, error)

func (f DocinfoProcessorFunc) Process(doc *Document) (string, error) { return f(doc) }

// BlockProcessorFunc adapts a function to a BlockProcessor.
type BlockProcessorFunc func(parent StructuralNode, reader *Reader, attributes map[string]any) (StructuralNode, error)

func (f BlockProcessorFunc) Process(parent StructuralNode, reader *Reader, attributes map[string]any) (StructuralNode, error) {
	return f(parent, reader, attributes)
}

// BlockMacroProcessorFunc adapts a function to a BlockMacroProcessor.
type BlockMacroProcessorFunc func(parent StructuralNode, target string, attributes map[string]any) (StructuralNode, error)

func (f BlockMacroProcessorFunc) Process(parent StructuralNode, target string, attributes map[string]any) (StructuralNode, error) {
	return f(parent, target, attributes)
}

// InlineMacroProcessorFunc adapts a function to an InlineMacroProcessor.
type InlineMacroProcessorFunc func(parent Node, target string, attributes map[string]any) (*PhraseNode, error)

func (f InlineMacroProcessorFunc) Process(parent Node, target string, attributes map[string]any) (*PhraseNode, error) {
	return f(parent, target, attributes)
}

// IncludeProcessorFuncs adapts a pair of functions to an IncludeProcessor.
type IncludeProcessorFuncs struct {
	HandlesFunc func(doc *Document, target string) bool
	ProcessFunc func(doc *Document, reader *PreprocessorReader, target string, attributes map[string]any) error
}

func (f IncludeProcessorFuncs) Handles(doc *Document, target string) bool {
	return f.HandlesFunc(doc, target)
}

func (f IncludeProcessorFuncs) Process(doc *Document, reader *PreprocessorReader, target string, attributes map[string]any) error {
	return f.ProcessFunc(doc, reader, target, attributes)
}

// Content models of block processors and macros.
const (
	ContentModelCompound   = "compound"
	ContentModelSimple     = "simple"
	ContentModelVerbatim   = "verbatim"
	ContentModelRaw        = "raw"
	ContentModelEmpty      = "empty"
	ContentModelAttributes = "attributes"
	ContentModelText       = "text"
)

// DocumentProcessorOptions configure preprocessors, include processors, tree
// processors, postprocessors and docinfo processors.
type DocumentProcessorOptions struct {
	// Prefer runs the processor before the others of its kind.
	Prefer bool
	// Location places docinfo content: "head" (the default) or "footer".
	// It applies to docinfo processors only.
	Location string
	// Config holds additional processor configuration, by name.
	Config map[string]any
}

// BlockOptions configure a block processor.
type BlockOptions struct {
	// Contexts are the block types the processor applies to, such as
	// "paragraph", "open", "listing", "example", "literal", "pass", "quote",
	// "sidebar". The default is "open" and "paragraph".
	Contexts []string
	// ContentModel says how the block content is read: ContentModelCompound
	// (the default), Simple, Verbatim, Raw, Empty or Attributes.
	ContentModel string
	// PositionalAttributes names the positional attributes after the style.
	PositionalAttributes []string
	// DefaultAttributes are attribute defaults.
	DefaultAttributes map[string]any
	// Config holds additional processor configuration, by name.
	Config map[string]any
}

// MacroOptions configure a block or inline macro processor.
type MacroOptions struct {
	// ContentModel is ContentModelAttributes (the default: the attribute
	// list is parsed) or ContentModelText (it is passed as the "text"
	// attribute).
	ContentModel string
	// PositionalAttributes names the positional attributes.
	PositionalAttributes []string
	// DefaultAttributes are attribute defaults.
	DefaultAttributes map[string]any
	// Format of an inline macro: "long" (name:target[attrs], the default) or
	// "short" (name:[attrs]).
	Format string
	// Regexp, if set, is the Ruby regular expression source that matches an
	// inline macro, replacing the one derived from the name and format. The
	// first group is the target and the second the attribute list.
	Regexp string
	// Config holds additional processor configuration, by name.
	Config map[string]any
}

func (o *DocumentProcessorOptions) config() map[ruby.Symbol]any {
	c := map[ruby.Symbol]any{}
	if o == nil {
		return c
	}
	for k, v := range o.Config {
		c[ruby.Symbol(k)] = v
	}
	if o.Prefer {
		c["position"] = ruby.Symbol(">>")
	}
	if o.Location != "" {
		c["location"] = ruby.Symbol(o.Location)
	}
	return c
}

func syntaxConfig(extra map[string]any, contentModel string, positional []string, defaults map[string]any) map[ruby.Symbol]any {
	c := map[ruby.Symbol]any{}
	for k, v := range extra {
		c[ruby.Symbol(k)] = v
	}
	if contentModel != "" {
		c["content_model"] = ruby.Symbol(contentModel)
	}
	if positional != nil {
		c["positional_attrs"] = append([]string{}, positional...)
	}
	if defaults != nil {
		c["default_attrs"] = defaults
	}
	return c
}

func (o *BlockOptions) config() map[ruby.Symbol]any {
	if o == nil {
		return map[ruby.Symbol]any{}
	}
	c := syntaxConfig(o.Config, o.ContentModel, o.PositionalAttributes, o.DefaultAttributes)
	if len(o.Contexts) > 0 {
		ctx := make([]any, len(o.Contexts))
		for i, s := range o.Contexts {
			ctx[i] = ruby.Symbol(s)
		}
		c["contexts"] = ctx
	}
	return c
}

func (o *MacroOptions) config() map[ruby.Symbol]any {
	if o == nil {
		return map[ruby.Symbol]any{}
	}
	c := syntaxConfig(o.Config, o.ContentModel, o.PositionalAttributes, o.DefaultAttributes)
	if o.Format != "" {
		c["format"] = ruby.Symbol(o.Format)
	}
	return c
}

// ExtensionRegistry registers Go extensions with an Asciidoctor instance; they
// apply to every document the instance processes.
type ExtensionRegistry struct {
	a *Asciidoctor

	mu     sync.Mutex
	groups map[string]*ExtensionGroup
	nextID int
}

// Extensions returns the instance's extension registry.
func (a *Asciidoctor) Extensions() *ExtensionRegistry {
	a.extOnce.Do(func() { a.ext = &ExtensionRegistry{a: a, groups: map[string]*ExtensionGroup{}} })
	return a.ext
}

// ExtensionGroup is a set of extensions registered and unregistered together.
type ExtensionGroup struct {
	r          *ExtensionRegistry
	name       string
	specs      []extensionSpec
	errs       []error
	registered bool
}

type extensionSpec struct {
	kind      string // registry method: preprocessor, block, ...
	class     string // proxy class in AsciidoctorGo
	name      string // syntax processors only
	handler   extensionHandler
	config    map[ruby.Symbol]any
	rubyClass string // for a processor written in Ruby: its class name
}

// Group returns a new, empty group with the given name ("" picks a unique
// name). Add extensions to it, then call Register.
func (r *ExtensionRegistry) Group(name string) *ExtensionGroup {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "" {
		r.nextID++
		name = "asciidoctor_go_" + strconv.Itoa(r.nextID)
	}
	return &ExtensionGroup{r: r, name: name}
}

// Name is the group name.
func (g *ExtensionGroup) Name() string { return g.name }

func (g *ExtensionGroup) add(nilProcessor bool, kind, class, name string, h extensionHandler, config map[ruby.Symbol]any) *ExtensionGroup {
	if nilProcessor {
		g.errs = append(g.errs, fmt.Errorf("asciidoctor: nil %s added to extension group %s", kind, g.name))
	}
	g.specs = append(g.specs, extensionSpec{kind: kind, class: class, name: name, handler: h, config: config})
	return g
}

// validate reports the errors found while extensions were added, and Go
// block and macro processors without a name.
func (g *ExtensionGroup) validate() error {
	errs := append([]error(nil), g.errs...)
	for _, s := range g.specs {
		if s.rubyClass == "" && s.name == "" && (s.kind == "block" || s.kind == "block_macro" || s.kind == "inline_macro") {
			errs = append(errs, fmt.Errorf("asciidoctor: %s processor added to extension group %s without a name", s.kind, g.name))
		}
	}
	return errors.Join(errs...)
}

// Preprocessor adds a preprocessor; opts may be nil.
func (g *ExtensionGroup) Preprocessor(p Preprocessor, opts *DocumentProcessorOptions) *ExtensionGroup {
	return g.add(p == nil, "preprocessor", "Preprocessor", "", &preprocessorHandler{p: p}, opts.config())
}

// IncludeProcessor adds an include processor; opts may be nil.
func (g *ExtensionGroup) IncludeProcessor(p IncludeProcessor, opts *DocumentProcessorOptions) *ExtensionGroup {
	return g.add(p == nil, "include_processor", "IncludeProcessor", "", &includeProcessorHandler{p: p}, opts.config())
}

// Treeprocessor adds a tree processor; opts may be nil.
func (g *ExtensionGroup) Treeprocessor(p Treeprocessor, opts *DocumentProcessorOptions) *ExtensionGroup {
	return g.add(p == nil, "tree_processor", "TreeProcessor", "", &treeprocessorHandler{p: p}, opts.config())
}

// Postprocessor adds a postprocessor; opts may be nil.
func (g *ExtensionGroup) Postprocessor(p Postprocessor, opts *DocumentProcessorOptions) *ExtensionGroup {
	return g.add(p == nil, "postprocessor", "Postprocessor", "", &postprocessorHandler{p: p}, opts.config())
}

// DocinfoProcessor adds a docinfo processor; opts may be nil.
func (g *ExtensionGroup) DocinfoProcessor(p DocinfoProcessor, opts *DocumentProcessorOptions) *ExtensionGroup {
	return g.add(p == nil, "docinfo_processor", "DocinfoProcessor", "", &docinfoProcessorHandler{p: p}, opts.config())
}

// Block adds a block processor for blocks styled [name]; opts may be nil.
func (g *ExtensionGroup) Block(name string, p BlockProcessor, opts *BlockOptions) *ExtensionGroup {
	return g.add(p == nil, "block", "BlockProcessor", name, &blockProcessorHandler{p: p}, opts.config())
}

// BlockMacro adds a processor for the block macro name::target[]; opts may
// be nil.
func (g *ExtensionGroup) BlockMacro(name string, p BlockMacroProcessor, opts *MacroOptions) *ExtensionGroup {
	return g.add(p == nil, "block_macro", "BlockMacroProcessor", name, &blockMacroProcessorHandler{p: p}, opts.config())
}

// InlineMacro adds a processor for the inline macro name:target[]; opts may
// be nil.
func (g *ExtensionGroup) InlineMacro(name string, p InlineMacroProcessor, opts *MacroOptions) *ExtensionGroup {
	h := &inlineMacroProcessorHandler{p: p}
	if opts != nil && opts.Regexp != "" {
		h.regexp = opts.Regexp
	}
	return g.add(p == nil, "inline_macro", "InlineMacroProcessor", name, h, opts.config())
}

// ExtensionKind names a kind of processor.
type ExtensionKind string

const (
	KindPreprocessor     ExtensionKind = "preprocessor"
	KindIncludeProcessor ExtensionKind = "include_processor"
	KindTreeprocessor    ExtensionKind = "tree_processor"
	KindPostprocessor    ExtensionKind = "postprocessor"
	KindDocinfoProcessor ExtensionKind = "docinfo_processor"
	KindBlock            ExtensionKind = "block"
	KindBlockMacro       ExtensionKind = "block_macro"
	KindInlineMacro      ExtensionKind = "inline_macro"
)

// RubyProcessor adds a processor written in Ruby, by class name (load the
// class first, for example with Asciidoctor.LoadRubySource). name is the
// block or macro name for KindBlock, KindBlockMacro and KindInlineMacro (""
// uses the name the class configures), and is ignored for the other kinds.
func (g *ExtensionGroup) RubyProcessor(kind ExtensionKind, className, name string) *ExtensionGroup {
	switch kind {
	case KindBlock, KindBlockMacro, KindInlineMacro:
	default:
		name = ""
	}
	g.specs = append(g.specs, extensionSpec{kind: string(kind), name: name, rubyClass: className})
	return g
}

// RubyProcessor registers a processor written in Ruby; see
// ExtensionGroup.RubyProcessor.
func (r *ExtensionRegistry) RubyProcessor(kind ExtensionKind, className, name string) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.RubyProcessor(kind, className, name) })
}

// Register activates the group. Registering a group again replaces it.
func (g *ExtensionGroup) Register() error {
	r := g.r
	a := r.a
	if err := g.validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.groups[g.name]; ok && old != g {
		if err := old.unregisterLocked(); err != nil {
			return err
		}
	}
	err := a.do(func() error {
		glue, err := a.glue()
		if err != nil {
			return err
		}
		specs := make([]any, 0, len(g.specs))
		for i := range g.specs {
			s := &g.specs[i]
			if s.rubyClass != "" {
				// Asciidoctor resolves class names when a document activates
				// the group; resolve now so an unknown class fails here.
				if _, err := glue.Call("resolve_class", s.rubyClass); err != nil {
					return err
				}
				var name any
				if s.name != "" {
					name = ruby.Symbol(s.name)
				}
				specs = append(specs, []any{ruby.Symbol(s.kind), s.rubyClass, name})
				continue
			}
			s.handler.bind(a)
			id := a.b.Register(s.handler)
			s.handler.setID(id)
			class, err := a.b.Const("AsciidoctorGo::" + s.class)
			if err != nil {
				return err
			}
			var proc ruby.Value
			if s.name != "" {
				proc, err = class.Call("new", id, ruby.Symbol(s.name), s.config)
			} else {
				proc, err = class.Call("new", id, s.config)
			}
			if err != nil {
				return err
			}
			if rh, ok := s.handler.(*inlineMacroProcessorHandler); ok && rh.regexp != "" {
				if err := setInlineRegexp(a, proc, rh.regexp); err != nil {
					return err
				}
			}
			// Asciidoctor validates macro names lazily; check them now so an
			// invalid name fails the registration.
			switch s.kind {
			case "block_macro":
				if _, err := proc.Call("name"); err != nil {
					return err
				}
			case "inline_macro":
				if _, err := proc.Call("regexp"); err != nil {
					return err
				}
			}
			var name any
			if s.name != "" {
				name = ruby.Symbol(s.name)
			}
			specs = append(specs, []any{ruby.Symbol(s.kind), proc, name})
		}
		_, err = glue.Call("register_group", ruby.Symbol(g.name), specs)
		return err
	})
	if err != nil {
		g.releaseHandles()
		return err
	}
	g.registered = true
	r.groups[g.name] = g
	return nil
}

// Unregister deactivates the group.
func (g *ExtensionGroup) Unregister() error {
	g.r.mu.Lock()
	defer g.r.mu.Unlock()
	return g.unregisterLocked()
}

func (g *ExtensionGroup) unregisterLocked() error {
	if !g.registered {
		return nil
	}
	a := g.r.a
	err := a.do(func() error {
		ext, err := a.b.Const("Asciidoctor::Extensions")
		if err != nil {
			return err
		}
		_, err = ext.Call("unregister", ruby.Symbol(g.name))
		return err
	})
	if err != nil {
		return err
	}
	g.releaseHandles()
	g.registered = false
	delete(g.r.groups, g.name)
	return nil
}

func (g *ExtensionGroup) releaseHandles() {
	for _, s := range g.specs {
		if s.handler == nil {
			continue
		}
		if id := s.handler.id(); id != 0 {
			g.r.a.b.Unregister(id)
			s.handler.setID(0)
		}
	}
}

// register adds one extension as its own group.
func (r *ExtensionRegistry) register(add func(*ExtensionGroup)) (*ExtensionGroup, error) {
	g := r.Group("")
	add(g)
	return g, g.Register()
}

// Preprocessor registers a preprocessor. The returned group unregisters it.
func (r *ExtensionRegistry) Preprocessor(p Preprocessor, opts *DocumentProcessorOptions) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.Preprocessor(p, opts) })
}

// IncludeProcessor registers an include processor.
func (r *ExtensionRegistry) IncludeProcessor(p IncludeProcessor, opts *DocumentProcessorOptions) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.IncludeProcessor(p, opts) })
}

// Treeprocessor registers a tree processor.
func (r *ExtensionRegistry) Treeprocessor(p Treeprocessor, opts *DocumentProcessorOptions) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.Treeprocessor(p, opts) })
}

// Postprocessor registers a postprocessor.
func (r *ExtensionRegistry) Postprocessor(p Postprocessor, opts *DocumentProcessorOptions) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.Postprocessor(p, opts) })
}

// DocinfoProcessor registers a docinfo processor.
func (r *ExtensionRegistry) DocinfoProcessor(p DocinfoProcessor, opts *DocumentProcessorOptions) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.DocinfoProcessor(p, opts) })
}

// Block registers a block processor for blocks styled [name].
func (r *ExtensionRegistry) Block(name string, p BlockProcessor, opts *BlockOptions) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.Block(name, p, opts) })
}

// BlockMacro registers a block macro processor.
func (r *ExtensionRegistry) BlockMacro(name string, p BlockMacroProcessor, opts *MacroOptions) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.BlockMacro(name, p, opts) })
}

// InlineMacro registers an inline macro processor.
func (r *ExtensionRegistry) InlineMacro(name string, p InlineMacroProcessor, opts *MacroOptions) (*ExtensionGroup, error) {
	return r.register(func(g *ExtensionGroup) { g.InlineMacro(name, p, opts) })
}

// Unregister unregisters the named group.
func (r *ExtensionRegistry) Unregister(name string) error {
	r.mu.Lock()
	g, ok := r.groups[name]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("asciidoctor: no extension group named %q", name)
	}
	return g.Unregister()
}

// UnregisterAll unregisters every extension group, including those
// registered from Ruby.
func (r *ExtensionRegistry) UnregisterAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.a
	err := a.do(func() error {
		ext, err := a.b.Const("Asciidoctor::Extensions")
		if err != nil {
			return err
		}
		_, err = ext.Call("unregister_all")
		return err
	})
	if err != nil {
		return err
	}
	for name, g := range r.groups {
		g.releaseHandles()
		g.registered = false
		delete(r.groups, name)
	}
	return nil
}
