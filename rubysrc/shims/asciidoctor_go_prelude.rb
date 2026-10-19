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
# - UNAVAILABLE lists native libraries too incomplete for Asciidoctor to use.
#   Requiring one raises the LoadError of a missing gem, so Asciidoctor degrades
#   exactly as it does on MRI when the optional gem is not installed.
module AsciidoctorGoReplacedLibraries
  SHIMS = { 'logger' => (File.join __dir__, 'logger.rb') }.freeze
  UNAVAILABLE = %w(rouge).freeze
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
