# frozen_string_literal: true

# Rouge for rbgo, on the go-rouge library (AsciidoctorGo::RougeNative), with
# the parts of the gem's API that Asciidoctor's adapter and RougeExt use:
# tokens, lexers (find, find_fancy with cgi-style options, lex), formatters
# (Formatter#format/stream/token_lines, HTML, HTMLInline) and themes (find,
# render, base_style, palette). rbgo's native Rouge lacks this API.
module Rouge
  def self.version
    '5.0.0'
  end

  class Token
    attr_reader :name, :qualname, :shortname, :parent, :sub_tokens

    def initialize name, qualname, shortname, parent
      @name, @qualname, @shortname, @parent = name, qualname, shortname, parent
      @sub_tokens = {}
      parent.sub_tokens[name] = self if parent
    end

    def token_chain
      @parent ? [*@parent.token_chain, self] : [self]
    end

    def matches? other
      token_chain.include? other
    end

    def === other
      ::Rouge::Token === other && (other.matches? self)
    end

    def to_s
      @qualname
    end

    def inspect
      %(<Token #{@qualname}>)
    end

    REGISTRY = {}

    def self.[] qualname
      REGISTRY[qualname.to_s]
    end

    def self.each_token(&block)
      REGISTRY.each_value(&block)
    end

    module Tokens; end

    ::AsciidoctorGo::RougeNative.tokens.each do |qualname, shortname|
      *parents, name = qualname.split '.'
      parent = parents.empty? ? nil : REGISTRY[parents.join '.']
      REGISTRY[qualname] = token = new name, qualname, shortname, parent
      namespace = parents.reduce(Tokens) {|ns, part| ns.const_get part, false }
      namespace.const_set name, ::Module.new unless namespace.const_defined? name, false
      namespace.const_get(name, false).define_singleton_method(:token) { token }
    end
  end

  class Lexer
    attr_reader :options

    class << self
      attr_reader :tag, :title, :aliases

      def lexers
        @lexers ||= {}
      end

      # Returns the lexer class for a tag or alias, or nil.
      def find name
        Lexer.lexers[name.to_s.downcase]
      end

      # Returns a lexer for a name with optional cgi-style options
      # ("ruby?start_inline=true"), or nil.
      def find_fancy str, code = nil, default_options = {}
        name, query = str.to_s.split '?', 2
        options = default_options.transform_keys(&:to_s)
        query&.split('&')&.each do |pair|
          key, value = pair.split '=', 2
          options[key] = value.nil? ? true : value
        end
        if name == 'guess'
          klass = find(::AsciidoctorGo::RougeNative.guess code.to_s)
        else
          return unless (klass = find name)
        end
        klass.new options
      end

      def guess source:, **_
        find ::AsciidoctorGo::RougeNative.guess source
      end

      def all
        Lexer.lexers.values.uniq
      end

      def lex source, opts = {}, &block
        new(opts).lex source, &block
      end

      def desc
        @title
      end
    end

    def initialize opts = {}
      @options = opts.transform_keys(&:to_s)
    end

    def tag
      self.class.tag
    end

    # Lexes source into [token, value] pairs; yields each pair when given a
    # block, and otherwise returns them.
    def lex source, opts = {}, &block
      spec = @options.empty? ? tag : %(#{tag}?#{@options.map {|k, v| %(#{k}=#{v}) }.join '&'})
      tokens = ::AsciidoctorGo::RougeNative.lex(spec, source).map {|qualname, value| [Token[qualname], value] }
      return tokens.each(&block) if block
      tokens.each
    end
  end

  module Lexers
    CLASS_NAMES = {
      'plaintext' => :PlainText, 'json' => :JSON, 'diff' => :Diff, 'go' => :Go, 'sql' => :SQL,
      'css' => :CSS, 'shell' => :Shell, 'yaml' => :YAML, 'python' => :Python, 'javascript' => :Javascript,
      'html' => :HTML, 'markdown' => :Markdown, 'ruby' => :Ruby, 'php' => :PHP, 'tex' => :TeX,
    }.freeze

    ::AsciidoctorGo::RougeNative.lexers.each do |tag, title, aliases|
      klass = ::Class.new ::Rouge::Lexer
      klass.instance_variable_set :@tag, tag
      klass.instance_variable_set :@title, title
      klass.instance_variable_set :@aliases, aliases
      const_set CLASS_NAMES[tag] || tag.capitalize.to_sym, klass
      [tag, *aliases].each {|name| ::Rouge::Lexer.lexers[name.downcase] = klass }
    end
  end

  module Lexers
    # Rouge's console lexer (go-rouge has none): a line that starts with a
    # prompt is a Generic.Prompt token followed by a command, lexed with the
    # shell lexer (or the lexer named by the lang option) together with the
    # continuation lines that follow a trailing backslash; other lines are
    # Generic.Output. The prompt option (comma-separated) replaces the default
    # prompts ($, #, >, ;).
    class ConsoleLexer < ::Rouge::Lexer
      @tag = 'console'
      @title = 'Console'
      @aliases = %w(terminal shell_session shell-session)

      def prompts
        if (custom = @options['prompt'])
          custom.to_s.split(',').reject(&:empty?)
        else
          %w($ # > ;)
        end
      end

      def lex source, _opts = {}, &block
        prompt_rx = /\A(.*?(?:#{prompts.map {|p| ::Regexp.escape p }.join '|'}))/
        command_lexer = (lang = @options['lang']) && (::Rouge::Lexer.find lang) || ::Rouge::Lexers::Shell
        tokens = []
        command = nil
        flush = proc do
          if command
            tokens.concat command_lexer.new.lex(command).to_a
            command = nil
          end
        end
        source.each_line do |line|
          if command
            command += line
          elsif (match = prompt_rx.match line)
            tokens << [::Rouge::Token['Generic.Prompt'], match[1]]
            command = line[match[1].length..]
          else
            tokens << [::Rouge::Token['Generic.Output'], line]
          end
          flush.call if command && !(command.chomp.end_with? '\\')
        end
        flush.call
        block ? tokens.each(&block) : tokens.each
      end
    end
    ['console', *ConsoleLexer.aliases].each {|name| ::Rouge::Lexer.lexers[name] = ConsoleLexer }
  end

  class Formatter
    def self.format tokens, *args, &block
      new(*args).format tokens, &block
    end

    def self.tag value = nil
      value ? (@tag = value) : @tag
    end

    def format tokens, &block
      return stream(tokens, &block) if block
      out = +''
      stream(tokens) {|piece| out << piece }
      out
    end

    def stream _tokens
      raise ::NotImplementedError, %(#{self.class} must implement stream)
    end

    # Yields the tokens of each line (split on newlines).
    def token_lines tokens, &block
      return enum_for :token_lines, tokens unless block
      out = []
      tokens.each do |token, value|
        value.scan(/\n|[^\n]+/) do |segment|
          if segment == "\n"
            yield out
            out = []
          else
            out << [token, segment]
          end
        end
      end
      yield out unless out.empty?
    end
  end

  module Formatters
    class HTML < ::Rouge::Formatter
      tag 'html'

      def stream tokens
        yield ::AsciidoctorGo::RougeNative.format_html(tokens.map {|token, value| [token.qualname, value] })
      end
    end

    class HTMLInline < HTML
      tag 'html_inline'

      def initialize theme
        @theme = ::Class === theme ? theme : theme.class
      end

      def stream tokens
        yield ::AsciidoctorGo::RougeNative.format_html_inline(@theme.name, tokens.map {|token, value| [token.qualname, value] })
      end
    end
  end

  class Theme
    class << self
      def registry
        @registry ||= {}
      end

      def find name
        Theme.registry[name.to_s]
      end

      def name value = nil
        value ? (@name = value) : @name
      end

      # [[token, declarations], ...] for the tokens the theme styles.
      def styles
        @styles ||= ::AsciidoctorGo::RougeNative.theme_styles(@name).map {|qualname, rules| [Token[qualname], rules] }
      end

      def style_for token
        token.token_chain.reverse_each {|t| (pair = styles.find {|s, _| s.equal? t }) && (return pair[1]) }
        (pair = styles.find {|s, _| s.qualname == 'Text' }) ? pair[1] : ''
      end

      # The foreground (:fg) and background (:bg) colors of the base style.
      def base_style
        result = {}
        (style_for Token['Text']).split(';').each do |rule|
          property, value = rule.split ':', 2
          case property.strip
          when 'color' then result[:fg] = value.strip
          when 'background-color' then result[:bg] = value.strip
          end
        end
        result
      end

      # Colors are resolved already.
      def palette value
        value
      end

      # Renders the theme as CSS rules below scope, as Rouge's CSSTheme does.
      def render scope: '.highlight', &block
        return enum_for(:render, scope: scope).to_a.join "\n" unless block
        yield %(#{scope} table td { padding: 5px; })
        yield %(#{scope} table pre { margin: 0; })
        styled = styles.map(&:first)
        styles.each do |token, rules|
          selector = (inflate token, styled).map {|t| t.qualname == 'Text' ? scope : %(#{scope} .#{t.shortname}) }.join ', '
          yield %(#{selector} {)
          rules.split(';').each {|rule| yield %(  #{rule.strip};) }
          yield '}'
        end
      end

      private

      def inflate token, styled, result = []
        result << token
        token.sub_tokens.each_value {|sub| inflate sub, styled, result unless styled.include? sub }
        result
      end
    end

    def name
      self.class.name
    end
  end

  class CSSTheme < Theme; end

  module Themes
    CLASS_NAMES = { 'base16' => :Base16, 'github' => :Github, 'thankful_eyes' => :ThankfulEyes }.freeze

    ::AsciidoctorGo::RougeNative.themes.each do |theme_name|
      klass = ::Class.new ::Rouge::CSSTheme
      klass.name theme_name
      const_set CLASS_NAMES[theme_name], klass
      ::Rouge::Theme.registry[theme_name] = klass
    end
  end

  # Highlights text with the lexer and formatter given by tag (or instance).
  def self.highlight text, lexer, formatter, &block
    lexer = Lexer.find lexer unless Lexer === lexer || (::Class === lexer && lexer < Lexer)
    raise %(unknown lexer #{lexer}) unless lexer
    formatter = { 'html' => Formatters::HTML, 'html_inline' => Formatters::HTMLInline }[formatter] || formatter
    formatter = formatter.new if ::Class === formatter && formatter == Formatters::HTML
    formatter.format lexer.lex(text), &block
  end
end
