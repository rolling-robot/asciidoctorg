package asciidoctorj

import (
	"strings"
	"testing"
	"unicode"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's BlockMacroRegistrationTest,
// InlineMacroRegistrationTest and BlockProcessorRegistrationTest. AsciidoctorJ
// takes a processor's name from the registration call, an annotation or the
// processor itself; a Go processor is named at registration, so the tests
// check explicit names, one processor under several names, and that a
// processor without a name (or a nil one) is rejected.

type registrationCase struct {
	// register adds the processor under name.
	register func(g *asciidoctor.ExtensionGroup, name string)
	// document uses the processor named name on text.
	document func(name, text string) string
	// transform is what the processor makes of text.
	transform func(text string) string
	// register a nil processor.
	registerNil func(g *asciidoctor.ExtensionGroup, name string)
}

var registrationCases = map[string]registrationCase{
	"block macro": {
		register: func(g *asciidoctor.ExtensionGroup, name string) {
			g.BlockMacro(name, asciidoctor.BlockMacroProcessorFunc(func(parent asciidoctor.StructuralNode, target string, _ map[string]any) (asciidoctor.StructuralNode, error) {
				return asciidoctor.CreateParagraph(parent, strings.ToUpper(target), nil, nil)
			}), nil)
		},
		document:  func(name, text string) string { return "= Test\n\n== A section\n\n" + name + "::" + text + "[]" },
		transform: strings.ToUpper,
		registerNil: func(g *asciidoctor.ExtensionGroup, name string) {
			g.BlockMacro(name, nil, nil)
		},
	},
	"inline macro": {
		register: func(g *asciidoctor.ExtensionGroup, name string) {
			g.InlineMacro(name, asciidoctor.InlineMacroProcessorFunc(func(parent asciidoctor.Node, target string, _ map[string]any) (*asciidoctor.PhraseNode, error) {
				return asciidoctor.CreatePhraseNode(parent, "quoted", splitWords(target), nil, nil)
			}), nil)
		},
		document:  func(name, text string) string { return "= Test\n\n== A section\n\nHello " + name + ":" + text + "[]" },
		transform: func(text string) string { return "Hello " + splitWords(text) },
		registerNil: func(g *asciidoctor.ExtensionGroup, name string) {
			g.InlineMacro(name, nil, nil)
		},
	},
	"block processor": {
		register: func(g *asciidoctor.ExtensionGroup, name string) {
			g.Block(name, asciidoctor.BlockProcessorFunc(func(parent asciidoctor.StructuralNode, reader *asciidoctor.Reader, _ map[string]any) (asciidoctor.StructuralNode, error) {
				lines := reader.ReadLines()
				for i, l := range lines {
					lines[i] = strings.Join(strings.Split(l, ""), " ")
				}
				return asciidoctor.CreateParagraph(parent, lines, nil, nil)
			}), nil)
		},
		document:  func(name, text string) string { return "= Test\n\n== A section\n\n[" + name + "]\n" + text + "\n" },
		transform: func(text string) string { return strings.Join(strings.Split(text, ""), " ") },
		registerNil: func(g *asciidoctor.ExtensionGroup, name string) {
			g.Block(name, nil, nil)
		},
	},
}

// splitWords splits a camel-case word and upper-cases it: "HelloWorld"
// becomes "HELLO WORLD".
func splitWords(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsUpper(r) {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(strings.ToUpper(b.String()))
}

func registrationResult(t *testing.T, a *asciidoctor.Asciidoctor, src string) string {
	t.Helper()
	return text(parseHTML(t, mustConvert(t, a, src, nil)).Select("div.paragraph > p").First())
}

func TestAJProcessorRegistration(t *testing.T) {
	for kind, tc := range registrationCases {
		t.Run(kind, func(t *testing.T) {
			t.Run("explicit name", func(t *testing.T) {
				a := newInstance(t)
				g := a.Extensions().Group("")
				tc.register(g, "explicitblockname")
				if err := g.Register(); err != nil {
					t.Fatal(err)
				}
				eqv(t, "result", registrationResult(t, a, tc.document("explicitblockname", "HelloExplicit")), tc.transform("HelloExplicit"))
			})
			t.Run("several names", func(t *testing.T) {
				a := newInstance(t)
				g := a.Extensions().Group("")
				tc.register(g, "annotated")
				tc.register(g, "anotherName")
				if err := g.Register(); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"annotated", "anotherName"} {
					eqv(t, name, registrationResult(t, a, tc.document(name, "HelloWorld")), tc.transform("HelloWorld"))
				}
			})
			t.Run("without name", func(t *testing.T) {
				a := newInstance(t)
				g := a.Extensions().Group("")
				tc.register(g, "")
				if err := g.Register(); err == nil || !strings.Contains(err.Error(), "without a name") {
					t.Fatalf("err = %v", err)
				}
				// Nothing was registered: the document converts as usual.
				if got := registrationResult(t, a, tc.document("foo", "HelloWorld")); got == tc.transform("HelloWorld") {
					t.Errorf("processor ran: %q", got)
				}
			})
			t.Run("nil processor", func(t *testing.T) {
				g := newInstance(t).Extensions().Group("")
				tc.registerNil(g, "somename")
				if err := g.Register(); err == nil || !strings.Contains(err.Error(), "nil") {
					t.Fatalf("err = %v", err)
				}
			})
		})
	}
}
