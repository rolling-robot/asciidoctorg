# frozen_string_literal: true

# open-uri for rbgo, which does not ship it. Asciidoctor uses
# OpenURI.open_uri to read remote includes, images and assets (allow-uri-read).
# This implements open-uri's behavior for HTTP and HTTPS on rbgo's Net::HTTP:
# redirects are followed, a response other than 2xx raises OpenURI::HTTPError,
# and the result is an IO carrying the response metadata.
require 'uri'
require 'stringio'
require 'net/http'

module OpenURI
  class HTTPError < ::StandardError
    attr_reader :io

    def initialize message, io
      super message
      @io = io
    end
  end

  class HTTPRedirect < HTTPError
    attr_reader :uri

    def initialize message, io, uri
      super message, io
      @uri = uri
    end
  end

  class TooManyRedirects < HTTPError; end

  # Response metadata, as open-uri adds it to the returned IO.
  module Meta
    attr_accessor :base_uri, :status
    attr_reader :meta

    def meta_add_field name, value
      (@meta ||= {})[name.downcase] = value
    end

    def content_type
      ((meta || {})['content-type'] || 'application/octet-stream').split(';').first.strip.downcase
    end

    def charset
      if (type = (meta || {})['content-type']) && (match = /;\s*charset\s*=\s*"?([^";\s]+)/i.match type)
        match[1].downcase
      elsif block_given?
        yield
      end
    end

    def last_modified
      (value = (meta || {})['last-modified']) ? (::Time.httpdate value rescue nil) : nil
    end
  end

  MAX_REDIRECTS = 64

  def self.open_uri name, *rest, **options
    mode = ::String === rest[0] ? rest[0] : 'r'
    uri = ::URI === name ? name : (::URI.parse name.to_s)
    unless %w(http https).include? uri.scheme
      raise ::ArgumentError, %(open-uri: unsupported scheme for #{name})
    end
    io = fetch uri, options
    apply_mode io, mode
    if block_given?
      begin
        yield io
      ensure
        io.close unless io.closed?
      end
    else
      io
    end
  end

  def self.fetch uri, options
    redirects = 0
    loop do
      unless %w(http https).include? uri.scheme
        raise ::ArgumentError, %(open-uri: redirection to unsupported scheme: #{uri})
      end
      response = ::Net::HTTP.get_response uri
      io = ::StringIO.new(response.body || '').extend Meta
      io.base_uri = uri
      io.status = [response.code.to_s, response.message.to_s]
      response.each_header {|key, value| io.meta_add_field key, value }
      case response.code.to_i
      when 200..299
        return io
      when 301, 302, 303, 307, 308
        location = (io.meta || {})['location']
        raise HTTPError.new %(#{response.code} #{response.message}), io unless location
        if options[:redirect] == false
          raise HTTPRedirect.new %(#{response.code} #{response.message} (Redirection)), io, (::URI.join uri.to_s, location)
        end
        raise TooManyRedirects.new 'too many redirects', io if (redirects += 1) > MAX_REDIRECTS
        uri = ::URI.parse(::URI.join(uri.to_s, location).to_s)
      else
        raise HTTPError.new %(#{response.code} #{response.message}), io
      end
    end
  end

  # Applies the external encoding of an open mode ("rb", "r:UTF-8",
  # "rb:UTF-8:UTF-8") to the body.
  def self.apply_mode io, mode
    _, external, = mode.split ':'
    body = io.string
    if external
      body.force_encoding external
    elsif mode.include? 'b'
      body.force_encoding ::Encoding::BINARY
    elsif (charset = io.charset)
      body.force_encoding charset rescue body.force_encoding ::Encoding::BINARY
    else
      body.force_encoding ::Encoding::UTF_8
    end
    io.string = body
    io
  end
  private_class_method :fetch, :apply_mode
end

module URI
  # Opens a URI with open-uri; any other name is opened as a file.
  def self.open name, *rest, **options, &block
    if ::URI === name || (::String === name && /\A(?:https?|ftp):\/\//i.match?(name))
      ::OpenURI.open_uri name, *rest, **options, &block
    else
      ::File.open name, *rest, **options, &block
    end
  end
end
