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

# Libraries whose native rbgo implementation is replaced by a shim in this
# directory. rbgo resolves its native libraries before $LOAD_PATH, so require
# is intercepted for these names and loads the shim (once) instead.
module AsciidoctorGoReplacedLibraries
  SHIMS = { 'logger' => (File.join __dir__, 'logger.rb') }.freeze
  LOADED = {}

  private

  def require name
    if (shim = SHIMS[name = name.to_s])
      return false if LOADED[name]
      LOADED[name] = true
      load shim
      true
    else
      super
    end
  end
end
Object.prepend AsciidoctorGoReplacedLibraries

# rbgo predefines a native ::Logger; remove it so the shim defines the class.
Object.send :remove_const, :Logger if defined? ::Logger
require 'logger'

require 'strscan'

unless StringScanner.method_defined? :get_byte
  # StringScanner#get_byte is missing from rbgo. It scans exactly one byte,
  # which may be the lead byte of a multibyte character, and leaves the
  # scanner in a state that #unscan can revert.
  module AsciidoctorGoStringScannerGetByte
    ASCII_BYTE_RX = /[\x00-\x7f]/

    def get_byte
      @__get_byte_origin = nil
      return if eos?
      # ASCII byte: a regular scan, so matched, matched? and unscan behave natively
      return scan ASCII_BYTE_RX if (byte = peek 1).ord < 0x80
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
