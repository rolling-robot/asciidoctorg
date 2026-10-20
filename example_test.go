package asciidoctor_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/asciidoctor-go/asciidoctor"
)

func Example() {
	html, err := asciidoctor.Convert("A *bold* paragraph.", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(html)
	// Output:
	// <div class="paragraph">
	// <p>A <strong>bold</strong> paragraph.</p>
	// </div>
}

func ExampleAsciidoctor_Convert() {
	a, err := asciidoctor.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	out, err := a.Convert("Version {version}.", &asciidoctor.Options{
		Backend:    "docbook5",
		Attributes: asciidoctor.Attributes{"version": "1.2"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out)
	// Output:
	// <simpara>Version 1.2.</simpara>
}

func ExampleAsciidoctor_ConvertFile() {
	dir, err := os.MkdirTemp("", "example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	src := filepath.Join(dir, "guide.adoc")
	if err := os.WriteFile(src, []byte("= Guide\n\nHello.\n"), 0o644); err != nil {
		log.Fatal(err)
	}

	a, err := asciidoctor.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	// Like the asciidoctor command, ConvertFile writes guide.html next to
	// the input, as a standalone document.
	if _, err := a.ConvertFile(src, &asciidoctor.Options{Safe: asciidoctor.SafeModeSafe}); err != nil {
		log.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(dir, "guide.html"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(strings.Contains(string(html), "<title>Guide</title>"))
	// Output:
	// true
}

func ExampleAttributes() {
	attrs := asciidoctor.Attributes{}.
		TableOfContents(asciidoctor.PlacementLeft).
		SectionNumbers(true).
		Set("product", "Widget")
	out, err := asciidoctor.Convert("= Manual\n\n== Using the {product}", &asciidoctor.Options{
		Standalone: asciidoctor.Bool(false),
		Attributes: attrs,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(strings.Contains(out, `<h2 id="_using_the_widget">1. Using the Widget</h2>`))
	// Output:
	// true
}

func ExampleAsciidoctor_Load() {
	a, err := asciidoctor.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	doc, err := a.Load("= Release Notes\nDoc Writer <doc@example.org>\n\n== Fixes\n\n* one\n* two\n", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(doc.Doctitle())
	fmt.Println(doc.Authors()[0].Name)
	lists, err := doc.FindBy(asciidoctor.Selector{Context: "ulist"})
	if err != nil {
		log.Fatal(err)
	}
	for _, item := range lists[0].(*asciidoctor.List).Items() {
		fmt.Println("-", item.Text())
	}
	// Output:
	// Release Notes
	// Doc Writer
	// - one
	// - two
}

func ExampleExtensionRegistry_InlineMacro() {
	a, err := asciidoctor.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	// issue:42[] links to the issue tracker.
	_, err = a.Extensions().InlineMacro("issue", asciidoctor.InlineMacroProcessorFunc(
		func(parent asciidoctor.Node, target string, attrs map[string]any) (*asciidoctor.PhraseNode, error) {
			return asciidoctor.CreateAnchor(parent, "#"+target, nil, map[string]any{
				"type": "link", "target": "https://example.org/issues/" + target,
			})
		}), nil)
	if err != nil {
		log.Fatal(err)
	}
	out, err := a.Convert("Fixed in issue:42[].", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out)
	// Output:
	// <div class="paragraph">
	// <p>Fixed in <a href="https://example.org/issues/42">#42</a>.</p>
	// </div>
}

func ExampleExtensionRegistry_Block() {
	a, err := asciidoctor.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	// A [shout] paragraph is converted to upper case.
	_, err = a.Extensions().Block("shout", asciidoctor.BlockProcessorFunc(
		func(parent asciidoctor.StructuralNode, reader *asciidoctor.Reader, attrs map[string]any) (asciidoctor.StructuralNode, error) {
			return asciidoctor.CreateParagraph(parent, strings.ToUpper(reader.Read()), attrs, nil)
		}), &asciidoctor.BlockOptions{Contexts: []string{"paragraph"}, ContentModel: asciidoctor.ContentModelSimple})
	if err != nil {
		log.Fatal(err)
	}
	out, err := a.Convert("[shout]\nquiet please", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out)
	// Output:
	// <div class="paragraph">
	// <p>QUIET PLEASE</p>
	// </div>
}

func ExampleExtensionRegistry_Treeprocessor() {
	a, err := asciidoctor.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	// Number every section title with its level.
	_, err = a.Extensions().Treeprocessor(asciidoctor.TreeprocessorFunc(func(doc *asciidoctor.Document) (*asciidoctor.Document, error) {
		sections, err := doc.FindBy(asciidoctor.Selector{Context: "section"})
		for _, s := range sections {
			s.SetTitle(fmt.Sprintf("[%d] %s", s.Level(), s.Title()))
		}
		return doc, err
	}), nil)
	if err != nil {
		log.Fatal(err)
	}
	out, err := a.Convert("== Top\n\n=== Nested", nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "<h") {
			fmt.Println(line)
		}
	}
	// Output:
	// <h2 id="_top">[1] Top</h2>
	// <h3 id="_nested">[2] Nested</h3>
}

func ExampleAsciidoctor_RegisterConverter() {
	a, err := asciidoctor.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	// A converter for a "text" backend, handling the nodes this document has.
	text := asciidoctor.ConverterFunc(func(node asciidoctor.Node, transform string, _ map[string]any) (string, error) {
		switch n := node.(type) {
		case *asciidoctor.Section:
			content, err := n.Content()
			return strings.ToUpper(n.Title()) + "\n" + content.(string), err
		case asciidoctor.StructuralNode:
			content, err := n.Content()
			s, _ := content.(string)
			if transform == "paragraph" {
				s += "\n"
			}
			return s, err
		}
		return "", nil
	})
	if err := a.RegisterConverter(text, &asciidoctor.BackendTraits{OutfileSuffix: ".txt"}, "text"); err != nil {
		log.Fatal(err)
	}
	out, err := a.Convert("== Intro\n\nFirst words.", &asciidoctor.Options{Backend: "text"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out)
	// Output:
	// INTRO
	// First words.
}

func ExampleAsciidoctor_RegisterLogHandler() {
	a, err := asciidoctor.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	unregister, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(r asciidoctor.LogRecord) {
		fmt.Printf("%s at %s: %s\n", r.Severity, r.Cursor, r.Message)
	}))
	if err != nil {
		log.Fatal(err)
	}
	defer unregister()
	if _, err := a.Convert("= Title\n\n=== Too deep\n", nil); err != nil {
		log.Fatal(err)
	}
	// Output:
	// WARN at <stdin>: line 3: section title out of sequence: expected level 1, got level 2
}

func ExampleAsciidoctor_RunCLI() {
	a, err := asciidoctor.New(asciidoctor.WithStdout(os.Stdout))
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	code, err := a.RunCLI([]string{"--embedded", "--out-file", "-", "-"}, strings.NewReader("Hello, _world_."))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("exit code", code)
	// Output:
	// <div class="paragraph">
	// <p>Hello, <em>world</em>.</p>
	// </div>
	// exit code 0
}

func ExamplePool() {
	pool, err := asciidoctor.NewPool(2, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	results := make(chan string, 3)
	for _, src := range []string{"one", "two", "three"} {
		go func() {
			out, err := pool.Convert(src, nil)
			if err != nil {
				log.Fatal(err)
			}
			results <- strings.TrimSpace(out)
		}()
	}
	count := 0
	for range 3 {
		if strings.HasPrefix(<-results, `<div class="paragraph">`) {
			count++
		}
	}
	fmt.Println(count, "documents converted")
	// Output:
	// 3 documents converted
}
