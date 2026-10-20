package asciidoctor

import (
	"sort"
	"strconv"

	ruby "github.com/go-embedded-ruby/ruby"
)

// Node is a node of the document tree: a block, a section, the document
// itself, or an inline phrase. It wraps the live Asciidoctor object, so
// changes made through a Node are seen by Asciidoctor and vice versa.
//
// Node methods panic with an *Error if the interpreter fails, which happens
// only when the instance is closed or Asciidoctor raises unexpectedly; methods
// that do real processing, such as Convert, return the error instead.
type Node interface {
	// ID is the node's id, or "" if it has none.
	ID() string
	SetID(id string)
	// NodeName is the converter's name for the node, such as "paragraph",
	// "section" or "inline_quoted".
	NodeName() string
	// Context is the node type, such as "paragraph", "section", "ulist",
	// "image" or "quoted".
	Context() string
	SetContext(context string)
	// Parent is the enclosing node, or nil for the document.
	Parent() Node
	// Document is the document the node belongs to.
	Document() *Document
	IsBlock() bool
	IsInline() bool

	// Attr returns the value of the node attribute name, or nil. Values are
	// usually strings; a few built-in attributes are integers (int64).
	Attr(name string) any
	// AttrOr returns Attr(name), or def if the attribute is not set.
	AttrOr(name string, def any) any
	// InheritedAttr returns the node attribute name or, when the node does
	// not set it, the document attribute of the same name.
	InheritedAttr(name string) any
	// AttrString returns the attribute formatted as a string ("" if unset).
	AttrString(name string) string
	// HasAttr reports whether the node sets the attribute name.
	HasAttr(name string) bool
	// HasInheritedAttr reports whether the node or the document sets name.
	HasInheritedAttr(name string) bool
	// IsAttr reports whether the attribute name is set to expected.
	IsAttr(name string, expected any) bool
	// SetAttr sets the attribute name to value. Unless overwrite is true, an
	// attribute that is already set is left alone. It reports whether the
	// attribute was set.
	SetAttr(name string, value any, overwrite bool) bool
	// RemoveAttr unsets the attribute name and returns its previous value.
	RemoveAttr(name string) any
	// Attributes returns a copy of the node's attributes. Positional
	// attributes appear under their position ("1", "2", ...).
	Attributes() map[string]any

	// HasOption reports whether the option name is enabled (the attribute
	// "<name>-option" is set).
	HasOption(name string) bool
	// SetOption enables the option name.
	SetOption(name string)
	// EnabledOptions lists the enabled options, sorted.
	EnabledOptions() []string

	// Role is the value of the role attribute ("" if unset).
	Role() string
	// Roles lists the roles.
	Roles() []string
	HasRole(role string) bool
	// IsRole reports whether the node has any role.
	IsRole() bool
	AddRole(role string) bool
	RemoveRole(role string) bool

	// Reftext is the converted reference text ("" if unset).
	Reftext() string
	HasReftext() bool

	// IconURI resolves the URI of the named icon.
	IconURI(name string) string
	// ImageURI resolves the URI of an image, against the imagesdir attribute.
	ImageURI(target string) string
	// ImageURIFrom resolves the URI of an image against the directory named
	// by the assetDirKey attribute (such as "iconsdir").
	ImageURIFrom(target, assetDirKey string) string
	// MediaURI resolves the URI of an audio or video file, against imagesdir.
	MediaURI(target string) string
	// NormalizeWebPath normalizes a web path, resolving it against start.
	NormalizeWebPath(target, start string, preserveURITarget bool) string
	// ReadAsset reads the file at path, as allowed by the safe mode; it
	// returns "" (and logs a warning when warn is true) if it cannot be read.
	ReadAsset(path string, warn bool) string

	// Value is the underlying Ruby object. Use it only inside a callback or
	// with Asciidoctor.Do.
	Value() ruby.Value

	base() *node
}

// node implements Node.
type node struct {
	a *Asciidoctor
	v ruby.Value
}

func (n *node) base() *node       { return n }
func (n *node) Value() ruby.Value { return n.v }

// call invokes a Ruby method on the node, panicking with an *Error on failure.
func (n *node) call(method string, args ...any) ruby.Value {
	v, err := n.callErr(method, args...)
	if err != nil {
		panic(err)
	}
	return v
}

// callErr invokes a Ruby method on the node.
func (n *node) callErr(method string, args ...any) (ruby.Value, error) {
	return n.a.callValue(n.v, method, args...)
}

func (n *node) ID() string          { return rubyString(n.call("id")) }
func (n *node) SetID(id string)     { n.call("id=", nilIfEmpty(id)) }
func (n *node) NodeName() string    { return rubyString(n.call("node_name")) }
func (n *node) Context() string     { return rubyString(n.call("context")) }
func (n *node) IsBlock() bool       { return rubyBool(n.call("block?")) }
func (n *node) IsInline() bool      { return rubyBool(n.call("inline?")) }
func (n *node) Parent() Node        { return n.a.wrapNode(n.call("parent")) }
func (n *node) Document() *Document { return n.a.wrapDocument(n.call("document")) }

func (n *node) SetContext(context string) {
	n.call("context=", ruby.Symbol(context))
}

func (n *node) Attr(name string) any { return goValue(n.call("attr", name)) }

func (n *node) AttrOr(name string, def any) any {
	if v := n.Attr(name); v != nil {
		return v
	}
	return def
}

func (n *node) InheritedAttr(name string) any { return goValue(n.call("attr", name, nil, true)) }

func (n *node) AttrString(name string) string { return rubyString(n.call("attr", name)) }

func (n *node) HasAttr(name string) bool { return rubyBool(n.call("attr?", name)) }

func (n *node) HasInheritedAttr(name string) bool { return rubyBool(n.call("attr?", name, nil, true)) }

func (n *node) IsAttr(name string, expected any) bool {
	return rubyBool(n.call("attr?", name, expected))
}

func (n *node) SetAttr(name string, value any, overwrite bool) bool {
	return rubyBool(n.call("set_attr", name, value, overwrite))
}

func (n *node) RemoveAttr(name string) any { return goValue(n.call("remove_attr", name)) }

func (n *node) Attributes() map[string]any {
	m, _ := goValue(n.call("attributes")).(map[any]any)
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch key := k.(type) {
		case string:
			out[key] = v
		case int64: // positional attributes have Integer keys
			out[strconv.FormatInt(key, 10)] = v
		}
	}
	return out
}

func (n *node) HasOption(name string) bool { return rubyBool(n.call("option?", name)) }
func (n *node) SetOption(name string)      { n.call("set_option", name) }

func (n *node) EnabledOptions() []string {
	set := n.call("enabled_options")
	arr, err := n.a.callValue(set, "to_a")
	if err != nil {
		panic(err)
	}
	opts := rubyStrings(arr)
	sort.Strings(opts)
	return opts
}

func (n *node) Role() string             { return rubyString(n.call("role")) }
func (n *node) Roles() []string          { return rubyStrings(n.call("roles")) }
func (n *node) HasRole(role string) bool { return rubyBool(n.call("has_role?", role)) }
func (n *node) IsRole() bool             { return rubyBool(n.call("role?")) }
func (n *node) AddRole(role string) bool { return rubyBool(n.call("add_role", role)) }
func (n *node) RemoveRole(role string) bool {
	return rubyBool(n.call("remove_role", role))
}

func (n *node) Reftext() string  { return rubyString(n.call("reftext")) }
func (n *node) HasReftext() bool { return rubyBool(n.call("reftext?")) }

func (n *node) IconURI(name string) string    { return rubyString(n.call("icon_uri", name)) }
func (n *node) ImageURI(target string) string { return rubyString(n.call("image_uri", target)) }
func (n *node) ImageURIFrom(target, assetDirKey string) string {
	return rubyString(n.call("image_uri", target, assetDirKey))
}
func (n *node) MediaURI(target string) string { return rubyString(n.call("media_uri", target)) }

func (n *node) NormalizeWebPath(target, start string, preserveURITarget bool) string {
	return rubyString(n.call("normalize_web_path", target, nilIfEmpty(start), preserveURITarget))
}

func (n *node) ReadAsset(path string, warn bool) string {
	return rubyString(n.call("read_asset", path, map[ruby.Symbol]any{"warn_on_failure": warn}))
}

// rubyString converts a Ruby value to a string; nil becomes "".
func rubyString(v ruby.Value) string {
	if v.IsNil() {
		return ""
	}
	return v.String()
}

// rubyBool reports whether a Ruby value is truthy.
func rubyBool(v ruby.Value) bool {
	if v.IsNil() {
		return false
	}
	b, ok := v.Go().(bool)
	return !ok || b
}

// rubyStrings converts a Ruby Array to a slice of strings.
func rubyStrings(v ruby.Value) []string {
	items, _ := v.Go().([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch s := it.(type) {
		case string:
			out = append(out, s)
		case ruby.Symbol:
			out = append(out, string(s))
		case nil:
			out = append(out, "")
		default:
			if rv, ok := it.(ruby.Value); ok {
				out = append(out, rv.String())
			}
		}
	}
	return out
}

// rubyInt converts a Ruby Integer to int; nil (or a non-integer) becomes 0.
func rubyInt(v ruby.Value) int {
	switch i := v.Go().(type) {
	case int64:
		return int(i)
	case float64:
		return int(i)
	}
	return 0
}

// goValue converts a Ruby value to Go as Value.Go does, with Symbols as
// strings.
func goValue(v ruby.Value) any {
	return normalize(v.Go())
}

func normalize(x any) any {
	switch t := x.(type) {
	case ruby.Symbol:
		return string(t)
	case []any:
		for i := range t {
			t[i] = normalize(t[i])
		}
		return t
	case map[any]any:
		out := make(map[any]any, len(t))
		for k, v := range t {
			out[normalize(k)] = normalize(v)
		}
		return out
	}
	return x
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
