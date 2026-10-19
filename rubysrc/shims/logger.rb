# frozen_string_literal: true

# A pure-Ruby implementation of the standard library Logger (the logger gem,
# 1.6 API). rbgo's native Logger keeps its state in a Go struct and its
# Logger.new ignores the receiver, so subclasses such as Asciidoctor::Logger,
# MemoryLogger and NullLogger cannot work on top of it. This implementation
# replaces it; the prelude routes require 'logger' here.
class Logger
  VERSION = '1.6.4'
  ProgName = %(logger.rb/v#{VERSION})

  class Error < ::RuntimeError; end
  class ShiftingError < Error; end

  module Severity
    DEBUG = 0
    INFO = 1
    WARN = 2
    ERROR = 3
    FATAL = 4
    UNKNOWN = 5

    # NOTE no lookup table constant: Severity.constants must list only the levels
    def self.coerce severity
      return severity if ::Integer === severity
      case severity.to_s.downcase
      when 'debug' then DEBUG
      when 'info' then INFO
      when 'warn' then WARN
      when 'error' then ERROR
      when 'fatal' then FATAL
      when 'unknown' then UNKNOWN
      else raise ::ArgumentError, %(invalid log level: #{severity})
      end
    end
  end
  include Severity

  class Formatter
    Format = "%.1s, [%s #%d] %5s -- %s: %s\n"
    DatetimeFormat = '%Y-%m-%dT%H:%M:%S.%6N'

    attr_accessor :datetime_format

    def initialize
      @datetime_format = nil
    end

    def call severity, time, progname, msg
      format Format, severity, (format_datetime time), ::Process.pid, severity, progname, (msg2str msg)
    end

    private

    def format_datetime time
      time.strftime(@datetime_format || DatetimeFormat)
    end

    def msg2str msg
      case msg
      when ::String
        msg
      when ::Exception
        %(#{msg.message} (#{msg.class})\n#{(msg.backtrace || []).join "\n"})
      else
        msg.inspect
      end
    end
  end

  module Period
    SiD = 24 * 60 * 60

    module_function

    def next_rotate_time now, shift_age
      case shift_age
      when 'daily', :daily
        t = ::Time.mktime now.year, now.month, now.mday + 1
      when 'weekly', :weekly
        t = ::Time.mktime now.year, now.month, now.mday + 7 - now.wday
      when 'monthly', :monthly
        t = now.month == 12 ? (::Time.mktime now.year + 1, 1, 1) : (::Time.mktime now.year, now.month + 1, 1)
      when 'now', 'everytime', :now, :everytime
        return now
      else
        raise ::ArgumentError, %(invalid :shift_age #{shift_age.inspect}, should be daily, weekly, monthly, or everytime)
      end
      t
    end

    def previous_period_end now, shift_age
      case shift_age
      when 'daily', :daily
        t = ::Time.mktime now.year, now.month, now.mday
      when 'weekly', :weekly
        t = ::Time.mktime now.year, now.month, now.mday - now.wday
      when 'monthly', :monthly
        t = ::Time.mktime now.year, now.month, 1
      when 'now', 'everytime', :now, :everytime
        return now
      else
        raise ::ArgumentError, %(invalid :shift_age #{shift_age.inspect}, should be daily, weekly, monthly, or everytime)
      end
      t - 1
    end
  end

  # Writes to an IO or to a file, rotating the file by size or by period.
  class LogDevice
    include Period

    attr_reader :dev, :filename

    def initialize log = nil, shift_age: nil, shift_size: nil, shift_period_suffix: nil, binmode: false, reraise_write_errors: []
      @dev = @filename = @shift_age = @shift_size = @shift_period_suffix = nil
      @binmode = binmode
      @reraise_write_errors = reraise_write_errors
      @mon = ::Mutex.new
      set_dev log
      return unless @filename
      @shift_age = shift_age || 7
      @shift_size = shift_size || 1048576
      @shift_period_suffix = shift_period_suffix || '%Y%m%d'
      unless ::Integer === @shift_age
        base_time = @dev.respond_to?(:stat) ? @dev.stat.mtime : ::Time.now
        @next_rotate_time = next_rotate_time base_time, @shift_age
      end
    end

    def write message
      @mon.synchronize do
        if @shift_age && @dev.respond_to?(:stat)
          begin
            check_shift_log
          rescue *@reraise_write_errors
            raise
          rescue ::StandardError
            warn %(log shifting failed. #{$!})
          end
        end
        begin
          @dev.write message
        rescue *@reraise_write_errors
          raise
        rescue ::StandardError
          warn %(log writing failed. #{$!})
        end
      end
    rescue *@reraise_write_errors
      raise
    rescue ::StandardError => e
      warn %(log writing failed. #{e})
    end

    def close
      @mon.synchronize do
        @dev.close rescue nil
      end
    end

    def reopen log = nil
      @mon.synchronize do
        if @filename && @dev
          @dev.close rescue nil
          set_dev(log || @filename)
        elsif log
          set_dev log
        end
      end
      self
    end

    private

    def set_dev log
      if log.respond_to?(:write) && log.respond_to?(:close)
        @dev = log
        @filename = log.path if log.respond_to?(:path) && (::File === log)
        @dev.binmode if @binmode && @dev.respond_to?(:binmode)
      else
        @dev = open_logfile log
        @dev.sync = true
        @dev.binmode if @binmode
        @filename = log
      end
    end

    def open_logfile filename
      if ::File.exist? filename
        ::File.open filename, 'a'
      else
        create_logfile filename
      end
    end

    def create_logfile filename
      logdev = ::File.open filename, 'a'
      logdev.sync = true
      logdev.binmode if @binmode
      add_log_header logdev
      logdev
    end

    def add_log_header file
      file.write format("# Logfile created on %s by %s\n", ::Time.now.to_s, ProgName) if file.size == 0
    end

    def check_shift_log
      if ::Integer === @shift_age
        # Note: always returns false if '0'
        shift_log_age if @filename && (@shift_age > 0) && (@dev.stat.size > @shift_size)
      else
        now = ::Time.now
        if now >= @next_rotate_time
          @next_rotate_time = next_rotate_time now, @shift_age
          shift_log_period(previous_period_end now, @shift_age)
        end
      end
    end

    def shift_log_age
      (@shift_age - 3).downto 0 do |i|
        ::File.rename %(#{@filename}.#{i}), %(#{@filename}.#{i + 1}) if ::File.exist? %(#{@filename}.#{i})
      end
      @dev.close rescue nil
      ::File.rename @filename, %(#{@filename}.0)
      @dev = create_logfile @filename
      true
    end

    def shift_log_period period_end
      suffix = period_end.strftime @shift_period_suffix
      age_file = %(#{@filename}.#{suffix})
      if ::File.exist? age_file
        idx = 0
        idx += 1 while ::File.exist?(age_file = %(#{@filename}.#{suffix}.#{idx}))
      end
      @dev.close rescue nil
      ::File.rename @filename, age_file
      @dev = create_logfile @filename
      true
    end
  end

  SEV_LABEL = %w(DEBUG INFO WARN ERROR FATAL ANY).freeze

  attr_accessor :progname
  attr_accessor :formatter

  def initialize logdev, shift_age = 0, shift_size = 1048576, level: DEBUG, progname: nil, formatter: nil,
      datetime_format: nil, binmode: false, shift_period_suffix: '%Y%m%d', reraise_write_errors: []
    self.level = level
    self.progname = progname
    @default_formatter = Formatter.new
    self.datetime_format = datetime_format
    self.formatter = formatter
    @logdev = nil
    @level_override = {}
    if logdev && logdev != ::File::NULL
      @logdev = LogDevice.new logdev, shift_age: shift_age, shift_size: shift_size,
        shift_period_suffix: shift_period_suffix, binmode: binmode, reraise_write_errors: reraise_write_errors
    end
  end

  def level
    level_override[level_key] || @level
  end

  def level= severity
    @level = Severity.coerce severity
  end

  def with_level severity
    prev, level_override[level_key] = level, Severity.coerce(severity)
    begin
      yield
    ensure
      if prev
        level_override[level_key] = prev
      else
        level_override.delete level_key
      end
    end
  end

  alias sev_threshold level
  alias sev_threshold= level=

  def datetime_format= datetime_format
    @default_formatter.datetime_format = datetime_format
  end

  def datetime_format
    @default_formatter.datetime_format
  end

  def debug?; level <= DEBUG; end
  def debug!; self.level = DEBUG; end
  def info?; level <= INFO; end
  def info!; self.level = INFO; end
  def warn?; level <= WARN; end
  def warn!; self.level = WARN; end
  def error?; level <= ERROR; end
  def error!; self.level = ERROR; end
  def fatal?; level <= FATAL; end
  def fatal!; self.level = FATAL; end

  def reopen logdev = nil, shift_age = nil, shift_size = nil, shift_period_suffix: nil, binmode: nil
    @logdev&.reopen logdev
    self
  end

  def add severity, message = nil, progname = nil
    severity ||= UNKNOWN
    return true if @logdev.nil? || severity < level
    progname = @progname if progname.nil?
    if message.nil?
      if block_given?
        message = yield
      else
        message = progname
        progname = @progname
      end
    end
    @logdev.write(format_message (format_severity severity), ::Time.now, progname, message)
    true
  end
  alias log add

  def << msg
    @logdev&.write msg
  end

  def debug progname = nil, &block
    add DEBUG, nil, progname, &block
  end

  def info progname = nil, &block
    add INFO, nil, progname, &block
  end

  def warn progname = nil, &block
    add WARN, nil, progname, &block
  end

  def error progname = nil, &block
    add ERROR, nil, progname, &block
  end

  def fatal progname = nil, &block
    add FATAL, nil, progname, &block
  end

  def unknown progname = nil, &block
    add UNKNOWN, nil, progname, &block
  end

  def close
    @logdev&.close
  end

  private

  def level_override
    @level_override ||= {}
  end

  def level_key
    ::Fiber.current
  end

  def format_severity severity
    SEV_LABEL[severity] || 'ANY'
  end

  def format_message severity, datetime, progname, msg
    (@formatter || @default_formatter).call severity, datetime, progname, msg
  end
end
