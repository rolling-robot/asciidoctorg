package asciidoctor_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenConverterIsRegistered, with Go versions of
// its TextConverter, DummyConverter and LevelConverter.

const ajTextSource = "== Hello\n\nWorld!\n\n- a\n- b"
const ajTextResult = "== Hello ==\n\nWorld!\n\n-> a\n-> b\n"

// ajTextConverter converts sections, paragraphs and lists to plain text and
// logs a message when it converts the document.
type ajTextConverter struct{}

func (ajTextConverter) Convert(node asciidoctor.Node, transform string, _ map[string]any) (string, error) {
	content := func(n asciidoctor.StructuralNode) (string, error) {
		c, err := n.Content()
		if err != nil {
			return "", err
		}
		s, _ := c.(string)
		return s, nil
	}
	switch n := node.(type) {
	case *asciidoctor.Document:
		if err := n.Asciidoctor().Log(asciidoctor.LogRecord{Severity: asciidoctor.SeverityInfo, Message: "Now we're logging"}); err != nil {
			return "", err
		}
		return content(n)
	case *asciidoctor.Section:
		c, err := content(n)
		return "== " + n.Title() + " ==\n\n" + c, err
	case *asciidoctor.List:
		var b strings.Builder
		for _, item := range n.Items() {
			s, err := item.Convert()
			if err != nil {
				return "", err
			}
			b.WriteString(s + "\n")
		}
		return b.String(), nil
	case *asciidoctor.ListItem:
		return "-> " + n.Text(), nil
	case asciidoctor.StructuralNode:
		c, err := content(n)
		if transform == "paragraph" {
			return strings.ReplaceAll(c, "\n", " ") + "\n", err
		}
		return c, err
	}
	return "", nil
}

var ajDummyConverter = asciidoctor.ConverterFunc(func(asciidoctor.Node, string, map[string]any) (string, error) {
	return "Dummy", nil
})

// ajLevelConverter shows the level of each section.
var ajLevelConverter = asciidoctor.ConverterFunc(func(node asciidoctor.Node, _ string, _ map[string]any) (string, error) {
	switch n := node.(type) {
	case *asciidoctor.Document:
		c, err := n.Content()
		s, _ := c.(string)
		return s, err
	case *asciidoctor.Section:
		return "== " + strconv.Itoa(n.Level()) + " " + n.Title() + " ==", nil
	}
	return "", nil
})

func mustRegisterConverter(t *testing.T, a *asciidoctor.Asciidoctor, c asciidoctor.Converter, traits *asciidoctor.BackendTraits, backends ...string) {
	t.Helper()
	if err := a.RegisterConverter(c, traits, backends...); err != nil {
		t.Fatal(err)
	}
}

func TestAJConverterRegistryCleanUp(t *testing.T) {
	a := newInstance(t)
	mustRegisterConverter(t, a, ajTextConverter{}, nil, "test")
	mustRegisterConverter(t, a, ajDummyConverter, nil, "*")
	if err := a.UnregisterAllConverters(); err != nil {
		t.Fatal(err)
	}
	got, err := a.Converters()
	if err != nil {
		t.Fatal(err)
	}
	eqv(t, "converters", len(got), 0)
	if _, err := a.Convert(ajTextSource, &asciidoctor.Options{Backend: "test"}); err == nil {
		t.Error("unregistered backend still converts")
	}
}

func TestAJConverterRegisterAndExecute(t *testing.T) {
	a := newInstance(t)
	mustRegisterConverter(t, a, ajTextConverter{}, nil, "test")
	eqv(t, "result", mustConvert(t, a, ajTextSource, &asciidoctor.Options{Backend: "test"}), ajTextResult)
}

func TestAJConverterSeveralBackends(t *testing.T) {
	a := newInstance(t)
	mustRegisterConverter(t, a, ajTextConverter{}, nil, "annotatedtext")
	mustRegisterConverter(t, a, ajDummyConverter, nil, "*")
	eqv(t, "named backend", mustConvert(t, a, ajTextSource, &asciidoctor.Options{Backend: "annotatedtext"}), ajTextResult)
}

func TestAJConverterDefaultBackend(t *testing.T) {
	a := newInstance(t)
	mustRegisterConverter(t, a, ajDummyConverter, nil, "*")
	eqv(t, "result", mustConvert(t, a, ajTextSource, &asciidoctor.Options{Backend: "Undefined"}), "Dummy")
}

func TestAJConverterLogs(t *testing.T) {
	a := newInstance(t)
	logs := recordLogs(t, a)
	if err := a.SetLogLevel(asciidoctor.SeverityInfo); err != nil {
		t.Fatal(err)
	}
	mustRegisterConverter(t, a, ajTextConverter{}, nil, "annotatedtext")
	mustConvert(t, a, ajTextSource, &asciidoctor.Options{Backend: "annotatedtext"})
	recs := logs.Records()
	if len(recs) != 1 {
		t.Fatalf("records = %+v, want 1", recs)
	}
	eqv(t, "message", recs[0].Message, "Now we're logging")
	eqv(t, "severity", recs[0].Severity, asciidoctor.SeverityInfo)
}

func TestAJConvertersLookup(t *testing.T) {
	a := newInstance(t)
	text := ajTextConverter{}
	mustRegisterConverter(t, a, text, nil, "annotatedtext", "test2")
	mustRegisterConverter(t, a, ajDummyConverter, nil, "*")
	got, err := a.Converters()
	if err != nil {
		t.Fatal(err)
	}
	eqv(t, "backends", len(got), 3)
	for _, backend := range []string{"annotatedtext", "test2"} {
		if _, ok := got[backend].(ajTextConverter); !ok {
			t.Errorf("converter for %s = %T", backend, got[backend])
		}
	}
	if got["*"] == nil {
		t.Error("no catch-all converter")
	}
	if _, ok := got["html5"]; ok {
		t.Error("built-in converter listed")
	}
}

func TestAJConverterReceivesChangedLevels(t *testing.T) {
	a := newInstance(t)
	mustRegisterConverter(t, a, ajLevelConverter, nil, "test3")
	var visit func(asciidoctor.StructuralNode)
	visit = func(n asciidoctor.StructuralNode) {
		if s, ok := n.(*asciidoctor.Section); ok && s.Title() == "Test2" {
			s.SetLevel(42)
		}
		for _, b := range n.Blocks() {
			visit(b)
		}
	}
	mustRegister(t)(a.Extensions().Treeprocessor(asciidoctor.TreeprocessorFunc(func(doc *asciidoctor.Document) (*asciidoctor.Document, error) {
		visit(doc)
		return doc, nil
	}), nil))
	out := mustConvert(t, a, "= Test\n\n== Test\n== Test2\n", &asciidoctor.Options{Backend: "test3", Standalone: asciidoctor.Bool(false)})
	eqv(t, "result", out, "== 1 Test ==\n== 42 Test2 ==")
}

// AsciidoctorJ discovers ConverterRegistry implementations with Java's
// service loader; Go packages register a setup instead.
func TestAJConverterRegisteredBySetup(t *testing.T) {
	remove := asciidoctor.RegisterSetup(func(a *asciidoctor.Asciidoctor) error {
		return a.RegisterConverter(ajTextConverter{}, nil, "extensiontext")
	})
	defer remove()
	a := newInstance(t)
	eqv(t, "result", mustConvert(t, a, ajTextSource, &asciidoctor.Options{Backend: "extensiontext"}), ajTextResult)
	b := newInstance(t, asciidoctor.WithoutRegisteredSetups())
	if _, err := b.Convert(ajTextSource, &asciidoctor.Options{Backend: "extensiontext"}); err == nil {
		t.Error("setup ran despite WithoutRegisteredSetups")
	}
	remove()
	c := newInstance(t)
	if _, err := c.Convert(ajTextSource, &asciidoctor.Options{Backend: "extensiontext"}); err == nil {
		t.Error("removed setup ran")
	}
}

func TestAJConverterOutfileSuffix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		traits *asciidoctor.BackendTraits
		suffix string
	}{
		{"txt", &asciidoctor.BackendTraits{OutfileSuffix: ".txt"}, ".txt"},
		{"text", &asciidoctor.BackendTraits{OutfileSuffix: ".text"}, ".text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newInstance(t)
			mustRegisterConverter(t, a, ajTextConverter{}, tc.traits, "annotatedtext")
			dir := t.TempDir()
			mustConvertFile(t, a, ajResource(t, "simple.adoc"), &asciidoctor.Options{Backend: "annotatedtext", ToDir: dir, Safe: asciidoctor.SafeModeUnsafe})
			eqv(t, "simple.html exists", fileExists(filepath.Join(dir, "simple.html")), false)
			eqv(t, "simple"+tc.suffix+" exists", fileExists(filepath.Join(dir, "simple"+tc.suffix)), true)
		})
	}
}
