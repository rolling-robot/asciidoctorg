package asciidoctor_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/asciidoctor-go/asciidoctor"
)

// textConverter renders documents as plain text.
type textConverter struct{ handled []string }

func (c *textConverter) Convert(node asciidoctor.Node, transform string, opts map[string]any) (string, error) {
	c.handled = append(c.handled, transform)
	switch n := node.(type) {
	case *asciidoctor.Document:
		content, err := n.Content()
		if err != nil {
			return "", err
		}
		return strings.ToUpper(n.Doctitle()) + "\n\n" + content.(string), nil
	case *asciidoctor.Section:
		content, err := n.Content()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s\n%s", strings.Repeat("#", n.Level()), n.Title(), content), nil
	case *asciidoctor.Block:
		switch transform {
		case "paragraph":
			return n.Source() + "\n", nil
		case "preamble":
			content, err := n.Content()
			if err != nil {
				return "", err
			}
			return content.(string), nil
		}
		return "[" + transform + "]\n", nil
	}
	return "", fmt.Errorf("unexpected node %s", transform)
}

func TestCustomConverter(t *testing.T) {
	a := newInstance(t)
	conv := &textConverter{}
	check(t, a.RegisterConverter(conv, &asciidoctor.BackendTraits{OutfileSuffix: ".txt", Filetype: "txt"}, "text"))
	src := "= My Doc\n\nIntro.\n\n== Part\n\nBody.\n\n----\ncode\n----"
	out := mustConvert(t, a, src, &asciidoctor.Options{Backend: "text", Standalone: asciidoctor.Bool(true)})
	eq(t, "output", out, "MY DOC\n\nIntro.\n\n# Part\nBody.\n\n[listing]\n")
	eq(t, "transforms", conv.handled, []string{"document", "preamble", "paragraph", "section", "paragraph", "listing"})

	d, err := a.Load(src, &asciidoctor.Options{Backend: "text"})
	check(t, err)
	eq(t, "suffix", d.OutfileSuffix(), ".txt")
	eq(t, "attributes", []any{d.Attr("basebackend"), d.Attr("filetype"), d.Attr("outfilesuffix"), d.Attr("backend")}, []any{"text", "txt", ".txt", "text"})

	dir := t.TempDir()
	in := filepath.Join(dir, "doc.adoc")
	writeFile(t, in, src)
	_, err = a.ConvertFile(in, &asciidoctor.Options{Backend: "text", Safe: asciidoctor.SafeModeSafe})
	check(t, err)
	eq(t, "file", readFile(t, filepath.Join(dir, "doc.txt")), "MY DOC\n\nIntro.\n\n# Part\nBody.\n\n[listing]\n")
}

func TestConverterDelegatesToBuiltin(t *testing.T) {
	a := newInstance(t)
	html, err := a.BuiltinConverter("html5")
	check(t, err)
	conv := asciidoctor.ConverterFunc(func(node asciidoctor.Node, transform string, opts map[string]any) (string, error) {
		if transform == "paragraph" {
			return "<p class=\"custom\">" + node.(*asciidoctor.Block).Source() + "</p>", nil
		}
		return html.Convert(node, transform, opts)
	})
	check(t, a.RegisterConverter(conv, nil, "html5"))
	out := mustConvert(t, a, "para\n\n* item\n\nNOTE: note", nil)
	mustContain(t, out, `<p class="custom">para</p>`, `<div class="ulist">`, `<div class="admonitionblock note">`)
	// The builtin converter is still Asciidoctor's own after replacing html5.
	again, err := a.BuiltinConverter("html5")
	check(t, err)
	d, err := a.Load("para", nil)
	check(t, err)
	got, err := again.Convert(d.Blocks()[0], "paragraph", nil)
	check(t, err)
	eq(t, "builtin", got, "<div class=\"paragraph\">\n<p>para</p>\n</div>")
	// The outline (TOC) transform takes options.
	d, err = a.Load("= T\n\n== A\n\n=== B", nil)
	check(t, err)
	toc, err := again.Convert(d, "outline", map[string]any{"toclevels": 1})
	check(t, err)
	mustContain(t, toc, `<a href="#_a">A</a>`)
	mustNotContain(t, toc, "_b")
	if _, err := a.BuiltinConverter("pdf"); err == nil {
		t.Error("expected an error for an unknown builtin")
	}
}

type binaryConverter struct{}

func (binaryConverter) Convert(node asciidoctor.Node, transform string, opts map[string]any) (string, error) {
	return "payload:" + node.(*asciidoctor.Document).Doctitle(), nil
}

func (binaryConverter) WriteOutput(output string, w io.Writer) error {
	_, err := io.WriteString(w, "\x00BIN\x00"+output)
	return err
}

func TestConverterOutputWriter(t *testing.T) {
	a := newInstance(t)
	check(t, a.RegisterConverter(binaryConverter{}, &asciidoctor.BackendTraits{OutfileSuffix: ".bin"}, "bin"))
	dir := t.TempDir()
	_, err := a.Convert("= Title", &asciidoctor.Options{Backend: "bin", ToFile: filepath.Join(dir, "out.bin"), Safe: asciidoctor.SafeModeUnsafe})
	check(t, err)
	eq(t, "file", readFile(t, filepath.Join(dir, "out.bin")), "\x00BIN\x00payload:Title")
	var buf bytes.Buffer
	_, err = a.Convert("= Streamed", &asciidoctor.Options{Backend: "bin", Output: &buf})
	check(t, err)
	eq(t, "stream", buf.String(), "\x00BIN\x00payload:Streamed")
	out := mustConvert(t, a, "= In Memory", &asciidoctor.Options{Backend: "bin"})
	eq(t, "string", out, "payload:In Memory")
}

var errConverter = errors.New("converter failed")

func TestConverterErrors(t *testing.T) {
	a := newInstance(t)
	check(t, a.RegisterConverter(asciidoctor.ConverterFunc(func(asciidoctor.Node, string, map[string]any) (string, error) {
		return "", errConverter
	}), nil, "failing"))
	if _, err := a.Convert("x", &asciidoctor.Options{Backend: "failing"}); !errors.Is(err, errConverter) {
		t.Fatalf("got %v", err)
	}
	if err := a.RegisterConverter(&textConverter{}, nil); err == nil {
		t.Error("expected an error without backends")
	}
}

type handlesOnly struct{ textConverter }

func (handlesOnly) Handles(transform string) bool { return transform != "listing" }

func TestConverterHandles(t *testing.T) {
	a := newInstance(t)
	check(t, a.RegisterConverter(&handlesOnly{}, nil, "partial"))
	check(t, a.Do(func(in *ruby.Interp) error {
		v, err := in.Eval("c = Asciidoctor::Converter.create 'partial'; [c.handles?('paragraph'), c.handles?('listing'), c.basebackend, c.outfilesuffix]", "handles.rb", 1)
		if err != nil {
			return err
		}
		eq(t, "handles", v.Inspect(), `[true, false, "partial", ".partial"]`)
		return nil
	}))
}

// fakeHighlighter wraps source in spans and contributes a stylesheet.
type fakeHighlighter struct{ calls []string }

func (h *fakeHighlighter) HasDocinfo(location string) bool { return location == "head" }

func (h *fakeHighlighter) Docinfo(location string, doc *asciidoctor.Document, opts map[string]any) (string, error) {
	return fmt.Sprintf(`<link rel="stylesheet" href="fake.css" data-linkcss="%v">`, opts["linkcss"]), nil
}

func (h *fakeHighlighter) Highlight(node *asciidoctor.Block, source, lang string, opts map[string]any) (asciidoctor.HighlightResult, error) {
	h.calls = append(h.calls, lang)
	words := strings.Fields(source)
	for i, w := range words {
		words[i] = `<span class="tok">` + w + `</span>`
	}
	return asciidoctor.HighlightResult{Highlighted: strings.Join(words, " ")}, nil
}

type formattingHighlighter struct{ fakeHighlighter }

func (h *formattingHighlighter) Format(node *asciidoctor.Block, lang string, opts map[string]any) (string, error) {
	content, err := node.Content()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`<pre class="fmt" lang="%s">%s</pre>`, lang, content), nil
}

func TestSyntaxHighlighter(t *testing.T) {
	a := newInstance(t)
	hl := &fakeHighlighter{}
	check(t, a.RegisterSyntaxHighlighter(hl, "fake"))
	src := "= T\n\n[source,go]\n----\nx := 1\n----"
	out := mustConvert(t, a, src, &asciidoctor.Options{Standalone: asciidoctor.Bool(true), Attributes: asciidoctor.Attributes{"source-highlighter": "fake"}})
	mustContain(t, out,
		`<pre class="fake highlight"><code data-lang="go"><span class="tok">x</span> <span class="tok">:=</span> <span class="tok">1</span></code></pre>`,
		`<link rel="stylesheet" href="fake.css" data-linkcss="true">`, // secure mode forces linkcss
	)
	eq(t, "highlight calls", hl.calls, []string{"go"})

	check(t, a.RegisterSyntaxHighlighter(&formattingHighlighter{}, "fmt"))
	out = mustConvert(t, a, src, &asciidoctor.Options{Attributes: asciidoctor.Attributes{"source-highlighter": "fmt"}})
	mustContain(t, out, `<pre class="fmt" lang="go"><span class="tok">x</span>`)
	if err := a.RegisterSyntaxHighlighter(hl); err == nil {
		t.Error("expected an error without names")
	}
}

type stylesheetHighlighter struct {
	fakeHighlighter
	wrote string
}

func (h *stylesheetHighlighter) IsWriteStylesheet(doc *asciidoctor.Document) bool { return true }

func (h *stylesheetHighlighter) WriteStylesheet(doc *asciidoctor.Document, toDir string) error {
	h.wrote = toDir
	return os.WriteFile(filepath.Join(toDir, "fake.css"), []byte(".tok{}"), 0o644)
}

func TestSyntaxHighlighterStylesheet(t *testing.T) {
	a := newInstance(t)
	hl := &stylesheetHighlighter{}
	check(t, a.RegisterSyntaxHighlighter(hl, "styled"))
	dir := t.TempDir()
	_, err := a.Convert("[source,go]\n----\nx\n----", &asciidoctor.Options{
		ToFile: filepath.Join(dir, "out.html"), Safe: asciidoctor.SafeModeUnsafe,
		Attributes: asciidoctor.Attributes{"source-highlighter": "styled", "linkcss": true},
	})
	check(t, err)
	eq(t, "stylesheet dir", hl.wrote, dir)
	eq(t, "stylesheet", readFile(t, filepath.Join(dir, "fake.css")), ".tok{}")
}

func TestLogHandlers(t *testing.T) {
	var stderr bytes.Buffer
	a := newInstance(t, asciidoctor.WithStderr(&stderr))
	var records []asciidoctor.LogRecord
	unregister, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(r asciidoctor.LogRecord) { records = append(records, r) }))
	check(t, err)
	var second []string
	unregister2, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(r asciidoctor.LogRecord) { second = append(second, r.Message) }))
	check(t, err)
	mustConvert(t, a, "[[a]]\n== A\n\n[[a]]\n== B\n\n<<nowhere>>", nil)
	eq(t, "records", len(records), 1)
	eq(t, "record", []any{records[0].Severity, records[0].Message, records[0].Cursor.LineNumber, records[0].Cursor.Path},
		[]any{asciidoctor.SeverityWarn, "id assigned to section already in use: a", 5, "<stdin>"})
	eq(t, "second handler", second, []string{"id assigned to section already in use: a"})
	eq(t, "stderr while handled", stderr.String(), "")

	check(t, a.SetLogLevel(asciidoctor.SeverityInfo))
	records = nil
	mustConvert(t, a, "{missing}", &asciidoctor.Options{Attributes: asciidoctor.Attributes{"attribute-missing": "drop-line"}})
	eq(t, "info record", []any{len(records), records[0].Severity, records[0].Message, records[0].Cursor == nil},
		[]any{1, asciidoctor.SeverityInfo, "dropping line containing reference to missing attribute: missing", true})

	check(t, unregister())
	check(t, unregister()) // idempotent
	records = nil
	mustConvert(t, a, "[[a]]\n== A\n\n[[a]]\n== B", nil)
	eq(t, "after first unregister", []any{len(records), len(second)}, []any{0, 3})
	check(t, unregister2())
	check(t, a.SetLogLevel(asciidoctor.SeverityWarn))
	mustConvert(t, a, "[[a]]\n== A\n\n[[a]]\n== B", nil)
	eq(t, "stderr restored", stderr.String(), "asciidoctor: WARNING: <stdin>: line 5: id assigned to section already in use: a\n")
	eq(t, "severity names", []string{asciidoctor.SeverityDebug.String(), asciidoctor.SeverityFatal.String(), asciidoctor.Severity(9).String()}, []string{"DEBUG", "FATAL", "UNKNOWN"})
}

func TestLogThroughAsciidoctor(t *testing.T) {
	a := newInstance(t)
	var got []asciidoctor.LogRecord
	unregister, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(r asciidoctor.LogRecord) { got = append(got, r) }))
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	cursor := &asciidoctor.Cursor{LineNumber: 7, File: "/docs/a.adoc", Dir: "/docs", Path: "a.adoc"}
	if err := a.Log(asciidoctor.LogRecord{Severity: asciidoctor.SeverityError, Message: "broken", Cursor: cursor}); err != nil {
		t.Fatal(err)
	}
	if err := a.Log(asciidoctor.LogRecord{Severity: asciidoctor.SeverityInfo, Message: "below the level"}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("records = %+v", got)
	}
	eqv(t, "record", got[0], asciidoctor.LogRecord{Severity: asciidoctor.SeverityError, Message: "broken", Cursor: cursor})
}
