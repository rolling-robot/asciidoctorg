package asciidoctorj

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
	"github.com/asciidoctor-go/asciidoctor/internal/testutil"
)

// Ported from AsciidoctorJ's WhenCustomTemplatesAreUsed,
// WhenMultipleDocumentsAreRendered, WhenUserRequiresTheAsciidoctorRuntimeVersion,
// WhenAnAsciidoctorJInstanceIsRequired and WhenDocumentIsRenderedWithPreloading.
//
// AsciidoctorJ's GEM_PATH, class loader and unusual-package tests concern
// JRuby's gem paths and Java class loading; an embedded rbgo runtime loads
// only its embedded libraries, so they have no counterpart.

func TestAJCustomTemplates(t *testing.T) {
	for _, dir := range []string{"haml/html5-tweaks", "slim"} {
		t.Run(dir, func(t *testing.T) {
			res := ajResources(t)
			out := mustConvertFile(t, newInstance(t), filepath.Join(res, "rendersample.asciidoc"), &asciidoctor.Options{
				TemplateDirs: []string{filepath.Join(res, "src", "custom-backends", filepath.FromSlash(dir))},
				NoOutputFile: true,
			})
			// The Haml template wraps the content in div.content; the Slim
			// one replaces each paragraph with static text.
			if dir == "slim" {
				if !strings.Contains(out, "<p>This is static content</p>") || strings.Contains(out, "paragraph.") {
					t.Errorf("slim template not used: %s", out)
				}
			} else if parseHTML(t, out).Select("div.content").First() == nil {
				t.Errorf("no div.content: %s", out)
			}
		})
	}
}

func TestAJSameDocument100Times(t *testing.T) {
	a := instance(t)
	src := readFile(t, filepath.Join(ajResourceDir, "documenttitle.adoc"))
	var out string
	for range 100 {
		out = mustConvert(t, a, src, &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe})
	}
	if !strings.Contains(out, "content") {
		t.Errorf("output = %q", out)
	}
}

func TestAJRuntimeVersion(t *testing.T) {
	v, err := instance(t).AsciidoctorVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v == "" {
		t.Error("empty version")
	}
}

func TestAJFreshInstanceRubyExtension(t *testing.T) {
	a := newInstance(t)
	if err := a.LoadRubySource(readFile(t, filepath.Join(ajResourceDir, "ruby-extensions", "YellRubyBlock.rb")), "YellRubyBlock.rb"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Extensions().RubyProcessor(asciidoctor.KindBlock, "YellRubyBlock", "yell"); err != nil {
		t.Fatal(err)
	}
	if out := mustConvert(t, a, "[yell]\nHello World", &asciidoctor.Options{Standalone: asciidoctor.Bool(false)}); !strings.Contains(out, "HELLO WORLD") {
		t.Errorf("output = %s", out)
	}
}

// AsciidoctorJ preloads the gems an option needs (coderay, erubis, tilt,
// base64); here each library is embedded, and these tests check that the
// option works.
func TestAJOptionalLibraries(t *testing.T) {
	t.Run("coderay", func(t *testing.T) {
		out := mustConvert(t, newInstance(t), ajJavaSource, &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.SourceHighlighter("coderay")})
		if !strings.Contains(out, `class="CodeRay`) {
			t.Errorf("not highlighted: %s", out)
		}
	})
	t.Run("pygments falls back to plain", func(t *testing.T) {
		out := mustConvert(t, newInstance(t, asciidoctor.WithStderr(&strings.Builder{})), ajJavaSource,
			&asciidoctor.Options{Attributes: asciidoctor.Attributes{}.SourceHighlighter("pygments")})
		if strings.Contains(out, `class="CodeRay`) || !strings.Contains(out, "public class Main") {
			t.Errorf("output = %s", out)
		}
	})
	for _, eruby := range []string{"erb", "erubi"} {
		t.Run("eruby "+eruby, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "paragraph.html.erb"), `<p class="from-<%= "erb" %>"><%= content %></p>`)
			out := mustConvert(t, newInstance(t), "text", &asciidoctor.Options{TemplateDirs: []string{dir}, ERuby: eruby})
			eqv(t, "output", out, `<p class="from-erb">text</p>`)
		})
	}
	t.Run("data-uri", func(t *testing.T) {
		dir := t.TempDir()
		gif, err := os.ReadFile(testutil.Path("rubysrc/asciidoctor/test/fixtures/dot.gif"))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, "dot.gif"), string(gif))
		out := mustConvert(t, newInstance(t), "image::dot.gif[]", &asciidoctor.Options{
			Safe: asciidoctor.SafeModeSafe, BaseDir: dir, Attributes: asciidoctor.Attributes{}.DataURI(true),
		})
		want := "data:image/gif;base64," + base64.StdEncoding.EncodeToString(gif)
		if !strings.Contains(out, want) {
			t.Errorf("output lacks the embedded image: %s", out)
		}
	})
}
