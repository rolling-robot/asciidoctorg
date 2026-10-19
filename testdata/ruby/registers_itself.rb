# frozen_string_literal: true

# Requiring this file registers an extension group, as Ruby gems do.
Asciidoctor::Extensions.register :self_registered do
  inline_macro do
    named :sum
    process do |parent, target, attrs|
      create_inline parent, :quoted, (target.split('+').map(&:to_i).sum).to_s, type: :strong
    end
  end
end
