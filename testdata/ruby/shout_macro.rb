# frozen_string_literal: true

# A block macro written in Ruby, registered from Go by class name.
class ShoutBlockMacro < Asciidoctor::Extensions::BlockMacroProcessor
  use_dsl
  named :shout

  def process parent, target, attrs
    create_paragraph parent, %(#{target.upcase}!), { 'role' => attrs['role'] }
  end
end
