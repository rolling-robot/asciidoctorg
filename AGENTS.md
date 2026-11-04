# AGENTS.md

Guide for agents (and people) working on this repository: what the project
is, how it is built, the decisions behind it, where it stands and what is
missing. Detailed references: [PLAN.md](PLAN.md) (steps and status),
[docs/compatibility.md](docs/compatibility.md) (test results, feature
support, AsciidoctorJ mapping) and [docs/rbgo-gaps.md](docs/rbgo-gaps.md)
(every rbgo divergence from MRI and its workaround).

## What this is

Go bindings for [Asciidoctor](https://asciidoctor.org), built the way
AsciidoctorJ is: the real Asciidoctor (2.1.0.alpha.0, Ruby) runs unmodified
inside an embedded Ruby interpreter, and a typed Go API wraps it.
AsciidoctorJ embeds JRuby; this project embeds
[rbgo](https://github.com/go-embedded-ruby/ruby) v0.8.0, a Ruby interpreter
written in Go. No Ruby installation and no cgo are needed.

- Module: `github.com/rolling-robot/asciidoctorg` (GitHub
  `rolling-robot/asciidoctorg`, default branch `master`). The package name is
  `asciidoctor`, so callers write `asciidoctor.Convert(...)`.
- Current release: `v0.1.0`, which proxy.golang.org and sum.golang.org serve.
  The tag exists, but there is no entry on the GitHub Releases page.
- Go version: `go 1.27.2` (go.mod).

## Working rules

These rules are deliberate; keep to them.

1. **No stubs.** Every exported function does what it says. Anything
   unsupported returns an explicit error naming the limitation.
2. **Green commits.** `go vet ./... && go test ./...` passes on every commit.
   Make small, meaningful commits as you go. Commit messages end with the
   attribution line the session asks for.
3. **Upstream stays pristine.** Asciidoctor, CodeRay and AsciidoctorJ are git
   submodules, never copied and never edited in place. Where rbgo differs
   from MRI, the fix is, in order of preference:
   1. a Ruby shim in `rubysrc/shims`;
   2. an exact-match source patch in `internal/runtime/patch.go`, applied
      while the sources are materialized (it fails loudly if upstream
      changes, and must be equivalent on MRI);
   3. a Go binding in `internal/runtime`.

   Each fix carries its reason and a regression test, and gets a row in
   `docs/rbgo-gaps.md`.
4. **MRI is the oracle.** Output must be byte-identical with Asciidoctor on
   MRI Ruby 3.4.10. Check doubtful behaviour against MRI (`ruby` is installed
   locally) before calling it a bug. For example, `hello:[]` does not expand
   on MRI either, because an inline macro needs a target.
5. **Never skip silently.** Ruby tests that cannot apply to an embedded
   runtime go in `internal/rubytest/exclusions_test.go`, each with a reason.
   Skipped AsciidoctorJ cases use `t.Skip` with a reason.
6. **Do not touch** the user's untracked files: `docs/#rbgo-gaps.md#`,
   `docs/rbgo-gaps.adoc` and `docs/rbgo-gaps.adoc~`.

## Build and test

```sh
git submodule update --init          # required: the Ruby sources are submodules
go build ./... && go vet ./... && go test ./...
go test -v ./internal/rubytest -run 'TestAsciidoctorSuite/lists_test'   # one Ruby test file
ruby scripts/gen-goldens.rb          # regenerate the MRI goldens (needs MRI Ruby)
```

- A full `go test ./...` takes about 1–2 minutes. The slowest packages are
  `internal/asciidoctorj` (~50 s) and `internal/apitest` (~35 s).
- In the agent sandbox, Go has no network access, but `curl` does. Build
  offline with `GOPROXY=off`, since every dependency is in the local module
  cache. Do not retry a `go get` that was refused; ask the user to run it
  (`! <command>`).
- `gofmt -l .` must print nothing outside the submodules.

## Architecture

```
            Go API (package asciidoctor, repository root)
  Convert / Load / Options / Attributes / AST / Extensions / Converters / Logging / CLI
                         │  one mutex per instance
          ┌──────────────┴──────────────┐
          │  internal/bridge            │  Go ⇄ Ruby values, proxy dispatch,
          │                             │  writer adapter, callback re-entry
          └──────────────┬──────────────┘
          internal/runtime: boots rbgo, materializes the Ruby tree, applies
          patches, defines Go-backed Ruby libraries (Rouge, Haml/Slim compilers)
                         │
          rubysrc (go:embed): asciidoctor/{lib,data}, coderay/lib,
          shims/ (rbgo compatibility), glue/asciidoctor_go.rb (Ruby side of bridge)
```

### Runtime bootstrap (`internal/runtime`, `rubysrc`)

- `rubysrc/rubysrc.go` embeds Asciidoctor's `lib`, `data` and `LICENSE`,
  CodeRay's `lib`, the shims and the glue.
- `runtime.Boot` writes that tree, with the patches applied, into a
  content-addressed directory, by default `os.UserCacheDir()/asciidoctor-go/<hash>`
  (set it with `WithRuntimeDir`). A completion marker is written last. The
  tree is materialized once per hash and then reused.
- Asciidoctor then loads with ordinary `require`, so `__dir__`, `autoload`
  and the `data/` lookups work unchanged.
- Patches: 13 to Asciidoctor and 5 to CodeRay, in `patch.go`. Each has
  `File`, `Old` (which must occur exactly `Count` times), `New` and `Reason`.
- The prelude (`rubysrc/shims/asciidoctor_go_prelude.rb`) fixes many small
  core-library gaps. Other shims replace libraries outright: `logger.rb`,
  `open-uri.rb` (on `Net::HTTP`), `tilt.rb`, `rouge.rb`, `cgi/`,
  `pathname_ext.rb` and `slim_include.rb`.
- Frame-local `$~`: rbgo keeps one `$~` per interpreter (gap 27).
  `asciidoctor_go_frame_local_match.rb` wraps exactly the Asciidoctor methods
  that can set it, so each one restores its caller's match.
- Rouge: the Ruby `Rouge` API that Asciidoctor's adapter needs is backed by
  go-rouge (`rouge.go`). Haml and Slim templates compile through
  go-ruby-haml and go-ruby-slim (`templates.go`).

### Bridge (`internal/bridge`, `rubysrc/glue`)

- A `Bridge` owns one interpreter. Every entry point holds `mu`, because an
  interpreter is not goroutine-safe.
- Ruby calls back into Go (extensions, converters, log handlers) while `mu`
  is held. The bridge records the goroutines inside the critical section:
  the one that locked `mu`, plus each goroutine running a callback. A Ruby
  thread runs on a goroutine of its own, so its callbacks count too. Their
  goroutine IDs come from parsing `runtime.Stack`.
  - On those goroutines, `Do` runs directly instead of deadlocking.
  - Any other goroutine waits for `mu`, even while a callback is in progress.
  - Rule, documented on the public API: objects passed to a callback are used
    only by the goroutine running it.
- Each Go object is registered under a handle. The Ruby proxies call
  `AsciidoctorGo::Bridge.dispatch(handle, method, *args)`, which routes to
  the Go `Handler`.
- `AsciidoctorGo::Writer` adapts an `io.Writer` for `:to_file`.
- A panic in Go code called from Ruby is recovered and becomes the `Cause`
  of the returned `*Error`, as a `*PanicError` holding the panic's stack.
- `Close` runs the interpreter's `at_exit` handlers. The glue's own handler,
  registered first so it runs last, kills the Ruby threads still running, so
  their goroutines do not outlive the instance. A thread blocked in
  `TCPServer#accept` cannot be interrupted (rbgo gap 22); `Close` waits at
  most a second for such threads, then leaves them.

### Go API (repository root, package `asciidoctor`)

| File | Area |
|---|---|
| `asciidoctor.go`, `options.go`, `attributes.go`, `errors.go`, `pool.go`, `wrap.go` | instance (`New`, `Close`, `With*` options), `Options` struct, `Attributes` with chainable setters, typed errors with the Ruby backtrace, `Pool`, `Do` |
| `converter.go`, `load.go`, `walker.go` | `Convert*`, `Load*`, directory walkers (`AsciiDocDirectoryWalker`, `GlobDirectoryWalker`) |
| `document.go`, `node.go`, `structural_node.go`, `blocks.go`, `table.go`, `create.go`, `reader.go` | AST wrappers that hold the live `ruby.Value` and forward to it (like AsciidoctorJ's `*Impl`), with a per-document cache that keeps wrapper identity stable; `Create*` node constructors; `Reader` |
| `extensions.go`, `extension_handlers.go`, `ruby_extensions.go` | Go processors of all eight kinds; each becomes a Ruby subclass of the matching `Asciidoctor::Extensions::*Processor` whose `process` calls Go. Registration metadata comes from a Go struct (the analogue of AsciidoctorJ's annotations). Groups; `RegisterSetup` (service-loader analogue, called from `init`); Ruby extensions via `LoadRubySource`/`RequireLibrary` |
| `syntax_highlighter.go`, `logging.go` | Go syntax highlighters and log handlers, same proxy pattern |
| `cli.go`, `cmd/asciidoctor` | `RunCLI` runs Asciidoctor's own `Invoker` (all options, MRI-identical output and exit codes in the cases checked) |
| `doc.go`, `example_test.go` | package documentation, 12 runnable examples |

Package-level `Convert`/`Load` use a lazily created default instance. Booting
an instance takes about 0.13 s (about 1 s the first time a runtime directory
is materialized).

### Tests

| Package | What it checks |
|---|---|
| `internal/conformance` | 324 corpus conversions (Asciidoctor and AsciidoctorJ fixtures plus `testdata/corpus`) byte-identical with MRI goldens in `testdata/golden`, log messages included |
| `internal/apitest` | the public API (388 cases) and the same corpus through it; CLI tests |
| `internal/rubytest` | Asciidoctor's own `test/*_test.rb` inside rbgo, with a minitest-compatible runner and one Go subtest per Ruby test: 2864 pass, 13 excluded with reasons. Includes a Go-backed Nokogiri (go-nokogiri) for the CSS/XPath helpers |
| `internal/asciidoctorj` | AsciidoctorJ's document-processing tests ported to Go, on AsciidoctorJ's own fixtures: 284 cases pass, 1 skipped |
| `internal/runtime`, `internal/bridge` | boot, shims, patches (a regression test for each rbgo gap), value conversion |
| `internal/testutil` | shared helpers: `Main` gives each test binary a temporary runtime directory |

## Design decisions and the reasons for them

- **Embed real Asciidoctor rather than port it.** A port would drift from
  upstream and need its own test suite. Running the real code means upstream's
  tests are the specification, and byte-identity with MRI can be tested.
- **rbgo, not cgo plus libruby.** It gives pure Go, `CGO_ENABLED=0` and easy
  cross-compilation. The cost: rbgo's divergences (43 so far) must be shimmed,
  and conversion is about 10× slower than MRI (100 ms against 9.6 ms for
  Asciidoctor's README).
- **Materialize to disk instead of serving from memory.** Asciidoctor relies on
  `__dir__`, `autoload` and file paths for `data/`. Real files make that work
  without patching. Content addressing makes it safe to share and reuse the
  tree.
- **One interpreter per instance, guarded by a mutex; `Pool` for parallel
  work.** An rbgo interpreter is not goroutine-safe. Independent interpreters
  are simpler and safer than fine-grained locking.
- **API shaped after AsciidoctorJ**, mapped to Go idioms:
  - registration by value instead of by class or annotation;
  - `Func` adapters for single-method processors;
  - `RegisterSetup` instead of `ServiceLoader`;
  - an `Options` struct, with `Options.Extra` for anything not modelled.

  The full mapping is in `docs/compatibility.md`.
- **`Attributes()` returns a copy.** Changes go through `SetAttr`/`RemoveAttr`
  (and `SetBlock`/`InsertBlock`/`RemoveBlock` for block lists). A live map
  would hide interpreter calls behind plain map writes.
- **Log level defaults to WARN**, as in Asciidoctor (AsciidoctorJ uses INFO).
- **Public API in a single root package** (step 10, option A), so callers need
  one import, like AsciidoctorJ's single entry point. Tests moved to
  `internal/*`, and only `example_test.go` stays in the root. Splitting into
  public subpackages (`ast`, `extension`, `converter`) was set aside: it needs
  shared node and bridge types in an internal package, plus aliases to avoid
  import cycles.
- **Releases hold the submodule contents** (`scripts/release.sh`). Go module
  zips include neither submodules nor symbolic links, so a pseudo-version of
  `master` does not build.
  - A release is a commit outside `master`, whose parent is `HEAD`, whose
    tree is `HEAD`'s with every submodule replaced by its files, and which
    drops `.gitmodules`.
  - Symbolic links in a submodule become copies of their targets.
  - The commit is tagged.

  `master` keeps the submodules, so upstream updates stay a submodule bump.
- **Tests copy the source tree writable** (`internal/rubytest` `copyTree`
  adds `0o200`), so the suite also runs from the read-only module cache. A
  release passes `go test github.com/rolling-robot/asciidoctorg/...` in a
  module that requires it.

## Releasing

```sh
scripts/release.sh vX.Y.Z        # needs a clean tree and submodules at their recorded commits
git push origin vX.Y.Z
scripts/release.sh -n vX.Y.Z     # dry run: make the commit only and print its hash
```

The script never pushes. Pushing a tag publishes it, so do it only when the
user asks. Check a release with curl against
`https://proxy.golang.org/github.com/rolling-robot/asciidoctorg/@v/<tag>.info`
and `.zip` (the zip must contain `rubysrc/asciidoctor/lib/`).

## State

PLAN.md steps 1–9 are complete. Step 10 is partly done.

| Step | Status |
|---|---|
| 1 Runtime boots unmodified Asciidoctor | done |
| 2 MRI-differential corpus (324 conversions) | done |
| 3 Core API (`Convert*`, `Load*`, `Options`, errors, `Pool`) | done |
| 4 AST and node creation | done |
| 5 Go extensions (all processor kinds, `Reader`, groups, Ruby extensions) | done |
| 6 Converters, syntax highlighters, logging in Go; Rouge on go-rouge | done |
| 7 Asciidoctor's Ruby test suite under `go test` | done (2864 pass, 13 excluded) |
| 8 AsciidoctorJ tests ported | done (284 pass, 1 skipped) |
| 9 CLI, docs, `go get` distribution | done; v0.1.0 published |
| 10 Directory structure | tests moved out of the root; **grouping the API files by area with consistent names remains** |

## Gaps and open items

Functional gaps (details in `docs/compatibility.md`):

- **Pygments**: not available (it needs Python). Asciidoctor warns and shows
  the source unhighlighted.
- **Rouge** is limited to go-rouge v0.3.0:
  - 16 lexers (no Java, for instance) and 3 themes (base16, github,
    thankful_eyes; no monokai);
  - lexers and themes cannot be written in Ruby.

  go-rouge v0.6.0 exists, but the upgrade needs a network `go get` that the
  sandbox blocks; the user can run
  `GOPROXY=direct GOSUMDB=off go get github.com/go-rouge/rouge@v0.6.0`. After
  upgrading, update `rougeLexerTags` in `internal/runtime/rouge.go`, then
  re-check the skipped AsciidoctorJ Rouge-on-Java case and the excluded
  monokai test.
- **`cache-uri`**: the `open-uri-cached` gem is missing (2 Ruby tests
  excluded).
- **Other gems** (asciidoctor-pdf, -epub3, -diagram, -revealjs, …): not
  available. Only the embedded libraries and pure-Ruby files on the load path
  that run under rbgo can be required.
- **Other Tilt template engines**: only ERB, Erubi, Haml and Slim work.
- **Spawning `ruby`**: tests that spawn the `ruby` executable cannot apply (9
  excluded).
- **FIFO reads**: they block other threads (rbgo gap 34; 1 test excluded).
- **Speed**: about 10× slower than MRI.
- **rbgo differences that still reach user Ruby code** (Asciidoctor's own
  code is covered by shims):
  - `$~` is not frame-local in user methods;
  - some `super` forms fail (gaps 18, 43);
  - nested character classes in regexps match nothing;
  - `String` subclass `dup` is unusable;
  - several `StringScanner` methods are missing.

  These are upstream candidates for rbgo; see `docs/rbgo-gaps.md`.
- **`--version`** reports rbgo's Ruby version (3.4.1).

Project and housekeeping items:

- **No LICENSE file.** pkg.go.dev will not show documentation until one is
  added. Choosing the license is the owner's decision; the embedded
  Asciidoctor and CodeRay are MIT.
- **No GitHub Release entry** for v0.1.0 (only the tag). `gh` is not
  installed in the sandbox.
- **No CI.** Tests run only locally. A workflow would need
  `submodules: recursive` on checkout.
- **Large `go.mod`.** rbgo brings in many requirements, and some are
  unneeded by this module (for example `github.com/alicebob/miniredis/v2`;
  `go mod why` says the main module does not need it). `go.mod` also lists
  `github.com/go-embedded-ruby/ruby` in both require blocks. Try
  `go mod tidy` with network access before trimming by hand.
- **Step 10 remainder**: group the root API files by area (conversion, AST,
  extensions, converters, logging, CLI) with consistent names.

## Where to look first

- A conversion differs from MRI: run `internal/conformance` with `-v`, reduce
  the document, compare against `ruby -Irubysrc/asciidoctor/lib`. Then find
  the rbgo construct at fault and fix it according to working rule 3.
- A Ruby test fails: run `go test -v ./internal/rubytest -run 'TestAsciidoctorSuite/<file>/<name>'`.
- Adding API: mirror AsciidoctorJ's name and behaviour where one exists, add
  it to the mapping table in `docs/compatibility.md`, and test it in
  `internal/apitest` (or port the matching AsciidoctorJ test to
  `internal/asciidoctorj`).
- Bumping Asciidoctor: move the submodule, run the tests, fix any patches
  whose `Old` text no longer matches, regenerate the goldens on MRI, and
  update the version figures in `docs/compatibility.md`.
