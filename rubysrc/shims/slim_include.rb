# frozen_string_literal: true

# slim/include for rbgo. The include directive (`include name`) is expanded at
# compile time by the Tilt shim's SlimTemplate, which checks for this module.
require 'slim'

module Slim
  module Include; end

  class Engine
    # Slim::Engine.define_options declares engine options; the go-ruby-slim
    # compiler accepts any options, so the declaration only records them.
    def self.define_options **options
      (@defined_options ||= {}).update options
    end

    def self.defined_options
      @defined_options || {}
    end
  end unless Engine.respond_to? :define_options
end
