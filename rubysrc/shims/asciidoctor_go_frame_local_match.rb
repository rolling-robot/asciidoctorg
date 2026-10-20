# frozen_string_literal: true

# In Ruby, $~ (and $1, $2, ...) belongs to the method frame: a regexp match in
# a called method does not change the caller's. rbgo keeps a single last
# match for the whole interpreter, so a call made between a match and the use
# of its groups (Asciidoctor does this, e.g. `if Rx =~ s; x = sub_attributes $1;
# y = $2`) reads the callee's match instead. This wraps every method that
# Asciidoctor defines so the caller's $~ is restored when it returns, which
# gives Asciidoctor's code the frame-local semantics it is written for.
#
# Only a method that can itself set $~ needs the wrapper: one that does not
# leaves its caller's $~ alone, and its callees restore theirs. So a method is
# wrapped unless its source is found and has none of the operations that set
# $~ (matching, substitution, splitting, indexing, case/when, regexp
# literals and Rx constants). Attribute accessors (no source location) do no
# matching and are not wrapped.
module AsciidoctorGoFrameLocalMatch
  WRAPPED = {}
  WRAPPED_LOCATIONS = []

  # Source text that may set $~; deliberately broad (a false positive only
  # costs a wrapper).
  MATCHING_RX = ::Regexp.new '=~|!~|\bmatch\??\b|\b(?:g?sub|scan|split|slice|r?partition|r?index|start_with\?|end_with\?)\b|' \
    '===|\$~|\$[1-9`\'&+]|last_match|Rx\b|%r\{|(?:^|[\s(,=\[!])/(?![\s=])|\bwhen\b|\bcase\b'
  ONE_LINE_END_RX = ::Regexp.new '\bend\s*$'
  ENDLESS_DEF_RX = ::Regexp.new '\bdef\s+[\w.?!]+(?:\([^)]*\))?\s*=[^=~>]'
  IDENT_CHAR_RX = ::Regexp.new '[\w?!]'

  SOURCE_LINES = ::Hash.new {|cache, file| cache[file] = (::File.readlines file rescue []) }

  # The source of method, found by its definition line and the end at the
  # same indentation (Asciidoctor's layout), or nil if it cannot be found.
  def self.method_source method
    file, line = method.source_location
    lines = SOURCE_LINES[file]
    return unless (first = lines[line - 1]) && (first.include? 'def ')
    return first if (first.match? ONE_LINE_END_RX) || (first.match? ENDLESS_DEF_RX)
    indent = first[/\A */]
    body = [first]
    lines[line..].each do |l|
      body << l
      return body.join if l.start_with?(%(#{indent}end)) && !(l[indent.length + 3]&.match? IDENT_CHAR_RX)
    end
    nil
  end

  def self.needs_wrapper? method
    file, line = method.source_location
    if @needed && (file.start_with? @lib_prefix)
      return @needed.include? %(#{file.delete_prefix @lib_prefix}:#{line})
    end
    !(source = method_source method) || (source.match? MATCHING_RX)
  end

  # Scanning the sources takes most of a second in rbgo, so the locations of
  # the methods that need a wrapper are cached in a file next to this one,
  # keyed by the size and modification time of each Asciidoctor source file.
  # Without a usable cache the sources are scanned.
  CACHE_FILE = ::File.join __dir__, '.asciidoctor_go_frame_local_cache'

  def self.load_cache lib_dir
    @lib_prefix = %(#{lib_dir}/)
    @cache_key = (::Dir.glob '**/*.rb', base: lib_dir).sort.map do |rel|
      path = ::File.join lib_dir, rel
      %(#{rel} #{::File.size path} #{(::File.mtime path).to_i})
    end.join ' '
    lines = ::File.readlines CACHE_FILE, chomp: true
    @needed = lines.drop(1).to_h {|loc| [loc, true] } if lines[0] == @cache_key
  rescue ::SystemCallError, ::IOError
    nil
  end

  def self.save_cache wrapped
    return if @needed
    tmp = %(#{CACHE_FILE}.#{::Process.pid}.#{rand 1_000_000})
    ::File.write tmp, ([@cache_key] + wrapped).join("\n")
    ::File.rename tmp, CACHE_FILE
  rescue ::SystemCallError, ::IOError
    nil
  end

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
      next unless needs_wrapper? method
      file, line = method.source_location
      WRAPPED_LOCATIONS << %(#{file.delete_prefix @lib_prefix.to_s}:#{line}) if @lib_prefix && (file.start_with? @lib_prefix)
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
  AsciidoctorGoFrameLocalMatch.load_cache ::Asciidoctor::LIB_DIR
  AsciidoctorGoFrameLocalMatch.wrap_namespace ::Asciidoctor
  AsciidoctorGoFrameLocalMatch.save_cache AsciidoctorGoFrameLocalMatch::WRAPPED_LOCATIONS
end
