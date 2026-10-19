# frozen_string_literal: true

# Regenerates testdata/golden with Asciidoctor running on MRI Ruby, using the
# vendored Asciidoctor source. Run from anywhere: ruby scripts/gen-goldens.rb
#
# Writes, for each manifest entry <path> converted with <backend>:
#   testdata/golden/<path>.<backend>.out   converted output
#   testdata/golden/<path>.<backend>.log   log messages (only when non-empty)
# and testdata/golden/MANIFEST listing "<path> <backend>" per line.

abort 'gen-goldens.rb must run on MRI (the oracle)' unless RUBY_ENGINE == 'ruby'

repo = File.expand_path '..', __dir__
$LOAD_PATH.unshift File.join repo, 'rubysrc/asciidoctor/lib'
require 'asciidoctor'
require 'fileutils'
require File.join repo, 'testdata/corpus/convert.rb'

golden = File.join repo, 'testdata/golden'
FileUtils.rm_rf golden
manifest = CorpusConvert.manifest repo
manifest.each do |rel, backend|
  output, log = CorpusConvert.convert repo, rel, backend
  base = File.join golden, %(#{rel}.#{backend})
  FileUtils.mkdir_p File.dirname base
  File.write %(#{base}.out), output
  File.write %(#{base}.log), log unless log.empty?
end
File.write (File.join golden, 'MANIFEST'), manifest.map {|entry| %(#{entry.join ' '}\n) }.join
puts %(wrote #{manifest.size} goldens with Asciidoctor #{Asciidoctor::VERSION} on #{RUBY_DESCRIPTION})
