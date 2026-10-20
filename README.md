# asciidoctor-go

Go bindings for [Asciidoctor](https://asciidoctor.org), built the way
[AsciidoctorJ](https://github.com/asciidoctor/asciidoctorj) is built: the real
Asciidoctor runs, unmodified, inside an embedded Ruby interpreter, and a typed
Go API wraps it. AsciidoctorJ embeds JRuby; asciidoctor-go embeds
[rbgo](https://github.com/go-embedded-ruby/ruby), a Ruby interpreter written in
Go. No Ruby installation, no cgo.

```go
html, err := asciidoctor.Convert("= Hello\n\nA *bold* paragraph.", nil)
```

## What works

- **Conversion** with Asciidoctor 2.1.0.alpha.0 to HTML 5, DocBook 5 and man
  pages: includes, attributes and conditionals, macros, tables, tables of
  contents, cross references, footnotes, admonitions and icons, STEM, docinfo,
  custom templates (ERB, Erubi, Haml, Slim), and source highlighting with Rouge,
  CodeRay, highlight.js or prettify.
- **The document tree**: load a document and read or change it through typed
  nodes.
- **Extensions in Go**: preprocessors, include processors, tree processors,
  postprocessors, docinfo processors, block processors, and block and inline
  macros, registered alone or in groups. Extensions written in Ruby load too.
- **Converters, syntax highlighters and log handlers in Go.**
- **The `asciidoctor` command**, with all of Asciidoctor's options.

## Install

```sh
go get github.com/asciidoctor-go/asciidoctor
go install github.com/asciidoctor-go/asciidoctor/cmd/asciidoctor@latest
```

> The embedded Ruby sources come from git submodules, which `go get` does not
> download yet. Until a release that includes them is published, build from a
> clone: `git clone --recursive`, then `go build ./...`.

## Examples

Convert a file, writing `guide.html` next to it:

```go
a, err := asciidoctor.New()
if err != nil {
	log.Fatal(err)
}
defer a.Close()
_, err = a.ConvertFile("guide.adoc", &asciidoctor.Options{
	Safe:       asciidoctor.SafeModeSafe,
	Attributes: asciidoctor.Attributes{}.TableOfContents(asciidoctor.PlacementLeft).SectionNumbers(true),
})
```

Add an inline macro, `issue:42[]`:

```go
a.Extensions().InlineMacro("issue", asciidoctor.InlineMacroProcessorFunc(
	func(parent asciidoctor.Node, target string, attrs map[string]any) (*asciidoctor.PhraseNode, error) {
		return asciidoctor.CreateAnchor(parent, "#"+target, nil, map[string]any{
			"type": "link", "target": "https://example.org/issues/" + target,
		})
	}), nil)
```

Read the document tree:

```go
doc, err := a.Load(source, nil)
sections, err := doc.FindBy(asciidoctor.Selector{Context: "section"})
for _, s := range sections {
	fmt.Println(s.Level(), s.Title())
}
```

More, runnable, in [example_test.go](example_test.go) and the package
documentation (`go doc github.com/asciidoctor-go/asciidoctor`).

## How it is tested

- **Against Asciidoctor on MRI Ruby.** A corpus of 324 conversions (the
  Asciidoctor and AsciidoctorJ test documents and this repository's own) must
  produce output and log messages byte-identical with MRI Ruby 3.4
  (`internal/conformance`; regenerate the goldens with
  `ruby scripts/gen-goldens.rb`).
- **Asciidoctor's own test suite** runs inside the embedded interpreter
  (`internal/rubytest`): 2864 tests pass; 13 that cannot apply to an embedded
  runtime are skipped, each with its reason.
- **AsciidoctorJ's tests**, ported to the Go API, on AsciidoctorJ's fixtures
  (`asciidoctorj_*_test.go`).

```sh
go test ./...
```

## Layout

| Path | What it holds |
|---|---|
| `*.go` | the Go API (package `asciidoctor`) |
| `cmd/asciidoctor` | the command line |
| `internal/runtime` | boots rbgo with Asciidoctor; source patches; Go-backed Ruby libraries |
| `internal/bridge` | calls between Go and Ruby |
| `rubysrc/asciidoctor` | Asciidoctor (git submodule, unmodified) |
| `rubysrc/coderay` | CodeRay (git submodule) |
| `rubysrc/shims`, `rubysrc/glue` | Ruby that adapts Asciidoctor to rbgo, and the Ruby side of the bridge |
| `third_party/asciidoctorj` | AsciidoctorJ (git submodule): API reference and test fixtures |
| `docs/rbgo-gaps.md` | every rbgo divergence from MRI found, with its workaround |

Where rbgo differs from MRI, the fix is a Ruby shim, or else an exact-match
source patch applied when the sources are materialized (it fails loudly if the
upstream source changes); each has a regression test. See
[docs/rbgo-gaps.md](docs/rbgo-gaps.md) and [PLAN.md](PLAN.md).

## License

asciidoctor-go embeds Asciidoctor and CodeRay, both under the MIT license.
