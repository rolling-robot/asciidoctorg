# frozen_string_literal: true

# Ruby half of the asciidoctor-go bridge. The Go side defines
# AsciidoctorGo::Bridge.dispatch(handle, method, *args), which forwards a call
# to the Go object registered under handle, and AsciidoctorGo::Writer, an
# IO-like wrapper of a Go io.Writer.
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
end
