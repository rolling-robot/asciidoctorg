// Package asciidoctor converts AsciiDoc documents with Asciidoctor, from Go.
//
// It runs the original Asciidoctor implementation, unmodified, in an embedded
// pure-Go Ruby interpreter (rbgo), in the way AsciidoctorJ runs it on JRuby.
// Output is therefore Asciidoctor's own, and Go code can extend the processor
// with the same extension points Ruby code has. No Ruby installation and no
// cgo are needed.
//
//	html, err := asciidoctor.Convert("= Hello\n\nA *bold* paragraph.", nil)
//
// # Instances
//
// An Asciidoctor (created with New) holds one interpreter, with Asciidoctor
// loaded and the extensions, converters and log handlers registered on it.
// Release it with Close. Calls on an instance are serialized; create several
// instances, or a Pool, to convert in parallel. The package-level Convert,
// ConvertFile and Load use a shared default instance.
//
// The embedded Ruby sources are written once to a cache directory (the user
// cache directory unless WithRuntimeDir says otherwise) and shared by every
// instance and process of the same version.
//
// # Converting
//
// Convert converts a string, ConvertFile a file, and ConvertFiles and
// ConvertDirectory (with an AsciiDocDirectoryWalker or GlobDirectoryWalker)
// several. Options sets Asciidoctor's API options: the safe mode (SafeModeSecure
// by default, as in Asciidoctor's API), backend, doctype, output file or
// directory, base directory, templates and so on; Options.Extra passes any
// other option by its Ruby name. Attributes sets document attributes, with
// chainable setters for the well-known ones:
//
//	opts := &asciidoctor.Options{
//		Safe:       asciidoctor.SafeModeSafe,
//		Attributes: asciidoctor.Attributes{}.TableOfContents(asciidoctor.PlacementLeft).SectionNumbers(true),
//	}
//
// ConvertDocument and ConvertFileDocument also return the converted Document,
// for example to read its catalog of referenced images and links.
//
// The built-in backends are html5, docbook5 and manpage. Source highlighting
// works with Rouge (backed by go-rouge), CodeRay (embedded), highlight.js and
// prettify; RegisterSyntaxHighlighter adds a highlighter written in Go.
//
// # Document tree
//
// Load parses a document without converting it and returns its Document, the
// root of a tree of nodes: Section, Block, List, ListItem, DescriptionList,
// Table and its rows, columns and cells, and inline PhraseNode values. Nodes
// expose Asciidoctor's node API (attributes, roles, options, titles,
// substitutions, child blocks, FindBy queries, conversion) and can be
// changed. The nodes are live views of Asciidoctor's objects.
//
// # Extensions
//
// Extensions() returns the instance's ExtensionRegistry, where Go values
// implementing Preprocessor, IncludeProcessor, Treeprocessor, Postprocessor,
// DocinfoProcessor, BlockProcessor, BlockMacroProcessor and
// InlineMacroProcessor are registered, alone or in a named ExtensionGroup that
// can be unregistered later. The Func types adapt plain functions. Inside a
// processor, the Create functions (CreateBlock, CreateSection, CreatePhraseNode,
// CreateTable, ParseContent, ...) build new nodes, and Reader and
// PreprocessorReader read and rewrite the source. Extensions written in Ruby
// are loaded with LoadRubySource or RequireLibrary and registered by class name
// with RubyProcessor.
//
// RegisterSetup registers a function that configures every new instance, so
// that a package providing extensions can enable them from its init function.
//
// # Converters and logging
//
// RegisterConverter adds a converter written in Go for a backend; it can
// delegate to Asciidoctor's own converters through BuiltinConverter, and write
// its output itself by implementing OutputWriter. RegisterLogHandler receives
// Asciidoctor's log messages, with their source location; SetLogLevel sets
// the lowest severity delivered, and Log lets extensions and converters log.
//
// # Command line
//
// RunCLI runs Asciidoctor's command line, with all its options; the
// cmd/asciidoctor command is a thin wrapper around it.
//
// # Ruby access
//
// Do runs a function with exclusive access to the interpreter, and Node.Value
// gives the underlying Ruby object, for anything the Go API does not cover.
// Nodes and readers handed to a callback may be used only by the goroutine
// running that callback, and only until it returns.
package asciidoctor

// Version is the version of this module.
const Version = "0.1.0-dev"
