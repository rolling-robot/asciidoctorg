# frozen_string_literal: true

# Loaded into a fresh interpreter before a test file of Asciidoctor's suite.
# The Go harness defines the native Nokogiri classes and sets
# $agox_support_dir and $agox_lib_dir first.

# Test-only libraries provided here instead of by rbgo or gems.
module AsciidoctorGoTestLibraries
  PROVIDED = {
    'minitest' => 'minitest.rb',
    'minitest/autorun' => 'minitest.rb',
    'nokogiri' => 'nokogiri.rb',
  }.freeze
  # Libraries the helper requires unconditionally that are not available;
  # tests that depend on them are excluded by the Go harness.
  IGNORED = %w(rouge simplecov).freeze
  LOADED = {}

  private

  def require name
    name = name.to_s
    if (file = PROVIDED[name])
      return false if LOADED[file]
      LOADED[file] = true
      load ::File.join(::AsciidoctorGoTest::SUPPORT_DIR, file)
      true
    elsif IGNORED.include? name
      false
    else
      super
    end
  end
end
Object.prepend AsciidoctorGoTestLibraries

module AsciidoctorGoTest
  SUPPORT_DIR = $agox_support_dir

  # Lists [class name, test method] pairs, in definition order.
  def self.tests
    Minitest::Test.test_classes.flat_map do |klass|
      next [] unless klass.name
      klass.test_methods.map {|m| [klass.name, m] }
    end
  end

  def self.run class_name, method
    status, message = (::Object.const_get class_name).new(method).run_test
    [status.to_s, message.to_s]
  end
end

require 'socket'
# Socket.ip_address_list is missing from rbgo; the suite's web server uses it
# to find a local IPv4 address.
unless Socket.respond_to? :ip_address_list
  def Socket.ip_address_list
    AsciidoctorGoTestHost.interface_addresses.map {|address| Addrinfo.ip address }
  end
end

# rbgo's Thread#exit does not interrupt a thread blocked in TCPServer#accept
# (and accept_nonblock and IO.select on sockets are unavailable), so the
# suite's web server thread could never be stopped. Exiting a thread that is
# accepting first closes its server, which wakes accept; the thread then
# exits as it would have.
class TCPServer
  alias_method :__agox_native_accept, :accept

  def accept
    ::Thread.current[:agox_accepting] = self
    __agox_native_accept
  rescue ::IOError, ::SystemCallError
    ::Thread.current[:agox_exiting] ? ::Thread.current.exit : raise
  ensure
    ::Thread.current[:agox_accepting] = nil
  end
end

module AsciidoctorGoThreadExitWakesAccept
  def exit
    self[:agox_exiting] = true
    if (server = self[:agox_accepting])
      server.close rescue nil
    end
    super
  end

  alias kill exit
  alias terminate exit
end
Thread.prepend AsciidoctorGoThreadExitWakesAccept

ENV['ASCIIDOCTOR_LIB_DIR'] = $agox_lib_dir
require 'minitest'
require 'nokogiri'
