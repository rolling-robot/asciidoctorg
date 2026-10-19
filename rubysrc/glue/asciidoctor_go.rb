# frozen_string_literal: true

# Ruby half of the asciidoctor-go bridge. The Go side defines
# AsciidoctorGo::Bridge.dispatch(handle, method, *args), which forwards a call
# to the Go object registered under handle, and AsciidoctorGo::Writer, an
# IO-like wrapper of a Go io.Writer.
# Load the built-in converters now. Each registers itself for its backend when
# loaded, so loading one lazily after a Go converter was registered for the
# same backend would replace the Go converter.
require 'asciidoctor/converter/html5'
require 'asciidoctor/converter/docbook5'
require 'asciidoctor/converter/manpage'

module AsciidoctorGo
  # Converts input as Asciidoctor.convert does, and returns the converted
  # String, or nil when the output was written to a file or stream.
  def self.convert input, options
    result = ::Asciidoctor.convert input, options
    ::Asciidoctor::Document === result ? nil : result
  end

  # Converts the file at path as Asciidoctor.convert_file does, and returns the
  # converted String, or nil when the output was written to a file or stream.
  def self.convert_file path, options
    result = ::Asciidoctor.convert_file path, options
    ::Asciidoctor::Document === result ? nil : result
  end

  # Returns a Symbol naming the Go node type that wraps node, or nil.
  def self.node_kind node
    case node
    when ::Asciidoctor::Document then :document
    when ::Asciidoctor::Section then :section
    when ::Asciidoctor::List then node.context == :dlist ? :dlist : :list
    when ::Asciidoctor::ListItem then :list_item
    when ::Asciidoctor::Table then :table
    when ::Asciidoctor::Table::Column then :column
    when ::Asciidoctor::Table::Cell then :cell
    when ::Asciidoctor::Inline then :inline
    when ::Asciidoctor::AbstractBlock then :block
    end
  end

  # Runs node.find_by(selector), with the Go filter registered under handle
  # (if any) as the filter block.
  def self.find_by node, selector, handle
    if handle
      node.find_by(selector) {|candidate| Bridge.dispatch handle, :filter, candidate }
    else
      node.find_by selector
    end
  end

  # Runs reader.read_lines_until(options), with the Go predicate registered
  # under handle (if any) as the break condition.
  def self.read_lines_until reader, options, handle
    if handle
      reader.read_lines_until(options) {|line| Bridge.dispatch handle, :break, line }
    else
      reader.read_lines_until options
    end
  end

  # Processor helpers (create_block, parse_content, ...) need a processor
  # instance; they depend only on their arguments.
  HELPER = ::Asciidoctor::Extensions::Processor.new

  # Creates a table cell, as AsciidoctorJ does: Table::Cell.new with an
  # optional inner document for AsciiDoc cells.
  def self.create_table_cell column, text, attributes, inner_document
    cell = ::Asciidoctor::Table::Cell.new column, text, attributes
    if inner_document
      cell.style = :asciidoc
      cell.instance_variable_set :@inner_document, inner_document
    end
    cell
  end

  # Sets the regular expression of an inline macro processor from its source.
  def self.set_regexp processor, source
    processor.config[:regexp] = ::Regexp.new source
    nil
  end

  # Creates a nested document whose parent is parent_document.
  def self.create_document parent_document
    ::Asciidoctor::Document.new [], parent: parent_document
  end

  # Resolves a processor class by name, raising NameError if it is undefined.
  def self.resolve_class name
    ::Asciidoctor::Helpers.class_for_name name
  end

  # Registers processors in a named extension group. specs is an Array of
  # [kind, processor, name] triples; name is nil for document processors.
  def self.register_group group, specs
    ::Asciidoctor::Extensions.register group do
      specs.each do |kind, processor, name|
        name ? (send kind, processor, name) : (send kind, processor)
      end
    end
    nil
  end

  # Proxies: subclasses of Asciidoctor's processor classes whose methods
  # forward to the Go processor registered under handle.
  module Proxy
    def go_handle
      @go_handle
    end
  end

  class Preprocessor < ::Asciidoctor::Extensions::Preprocessor
    include Proxy

    def initialize handle, config = {}
      super config
      @go_handle = handle
    end

    def process document, reader
      Bridge.dispatch @go_handle, :process, document, reader
    end
  end

  class IncludeProcessor < ::Asciidoctor::Extensions::IncludeProcessor
    include Proxy

    def initialize handle, config = {}
      super config
      @go_handle = handle
    end

    def handles? document, target
      Bridge.dispatch @go_handle, :handles, document, target
    end

    def process document, reader, target, attributes
      Bridge.dispatch @go_handle, :process, document, reader, target, attributes
      nil
    end
  end

  class TreeProcessor < ::Asciidoctor::Extensions::TreeProcessor
    include Proxy

    def initialize handle, config = {}
      super config
      @go_handle = handle
    end

    def process document
      Bridge.dispatch @go_handle, :process, document
    end
  end

  class Postprocessor < ::Asciidoctor::Extensions::Postprocessor
    include Proxy

    def initialize handle, config = {}
      super config
      @go_handle = handle
    end

    def process document, output
      Bridge.dispatch @go_handle, :process, document, output
    end
  end

  class DocinfoProcessor < ::Asciidoctor::Extensions::DocinfoProcessor
    include Proxy

    def initialize handle, config = {}
      super config
      @go_handle = handle
    end

    def process document
      Bridge.dispatch @go_handle, :process, document
    end
  end

  class BlockProcessor < ::Asciidoctor::Extensions::BlockProcessor
    include Proxy

    def initialize handle, name, config = {}
      super name, config
      @go_handle = handle
    end

    def process parent, reader, attributes
      Bridge.dispatch @go_handle, :process, parent, reader, attributes
    end
  end

  class BlockMacroProcessor < ::Asciidoctor::Extensions::BlockMacroProcessor
    include Proxy

    def initialize handle, name, config = {}
      super name, config
      @go_handle = handle
    end

    def process parent, target, attributes
      Bridge.dispatch @go_handle, :process, parent, target, attributes
    end
  end

  class InlineMacroProcessor < ::Asciidoctor::Extensions::InlineMacroProcessor
    include Proxy

    def initialize handle, name, config = {}
      super name, config
      @go_handle = handle
    end

    def process parent, target, attributes
      Bridge.dispatch @go_handle, :process, parent, target, attributes
    end
  end

  # Converter proxy. Each registration creates a subclass carrying the Go
  # handle and backend traits; Asciidoctor instantiates it per document, and
  # every instance forwards to the same Go converter.
  class Converter < ::Asciidoctor::Converter::Base
    class << self
      attr_accessor :go_handle, :go_traits, :go_writer
    end

    def initialize backend, opts = {}
      super
      traits = ::Asciidoctor::Converter.derive_backend_traits backend, self.class.go_traits[:basebackend]
      init_backend_traits traits.merge(self.class.go_traits.compact)
    end

    def convert node, transform = nil, opts = nil
      Bridge.dispatch self.class.go_handle, :convert, node, (transform || node.node_name), opts
    end

    def handles? transform
      Bridge.dispatch self.class.go_handle, :handles, transform
    end
  end

  # Adds Writer behavior to a converter whose Go side writes its own output.
  module ConverterWriter
    include ::Asciidoctor::Writer

    def write output, target
      Bridge.dispatch self.class.go_handle, :write, output, target
      nil
    end
  end

  def self.register_converter handle, traits, writer, backends
    converter = ::Class.new Converter
    converter.go_handle = handle
    converter.go_traits = traits
    converter.include ConverterWriter if writer
    ::Asciidoctor::Converter.register converter, *backends
    converter
  end

  BUILTIN_CONVERTERS = { 'html5' => :Html5Converter, 'docbook5' => :DocBook5Converter, 'manpage' => :ManPageConverter }

  # Returns a new instance of Asciidoctor's own converter for backend (even if
  # another converter has been registered for it since), or nil.
  def self.builtin_converter backend
    return unless (class_name = BUILTIN_CONVERTERS[backend])
    require %(asciidoctor/converter/#{backend})
    ::Asciidoctor::Converter.const_get(class_name, false).new backend
  end

  # Syntax highlighter proxy; capabilities says which parts Go implements.
  class SyntaxHighlighter < ::Asciidoctor::SyntaxHighlighter::Base
    class << self
      attr_accessor :go_handle, :go_capabilities
    end

    def docinfo? location
      Bridge.dispatch self.class.go_handle, :has_docinfo, location
    end

    def docinfo location, doc, opts
      Bridge.dispatch self.class.go_handle, :docinfo, location, doc, opts
    end

    def highlight?
      self.class.go_capabilities.include? :highlight
    end

    def highlight node, source, lang, opts
      Bridge.dispatch self.class.go_handle, :highlight, node, source, lang, opts
    end

    def format node, lang, opts
      if self.class.go_capabilities.include? :format
        Bridge.dispatch self.class.go_handle, :format, node, lang, (opts.reject {|k, _| k == :transform })
      else
        super
      end
    end

    def write_stylesheet? doc
      (self.class.go_capabilities.include? :stylesheet) && (Bridge.dispatch self.class.go_handle, :write_stylesheet?, doc)
    end

    def write_stylesheet doc, to_dir
      Bridge.dispatch self.class.go_handle, :write_stylesheet, doc, to_dir
    end
  end

  def self.register_syntax_highlighter handle, capabilities, names
    highlighter = ::Class.new SyntaxHighlighter
    highlighter.go_handle = handle
    highlighter.go_capabilities = capabilities
    ::Asciidoctor::SyntaxHighlighter.register highlighter, *names
    highlighter
  end

  # Logger that forwards messages to Go log handlers (through handle) while
  # tracking the maximum severity like Asciidoctor::Logger.
  class Logger < ::Asciidoctor::Logger
    def initialize handle, level
      super nil, level: level
      @go_handle = handle
    end

    def add severity, message = nil, progname = nil
      severity ||= UNKNOWN
      super severity, message, progname
      return true if severity < level
      message = block_given? ? yield : progname if message.nil?
      if ::Hash === message && (message.key? :text)
        Bridge.dispatch @go_handle, :log, severity, message[:text].to_s, message[:source_location]
      else
        Bridge.dispatch @go_handle, :log, severity, (::String === message ? message : message.inspect), nil
      end
      true
    end
  end
end
