# frozen_string_literal: true

# Cancellation checkpoints. rbgo schedules Ruby threads cooperatively and has
# no way to interrupt running code from Go, so a conversion is cancelled where
# it checks: AsciidoctorGo::Bridge.checkpoint (defined in Go) raises
# AsciidoctorGo::Canceled once the context of the call in progress is done.
# It is called before each block is parsed and before each block is converted;
# every Go callback is a checkpoint too. Work inside one block (a single
# paragraph's substitutions, say) runs to its end.
module AsciidoctorGo
  # Canceled descends from Exception, not StandardError, so the bare rescue
  # clauses in Asciidoctor (and in extensions) let it through.
  class Canceled < ::Exception; end
end

class << ::Asciidoctor::Parser
  alias_method :__agox_cancel_next_block, :next_block

  def next_block reader, parent, attributes = {}, options = {}
    ::AsciidoctorGo::Bridge.checkpoint
    __agox_cancel_next_block reader, parent, attributes, options
  end
end

class ::Asciidoctor::AbstractBlock
  alias_method :__agox_cancel_convert, :convert

  # List#convert and Document#convert reach this through super or by
  # converting their blocks.
  def convert
    ::AsciidoctorGo::Bridge.checkpoint
    __agox_cancel_convert
  end
end
