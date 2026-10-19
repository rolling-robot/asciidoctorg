# frozen_string_literal: true

# Completes rbgo's native Pathname with the public methods it lacks. As in
# Ruby's pathname library, these delegate to File, FileTest, Dir, FileUtils
# and Find. Each is defined only if missing.
require 'pathname'

class Pathname
  {
    File => %i(atime birthtime ctime mtime ftype lstat stat size readlink truncate chmod lchmod chown lchown utime lutime),
    FileTest => %i(blockdev? chardev? executable? executable_real? grpowned? owned? pipe? readable? readable_real? setgid? setuid? size? socket? sticky? symlink? world_readable? world_writable? writable? writable_real? zero?),
  }.each do |target, names|
    names.each do |name|
      next if method_defined? name
      case name
      when :chmod, :lchmod
        define_method(name) {|mode| target.public_send name, mode, @path }
      when :chown, :lchown
        define_method(name) {|owner, group| target.public_send name, owner, group, @path }
      when :utime, :lutime
        define_method(name) {|atime, mtime| target.public_send name, atime, mtime, @path }
      when :truncate
        define_method(name) {|length| target.public_send name, @path, length }
      else
        define_method(name) { target.public_send name, @path }
      end
    end
  end

  def === other
    self == other
  end unless method_defined? :===

  def empty?
    ::File.directory?(@path) ? ::Dir.empty?(@path) : ::FileTest.empty?(@path)
  end unless method_defined? :empty?

  def binread(*args) = ::File.binread(@path, *args) unless method_defined? :binread
  def binwrite(*args, **opts) = ::File.binwrite(@path, *args, **opts) unless method_defined? :binwrite
  def readlines(*args, **opts) = ::File.readlines(@path, *args, **opts) unless method_defined? :readlines
  def sysopen(*args) = ::IO.sysopen(@path, *args) unless method_defined? :sysopen
  def rename(to) = ::File.rename(@path, to) unless method_defined? :rename
  def make_link(old) = ::File.link(old, @path) unless method_defined? :make_link
  def make_symlink(old) = ::File.symlink(old, @path) unless method_defined? :make_symlink
  def realpath(basedir = nil) = self.class.new(::File.realpath(@path, basedir)) unless method_defined? :realpath
  def realdirpath(basedir = nil) = self.class.new(::File.realdirpath(@path, basedir)) unless method_defined? :realdirpath
  def expand_path(*args) = self.class.new(::File.expand_path(@path, *args)) unless method_defined? :expand_path

  def fnmatch(pattern, *args) = ::File.fnmatch(pattern, @path, *args) unless method_defined? :fnmatch
  alias fnmatch? fnmatch unless method_defined? :fnmatch?

  def sub(*args, &block)
    self.class.new(block ? (@path.sub(*args, &block)) : (@path.sub(*args)))
  end unless method_defined? :sub

  def delete
    ::File.directory?(@path) ? ::Dir.unlink(@path) : ::File.unlink(@path)
  end unless method_defined? :delete
  alias unlink delete unless method_defined? :unlink

  def mkdir(*args) = ::Dir.mkdir(@path, *args) unless method_defined? :mkdir
  def rmdir = ::Dir.rmdir(@path) unless method_defined? :rmdir
  def opendir(&block) = ::Dir.open(@path, &block) unless method_defined? :opendir

  def entries
    ::Dir.entries(@path).map {|e| self.class.new e }
  end unless method_defined? :entries

  def each_entry(&block)
    ::Dir.foreach(@path) {|e| yield self.class.new(e) }
  end unless method_defined? :each_entry

  def children with_directory = true
    result = []
    ::Dir.foreach @path do |e|
      next if e == '.' || e == '..'
      result << (with_directory ? (self + e) : (self.class.new e))
    end
    result
  end unless method_defined? :children

  def each_child(with_directory = true, &block)
    children(with_directory).each(&block)
  end unless method_defined? :each_child

  def glob(*patterns, **opts, &block)
    result = ::Dir.glob(patterns, **opts, base: @path).map {|p| self + p }
    block ? result.each(&block) && nil : result
  end unless method_defined? :glob

  def mkpath(mode: nil)
    require 'fileutils'
    mode ? ::FileUtils.mkpath(@path, mode: mode) : ::FileUtils.mkpath(@path)
    self
  end unless method_defined? :mkpath

  def rmtree(noop: nil, verbose: nil, secure: nil)
    require 'fileutils'
    ::FileUtils.rm_rf @path
    self
  end unless method_defined? :rmtree

  def find(ignore_error: true)
    return to_enum(__method__, ignore_error: ignore_error) unless block_given?
    require 'find'
    if @path == '.'
      ::Find.find(@path, ignore_error: ignore_error) {|f| yield self.class.new(f.sub(%r{\A\./}, '')) }
    else
      ::Find.find(@path, ignore_error: ignore_error) {|f| yield self.class.new(f) }
    end
  end unless method_defined? :find

  def mountpoint?
    stat1 = lstat
    stat2 = (parent.lstat rescue (return false))
    stat1.dev != stat2.dev || stat1.ino == stat2.ino
  end unless method_defined? :mountpoint?

  class << self
    def getwd = new(::Dir.getwd) unless method_defined? :getwd
    alias pwd getwd unless method_defined? :pwd

    def glob(*args, **opts)
      result = ::Dir.glob(*args, **opts).map {|p| new p }
      block_given? ? result.each {|p| yield p } && nil : result
    end unless method_defined? :glob

    def mktmpdir(*args)
      require 'tmpdir'
      if block_given?
        ::Dir.mktmpdir(*args) {|dir| yield new(dir) }
      else
        new ::Dir.mktmpdir(*args)
      end
    end unless method_defined? :mktmpdir
  end
end
