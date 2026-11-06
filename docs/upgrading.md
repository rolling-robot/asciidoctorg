# Upgrading from v0.1.0 to v0.2.0

This guide covers the changes in `v0.2.0`. That release fixes the defects
found in the code review of 2026-11-07, and adds cancellation and `log/slog`
support. It is a **minor** release: the API grows, and nothing is removed.

## Summary

| | Change | Action needed |
|---|---|---|
| Compile-time API | Additions only. No exported identifier was removed or renamed, and no signature or interface changed | none |
| `go.mod` | Unchanged. No new dependencies; `log/slog` is in the standard library | none |
| Behaviour | Seven changes, listed below. Most turn a hang or a leak into an error | review the ones that apply |

The API comparison was made with `go doc -all` on `v0.1.0` and on
`v0.2.0`. The one signature difference in that output is cosmetic:
`Pool.Convert` and `Pool.ConvertFile` no longer name their results
(`(out string, err error)` became `(string, error)`). Named results do not
affect callers.

## New API

### Cancellation and deadlines

Every conversion and load entry point has a variant that takes a
`context.Context`:

| Without context | With context |
|---|---|
| `(*Asciidoctor).Convert` | `ConvertContext` |
| `(*Asciidoctor).ConvertFile` | `ConvertFileContext` |
| `(*Asciidoctor).ConvertDocument` | `ConvertDocumentContext` |
| `(*Asciidoctor).ConvertFileDocument` | `ConvertFileDocumentContext` |
| `(*Asciidoctor).ConvertReader` | `ConvertReaderContext` |
| `(*Asciidoctor).ConvertFiles` | `ConvertFilesContext` |
| `(*Asciidoctor).Load` | `LoadContext` |
| `(*Asciidoctor).LoadFile` | `LoadFileContext` |
| `(*Pool).Do` | `DoContext` |
| `(*Pool).Convert` | `ConvertContext` |
| `(*Pool).ConvertFile` | `ConvertFileContext` |

The methods without a context are unchanged; they use `context.Background()`.
The package-level `Convert`, `ConvertFile`, `Load` and `LoadFile` have no
context variant. Call the method on `Default()` instead.

```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
defer cancel()
html, err := a.ConvertContext(ctx, source, opts)
if errors.Is(err, context.DeadlineExceeded) {
	// the conversion was stopped; the instance is still usable
}
```

What the context does:

- **It bounds the wait** for an instance that another goroutine is using, and
  for an idle instance of a `Pool`.
- **It stops the conversion at the next checkpoint**: before each block is
  parsed, before each block is converted, and at each call into Go code (an
  extension, converter, syntax highlighter or log handler). The embedded
  interpreter cannot be interrupted between checkpoints, so the work on a
  single block, such as the substitutions of one very long paragraph, runs
  to its end.
- **The error** satisfies `errors.Is(err, ctx.Err())`. A conversion stopped
  at a checkpoint returns an `*Error` whose `Class` is
  `"AsciidoctorGo::Canceled"` and whose `Cause` is `ctx.Err()`. A call
  stopped while waiting returns `ctx.Err()` itself.

Recommended: use a deadline when converting documents from untrusted
sources.

### `log/slog` integration

- **`SlogHandler{Logger: l}`** is a `LogHandler` that writes Asciidoctor's
  messages to a `*slog.Logger` (`slog.Default()` when `Logger` is nil).
  - The level comes from `Severity.SlogLevel`.
  - The source location is an `asciidoc` group with `path`, `file`, `dir` and
    `line`. Empty values are left out.
- **`ContextLogHandler`** is an optional interface (a `LogHandler` plus
  `LogContext(ctx, record)`). A handler that implements it receives the
  context of the call that logged, so trace and request values reach the log
  records. `SlogHandler` implements it.
- **`Severity.SlogLevel()`** maps Debug, Info, Warn and Error to the slog
  levels of the same name, Fatal to `slog.LevelError+4` and Unknown to
  `slog.LevelError+8`.

Migrating a hand-written handler that forwarded to slog:

```go
// before
a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(r asciidoctor.LogRecord) {
	slog.Warn(r.Message, "line", r.Cursor.LineNumber) // level and nil cursor handled by hand
}))

// after
a.RegisterLogHandler(asciidoctor.SlogHandler{Logger: slog.Default()})
```

The instance's log level (`SetLogLevel`, WARN by default) still decides which
messages reach any handler. The logger's own level then applies on top.

### `ErrCloseInCallback`

This is a new sentinel error; see behaviour change 1.

## Behaviour changes

1. **`Close` from a callback returns `ErrCloseInCallback`.** Before, calling
   `Close` on an instance from one of its own callbacks (an extension,
   converter, syntax highlighter or log handler), or from inside `Do`, hung
   forever. It now returns `ErrCloseInCallback`, and the instance stays open.
   *Action:* if you close an instance in response to something a callback
   sees, record that, and call `Close` after the conversion returns.

2. **`RegisterLogHandler(nil)` returns an error.** Before, the nil handler
   was accepted, and the first log message then failed the conversion with a
   panic error.
   *Action:* none, unless you relied on registering nil.

3. **A log handler with a `LogContext` method gets `LogContext` instead of
   `Log`.** This applies to any handler whose type already had a method
   `LogContext(context.Context, LogRecord)`: it now satisfies
   `ContextLogHandler` and receives that call instead.
   *Action:* rename such a method if it was not meant for this.

4. **`Error.Cause` is set only for the exception a Go callback raised.**
   Before, the error a Go callback returned was attached to any exception
   whose message contained its text, so a callback error that Ruby code had
   rescued could be blamed for an unrelated exception later in the call. A
   Go error with an empty message matched every exception. Now the exception
   must also be a `RuntimeError` (the class Go callback errors raise), and an
   empty message matches only an empty one. The usual case, a callback error
   that ends the conversion, is unchanged.
   *Action:* none, unless you relied on `Cause` or `errors.Is` matching
   exceptions of other classes.

5. **`Default()` retries after a failure.** Before, if the first start of
   the shared instance failed (for example, an unwritable cache directory),
   `Default`, and with it the package-level `Convert`, `ConvertFile`, `Load`
   and `LoadFile`, returned that error for the rest of the process. Now each
   call tries again until an instance starts. That instance is then kept.
   *Action:* none. Each failing call now pays the cost of a start attempt.

6. **Node accessors after `Close` panic with `ErrClosed`.** This is not new
   behaviour. It is now documented in the package documentation and pinned
   by a test, so `recover()` can tell it apart from a bug.
   *Action:* do not use a `Document` (or its nodes) after closing the
   instance that produced it.

7. **Asciidoctor runs with cancellation checkpoints.** The glue wraps
   `Asciidoctor::Parser.next_block` and `Asciidoctor::AbstractBlock#convert`
   (by alias, under the names `__agox_cancel_next_block` and
   `__agox_cancel_convert`). It also defines `AsciidoctorGo::Canceled`, a
   subclass of `Exception`, so a bare `rescue` does not catch it. Output is
   unchanged: the MRI conformance corpus is byte-identical. A small
   conversion is about 4% slower.
   *Action:* Ruby extensions that rescue `Exception` should re-raise
   `AsciidoctorGo::Canceled`, so that cancellation still works.

## Fixed without visible change

These need no action. They are listed for completeness:

- Registering or unregistering an extension group from a callback while
  another goroutine does the same no longer deadlocks.
- A log handler registered while the last one is being removed no longer
  loses its messages.
- A failed instance start no longer leaves an interpreter running.
- Registering the same `ExtensionGroup` twice no longer leaks bridge handles.
