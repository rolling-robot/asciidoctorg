package asciidoctor_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenConvertingWithExpectedType: the result is
// the converted document unless the output goes to a file or a stream. (In
// AsciidoctorJ the result type follows the converter; a Go converter's
// result is a string, and an OutputWriter writes it out.)

const ajSampleContent = "= Document Title\n\nThis is the preamble\n"

// ajObjectConverter converts every document to "42" and writes it out as
// AsciidoctorJ's ObjectConverter does.
type ajObjectConverter struct{}

func (ajObjectConverter) Convert(asciidoctor.Node, string, map[string]any) (string, error) {
	return strconv.Itoa(42), nil
}

func (ajObjectConverter) WriteOutput(output string, w io.Writer) error {
	_, err := io.WriteString(w, output)
	return err
}

func TestAJConvertResultType(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend string
		options func(dir string, stream *bytes.Buffer) *asciidoctor.Options
		result  string
		written string
	}{
		{"string", "", func(string, *bytes.Buffer) *asciidoctor.Options { return &asciidoctor.Options{} }, "nonempty", ""},
		{"to file", "", func(dir string, _ *bytes.Buffer) *asciidoctor.Options {
			return &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, ToFile: filepath.Join(dir, "output")}
		}, "", "file"},
		{"to stream", "", func(_ string, s *bytes.Buffer) *asciidoctor.Options { return &asciidoctor.Options{Output: s} }, "", "stream"},
		{"converter result", "42", func(string, *bytes.Buffer) *asciidoctor.Options { return &asciidoctor.Options{} }, "42", ""},
		{"converter to file", "42", func(dir string, _ *bytes.Buffer) *asciidoctor.Options {
			return &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, ToFile: filepath.Join(dir, "output")}
		}, "", "file"},
		{"converter to stream", "42", func(_ string, s *bytes.Buffer) *asciidoctor.Options { return &asciidoctor.Options{Output: s} }, "", "stream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newInstance(t)
			mustRegisterConverter(t, a, ajObjectConverter{}, nil, "42")
			dir := t.TempDir()
			var stream bytes.Buffer
			o := tc.options(dir, &stream)
			o.Backend = tc.backend
			out := mustConvert(t, a, ajSampleContent, o)
			switch tc.result {
			case "nonempty":
				if out == "" {
					t.Error("empty result")
				}
			default:
				eqv(t, "result", out, tc.result)
			}
			var written string
			switch tc.written {
			case "file":
				written = readFile(t, filepath.Join(dir, "output"))
			case "stream":
				written = stream.String()
			default:
				return
			}
			if written == "" {
				t.Error("nothing written")
			}
			if tc.backend == "42" {
				eqv(t, "written", written, "42")
			}
		})
	}
}

// Ported from AsciidoctorJ's SectionImplTest.

const ajSectionsSample = "= Document Title\n\n== Section A\n\nSection A paragraph.\n\n=== Section A Subsection\n\nSection A 'subsection' paragraph.\n\n"

func sectionAtLevel(t *testing.T, sectnums bool, level int) *asciidoctor.Section {
	t.Helper()
	d := mustLoad(t, ajSectionsSample, &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.SectionNumbers(sectnums)})
	sections, err := d.FindBy(asciidoctor.Selector{Context: "section"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sections {
		if s.Level() == level {
			return s.(*asciidoctor.Section)
		}
	}
	t.Fatalf("no section at level %d", level)
	return nil
}

func TestAJSectnum(t *testing.T) {
	for _, tc := range []struct {
		sectnums        bool
		level           int
		sectnum         string
		delimiter, with string
		numbered        bool
	}{
		{true, 1, "1.", "_", "1_", true},
		{true, 2, "1.1.", "*", "1*1*", true},
		{false, 1, ".", "_", "_", false},
		{false, 2, "..", "*", "**", false},
	} {
		s := sectionAtLevel(t, tc.sectnums, tc.level)
		eqv(t, "sectnum", s.Sectnum(), tc.sectnum)
		eqv(t, "sectnum with delimiter", s.SectnumWith(tc.delimiter, ""), tc.with)
		eqv(t, "numbered", s.IsNumbered(), tc.numbered)
	}
}

func TestAJSectionNumberedByDoctype(t *testing.T) {
	for _, doctype := range []string{"article", "book"} {
		for _, sectnums := range []bool{true, false} {
			src := "= Sample Document\n:doctype: " + doctype + "\n"
			if sectnums {
				src += ":sectnums:"
			}
			src += "\n\n== Test\n\nTest"
			s := mustLoad(t, src, nil).Blocks()[0].(*asciidoctor.Section)
			eqv(t, doctype+" numbered", s.IsNumbered(), sectnums)
		}
	}
}

// Ported from AsciidoctorJ's WhenSourceHighlightingIsUsed.

const ajJavaSource = "[source,java]\n----\npublic class Main {\n  public static void main(String[] args) {\n    println(\"Hello World\")\n  }\n}\n----\n"

// go-rouge (v0.3.0) has no Java lexer, so the Rouge case also runs on Ruby
// source, which it lexes.
const ajRubySource = "[source,ruby]\n----\ndef greet(name)\n  puts \"Hello, #{name}\"\nend\n----\n"

func TestAJSourceHighlighting(t *testing.T) {
	for _, tc := range []struct {
		name, highlighter, source, selector, css, skip string
	}{
		{"rouge java", "rouge", ajJavaSource, "pre.rouge span.kd", "pre.rouge .kd",
			"go-rouge v0.3.0 has no Java lexer"},
		{"rouge ruby", "rouge", ajRubySource, "pre.rouge span.k", "pre.rouge .k", ""},
		{"coderay java", "coderay", ajJavaSource, "pre.CodeRay span.class", ".CodeRay .class", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip != "" {
				t.Skip(tc.skip)
			}
			out := mustConvert(t, instance(t), tc.source, &asciidoctor.Options{
				Standalone: asciidoctor.Bool(true), Safe: asciidoctor.SafeModeUnsafe,
				Attributes: asciidoctor.Attributes{}.SourceHighlighter(tc.highlighter),
			})
			if len(parseHTML(t, out).Select(tc.selector)) == 0 {
				t.Errorf("no %s elements", tc.selector)
			}
			if !strings.Contains(out, tc.css) {
				t.Errorf("stylesheet lacks %s", tc.css)
			}
		})
	}
}

// Ported from AsciidoctorJ's ContextChangeTest.
func TestAJContextChange(t *testing.T) {
	d := mustLoad(t, "= Document Title\n\n== Section A\n\n. This it item 1 in an ordered list\n\n"+
		". This is item 2 in an ordered list\n\n. This is item 3 in and ordered list\n\n",
		&asciidoctor.Options{Attributes: asciidoctor.Attributes{}.SectionNumbers(false)})
	eqv(t, "blocks", len(d.Blocks()), 1)
	list := d.Blocks()[0].Blocks()[0]
	eqv(t, "context", list.Context(), "olist")
	list.SetContext("colist")
	eqv(t, "changed context", list.Context(), "colist")
}
