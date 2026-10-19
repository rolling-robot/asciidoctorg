# frozen_string_literal: true

# Converts corpus documents with fixed, reproducible options. Shared by the MRI
# golden generator (scripts/gen-goldens.rb) and the Go conformance test, so
# both sides convert identically.
module CorpusConvert
  ATTRIBUTES = {
    'linkcss' => '',
    'docdate' => '2020-01-01',
    'doctime' => '00:00:00 +0000',
    'docdatetime' => '2020-01-01 00:00:00 +0000',
    'docyear' => '2020',
    'localdate' => '2020-01-01',
    'localtime' => '00:00:00 +0000',
    'localdatetime' => '2020-01-01 00:00:00 +0000',
    'localyear' => '2020',
  }.freeze

  # Globs, relative to the repository root, selecting the corpus documents: the
  # hand-written fixtures and the documentation and fixtures of the asciidoctor
  # and asciidoctorj submodules.
  SOURCES = %w(
    testdata/corpus/**/*.adoc
    rubysrc/asciidoctor/*.adoc
    rubysrc/asciidoctor/{docs,man,test/fixtures,data/reference,benchmark/sample-data}/**/*.adoc
    third_party/asciidoctorj/*.adoc
    third_party/asciidoctorj/docs/**/*.adoc
    third_party/asciidoctorj/asciidoctorj-documentation/src/**/*.adoc
    third_party/asciidoctorj/asciidoctorj-core/src/test/resources/**/*.adoc
    third_party/asciidoctorj/asciidoctorj-distribution/src/test/resources/**/*.adoc
    third_party/asciidoctorj/asciidoctorj-cli/src/**/*.adoc
  ).freeze

  # Documents converted with additional backends besides html5.
  EXTRA_BACKENDS = {
    'docbook5' => %r{\A(testdata/corpus/complex/[^/]+|rubysrc/asciidoctor/test/fixtures/[^/]+)\.adoc\z},
    'manpage' => %r{\A(testdata/corpus/complex/manpage-doc|rubysrc/asciidoctor/man/asciidoctor)\.adoc\z},
  }.freeze

  # Returns [[relative path, backend], ...] for every corpus document below the
  # repository root.
  def self.manifest root
    (SOURCES.flat_map {|glob| Dir.glob glob, base: root }).uniq.sort.flat_map do |rel|
      [[rel, 'html5']] + (EXTRA_BACKENDS.select {|_, rx| rx.match? rel }.map {|backend, _| [rel, backend] })
    end
  end

  # Converts one document. Returns [output, log], with the absolute repository
  # root replaced by {repo} so results do not depend on the checkout location.
  def self.convert root, rel, backend
    logger = Asciidoctor::MemoryLogger.new
    old_logger, Asciidoctor::LoggerManager.logger = Asciidoctor::LoggerManager.logger, logger
    begin
      output = Asciidoctor.convert_file (File.join root, rel), safe: :safe, backend: backend, standalone: true,
        to_file: false, attributes: ATTRIBUTES
    rescue ::StandardError => e
      output = %(ERROR #{e.class}: #{e.message})
    ensure
      Asciidoctor::LoggerManager.logger = old_logger
    end
    log = logger.messages.map do |entry|
      message = entry[:message]
      %(#{entry[:severity]}: #{::String === message ? message : message.inspect}\n)
    end.join
    [output.to_s, log].map {|text| text.gsub root, '{repo}' }
  end
end
