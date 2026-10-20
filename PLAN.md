# asciidoctor-go — implementation plan

Go bindings for [Asciidoctor](https://asciidoctor.org), built the way AsciidoctorJ
is built: the real Asciidoctor Ruby implementation runs inside an embedded Ruby
VM, and a typed Go API wraps it. AsciidoctorJ embeds JRuby; asciidoctor-go embeds
[rbgo](https://github.com/go-embedded-ruby/ruby) (pure Go, `CGO_ENABLED=0`).

Upstream sources:

| Path | Kind | Role |
|---|---|---|
| `rubysrc/asciidoctor` | git submodule (asciidoctor/asciidoctor) | Asciidoctor 2.1.0.alpha.0, run unmodified; Ruby test suite; corpus documents |
| `rubysrc/coderay` | git submodule (rubychan/coderay) | CodeRay 1.1.3 for the `coderay` source highlighter |
| `third_party/asciidoctorj` | git submodule (asciidoctor/asciidoctorj) | API design reference; Java tests to port; corpus documents |
| rbgo v0.8.0 | Go module dependency | the Ruby interpreter |

Clone with `git clone --recursive` (or run `git submodule update --init`).
Go module downloads do not include submodule contents, so releases are tags
on commits that hold them as ordinary files (`scripts/release.sh`, step 9).

## Ground rules

- **No stubs.** Every exported function does what it says. Anything not
  supported returns an explicit error naming the limitation.
- **Green commits.** `go vet ./... && go test ./...` passes on every commit.
- **Asciidoctor source stays pristine.** It comes from the submodule, unmodified. Gaps in rbgo
  are closed by (in order of preference): a Ruby shim in `ruby/shims`, a
  documented load-time source patch (exact-match, fails loudly on drift), or a Go
  binding. Every shim and patch carries the reason and a regression test.
- **MRI is the oracle.** Output is compared byte-for-byte with Asciidoctor on MRI.

## Architecture

```
            Go API (package asciidoctor)
  Convert / Load / Options / Attributes / AST / Extensions / Converters / Logging
                         │  (one mutex per instance)
          ┌──────────────┴──────────────┐
          │  bridge (internal/bridge)   │  Go ⇄ Ruby values, proxy classes,
          │                             │  node wrappers, live hash views
          └──────────────┬──────────────┘
                 rbgo *ruby.Interp
       shims (ruby/shims) + vendored Asciidoctor (ruby/asciidoctor)
```

- **Runtime bootstrap.** The Ruby tree (Asciidoctor `lib` + `data` from the submodule,
  shims, glue) is embedded with `go:embed` and materialized once into a
  content-addressed directory (default `os.UserCacheDir()/asciidoctor-go/<hash>`,
  overridable), applying source patches during materialization. Asciidoctor then
  loads with ordinary `require`, so `__dir__`, `autoload` and `data/` lookups
  work unchanged.
- **Instances.** `asciidoctor.New()` boots one interpreter (~60 ms). An
  `Interp` is not goroutine-safe, so each `*Asciidoctor` serializes calls with a
  mutex; a `Pool` hands out several instances for parallel work. Package-level
  `Convert`/`Load` use a lazily created default instance.
- **AST.** Go node types (`Document`, `Section`, `Block`, `List`, `ListItem`,
  `DescriptionList`, `Table`, `Column`, `Row`, `Cell`, `PhraseNode`, …) hold the
  live `ruby.Value` and forward to it, like AsciidoctorJ's `*Impl` classes. A
  per-document node cache keeps wrapper identity stable.
- **Extensions in Go.** For each Go processor, the bridge defines a Ruby
  subclass of the matching `Asciidoctor::Extensions::*Processor` whose `process`
  calls back into Go with wrapped arguments, then converts the result back.
  Registration metadata (name, contexts, content model, positional attributes,
  default attributes, format, location) comes from a Go struct — the analogue of
  AsciidoctorJ's annotations. Ruby extensions can be registered from source too.
- **Converters, syntax highlighters, loggers** in Go use the same proxy
  pattern (`Asciidoctor::Converter`, `Asciidoctor::SyntaxHighlighter`,
  `Asciidoctor::LoggerManager`).

## Steps

Each step ends with a commit, all tests green.

1. **Runtime boots unmodified Asciidoctor.**
   Module skeleton; Asciidoctor submodule; shims; source
   patches; embedded materialization; `internal/runtime` that returns a ready
   interpreter. Tests: boot, shim regression tests (each shim's MRI behaviour).
2. **Differential corpus.** Goldens generated with MRI from the `.adoc` files in
   both reference repos (plus a hand-written "complex" fixture set: macros,
   attributes, includes, conditionals, tables, lists, callouts, footnotes,
   xrefs, TOC, docinfo, manpage and DocBook backends). Test: every golden
   reproduced byte-for-byte through the Go API.
3. **Core Go API.** `New`, `Close`, `Convert`, `ConvertFile`, `ConvertFiles`,
   `ConvertDirectory` (walkers + glob), `Load`, `LoadFile`, `Options` (safe
   mode, backend, doctype, standalone, to_file/to_dir/mkdirs, base_dir,
   template dirs, attributes, sourcemap, catalog assets, parse_header_only,
   extensions), `Attributes` helpers, typed errors with Ruby backtrace,
   `Version`/`AsciidoctorVersion`, `Pool`.
4. **AST.** All node types and their accessors/mutators (attributes, roles,
   options, title/caption/numbering, blocks/find_by, content, convert, reftext,
   source location, catalog: refs/footnotes/images/links/callouts, document
   header/author/revision info, table structure). Node creation helpers used by
   extensions (`CreateBlock`, `CreateSection`, `CreateList`, `CreateListItem`,
   `CreateTable`/rows/columns/cells, `CreatePhraseNode`, `CreateDocument`,
   `ParseContent`).
5. **Extensions in Go.** Preprocessor + `PreprocessorReader`, IncludeProcessor,
   Treeprocessor, Postprocessor, DocinfoProcessor, BlockProcessor,
   BlockMacroProcessor, InlineMacroProcessor; `Reader` API; registry with
   instance/group registration, named groups, unregister. Ruby extension
   registry (source string or file).
6. **Converters, syntax highlighters, logging in Go.** Converter registry
   (`ConverterFor` analogue, base converter delegation, output-format writer),
   syntax-highlighter adapters (incl. a built-in Rouge-backed adapter fixed to
   the native go-rouge library), `LogHandler`/memory logger, log levels,
   failure level.
7. **Asciidoctor test suite in Go.** `go test` runs the original Ruby test
   files (`api`, `attributes`, `blocks`, `document`, `extensions`, `links`,
   `lists`, `paragraphs`, `parser`, `paths`, `preamble`, `reader`, `sections`,
   `substitutions`, `tables`, `text`, `converter`, `manpage`, `options`,
   `helpers`, `logger`, `attribute_list`, `syntax_highlighter`, `invoker`)
   inside the embedded VM. Support pieces: a minitest-compatible runner that
   reports each Ruby test as a Go subtest, and a Go-backed Nokogiri adapter
   (go-nokogiri) providing the CSS/XPath API the helper needs. rbgo gaps found
   here are fixed per the ground rules. Tests that cannot apply to an embedded
   runtime (e.g. spawning the `ruby` executable, JRuby-only cases, network
   servers) are listed in one exclusion table with a reason each — never
   silently skipped.
8. **AsciidoctorJ test suite ported to Go.** The document-processing tests of
   `asciidoctorj-core` (`WhenJavaExtensionIsRegistered`,
   `WhenAsciiDocIsLoadedToDocument`, `WhenAttributesAreUsedInAsciidoctor`,
   `WhenConverterIsRegistered`, block/inline/macro registration, reader
   manipulation, catalog assets, document header, multiple documents,
   source highlighting, logging, …) rewritten against the Go API, with the
   same fixtures.
9. **CLI, docs and distribution.** `cmd/asciidoctor` mirroring the Ruby CLI
   options, package docs with runnable examples, README, and a solution for
   `go get` consumers (who do not receive submodule contents): release tags
   point to commits outside `main` whose trees hold the submodules' files
   (`scripts/release.sh`); `main` keeps the submodules.
10. **Directory structure.** Option A was chosen: the public API stays in
    the root package (one import, like AsciidoctorJ's single entry point),
    and its tests moved out of the root — the API tests to `internal/apitest`,
    the AsciidoctorJ ports to `internal/asciidoctorj`, their shared helpers to
    `internal/testutil`; only `example_test.go` stays, as examples must sit
    with the package. Remaining: group the API files by area (conversion,
    AST, extensions, converters, logging, CLI) with consistent names.
    Splitting the API into public subpackages (`ast`, `extension`,
    `converter`) is set aside: it needs the shared node and bridge types in
    an internal package, with type aliases or wrappers to avoid import cycles.

## Status

Steps 1–9 are complete. Step 9's release tags were checked end to end: a
module that requires a tag made by `scripts/release.sh`, fetched with `go get`
from a git repository holding only that tag, builds, runs a Go macro
extension, and passes the library's own tests from the module cache. Step 10 (directory structure): the tests are moved out of the root; grouping the API files remains.

- Corpus (step 2): 324 conversions byte-identical with MRI (output and log),
  including CodeRay-highlighted listings in 20 languages.
- Asciidoctor's Ruby suite (step 7): 2864 tests pass; 13 are skipped, each with
  its reason in `internal/rubytest/exclusions_test.go` (spawning the `ruby`
  executable, the open-uri-cached gem, Rouge's monokai theme, a FIFO).
- AsciidoctorJ (step 8): the document-processing test classes of
  `asciidoctorj-core` are ported in `internal/asciidoctorj`, on AsciidoctorJ's
  own fixtures. Java-specific variants (registration by class or class name,
  annotations, class loaders, GEM_PATH) map to the Go equivalents (registration
  by value, `RegisterSetup` for service loading) or are noted as having none.
  One case is skipped: Rouge on Java source, as go-rouge v0.3.0 has no Java
  lexer.
- rbgo gaps found and worked around are listed in `docs/rbgo-gaps.md`.
