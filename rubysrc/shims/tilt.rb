# frozen_string_literal: true

# Tilt, the template interface Asciidoctor's template converter uses, for
# rbgo (which does not ship the tilt gem). As in Tilt, a template compiles to
# Ruby source once; rendering binds that source, as a method, to the scope
# object (so self and its instance variables are the template's), assigns the
# locals, and passes the block (for yield). The engines are rbgo's ERB and
# Erubi, and the go-ruby-haml and go-ruby-slim compilers, which take the
# format and escaping options Asciidoctor passes.
module Tilt
  VERSION = '2.4.0'

  @template_map = {}

  class << self
    # Registers template_class for the file extensions (without a dot).
    def register template_class, *extensions
      extensions.each {|ext| @template_map[ext.to_s.downcase] = template_class }
      template_class
    end

    def registered? ext
      @template_map.key? ext.to_s.downcase.sub(/\A\./, '')
    end

    # Returns the template class for a file name or extension, or nil.
    def [] file
      name = ::File.basename file.to_s
      parts = name.downcase.split '.'
      parts = ['', *parts] if parts.size == 1
      (1...parts.size).each do |i|
        if (klass = @template_map[parts[i..].join '.'])
          return klass
        end
      end
      nil
    end

    def template_for file
      self[file] || (raise %(No template engine registered for #{::File.basename file.to_s}))
    end

    def new file, line = nil, options = {}, &block
      (template_for file).new file, line, options, &block
    end

    def default_mapping
      self
    end
  end

  class Template
    attr_reader :data, :file, :line, :options

    def self.default_mime_type
      nil
    end

    def initialize *args, &block
      @options = ::Hash === args.last ? args.pop.dup : {}
      @line = (args.find {|arg| ::Integer === arg }) || 1
      @file = (file = args.find {|arg| ::String === arg || (defined?(::Pathname) && ::Pathname === arg) }) && file.to_s
      if block
        @data = block.call self
      elsif @file
        @data = ::File.read @file, mode: 'rb:UTF-8:UTF-8'
      else
        raise ::ArgumentError, 'file or block required'
      end
      @compiled = {}
      prepare
    end

    def eval_file
      @file || '(__TEMPLATE__)'
    end

    def render scope = nil, locals = {}, &block
      scope = ::Object.new if scope.nil?
      locals ||= {}
      (compiled_method locals.keys).bind_call scope, locals, &block
    end

    # The Ruby source that evaluates to the rendered output.
    def precompiled_template _locals
      raise ::NotImplementedError, %(#{self.class} must implement precompiled_template)
    end

    private

    def prepare; end

    def compiled_method keys
      @compiled[keys] ||= begin
        assignments = keys.map {|key| %(#{key} = locals[#{key.inspect}]) }
        source = precompiled_template keys
        holder = ::Module.new
        holder.module_eval %(def __tilt_render locals\n#{assignments.join ';'}\n#{source}\nend), eval_file, @line - 2
        holder.instance_method :__tilt_render
      end
    end
  end

  class ERBTemplate < Template
    LEGACY_TRIM_MODES = { 0 => nil, 1 => '>', 2 => '<>' }.freeze

    def self.default_mime_type
      'text/html'
    end

    private

    def prepare
      require 'erb'
      trim = case (value = @options[:trim])
             when false then nil
             when nil, true then '<>'
             when ::Integer then LEGACY_TRIM_MODES.fetch value, nil
             else value
             end
      @outvar = (@options[:outvar] || '_erbout').to_s
      @engine = ::ERB.new @data, trim_mode: trim, eoutvar: @outvar
    end

    public

    def precompiled_template _locals
      @engine.src
    end
  end

  class ErubiTemplate < Template
    private

    def prepare
      require 'erubi'
      engine_options = @options.reject {|key, _| key == :engine_class || key == :outvar }
      engine_options[:bufvar] = @options[:outvar].to_s if @options[:outvar]
      @engine = (@options[:engine_class] || ::Erubi::Engine).new @data, engine_options
    end

    public

    def precompiled_template _locals
      @engine.src
    end
  end

  class HamlTemplate < Template
    private

    def prepare
      format = (@options[:format] || :html5).to_s
      escape = @options.fetch :escape_html, true
      @escape_attrs = @options.fetch :escape_attrs, true
      @source = ::AsciidoctorGo::TemplateCompiler.haml @data, format, (escape ? nil : '::Tilt::RawOutput.output')
    end

    public

    def precompiled_template _locals
      @source
    end

    def render scope = nil, locals = {}, &block
      stack = (::Thread.current[:agox_haml_escape_attrs] ||= [])
      stack.push @escape_attrs
      begin
        super
      ensure
        stack.pop
      end
    end
  end

  class SlimTemplate < Template
    INCLUDE_RX = /^([ \t]*)include[ \t]+(\S+)[ \t]*$/

    private

    def prepare
      escape = !(@options[:disable_escape])
      source = expand_includes @data, @file ? (::File.dirname @file) : nil, 0
      @source = ::AsciidoctorGo::TemplateCompiler.slim source, (escape ? nil : '::Tilt::RawOutput.output')
    end

    # Inlines `include name` lines (the slim/include plugin), resolving name
    # against the including file's directory and the :include_dirs option.
    def expand_includes data, dir, depth
      return data unless defined?(::Slim::Include) && (INCLUDE_RX.match? data)
      raise ::ArgumentError, 'slim include depth exceeded' if depth > 32
      data.gsub INCLUDE_RX do
        indent, name = $1, $2
        dirs = [dir, *(@options[:include_dirs] || [])].compact
        name = %(#{name}.slim) unless ::File.extname(name) == '.slim'
        path = dirs.map {|d| ::File.join d, name }.find {|p| ::File.file? p } ||
          (raise ::ArgumentError, %(Template #{name} not found))
        included = expand_includes((::File.read path, mode: 'rb:UTF-8:UTF-8'), (::File.dirname path), depth + 1)
        included.each_line.map {|line| line.strip.empty? ? line : indent + line }.join.chomp
      end
    end

    public

    def precompiled_template _locals
      @source
    end

    def render scope = nil, locals = {}, &block
      scope.extend ::Slim::Helpers if scope && defined?(::Slim::Helpers) && !(::Slim::Helpers === scope)
      super
    end
  end

  # The "escape" function of templates compiled without escaping.
  module RawOutput
    def self.output value
      value.to_s
    end
  end

  register ERBTemplate, 'erb', 'rhtml'
end

# As in the haml and slim gems, Haml::Template and Slim::Template are Tilt
# templates (rbgo's native classes of those names are not, and keep serving as
# Haml::Engine and Slim::Engine).
require 'haml'
require 'slim'

module Haml
  remove_const :Template if const_defined?(:Template, false) && !(const_get(:Template, false) < ::Tilt::Template)
  class Template < ::Tilt::HamlTemplate; end unless const_defined?(:Template, false)
end

module Slim
  remove_const :Template if const_defined?(:Template, false) && !(const_get(:Template, false) < ::Tilt::Template)
  class Template < ::Tilt::SlimTemplate; end unless const_defined?(:Template, false)
end

Tilt.register Haml::Template, 'haml'

# The go-ruby-haml compiler emits Haml::HamlAttributes.render(format, attrs);
# rbgo's runtime helper takes only the attributes. This renders dynamic
# attributes for the format (boolean attributes are bare in html5 and html4,
# name="name" in xhtml) and escapes values unless the template being rendered
# was created with escape_attrs: false.
module Haml
  module HamlAttributes
    BOOLEAN_ATTRIBUTES = %w(disabled readonly multiple checked selected hidden required async defer novalidate
      autofocus open reversed ismap muted controls loop autoplay formnovalidate itemscope nomodule playsinline).freeze

    def self.render *args
      format, attributes = args.size == 2 ? args : ['html5', args[0]]
      pairs = {}
      attributes.each do |key, value|
        key = key.to_s
        if key == 'data' && ::Hash === value
          value.each {|data_key, data_value| pairs[%(data-#{data_key.to_s.tr '_', '-'})] = data_value }
        elsif key == 'class'
          merged = [pairs['class'], (::Array === value ? (value.compact.join ' ') : value)].compact.reject {|c| c == '' || c == false }
          pairs['class'] = merged.empty? ? nil : (merged.join ' ')
        elsif key == 'id'
          ids = [pairs['id'], (::Array === value ? (value.compact.join '_') : value)].compact.reject {|i| i == '' || i == false }
          pairs['id'] = ids.empty? ? nil : (ids.join '_')
        else
          pairs[key] = value
        end
      end
      escape = (::Thread.current[:agox_haml_escape_attrs] || []).last != false
      output = +''
      pairs.keys.sort.each do |key|
        value = pairs[key]
        next if value.nil? || value == false
        if value == true || (BOOLEAN_ATTRIBUTES.include?(key) && value)
          output << (format.to_s == 'xhtml' ? %( #{key}="#{key}") : %( #{key}))
        else
          value = value.to_s
          output << %( #{key}="#{escape ? (::Haml::Util.escape_html value) : value}")
        end
      end
      output
    end
  end
end
Tilt.register Slim::Template, 'slim'
