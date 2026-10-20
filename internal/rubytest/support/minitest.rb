# frozen_string_literal: true

# A minitest-compatible test core for running Asciidoctor's test suite under
# rbgo. rbgo's native Minitest finds only classes declared with `class X <`
# syntax, while the suite's helper creates test classes dynamically
# (Class.new + const_set), so the suite runs on this implementation instead.
# The Go harness enumerates the tests and runs each one as a Go subtest.

Object.send :remove_const, :Minitest if defined? ::Minitest
Object.send :remove_const, :MiniTest if defined? ::MiniTest

module Minitest
  # Raised by a failed assertion.
  class Assertion < ::Exception
    def result_label
      'Failure'
    end
  end

  # Raised by #skip.
  class Skip < Assertion
    def result_label
      'Skipped'
    end
  end

  module Assertions
    UNDEFINED = ::Object.new

    def assertions
      @assertions ||= 0
    end

    attr_writer :assertions

    def mu_pp obj
      obj.inspect
    end

    def message msg = nil, ending = nil, &default
      proc do
        custom = msg.respond_to?(:call) ? msg.call : msg
        text = default ? default.call : ''
        custom = custom.to_s
        custom.empty? ? %(#{text}#{ending}) : %(#{custom}.\n#{text}#{ending})
      end
    end

    def assert test, msg = nil
      self.assertions += 1
      unless test
        msg ||= 'Expected false to be truthy.'
        msg = msg.call if ::Proc === msg
        raise Assertion, msg
      end
      true
    end

    def refute test, msg = nil
      msg ||= message { %(Expected #{mu_pp test} to not be truthy) }
      assert !test, msg
    end

    def flunk msg = nil
      raise Assertion, (msg || 'Epic Fail!')
    end

    def pass _msg = nil
      assert true
    end

    def skip msg = nil, _bt = caller
      raise Skip, (msg || 'Skipped, no message given')
    end

    def assert_equal exp, act, msg = nil
      msg = message(msg, '') { %(Expected: #{mu_pp exp}\n  Actual: #{mu_pp act}) }
      assert exp == act, msg
    end

    def refute_equal exp, act, msg = nil
      msg = message(msg) { %(Expected #{mu_pp act} to not be equal to #{mu_pp exp}) }
      refute exp == act, msg
    end

    def assert_nil obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp obj} to be nil) }
      assert obj.nil?, msg
    end

    def refute_nil obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp obj} to not be nil) }
      refute obj.nil?, msg
    end

    def assert_empty obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp obj} to be empty) }
      assert_respond_to obj, :empty?
      assert obj.empty?, msg
    end

    def refute_empty obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp obj} to not be empty) }
      assert_respond_to obj, :empty?
      refute obj.empty?, msg
    end

    def assert_includes collection, obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp collection} to include #{mu_pp obj}) }
      assert_respond_to collection, :include?
      assert collection.include?(obj), msg
    end

    def refute_includes collection, obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp collection} to not include #{mu_pp obj}) }
      assert_respond_to collection, :include?
      refute collection.include?(obj), msg
    end

    def assert_match matcher, obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp matcher} to match #{mu_pp obj}) }
      assert_respond_to matcher, :=~
      matcher = ::Regexp.new ::Regexp.escape matcher if ::String === matcher
      assert (last_match = matcher =~ obj), msg
      last_match
    end

    def refute_match matcher, obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp matcher} to not match #{mu_pp obj}) }
      assert_respond_to matcher, :=~
      matcher = ::Regexp.new ::Regexp.escape matcher if ::String === matcher
      refute matcher =~ obj, msg
    end

    def assert_kind_of cls, obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp obj} to be a kind of #{cls}, not #{obj.class}) }
      assert obj.kind_of?(cls), msg
    end

    def refute_kind_of cls, obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp obj} to not be a kind of #{cls}) }
      refute obj.kind_of?(cls), msg
    end

    def assert_instance_of cls, obj, msg = nil
      msg = message(msg) { %(Expected #{mu_pp obj} to be an instance of #{cls}, not #{obj.class}) }
      assert obj.instance_of?(cls), msg
    end

    def assert_same exp, act, msg = nil
      msg = message(msg) { %(Expected #{mu_pp act} (oid=#{act.object_id}) to be the same as #{mu_pp exp} (oid=#{exp.object_id})) }
      assert exp.equal?(act), msg
    end

    def refute_same exp, act, msg = nil
      msg = message(msg) { %(Expected #{mu_pp act} to not be the same as #{mu_pp exp}) }
      refute exp.equal?(act), msg
    end

    def assert_respond_to obj, meth, msg = nil, include_all: false
      msg = message(msg) { %(Expected #{mu_pp obj} (#{obj.class}) to respond to ##{meth}) }
      assert obj.respond_to?(meth, include_all), msg
    end

    def refute_respond_to obj, meth, msg = nil, include_all: false
      msg = message(msg) { %(Expected #{mu_pp obj} to not respond to #{meth}) }
      refute obj.respond_to?(meth, include_all), msg
    end

    def assert_path_exists path, msg = nil
      msg = message(msg) { %(Expected path '#{path}' to exist) }
      assert ::File.exist?(path), msg
    end

    def refute_path_exists path, msg = nil
      msg = message(msg) { %(Expected path '#{path}' to not exist) }
      refute ::File.exist?(path), msg
    end

    def assert_operator o1, op, o2 = UNDEFINED, msg = nil
      return assert_predicate o1, op, msg if UNDEFINED.equal? o2
      msg = message(msg) { %(Expected #{mu_pp o1} to be #{op} #{mu_pp o2}) }
      assert o1.__send__(op, o2), msg
    end

    def assert_predicate o1, op, msg = nil
      msg = message(msg) { %(Expected #{mu_pp o1} to be #{op}) }
      assert o1.__send__(op), msg
    end

    def assert_raises *exp
      msg = ::String === exp.last ? exp.pop : nil
      exp << ::StandardError if exp.empty?
      begin
        yield
      rescue *exp => e
        pass
        return e
      rescue Assertion
        raise
      rescue ::Exception => e
        flunk %(#{msg ? %(#{msg}.\n) : ''}#{mu_pp exp} exception expected, not #{e.class}: #{e.message})
      end
      flunk %(#{msg ? %(#{msg}.\n) : ''}#{mu_pp exp} expected but nothing was raised.)
    end

    def assert_output stdout = nil, stderr = nil
      out, err = capture_io { yield }
      assert_equal stdout, out if ::String === stdout
      assert_match stdout, out if ::Regexp === stdout
      assert_equal stderr, err if ::String === stderr
      assert_match stderr, err if ::Regexp === stderr
    end

    def capture_io
      require 'stringio'
      captured_stdout, captured_stderr = ::StringIO.new, ::StringIO.new
      orig_stdout, orig_stderr = $stdout, $stderr
      $stdout, $stderr = captured_stdout, captured_stderr
      begin
        yield
      ensure
        $stdout, $stderr = orig_stdout, orig_stderr
      end
      [captured_stdout.string, captured_stderr.string]
    end
  end

  # Platform predicates, available on test classes and instances.
  module Guard
    def jruby? platform = ::RUBY_PLATFORM
      platform == 'java'
    end

    def mri? platform = ::RUBY_DESCRIPTION
      platform.start_with? 'ruby'
    end

    def windows? platform = ::RUBY_PLATFORM
      /mswin|mingw/.match? platform
    end

    def osx? platform = ::RUBY_PLATFORM
      platform.include? 'darwin'
    end

    def rubinius? platform = defined?(::RUBY_ENGINE) && ::RUBY_ENGINE
      platform == 'rbx'
    end

    def maglev? platform = defined?(::RUBY_ENGINE) && ::RUBY_ENGINE
      platform == 'maglev'
    end
  end

  class Test
    include Assertions
    include Guard
    extend Guard

    @test_classes = []

    class << self
      # Every class that inherits from Test, in definition order.
      def test_classes
        Test.instance_variable_get :@test_classes
      end

      # NOTE no super: rbgo cannot resolve super from a class method to
      # Class#inherited (see docs/rbgo-gaps.md)
      def inherited subclass
        Test.instance_variable_get(:@test_classes) << subclass
      end

      # The test methods the class runs (including inherited ones, as minitest
      # does; the suite's helper undefines inherited tests in contexts).
      def test_methods
        public_instance_methods(true).map(&:to_s).select {|name| name.start_with? 'test_' }.sort
      end
    end

    attr_reader :name

    def initialize name
      @name = name
      @assertions = 0
    end

    def setup; end

    def teardown; end

    # Runs one test; returns [status, message] where status is :pass, :fail,
    # :error or :skip.
    def run_test
      result = [:pass, nil]
      begin
        setup
        __send__ @name
      rescue Skip => e
        result = [:skip, e.message]
      rescue Assertion => e
        result = [:fail, %(#{e.message}\n#{Test.trim_backtrace e})]
      rescue ::Exception => e
        result = [:error, %(#{e.class}: #{e.message}\n#{Test.trim_backtrace e})]
      ensure
        begin
          teardown
        rescue ::Exception => e
          result = [:error, %(teardown: #{e.class}: #{e.message}\n#{Test.trim_backtrace e})] if result[0] == :pass
        end
      end
      result
    end

    def self.trim_backtrace e
      ((e.backtrace || []).reject {|l| l.include? 'support/minitest.rb' }.first 12).join "\n"
    end
  end

  # Minitest::Unit::TestCase is the minitest 4 name.
  module Unit
    TestCase = Test
  end

  def self.autorun; end
end

MiniTest = Minitest
