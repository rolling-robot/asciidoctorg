package asciidoctor

import (
	"fmt"
	"io"
	"sort"
	"strconv"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/asciidoctor-go/asciidoctor/internal/bridge"
)

// SafeMode restricts what a document may access. The zero value selects
// Asciidoctor's API default, SafeModeSecure.
type SafeMode int

const (
	// SafeModeDefault is Asciidoctor's API default, SafeModeSecure.
	SafeModeDefault SafeMode = iota
	// SafeModeUnsafe disables every security restriction.
	SafeModeUnsafe
	// SafeModeSafe prevents access to files outside the parent directory of
	// the source file.
	SafeModeSafe
	// SafeModeServer also prevents the document from setting attributes that
	// affect conversion, such as source-highlighter and linkcss.
	SafeModeServer
	// SafeModeSecure also disables includes and docinfo, and prevents
	// embedding or linking to files outside the document.
	SafeModeSecure
)

// Level is the numeric level Asciidoctor uses (0, 1, 10 or 20).
func (m SafeMode) Level() int {
	switch m {
	case SafeModeUnsafe:
		return 0
	case SafeModeSafe:
		return 1
	case SafeModeServer:
		return 10
	default:
		return 20
	}
}

func (m SafeMode) String() string {
	switch m {
	case SafeModeUnsafe:
		return "unsafe"
	case SafeModeSafe:
		return "safe"
	case SafeModeServer:
		return "server"
	default:
		return "secure"
	}
}

// safeModeFromLevel maps Asciidoctor's numeric level to a SafeMode.
func safeModeFromLevel(level int64) SafeMode {
	switch {
	case level < 1:
		return SafeModeUnsafe
	case level < 10:
		return SafeModeSafe
	case level < 20:
		return SafeModeServer
	default:
		return SafeModeSecure
	}
}

// Attributes are document attributes passed to the API. They override
// attributes set in the document unless the value ends in "@" (a soft set).
//
// Values are converted as follows: a string is used as is, true sets the
// attribute to the empty string, false or nil unsets it, and any other value
// (numbers, for instance) is formatted with fmt.Sprint.
type Attributes map[string]any

func (a Attributes) toRuby() map[string]any {
	out := make(map[string]any, len(a))
	for k, v := range a {
		switch v := v.(type) {
		case nil:
			out[k] = nil
		case string:
			out[k] = v
		case bool:
			if v {
				out[k] = ""
			} else {
				out[k] = false
			}
		case int:
			out[k] = strconv.Itoa(v)
		default:
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

// Options control how a document is loaded and converted. The zero value is
// usable and selects Asciidoctor's defaults.
type Options struct {
	// Safe is the safe mode. The default is SafeModeSecure.
	Safe SafeMode
	// Backend is the output format: "html5" (the default), "docbook5",
	// "manpage", or a backend added by a registered converter.
	Backend string
	// Doctype is the document type: "article" (the default), "book",
	// "manpage" or "inline".
	Doctype string
	// Standalone, when set, chooses between a standalone document (with
	// header and footer) and embeddable output. By default, output written to
	// a file is standalone and output returned as a string is embeddable.
	Standalone *bool
	// Attributes are document attributes; see Attributes.
	Attributes Attributes

	// ToFile is the path of the output file. A relative path is resolved
	// against ToDir, or against BaseDir (default: the working directory).
	ToFile string
	// ToDir is the directory the output file is written to.
	ToDir string
	// MkDirs creates missing output directories.
	MkDirs bool
	// Output receives the converted document instead of a file or a string.
	Output io.Writer
	// NoOutputFile makes ConvertFile return the converted document instead of
	// writing it next to the input file (Asciidoctor's to_file: false).
	NoOutputFile bool

	// BaseDir is the base directory for resolving relative paths such as
	// includes and images. The default is the directory of the input file,
	// or the working directory.
	BaseDir string
	// TemplateDirs are directories of converter templates.
	TemplateDirs []string
	// TemplateEngine restricts templates to one engine (such as "erb").
	TemplateEngine string
	// TemplateCache, when set, enables or disables the template cache.
	TemplateCache *bool
	// ERuby is the ERB implementation used by templates: "erb" or "erubi".
	ERuby string

	// CatalogAssets records images and links in the document catalog.
	CatalogAssets bool
	// SourceMap records the source file and line of each block.
	SourceMap bool
	// ParseHeaderOnly stops parsing after the document header.
	ParseHeaderOnly bool

	// Extra holds any other Asciidoctor API option, by its Ruby name (for
	// example "eruby" or "timings"). Values are converted as by the
	// interpreter: strings, numbers, booleans, slices and maps.
	Extra map[string]any
}

// Bool returns a pointer to v, for the optional boolean fields of Options.
func Bool(v bool) *bool { return &v }

// rubyOptions builds the Ruby options Hash. writer, when non-nil, is the
// stream the output is written to.
func (o *Options) rubyOptions(writer *bridge.Writer) map[ruby.Symbol]any {
	opts := map[ruby.Symbol]any{"safe": SafeModeDefault.Level()}
	if o == nil {
		return opts
	}
	// Extra first, so the typed fields take precedence.
	keys := make([]string, 0, len(o.Extra))
	for k := range o.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		opts[ruby.Symbol(k)] = o.Extra[k]
	}
	opts["safe"] = o.Safe.Level()
	if o.Backend != "" {
		opts["backend"] = o.Backend
	}
	if o.Doctype != "" {
		opts["doctype"] = o.Doctype
	}
	if o.Standalone != nil {
		opts["standalone"] = *o.Standalone
	}
	if o.Attributes != nil {
		opts["attributes"] = o.Attributes.toRuby()
	}
	switch {
	case writer != nil:
		opts["to_file"] = writer
	case o.ToFile != "":
		opts["to_file"] = o.ToFile
	case o.NoOutputFile:
		opts["to_file"] = false
	}
	if o.ToDir != "" {
		opts["to_dir"] = o.ToDir
	}
	if o.MkDirs {
		opts["mkdirs"] = true
	}
	if o.BaseDir != "" {
		opts["base_dir"] = o.BaseDir
	}
	if len(o.TemplateDirs) > 0 {
		opts["template_dirs"] = append([]string(nil), o.TemplateDirs...)
	}
	if o.TemplateEngine != "" {
		opts["template_engine"] = o.TemplateEngine
	}
	if o.TemplateCache != nil {
		opts["template_cache"] = *o.TemplateCache
	}
	if o.ERuby != "" {
		opts["eruby"] = o.ERuby
	}
	if o.CatalogAssets {
		opts["catalog_assets"] = true
	}
	if o.SourceMap {
		opts["sourcemap"] = true
	}
	if o.ParseHeaderOnly {
		opts["parse_header_only"] = true
	}
	return opts
}
