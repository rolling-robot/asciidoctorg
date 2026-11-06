# AGENTS.md

Guide for agents (and people) working on this repository: what the project
is, how it is built, the decisions behind it, where it stands and what is
missing. Detailed references: [PLAN.md](PLAN.md) (steps and status),
[docs/compatibility.md](docs/compatibility.md) (test results, feature
support, AsciidoctorJ mapping) and [docs/rbgo-gaps.md](docs/rbgo-gaps.md)
(every rbgo divergence from MRI and its workaround).

The [code review of 2026-11-07](#code-review-2026-11-07) at the end of this
file lists confirmed defects, design recommendations and a prioritized list
of action items. Read it before starting new work.

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
                         │  one lock per instance (cancellable)
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

- A `Bridge` owns one interpreter. Every entry point holds its lock (a
  one-slot channel, so a wait can be cancelled), because an interpreter is
  not goroutine-safe.
- Ruby calls back into Go (extensions, converters, log handlers) while the
  lock is held. The bridge records the goroutines inside the critical
  section: the one that took the lock, plus each goroutine running a
  callback. A Ruby thread runs on a goroutine of its own, so its callbacks
  count too. Their goroutine IDs come from parsing `runtime.Stack`.
  - On those goroutines, `Do` runs directly instead of deadlocking, and
    `Close` returns `ErrCloseInCallback`.
  - Any other goroutine waits for the lock, even while a callback is in
    progress.
  - **Lock order:** the bridge lock first, then any Go-side mutex
    (`ExtensionRegistry.mu`, `logging.mu`). No Go-side mutex is held while
    Ruby runs. Breaking this rule caused defects D3 and D4.
- Cancellation is cooperative, since rbgo cannot interrupt running Ruby code.
  `DoContext` records the call's context, and
  `AsciidoctorGo::Bridge.checkpoint` raises `AsciidoctorGo::Canceled` (an
  `Exception`, so bare rescues let it through) once the context is done. The
  glue (`rubysrc/glue/asciidoctor_go_cancel.rb`) calls it in
  `Parser.next_block` and `AbstractBlock#convert`, and every dispatch to Go
  checks it as well. This costs about 4% on a small conversion.
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
| `converter.go`, `load.go`, `walker.go` | `Convert*`, `Load*` (each with a `*Context` variant), directory walkers (`AsciiDocDirectoryWalker`, `GlobDirectoryWalker`) |
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

## Code review (2026-11-07)

A critical review of the whole module at commit `867822c` (v0.1.0 plus two
commits). Scope: the public package, `internal/bridge`, `internal/runtime`,
`cmd/asciidoctor`, the tests, the build and release tooling. The Ruby glue and
shims were read only where Go code depends on them. Each finding says how it
was established: **confirmed** (reproduced), **measured**, or **from reading**
(not reproduced).

### Overall assessment

The project is in good shape for a 0.1 release. The architecture (run the
real Asciidoctor, test byte-identity against MRI, keep upstream pristine) is
sound and well documented. The tests are unusually strong: four independent
oracles (MRI goldens, Asciidoctor's own suite, AsciidoctorJ's tests, API
tests). `go vet` and `gofmt` are clean. `go test -race` passes for
`internal/bridge` and `internal/apitest`. Combined statement coverage of the
root package, `internal/bridge` and `internal/runtime` is **82.1%**.

The main weaknesses, in order of impact:

1. **Dependency weight.** Every importer links 398 modules (ActiveRecord,
   LDAP, ACME, ActionMailer, image codecs, …) into a **183 MB** binary.
2. **No cancellation or time limit on conversion.** A conversion that hangs
   holds the instance mutex forever.
3. **Re-entrancy holes in the lock design.** A callback can deadlock its own
   instance (confirmed). There is also a lock-order inversion, and a race in
   logger installation.
4. **Panics as the error channel of the node API.** Every accessor panics once
   the instance is closed (confirmed).
5. **Housekeeping.** No LICENSE, no CI, no benchmarks, an untidy `go.mod`, and
   a `Version` constant that disagrees with the tag. These are already listed
   in "Gaps and open items".

### Measurements

| What | Result | How |
|---|---|---|
| `go vet ./...`, `gofmt -l` | clean | run locally, `GOPROXY=off` |
| `go test -race ./internal/bridge ./internal/apitest` | pass, no data races (bridge 25 s, apitest 132 s) | run locally |
| Coverage of `.`, `internal/bridge`, `internal/runtime` | 82.1% combined (apitest alone 70.3%, asciidoctorj alone 60.1%) | `-coverpkg` over apitest, asciidoctorj, bridge, runtime and root |
| Modules linked by `import "github.com/rolling-robot/asciidoctorg"` | 398 | `go list -deps -f '{{.Module.Path}}' . \| sort -u` |
| `cmd/asciidoctor` binary | 183 MB, about 10 s to build (warm cache) | `go build` |
| Same, with `-tags rbgo_core` | **26 MB**, but fails at boot: `cannot load such file -- optparse (not available in the rbgo_core build)` | `go build -tags rbgo_core`; the examples fail the same way |
| Small conversion (title, paragraph, two-item list) | ≈1.2 ms/op, 351 KB/op, 5367 allocs/op | throwaway benchmark (`-overlay`, not committed) |
| One node accessor (`Document.Context()`) | ≈9.5 µs/op | throwaway benchmark |

Zero-coverage functions include `PatchTree`, `Pool.ConvertFile`,
`node.InheritedAttr`, `node.SetOption`, `CreateOpenBlock`, `CreateExampleBlock`,
`CreateLiteralBlock`, most `Reader` accessors (`File`, `Dir`, `Source`,
`SkipBlankLines`, `Terminate`, …), `rubyIO.Write` (the `OutputWriter` path
for Ruby IO targets), and several chainable `Attributes` setters.

### Confirmed defects

**D1. Calling `Close` from inside a callback deadlocks the instance, for
good.** `bridge.Close` locks `mu` without checking whether the calling
goroutine already owns the critical section (`internal/bridge/bridge.go:175`).
A treeprocessor that calls `a.Close()` blocks forever. Every later call blocks
too, including the `Close` in test cleanup. Reproduced with a probe test: no
return within 5 s, and the stack shows `(*Bridge).Close` waiting on the
mutex.
*Fix:* in `Close`, if `b.owns(goroutineID())`, either return an error
("Close called from a callback") or mark the bridge closed and defer the real
close until the outermost `Do` returns. Add a regression test.

**D2. Node, Reader and Document accessors panic after `Close`.** The panic
value is `asciidoctor: instance is closed`. This behaviour is documented on
`Node`, but it means ordinary use of a `*Document` returned by `Load`, outside
any callback, can crash a server after a shutdown or a pool resize. The same
applies to `wrapNode`, `arrayItems`, `cursor`, `Reader.ReadLinesUntil`, and to
`mustCall` (`wrap.go`, `node.go:118`, `reader.go:139`).
*Options*, from least to most API churn:
1. Document the panic prominently in the package doc, and make `ErrClosed`
   the only panic that can come from a well-formed tree.
2. Add an `Err() error` method on `Document`. Accessors would return zero
   values once the instance is closed and record the error (the
   `bufio.Scanner` pattern), instead of panicking.
3. Before v1, give accessors that can fail for reasons other than a closed
   instance (`Convert`, `Content`, `ReadAsset`, `FindBy`) an `error` result.

### Defects found by reading (not reproduced)

**D3. Lock-order inversion between `ExtensionRegistry.mu` and the bridge
mutex.** `ExtensionGroup.Register`, `Unregister` and
`ExtensionRegistry.UnregisterAll` take `r.mu`, then the bridge (`a.do`). A
callback already holds the bridge. If it registers or unregisters a group
while another goroutine holds `r.mu` and waits in `a.do`, both goroutines
deadlock (`extensions.go:404-411`, `:584-588`).
*Fix:* take the bridge first (`a.do(func(){ r.mu.Lock() … })`), or drop `r.mu`
and keep the group map under the bridge mutex, which already serializes
everything.

**D4. Race between logger install and uninstall.** `RegisterLogHandler`
decides `needInstall` under `logging.mu`, releases the lock, then installs
under the bridge lock. `removeLogHandler` does the same for uninstall.
Interleavings that go wrong (`logging.go:72-114`):
- Two concurrent first registrations both install. The second overwrites the
  first logger and leaks its bridge handle.
- A registration that runs while the last handler is being removed can end
  with a handler registered and no Go logger installed, so its messages go to
  stderr.

*Fix:* do the whole decide-and-act sequence inside one `a.do`, with
`logging.mu` taken inside it. Then the bridge mutex is the outer lock, as in
D3.

**D5. Resource leaks on error paths during boot.**
- `runtime.Boot` creates an interpreter (`ruby.New`) and returns early on six
  error paths without closing it (`internal/runtime/runtime.go:54-79`).
- `bridge.New` does the same with the booted runtime when the glue fails to
  load (`internal/bridge/bridge.go:69-82`).

Each leaked interpreter can leave Ruby threads and goroutines running.
*Fix:* `defer func(){ if err != nil { in.Close() } }()` with a named error
result.

**D6. Missing argument checks in Go-defined Ruby methods. Checked: not a
crash.** Several native methods index `c.Args` without a length check: the
rouge bridge (`guess`, `format_html`, `format_html_inline`, `theme_styles`),
`converterHandler.Dispatch` (`"handles"`, `"write"`), `lineHandler.Dispatch`,
and the `AsciidoctorGo::Writer` methods. Testing showed that rbgo recovers a
panic in a native method and raises it as a Ruby `ArgumentError`, for example
`guess: runtime error: index out of range [0] with length 0`. The bridge
handlers additionally go through `safeDispatch`. The only cost is a poor error
message. `TestNativeMethodsCheckArguments` (runtime) and
`TestWriterCreatedFromRubyRaises` (bridge) guard this behaviour.

**D7. A failure in `Default()` is cached forever.** `sync.Once` stores the
first error, so a transient failure (cache directory not writable yet, disk
full) breaks `Convert`, `ConvertFile`, `Load` and `LoadFile` for the life of
the process (`asciidoctor.go:315`).
*Fix:* keep the instance under a mutex and retry while it is nil, or use
`sync.OnceValues` and document the behaviour.

**D8. Registering a registered group again leaks handles.** When
`old == g && g.registered`, `Register` skips `unregisterLocked`, so the
handlers get new bridge handles while the old ones stay in the handle map
(`extensions.go:406`).
*Fix:* call `releaseHandles()` before re-binding, or treat re-registering the
same group as a no-op.

**D9. Error-path matching in `Bridge.Do` is heuristic.** A Go callback error
is attached to the Ruby exception only if the Ruby message *contains* the Go
error's text (`bridge.go:115`). An error with an empty message matches every
exception. Two callbacks that fail during one top-level call keep only the
last error.
*Fix:* make `toRubyError` raise a dedicated Ruby exception class
(`AsciidoctorGo::CallbackError`) that carries the handle or an error ID, then
look the Go error up by that ID instead of by substring.

### Design recommendations

**R1. Cut the dependency footprint (highest impact for adopters).** rbgo links
every gem binding it has. rbgo already offers `-tags rbgo_core` (26 MB, 47
modules), and Asciidoctor needs only part of the libraries outside core:
`optparse` (CLI only), `uri`, `erb`, `cgi` (shimmed), `net/http` (open-uri,
behind `allow-uri-read`), `zlib` (maybe), and the template engines that are
already Go-backed.
1. Make the eager `require 'optparse'` in
   `rubysrc/shims/asciidoctor_go_prelude.rb:442` lazy: patch `OptionParser`
   when `asciidoctor/cli` is required. That alone may let the library boot
   under `rbgo_core`.
2. Run the conformance corpus with `-tags rbgo_core` and list every
   `LoadError`.
3. Upstream to rbgo a way to opt in to single libraries on top of core: a
   build tag per feature, or blank-import packages such as
   `_ "github.com/go-embedded-ruby/ruby/lib/uri"`.
4. Document the tag as a supported build, and add it to CI.

This also takes care of most of the "Large `go.mod`" item, and shrinks the
supply-chain surface.

**R2. Add `context.Context` to conversion entry points.** Provide
`ConvertContext`, `ConvertFileContext` and `LoadContext`, plus a context on
`Pool.Do`, or a `WithContext` option on `Options`. This needs an interrupt
hook in rbgo, which `interp.go` does not appear to have (no
`Interrupt`/`context` API); that is a prerequisite to upstream. Until then:
- `Pool.Do` can at least honour `ctx.Done()` while waiting for an idle
  instance;
- the docs should warn that documents from untrusted input can run without
  bound. The risks are regexp backtracking in rbgo's engine and deeply nested
  includes, the latter limited by `max-include-depth`.

**R3. Replace goroutine-ID parsing with explicit ownership, or keep it and
isolate it.** `goroutineID()` parses `runtime.Stack` on every `Do` and
every dispatch. It works, and the race detector agrees, but:
- it depends on an unspecified output format;
- it costs about 1 µs per call, which is part of the ≈9.5 µs per accessor;
- it returns 0 on a parse failure, which would silently merge owners.

Options:
- a re-entrancy token carried in callback arguments (each `node` already holds
  `a`, so a per-callback `ownerToken` can travel on the wrapper);
- rbgo exposing "the current interpreter thread", so ownership is keyed on the
  Ruby thread instead of the goroutine.

At a minimum, make a parse failure panic loudly, and add a unit test that
pins the `runtime.Stack` header format.

**R4. Batch interpreter round trips.** `wrapNode` takes the lock twice
(`glueValue`, then `node_kind`). `arrayItems` takes it 1+N times. `cursor`,
`StructuredDoctitle` and `Authors` take it once per field. Wrapping each
whole helper in one `a.do` (the lock is re-entrant for its owner) removes most
`goroutineID` and mutex costs when walking a tree outside callbacks. Better
still, add glue methods that return a whole struct at once, for example
`[lineno, file, dir, path]` for `cursor`, or `[kind, …]` for `wrapNode`.

**R5. Narrow the `Node` interface or document why it is wide.** `Node` has
about 45 methods and is sealed (unexported `base()`), so it works as a closed
union, not as an abstraction callers implement. That fits the AsciidoctorJ
mapping. For consistency with the "accept interfaces, return structs" rule:
- return concrete types where the kind is known (this is already done for
  `*Document`, `*PhraseNode` and `*Block`);
- say in the package doc that `Node` and `StructuralNode` are sealed and are
  not meant to be implemented.

**R6. Harden the runtime cache directory.**
- **Permissions.** `Materialize` creates directories with `0o755` and files
  with `0o644`. It trusts any existing `<hash>/.complete` without checking it.
  With `WithRuntimeDir` pointing at a shared or world-writable directory (for
  example `/tmp`), another local user can plant a tree under the predictable
  hash and get arbitrary Ruby run in the host process. Create the parent with
  `0o700`, check that the owner is the current user, and document that the
  directory must not be writable by others.
- **Stale trees.** Old trees are never removed. Each upgrade leaves about
  30 MB in the user cache. Remove sibling `<hash>` directories older than N
  days, or give `Materialize` an explicit `Prune` helper.
- **Startup cost.** `treeFiles` reads and patches the whole embedded tree and
  hashes it on the first boot of every process, before it can check the
  marker. Compute the hash at build time (`go generate` writes a constant,
  with a test that checks it), so the common path is a single `os.Stat`.

**R7. Observability (library-appropriate).** A library should not pull in
Prometheus or OTel. What fits:
- A ready-made `LogHandler` that forwards to `*slog.Logger`
  (`asciidoctor.SlogHandler(logger)`). Map `Severity` to `slog.Level`, and add
  `Cursor` as attributes (`file`, `lineno`).
- Optional hooks (`WithConvertHook(func(ctx, Stats))` or similar) that report
  boot time, conversion duration and document size. Users can then emit
  metrics and spans without the library depending on a vendor.
- Once R2 lands, pass `ctx` to `LogHandler` (a `LogHandlerContext` variant) so
  trace IDs reach the log lines.

**R8. Small Go idiom fixes.**
- `walker.go`: two `regexp.MustCompile` calls run on every `Scan`. Move them to
  package-level variables. `readDir` swallows I/O errors (a permission-denied
  directory is skipped silently), so return or collect them.
- `RegisterSetup` and `logging.order` remove IDs from the map, but
  `setupOrder` keeps growing. Compact it on removal.
- `callValue` rewrites its variadic `args` slice in place. That is harmless
  today, but surprising if a caller ever passes `xs...`. Copy the slice first.
- `Attributes.toRuby` has a dead `case int` (the `default` case's
  `fmt.Sprint` covers it).
- `doc.go` has `const Version = "0.1.0-dev"`, while the published tag is
  `v0.1.0`. Have `scripts/release.sh` stamp it, or derive it from
  `debug.ReadBuildInfo()`.
- `rougeLexerTags` (15 tags) duplicates what go-rouge registers, and AGENTS.md
  says 16 lexers. Ask go-rouge for a `Lexers()` function, or add a test that
  checks the list against `rouge.FindLexer`.
- `cmd/asciidoctor` ignores the error from `a.Close()`. That is fine for a CLI,
  but say so with `_ =`.

**R9. Project layout (step 10 remainder).** The root package has 22 files
of similar weight. Rather than splitting it into subpackages (correctly set
aside in the design decisions), rename files by area with a common prefix so
that related code sorts together:
- `convert.go`, `convert_options.go`, `convert_walker.go`;
- `ast_node.go`, `ast_structural.go`, `ast_blocks.go`, `ast_table.go`,
  `ast_create.go`, `ast_reader.go`;
- `ext_api.go`, `ext_handlers.go`, `ext_ruby.go`;
- `converter.go`, `highlighter.go`, `logging.go`, `cli.go`.

Use `git mv` only, with no content change, in one commit.

### Test and tooling gaps

- **No CI.** Add a GitHub Actions workflow with these jobs:
  - checkout with `submodules: recursive`, then `go vet`, `gofmt -l`,
    `go test ./...`;
  - `go test -race` on `internal/bridge` and `internal/apitest` (both pass
    today, so the job is green from the start);
  - `govulncheck ./...`;
  - `golangci-lint` (at least `errcheck`, `staticcheck`, `gocritic`,
    `copylocks`);
  - a `-tags rbgo_core` build once R1 lands.
- **No benchmarks.** Commit `BenchmarkConvert` (small document, Asciidoctor's
  README), `BenchmarkLoadWalk` (walk every block, read `Context`/`Title`),
  `BenchmarkBoot` and `BenchmarkPoolParallel`, and track them with
  `benchstat`. That gives R3 and R4 a baseline and backs the "about 10×
  slower than MRI" figure.
- **No tests for D1, D3, D4, D7.** Add concurrency tests:
  - `Close` from a callback;
  - registering from a callback while another goroutine registers;
  - parallel `RegisterLogHandler`/unregister under `-race`;
  - `Default()` retry.
- **No goroutine-leak check.** `Close` promises to stop Ruby threads. Add
  `go.uber.org/goleak` (test-only) to `internal/apitest`'s `TestMain`, with
  ignores for the known `TCPServer#accept` case (rbgo gap 22).
- **No fuzzing.** A `FuzzConvert` with a size cap and a per-input timeout
  (once R2 exists) would find inputs that panic or hang the bridge. Seed it
  from `testdata/corpus`.
- **Fill the zero-coverage API listed above**, especially `Pool.ConvertFile`,
  `rubyIO.Write` and the untested `Create*Block` helpers. Each is a one-line
  wrapper, but each is public API.
- **gopls / static tooling** is not installed in this environment. With it,
  the "Where to look first" workflow could use `go_symbol_references` before
  API changes.

### Action items (prioritized)

Each item is one small PR. Structural and behavioural changes go in separate
PRs. `go vet ./... && go test ./...` must be green at every commit (working
rule 2).

| # | Priority | Item | Kind | Size |
|---|---|---|---|---|
| A1 | P0 | Fix D1: `Close` from a callback returns an error (or closes after the outermost `Do`); regression test | behaviour | S |
| A2 | P0 | Fix D3: take the bridge before `ExtensionRegistry.mu` (or drop `r.mu`); concurrency test | behaviour | S |
| A3 | P0 | Fix D4: install/uninstall the logger inside a single `a.do`; `-race` test with parallel register/unregister | behaviour | S |
| A4 | P0 | Fix D5: close the interpreter on `Boot`/`bridge.New` error paths | behaviour | S |
| A5 | P0 | Add LICENSE (owner decision; the embedded code is MIT) | housekeeping | XS |
| A6 | P0 | Add CI (vet, gofmt, test, race subset, govulncheck, golangci-lint), submodules recursive | tooling | S |
| A7 | P3 | D6 (checked, rbgo raises `ArgumentError`): optional clearer arity messages | cleanup | XS |
| A8 | P1 | Fix D7 (`Default()` retry) and D8 (re-register leak) | behaviour | XS |
| A9 | P1 | R1 step 1: make `require 'optparse'` lazy; try booting under `-tags rbgo_core`; list remaining `LoadError`s | behaviour | M |
| A10 | P1 | R6: cache dir `0o700` and owner check, prune stale trees, build-time tree hash | behaviour | M |
| A11 | P1 | Commit benchmarks (convert, load+walk, boot, pool) and record the baseline in `docs/compatibility.md` | tests | S |
| A12 | P1 | Decide the node-API error story (D2, options 1–3); at least document the panic in the package doc and README | API/docs | S–M |
| A13 | P2 | R4: batch round trips in `wrapNode`, `arrayItems`, `cursor`, `Authors`, `StructuredDoctitle`; compare with A11's benchmarks | performance | M |
| A14 | P2 | R2: `Pool.Do` honours `ctx` while waiting; open an rbgo issue for an interpreter interrupt hook; then `ConvertContext`/`LoadContext` | API | M (+upstream) |
| A15 | P2 | R1 steps 2–4: upstream per-feature opt-in in rbgo; supported slim build | upstream | L |
| A16 | P2 | R7: `SlogHandler` adapter; optional conversion-stats hook | API | S |
| A17 | P2 | D9: tag callback errors by ID instead of substring matching | behaviour | S |
| A18 | P2 | R3: pin `runtime.Stack` header format in a test; panic on parse failure; evaluate a token-based ownership model | robustness | S (test) / L (redesign) |
| A19 | P3 | R8 idiom fixes (walker regexps and errors, `setupOrder`, `callValue` copy, dead `case int`, `Version` stamping, `rougeLexerTags` check) | cleanup | S |
| A20 | P3 | R9: rename root files by area (`git mv` only, single commit) — closes PLAN step 10 | structural | S |
| A21 | P3 | goleak in `TestMain`; `FuzzConvert` seeded from the corpus | tests | S |
| A22 | P3 | Fill zero-coverage public API (`Pool.ConvertFile`, `rubyIO.Write`, `Create{Open,Example,Literal}Block`, `Reader` accessors) | tests | S |
| A23 | P3 | Upgrade go-rouge to v0.6.0 and run `go mod tidy` with network (already listed under gaps; needs the user to run `go get`) | deps | S |

Suggested order: A1–A4 together in one session, since they share the
lock-ordering rule "bridge mutex first, then any Go-side mutex". Write that
rule into the Architecture → Bridge section once it holds. Then A5–A6, so that
CI guards everything after it. Then A7–A12. A9 and A15 are the items with the
largest payoff for users.
