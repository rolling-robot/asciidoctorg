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
end
