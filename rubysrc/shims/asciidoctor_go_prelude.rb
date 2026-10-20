# frozen_string_literal: true

# Compensates for parts of the Ruby runtime that rbgo does not provide, so that
# the unmodified Asciidoctor source can be loaded. Loaded before asciidoctor.

module Gem
  # Asciidoctor consults Gem.loaded_specs to decide whether to require logger.
  # The logger shim below is always available, so report it as a loaded spec.
  def self.loaded_specs
    @loaded_specs ||= { 'logger' => true }
  end unless respond_to? :loaded_specs
end

# Libraries whose native rbgo implementation is replaced. rbgo resolves its
# native libraries before $LOAD_PATH, so require is intercepted for these names:
# - SHIMS maps a library to a shim in this directory, loaded (once) instead.
# - UNAVAILABLE lists native libraries too incomplete for Asciidoctor to use
#   and not replaced. Requiring one raises the LoadError of a missing gem, so
#   Asciidoctor degrades exactly as it does on MRI when the gem is not installed.
module AsciidoctorGoReplacedLibraries
  SHIMS = {
    'logger' => (File.join __dir__, 'logger.rb'),
    'open-uri' => (File.join __dir__, 'open-uri.rb'),
    'tilt' => (File.join __dir__, 'tilt.rb'),
    'rouge' => (File.join __dir__, 'rouge.rb'),
    'slim/include' => (File.join __dir__, 'slim_include.rb'),
  }.freeze
  UNAVAILABLE = %w().freeze
  LOADED = {}

  private

  def require name
    if (shim = SHIMS[name = name.to_s])
      return false if LOADED[name]
      LOADED[name] = true
      load shim
      true
    elsif UNAVAILABLE.include?(name) || (UNAVAILABLE.include? (name.split '/', 2)[0])
      error = ::LoadError.new %(cannot load such file -- #{name})
      error.instance_variable_set :@path, name
      raise error
    else
      super
    end
  end
end
Object.prepend AsciidoctorGoReplacedLibraries

# rbgo resolves autoload paths natively too (Asciidoctor declares
# `autoload :OpenURI, 'open-uri'`), so map replaced libraries there as well.
# Moreover, rbgo does not trigger an autoload for an explicitly top-level
# reference (::OpenURI), which is how Asciidoctor refers to these constants,
# so a top-level autoload of one of the libraries Asciidoctor autoloads is
# satisfied right away instead.
module AsciidoctorGoReplacedAutoloads
  EAGER = %w(open-uri pathname strscan uri).freeze

  def self.register owner, const, path
    path = path.to_s
    if ::Object.equal?(owner) && (EAGER.include? path)
      ::Kernel.instance_method(:require).bind_call ::Object, path
      return :eager
    end
    AsciidoctorGoReplacedLibraries::SHIMS[path] || path
  end

  def autoload const, path
    target = AsciidoctorGoReplacedAutoloads.register self, const, path
    target == :eager ? nil : (super const, target)
  end
end
Module.prepend AsciidoctorGoReplacedAutoloads
module AsciidoctorGoReplacedKernelAutoloads
  private def autoload const, path
    target = AsciidoctorGoReplacedAutoloads.register ::Object, const, path
    target == :eager ? nil : (super const, target)
  end
end
Kernel.prepend AsciidoctorGoReplacedKernelAutoloads

# rbgo predefines constants for its native libraries without a require.
# Remove those of replaced and unavailable libraries.
Object.send :remove_const, :Rouge if defined? ::Rouge

# rbgo predefines a native ::Logger; remove it so the shim defines the class.
Object.send :remove_const, :Logger if defined? ::Logger
require 'logger'

require 'strscan'

unless StringScanner.method_defined? :get_byte
  # StringScanner#get_byte is missing from rbgo. On MRI it scans exactly one
  # byte, which may be the lead byte of a multibyte character. rbgo's scanner
  # cannot match a pattern starting in the middle of a character, so stopping
  # there would strand every later scan. Instead, a multibyte character is
  # scanned whole. Asciidoctor only compares the result with ASCII characters
  # or joins it with the text scanned next, so the outcome is the same as MRI's.
  # An invalid byte is still consumed alone, and #unscan can revert it.
  module AsciidoctorGoStringScannerGetByte
    ASCII_BYTE_RX = /[\x00-\x7f]/
    CHAR_RX = /./m

    def get_byte
      @__get_byte_origin = nil
      return if eos?
      # a regular scan, so matched, matched? and unscan behave natively
      return scan ASCII_BYTE_RX if (byte = peek 1).ord < 0x80
      return char if (char = scan CHAR_RX)
      origin = pos
      self.pos = origin + 1
      @__get_byte_origin = origin
      byte
    end

    def unscan
      if (origin = @__get_byte_origin) && pos == origin + 1
        @__get_byte_origin = nil
        self.pos = origin
        self
      else
        super
      end
    end

    %i(scan scan_until skip skip_until check check_until match? getch pos= reset terminate).each do |name|
      define_method name do |*args|
        @__get_byte_origin = nil
        super(*args)
      end
    end
  end

  StringScanner.prepend AsciidoctorGoStringScannerGetByte
end

# StringScanner methods missing from rbgo that derive exactly from the ones
# it has (CodeRay uses rest?).
class StringScanner
  def rest? = !eos? unless method_defined? :rest?
  def restsize = rest_size unless method_defined? :restsize
  def peep(len) = peek(len) unless method_defined? :peep
  def getbyte = get_byte unless method_defined? :getbyte
  def peek_byte = string.getbyte(pos) unless method_defined? :peek_byte

  unless method_defined? :exist?
    def exist? pattern
      (found = check_until pattern) && found.bytesize
    end
  end

  unless method_defined? :search_full
    def search_full pattern, advance, return_string
      found = advance ? (scan_until pattern) : (check_until pattern)
      found && (return_string ? found : found.bytesize)
    end
  end

  unless method_defined? :scan_full
    def scan_full pattern, advance, return_string
      found = advance ? (scan pattern) : (check pattern)
      found && (return_string ? found : found.bytesize)
    end
  end
end

# rbgo's Hash#dup drops the default value and default proc, and dup or clone
# of an instance of a Hash or Array subclass yields an object without the
# native storage, which fails on first use (CodeRay dups its WordList, a Hash
# subclass). The copies are made from an allocated instance, which has it.
module AsciidoctorGoCoreCopy
  def self.subclass_copy obj
    copy = obj.class.allocate
    copy.replace obj
    obj.instance_variables.each {|iv| copy.instance_variable_set iv, (obj.instance_variable_get iv) }
    copy
  end

  def self.finish_clone obj, copy, freeze
    obj.singleton_class.included_modules.each {|m| copy.extend m unless copy.is_a? m }
    copy.freeze if freeze || (freeze.nil? && obj.frozen?)
    copy
  end

  def self.copy_default from, to
    if (proc = from.default_proc)
      to.default_proc = proc
    else
      to.default = from.default
    end
    to
  end

  module HashMethods
    def dup
      AsciidoctorGoCoreCopy.copy_default self, (instance_of?(::Hash) ? super : (AsciidoctorGoCoreCopy.subclass_copy self))
    end

    # A plain Hash keeps the native clone, which copies singleton methods.
    def clone freeze: nil
      return AsciidoctorGoCoreCopy.finish_clone self, dup, freeze unless instance_of? ::Hash
      copy = AsciidoctorGoCoreCopy.copy_default self, (super freeze: false)
      copy.freeze if freeze || (freeze.nil? && frozen?)
      copy
    end
  end

  module ArrayMethods
    def dup
      instance_of?(::Array) ? super : (AsciidoctorGoCoreCopy.subclass_copy self)
    end

    def clone freeze: nil
      return super if instance_of? ::Array
      AsciidoctorGoCoreCopy.finish_clone self, dup, freeze
    end
  end
end
Hash.prepend AsciidoctorGoCoreCopy::HashMethods
Array.prepend AsciidoctorGoCoreCopy::ArrayMethods

# rbgo's StringScanner keeps its native state only in objects made by
# StringScanner.new, which also returns a StringScanner when called on a
# subclass, so subclasses (such as CodeRay's scanners) cannot scan. A
# subclass instead gets an initialize that wraps a native scanner and the
# scanner methods, forwarded to it; the subclass's own methods and
# initialize (calling super) work as on MRI.
class StringScanner
  module AsciidoctorGoSubclass
    (::StringScanner.public_instance_methods - ::Object.public_instance_methods).each do |name|
      define_method(name) {|*args, &block| @__agox_scanner.__send__ name, *args, &block }
    end

    def initialize string, *_args, **_opts
      @__agox_scanner = ::StringScanner.new string
    end
  end

  # NOTE no super: rbgo cannot resolve super from a class method to
  # Class#inherited (see docs/rbgo-gaps.md)
  def self.inherited subclass
    return if subclass.include? AsciidoctorGoSubclass
    subclass.include AsciidoctorGoSubclass
    subclass.define_singleton_method :new do |*args, **opts, &block|
      obj = allocate
      obj.__send__ :initialize, *args, **opts, &block
      obj
    end
  end
end

# File#mtime and friends are missing from rbgo; File#stat is present.
class File
  %i(mtime atime ctime).each do |name|
    define_method(name) { stat.public_send name } unless method_defined? name
  end
end

# rbgo's Module#=== ignores modules mixed into an object's singleton class with
# Object#extend, although is_a? honors them. Asciidoctor relies on this to tag
# strings (e.g., Parser::ListContinuationMarker). Rather than slow down every
# Module#=== call, give each module that gets used with extend a corrected ===.
module AsciidoctorGoExtendCaseEquality
  def extend *mods
    mods.each do |mod|
      next if ::Class === mod || (mod.instance_variable_defined? :@__agox_case_equality)
      mod.instance_variable_set :@__agox_case_equality, true
      mod.define_singleton_method(:===) {|other| other.is_a? self }
    end
    super
  end
end
Object.prepend AsciidoctorGoExtendCaseEquality unless (::Module.new.tap {|m| (o = ::Object.new).extend m; break m === o })

# rbgo makes NotImplementedError a StandardError; in Ruby it is a ScriptError,
# so a bare `rescue` does not catch it. Asciidoctor relies on this (for
# example, a missing converter must not be wrapped as a load failure). rbgo
# raises built-in exceptions by class name, so redefining the constant also
# applies to exceptions raised natively.
unless NotImplementedError.superclass == ScriptError
  Object.send :remove_const, :NotImplementedError
  class NotImplementedError < ScriptError; end
end

# rbgo defines no Class#inherited, and Class.new(superclass) does not call the
# hook at all (only `class X < Y` does). Provide the default hook, and call it
# from Class.new before the class body runs, as Ruby does. (A `super` call in
# an inherited hook still fails: rbgo does not resolve super from a class
# method to instance methods of Class.)
class Class
  private def inherited _subclass; end unless private_method_defined?(:inherited) || method_defined?(:inherited)
end

unless (::Class.new.tap {|base| base.define_singleton_method(:inherited) {|_| @__agox_seen = true } ; ::Class.new base }.instance_variable_get :@__agox_seen)
  class << Class
    alias_method :__agox_native_new, :new

    def new superclass = ::Object, &block
      klass = __agox_native_new superclass
      superclass.__send__ :inherited, klass
      klass.class_exec klass, &block if block
      klass
    end
  end
end

# rbgo's Exception#full_message leaves out the cause chain, which Ruby prints
# after the backtrace (each cause as "location: message (Class)" followed by
# its backtrace).
if (begin
  begin
    raise ::ArgumentError, 'agox cause probe'
  rescue ::ArgumentError
    raise ::IOError, 'agox probe'
  end
rescue ::IOError => probe
  !(probe.full_message(highlight: false).include? 'agox cause probe')
end)
  class Exception
    alias_method :__agox_native_full_message, :full_message

    def full_message highlight: false, order: :top
      message = __agox_native_full_message highlight: highlight, order: order
      seen = { self => true }
      cause = self.cause
      while cause && !seen[cause]
        seen[cause] = true
        backtrace = cause.backtrace || []
        message += %(#{backtrace[0] ? %(#{backtrace[0]}: ) : ''}#{cause.message} (#{cause.class})\n)
        message += (backtrace.drop 1).map {|line| %(\tfrom #{line}\n) }.join
        cause = cause.cause
      end
      message
    end
  end
end

# FileUtils.rmdir is missing from rbgo.
require 'fileutils'
unless FileUtils.respond_to? :rmdir
  module FileUtils
    # Removes the given (empty) directories; with parents, also the parent
    # directories that become empty.
    def self.rmdir list, parents: nil, noop: nil, verbose: nil
      list = [list].flatten.map(&:to_s)
      $stderr.puts %(rmdir #{parents ? '-p ' : ''}#{list.join ' '}) if verbose
      return if noop
      list.each do |dir|
        ::Dir.rmdir(dir = dir.chomp '/')
        next unless parents
        begin
          until (parent = ::File.dirname dir) == '.' || parent == dir
            ::Dir.rmdir(dir = parent)
          end
        rescue ::Errno::ENOTEMPTY, ::Errno::EEXIST, ::Errno::ENOENT, ::Errno::EACCES
        end
      end
      nil
    end

  end
end

# Kernel#Pathname and many Pathname methods are missing from rbgo's pathname.
require_relative 'pathname_ext'
module Kernel
  private def Pathname path
    ::Pathname === path ? path : (::Pathname.new path)
  end
end unless Kernel.private_method_defined?(:Pathname) || Kernel.method_defined?(:Pathname)

# On a string with an invalid byte sequence, Ruby's String#rstrip and #strip
# raise Encoding::CompatibilityError; rbgo's do not. Asciidoctor relies on the
# error to reject binary or invalid input ("source is either binary or
# contains invalid Unicode data").
unless (begin
  "\xFF ".dup.force_encoding(::Encoding::UTF_8).rstrip
  false
rescue ::Encoding::CompatibilityError
  true
end)
  module AsciidoctorGoStrictStrip
    %i(rstrip rstrip! strip strip!).each do |name|
      define_method name do
        unless valid_encoding?
          raise ::Encoding::CompatibilityError, %(invalid byte sequence in #{encoding})
        end
        super()
      end
    end
  end
  String.prepend AsciidoctorGoStrictStrip
end

# rbgo's URI objects are native: there is no path= (or query=, fragment=,
# user=, password=) setter, and dup shares state with the original. Asciidoctor
# derives the directory of an included URI by duplicating it and assigning the
# path; a source patch calls this helper instead, which builds that URI.
module AsciidoctorGoURI
  def self.with_path uri, path
    result = %(#{uri.scheme}:)
    if uri.host
      result += '//'
      result += %(#{uri.userinfo}@) if uri.userinfo
      result += uri.host
      result += %(:#{uri.port}) if uri.port && uri.port != uri.default_port
    end
    result += path.to_s
    result += %(?#{uri.query}) if uri.query
    result += %(##{uri.fragment}) if uri.fragment
    ::URI.parse result
  end
end

# rbgo's native OptionParser raises TypeError for nil elements in argv, which
# Ruby's skips, and reports an invalid separate argument without its value
# ("invalid argument: -d" instead of "invalid argument: -d chapter").
require 'optparse'
if (begin
  ::OptionParser.new.parse! [nil]
  false
rescue ::TypeError
  true
end)
  module AsciidoctorGoOptionParserCompat
    %i(parse! order! permute!).each do |name|
      define_method name do |*args, **kwargs, &block|
        argv = ::Array === args[0] ? args[0] : default_argv
        argv.compact!
        original = argv.dup
        begin
          super(*args, **kwargs, &block)
        rescue ::OptionParser::InvalidArgument => e
          if e.args.size == 1 && (index = original.index e.args[0]) && (value = original[index + 1])
            raise ::OptionParser::InvalidArgument.new(e.args[0], value)
          end
          raise
        end
      end
    end

    %i(parse order permute).each do |name|
      define_method name do |*args, **kwargs, &block|
        argv = (args.size == 1 && ::Array === args[0]) ? args[0].dup : args.dup
        __send__ :"#{name}!", argv, **kwargs, &block
      end
    end
  end
  ::OptionParser.prepend AsciidoctorGoOptionParserCompat
end
