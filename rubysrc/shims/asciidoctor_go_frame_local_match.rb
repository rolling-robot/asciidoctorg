# frozen_string_literal: true

# In Ruby, $~ (and $1, $2, ...) belongs to the method frame: a regexp match in
# a called method does not change the caller's. rbgo keeps a single last
# match for the whole interpreter, so a call made between a match and the use
# of its groups (Asciidoctor does this, e.g. `if Rx =~ s; x = sub_attributes $1;
# y = $2`) reads the callee's match instead. This wraps every method that
# Asciidoctor defines so the caller's $~ is restored when it returns, which
# gives Asciidoctor's code the frame-local semantics it is written for.
# Attribute accessors (no source location) do no matching and are not wrapped.
module AsciidoctorGoFrameLocalMatch
  WRAPPED = {}

  def self.needed?
    probe = ::Module.new
    probe.define_singleton_method(:inner) { 'q' =~ /q/ }
    /(a)/ =~ 'a'
    probe.inner
    $1 != 'a'
  end

  def self.load_asciidoctor lib_dir
    (::Dir.glob '**/*.rb', base: ::File.join(lib_dir, 'asciidoctor')).sort.each do |rel|
      # rouge_ext requires the rouge gem; js is for Asciidoctor.js
      next if rel == 'rouge_ext.rb' || rel.start_with?('js')
      require ::File.join(lib_dir, 'asciidoctor', rel.chomp('.rb'))
    end
  end

  def self.wrap_namespace root
    seen = {}
    queue = [root]
    until queue.empty?
      mod = queue.shift
      next if seen[mod]
      seen[mod] = true
      wrap_methods mod
      wrap_methods mod.singleton_class
      mod.constants(false).each do |name|
        next if mod.autoload?(name)
        value = mod.const_get name, false rescue next
        queue << value if ::Module === value && value.name&.start_with?('Asciidoctor')
      end
    end
  end

  def self.wrap_methods target
    return if WRAPPED[target]
    prefix = %(__agox_frame_#{WRAPPED.size}_)
    WRAPPED[target] = true
    # rbgo keeps a module's module_function mode on after its body, so a
    # method defined now would also replace the module's singleton copy;
    # a bare `public` ends the mode (each wrapper's visibility is set below)
    target.send :public
    visibility = {}
    target.public_instance_methods(false).each {|n| visibility[n] = :public }
    target.protected_instance_methods(false).each {|n| visibility[n] = :protected }
    target.private_instance_methods(false).each {|n| visibility[n] = :private }
    aliases = []
    count = 0
    visibility.each do |name, vis|
      next if name.to_s.start_with? '__agox_frame_'
      method = target.instance_method name
      next unless method.source_location
      # an alias is re-pointed at the wrapper of the method it names
      if method.original_name != name && visibility.key?(method.original_name)
        aliases << [name, method.original_name, vis]
        next
      end
      # The alias name is unique to target, so a wrapper reached through
      # super from a subclass calls this class's method, not the subclass's.
      # a plain identifier, so the wrapper can call it without send (rbgo's
      # send binds a trailing positional Hash as keywords)
      original = :"#{prefix}m#{count += 1}"
      target.send :alias_method, original, name
      target.module_eval (wrapper_source name, original, method.parameters), __FILE__, __LINE__
      target.send vis, name
      target.send :private, original
    end
    aliases.each do |name, primary, vis|
      target.send :alias_method, name, primary
      target.send vis, name
    end
  end

  # Marks an optional keyword the caller did not pass.
  UNSET = ::Object.new.freeze

  # Source of a wrapper with the same signature (and so the same arity and
  # keyword binding) as the original: required parameters before and after
  # the rest parameter keep their positions, optional ones are collected with
  # the rest, keywords are declared as the original declares them (an optional
  # keyword is passed on only when given), and ** only if the original has it.
  def self.wrapper_source name, original, parameters
    pre, post, keyreq, key = [], [], [], []
    rest = kwrest = false
    parameters.each do |type, param|
      case type
      when :req then (rest ? post : pre) << %(__p#{pre.size + post.size})
      when :opt, :rest then rest = true
      when :keyreq then keyreq << param
      when :key then key << param
      when :keyrest then kwrest = true
      end
    end
    signature = pre.dup
    signature << '*__rest' if rest
    signature.concat post
    signature.concat(keyreq.map {|k| %(#{k}:) })
    signature.concat(key.map {|k| %(#{k}: ::AsciidoctorGoFrameLocalMatch::UNSET) })
    signature << '**__kw' if kwrest
    signature << '&__blk'
    keywords = ['__keywords = {}']
    keywords << '__keywords.update __kw' if kwrest
    keyreq.each {|k| keywords << %(__keywords[#{k.inspect}] = #{k}) }
    key.each {|k| keywords << %(__keywords[#{k.inspect}] = #{k} unless ::AsciidoctorGoFrameLocalMatch::UNSET.equal? #{k}) }
    call = pre.dup
    call << '*__rest' if rest
    call.concat post
    call << '**__keywords' if kwrest || keyreq.any? || key.any?
    call << '&__blk'
    <<~RUBY
      def #{name}(#{signature.join ', '})
        __agox_saved_match = $~
        begin
          #{keywords.join "\n    "}
          #{original}(#{call.join ', '})
        ensure
          $~ = __agox_saved_match
        end
      end
    RUBY
  end
end

if AsciidoctorGoFrameLocalMatch.needed?
  AsciidoctorGoFrameLocalMatch.load_asciidoctor ::Asciidoctor::LIB_DIR
  AsciidoctorGoFrameLocalMatch.wrap_namespace ::Asciidoctor
end
