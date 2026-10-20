# Compatibility report

How asciidoctor-go compares with Asciidoctor on Ruby (the reference) and with
AsciidoctorJ (the design model). Test figures are from a run of commit
`e3b6c9a` (2026-10-20) with Asciidoctor 2.1.0.alpha.0 (`59c7287d`), CodeRay
1.1.3, rbgo v0.8.0 and go-rouge v0.3.0; the oracle is MRI Ruby 3.4.10.

## Summary

- Asciidoctor itself runs unmodified, apart from 13 exact-match source
  patches (and 5 to CodeRay), each equivalent on MRI
  (`internal/runtime/patch.go`).
- Every conversion checked produces the same bytes as Asciidoctor on MRI.
- Asciidoctor's own test suite passes, except 13 tests that cannot apply to
  an embedded runtime.
- AsciidoctorJ's document-processing tests pass against the Go API, except
  one (Rouge on Java source).
- Not available: Pygments, the `open-uri-cached` gem, Rouge themes other than
  three and Rouge lexers other than 16, and Ruby gems that are not embedded.
- Conversion is about 10× slower than on MRI.
- Ruby extensions run on rbgo, which still differs from MRI in a few ways that
  can affect them (see [Ruby code](#ruby-code)).

## Test results

`go test ./...` runs all of these; `go test -v` lists every case.

| Suite | Package | Pass | Skip | Fail |
|---|---|---:|---:|---:|
| Corpus against MRI goldens (output and log messages) | `internal/conformance` | 324 conversions | 0 | 0 |
| Corpus through the public API | `internal/apitest` | 324 conversions | 0 | 0 |
| Asciidoctor's Ruby test suite | `internal/rubytest` | 2864 | 13 | 0 |
| AsciidoctorJ tests, ported | `internal/asciidoctorj` | 284 cases | 1 | 0 |
| API tests | `internal/apitest` | 388 cases | 0 | 0 |
| Runnable examples | root package | 12 | 0 | 0 |

The corpus is the AsciiDoc documents of Asciidoctor's and AsciidoctorJ's test
fixtures and of `testdata/corpus`: 324 conversions (287 with the html5
backend, 35 with docbook5, 2 with manpage), among them CodeRay-highlighted
listings in 20 languages.

### Asciidoctor's test suite, by file

| Test file | Pass | Skip |
|---|---:|---:|
| `api_test` | 134 | |
| `attribute_list_test` | 33 | |
| `attributes_test` | 158 | |
| `blocks_test` | 292 | 2 |
| `converter_test` | 50 | |
| `document_test` | 157 | |
| `extensions_test` | 106 | |
| `helpers_test` | 23 | |
| `invoker_test` | 68 | 8 |
| `links_test` | 173 | |
| `lists_test` | 317 | |
| `logger_test` | 42 | 1 |
| `manpage_test` | 81 | |
| `options_test` | 46 | |
| `paragraphs_test` | 43 | |
| `parser_test` | 66 | |
| `paths_test` | 44 | |
| `preamble_test` | 12 | |
| `reader_test` | 223 | |
| `sections_test` | 229 | |
| `substitutions_test` | 335 | |
| `syntax_highlighter_test` | 65 | 2 |
| `tables_test` | 132 | |
| `text_test` | 35 | |
| **Total** | **2864** | **13** |

The skipped tests, with the reasons recorded in
`internal/rubytest/exclusions_test.go`:

| Tests | Reason |
|---|---|
| 9: 7 in `invoker_test`, 1 in `logger_test`, 1 in `syntax_highlighter_test` | They spawn the `ruby` executable to run `bin/asciidoctor` or a script; an embedded runtime has none. |
| 2 in `blocks_test` | They need the `open-uri-cached` gem (`cache-uri`). |
| 1 in `syntax_highlighter_test` | It uses Rouge's monokai theme, which go-rouge does not bundle. |
| 1 in `invoker_test` | It reads from a named pipe; rbgo holds its interpreter lock while opening a FIFO, so the writer never runs. |

The skipped AsciidoctorJ case is Rouge highlighting Java source: go-rouge
v0.3.0 has no Java lexer. The same test passes with CodeRay, and with Rouge on
Ruby source.

## Feature support

Compared with Asciidoctor 2.1 on Ruby.

| Area | Status |
|---|---|
| Parsing, substitutions, attributes, conditionals, includes, tables, lists, sections, cross references, footnotes, bibliography, admonitions, icons, STEM markup, docinfo | Same as Asciidoctor (byte-identical on the corpus and its test suite). |
| Backends | html5, xhtml5, docbook5, manpage; converters written in Go or Ruby. |
| Safe modes | All four; the API default is `SafeModeSecure`, as in Asciidoctor's API. |
| Remote content (`allow-uri-read`) | Supported, over `Net::HTTP` (HTTP and HTTPS). |
| `cache-uri` | Not available: the `open-uri-cached` gem is missing, so reading a URI with it set aborts, as on MRI without the gem. |
| `data-uri` | Supported (local and, with `allow-uri-read`, remote images). |
| Custom templates (`-T`, `TemplateDirs`) | ERB, Erubi, Haml and Slim (Haml and Slim compiled by go-ruby-haml and go-ruby-slim). Other Tilt engines are not available. |
| Source highlighting: CodeRay | Supported (CodeRay 1.1.3 embedded), all of its languages; output identical with MRI in the 20 languages tested. |
| Source highlighting: Rouge | Supported through go-rouge, with 16 lexers: plaintext, json, diff, go, sql, css, shell, console, yaml, python, javascript, html, markdown, ruby, php, tex. Themes: base16, github (the default), thankful_eyes. Other languages are shown unhighlighted. |
| Source highlighting: highlight.js, prettify | Supported (they highlight in the browser). |
| Source highlighting: Pygments | Not available (it needs Python); Asciidoctor warns and shows the source unhighlighted. |
| Other gems (asciidoctor-pdf, -epub3, -diagram, -revealjs, …) | Not available. Only the embedded libraries and pure-Ruby files on the load path that run under rbgo can be required. |
| Command line | `cmd/asciidoctor` runs Asciidoctor's own command line with all its options; in the cases checked, output, messages and exit codes match MRI's `bin/asciidoctor`. `--version` reports rbgo's Ruby version (3.4.1). |

## Ruby code

Asciidoctor runs as it does on MRI because the rbgo gaps it meets are shimmed
or patched. Ruby code of your own (extensions loaded with `LoadRubySource`,
`RequireLibrary` or `-r`) runs on the same rbgo, and these differences remain
for it (the numbers refer to [rbgo-gaps.md](rbgo-gaps.md)):

- `$~` and `$1`… are not frame-local (27): a method you call that matches a
  regexp replaces your last match. Asciidoctor's own methods are wrapped to
  restore it; yours are not, so read groups before calling other methods.
- `super` from a module extended into a class, from a class method to
  `Class`/`Module` instance methods, or from a module prepended to a singleton
  class does not work (18, 43).
- A regexp with a nested character class (`[a[bc]]`, `[a-z&&[^aeiou]]`)
  matches nothing (36); in `StringScanner`, octal escapes in a character class
  do not match (37).
- Copies (`dup`, `clone`) of `String` subclass instances are unusable (39),
  and the resulting error escapes `rescue` (41).
- Some `StringScanner` methods are missing: `captures`, `named_captures`,
  `size`, `values_at`, `scan_byte`, `scan_integer`, `fixed_anchor?` (40).
- `private_constant` constants are still listed by `constants` (13).
- Opening a FIFO for reading blocks every other thread (34).

The Rouge available to Ruby code is the subset of the gem's API that
Asciidoctor's Rouge adapter uses, backed by go-rouge; lexers or themes written
in Ruby cannot be added to it.

## AsciidoctorJ to Go

| AsciidoctorJ | asciidoctor-go |
|---|---|
| `Asciidoctor.Factory.create()` | `asciidoctor.New()` (and `Close`); `NewPool` for parallel conversion |
| `Factory.create(loadPaths)` | `New(WithLoadPath(...))` |
| `convert`, `convertFile`, `convertFiles`, `convertDirectory` | `Convert`, `ConvertFile`, `ConvertFiles`, `ConvertDirectory` |
| `convert(Reader, Writer, Options)` | `ConvertReader` |
| `convert(..., Document.class)` | `ConvertDocument`, `ConvertFileDocument` |
| `load`, `loadFile` | `Load`, `LoadFile` |
| `Options` / `OptionsBuilder` | `Options` struct; `Options.Extra` for any other option |
| `Attributes` / `AttributesBuilder` | `Attributes` map with chainable setters (`TableOfContents`, `SectionNumbers`, `Icons`, `LinkCSS`, …) |
| `AsciiDocDirectoryWalker`, `GlobDirectoryWalker` | same names |
| AST interfaces (`Document`, `Section`, `Block`, `List`, `Table`, `Cell`, `PhraseNode`, …) | same names, as Go types |
| `javaExtensionRegistry()` | `Extensions()` |
| `createGroup()` / `ExtensionGroup` | `Extensions().Group(name)` / `ExtensionGroup` |
| processor base classes and `create*` helpers | processor interfaces (with `Func` adapters) and `Create*` functions |
| `rubyExtensionRegistry().loadClass`, `requireLibrary` | `LoadRubySource`, `RequireLibrary`, then `RubyProcessor` |
| `javaConverterRegistry()`: `register`, `converters`, `unregisterAll` | `RegisterConverter`, `Converters`, `UnregisterAllConverters` |
| `@ConverterFor`, `setOutfileSuffix` | the backend names and `BackendTraits` passed to `RegisterConverter` |
| `syntaxHighlighterRegistry()` | `RegisterSyntaxHighlighter` |
| `registerLogHandler`, `unregisterLogHandler` | `RegisterLogHandler`, which returns the unregister function; `SetLogLevel` |
| `log(LogRecord)` in extensions and converters | `Asciidoctor.Log` (reach the instance with `Node.Asciidoctor()`) |
| Service-loaded `ExtensionRegistry`, `ConverterRegistry`, `LogHandler` | `RegisterSetup`, called from a package's `init` |
| `unwrap(AsciidoctorJRuby.class)`, JRuby runtime access | `Do` and `Node.Value` |
| `asciidoctorVersion()` | `AsciidoctorVersion` |
| `@Name` and other annotations, registration by class or class name | none: a processor is a value, named when registered |
| class loaders, `GEM_PATH`/`GEM_HOME` | none: the runtime loads only embedded libraries and the load path |

## Behaviour differences

Where the Go API behaves differently from AsciidoctorJ:

- **Log level.** Handlers receive WARN and above by default, as Asciidoctor's
  logger does; AsciidoctorJ also passes INFO (such as "possible invalid
  reference"). Call `SetLogLevel(SeverityInfo)` for the same.
- **`Attributes()` returns a copy**, while AsciidoctorJ's `getAttributes()` is
  a live map; change attributes with `SetAttr` and `RemoveAttr`. Block lists
  are changed with `SetBlock`, `InsertBlock` and `RemoveBlock`.
- **`ConvertFiles` and `ConvertDirectory`** return one result per file (`""`
  for a file written to disk); AsciidoctorJ returns only the strings.
- **Registration errors.** A nil processor, or a Go block or macro processor
  without a name, makes `ExtensionGroup.Register` fail; AsciidoctorJ throws
  when registering.
- **Log handlers cannot fail a conversion by returning an error**; one that
  panics aborts it, and the conversion returns the panic as its error.
- **`CompatModeDefault`** removes the `compat-mode` attribute (Asciidoctor
  enables compatibility mode whenever the attribute is present).
- **Standard input of the command line** is the `io.Reader` given to `RunCLI`.

## Performance

Converting Asciidoctor's README (503 lines, standalone HTML, after warm-up)
takes 100 ms per conversion here and 9.6 ms on MRI. Starting an instance takes
about 0.13 s (the first instance in a new runtime directory about 1 s). A
`Pool` of instances converts in parallel.

## Updating this report

The test figures come from `go test -v ./...`; the corpus goldens are
regenerated on MRI with `ruby scripts/gen-goldens.rb`. Every rbgo difference
found, with its workaround and a reproduction, is in
[rbgo-gaps.md](rbgo-gaps.md).
