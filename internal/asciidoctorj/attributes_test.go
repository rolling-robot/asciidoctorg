package asciidoctorj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenAttributesAreUsedInAsciidoctor.

func unsafeToDir(dir string, attrs asciidoctor.Attributes) *asciidoctor.Options {
	return &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, ToDir: dir, Attributes: attrs}
}

func mustConvertFile(t *testing.T, a *asciidoctor.Asciidoctor, path string, o *asciidoctor.Options) string {
	t.Helper()
	out, err := a.ConvertFile(path, o)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestAJQualifiedURLWithHideURIScheme(t *testing.T) {
	out := mustConvert(t, instance(t), "The AsciiDoc project is located at https://asciidoc.org.",
		&asciidoctor.Options{Attributes: asciidoctor.Attributes{}.HideURIScheme(true)})
	if got := text(parseHTML(t, out).Select("a").First()); got != "asciidoc.org" {
		t.Errorf("link text = %q", got)
	}
}

func TestAJCompatModeLegacy(t *testing.T) {
	out := mustConvert(t, instance(t), "The `AsciiDoc {version}` project.",
		&asciidoctor.Options{Attributes: asciidoctor.Attributes{}.Set("version", "1.0.0").CompatMode(asciidoctor.CompatModeLegacy)})
	if got := text(parseHTML(t, out).Select("code").First()); !strings.Contains(got, "{version}") {
		t.Errorf("code = %q, want literal {version}", got)
	}
}

func TestAJNoCompatMode(t *testing.T) {
	out := mustConvert(t, instance(t), "The `AsciiDoc {version}` project.",
		&asciidoctor.Options{Attributes: asciidoctor.Attributes{}.Set("version", "1.0.0").CompatMode(asciidoctor.CompatModeDefault)})
	if got := text(parseHTML(t, out).Select("code").First()); !strings.Contains(got, "1.0.0") {
		t.Errorf("code = %q, want 1.0.0", got)
	}
}

func TestAJCacheURI(t *testing.T) {
	out := mustConvert(t, instance(t), "read my lips", &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.CacheURI(true)})
	if !strings.Contains(out, "read my lips") {
		t.Errorf("output = %q", out)
	}
}

func TestAJAsciiMathDelimiters(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "math.asciidoc"), unsafeToDir(dir, asciidoctor.Attributes{}.Math("asciimath")))
	doc := parseHTMLFile(t, filepath.Join(dir, "math.html"))
	if n := len(doc.Select(`[type="text/x-mathjax-config"]`)); n != 1 {
		t.Errorf("mathjax config scripts = %d, want 1", n)
	}
}

func TestAJAppendixCaption(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "appendix.asciidoc"), unsafeToDir(dir, asciidoctor.Attributes{}.AppendixCaption("App")))
	doc := parseHTMLFile(t, filepath.Join(dir, "appendix.html"))
	if got := text(doc.ByID("_attribute_options")); !strings.HasPrefix(got, "App") {
		t.Errorf("appendix title = %q", got)
	}
}

func TestAJHardbreaks(t *testing.T) {
	out := mustConvert(t, instance(t), "read\nmy\nlips", &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.Hardbreaks(true)})
	if n := len(parseHTML(t, out).Select(`[class="paragraph"] br`)); n != 2 {
		t.Errorf("br count = %d, want 2", n)
	}
}

func TestAJSectNumLevels(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "multiple_levels.asciidoc"),
		unsafeToDir(dir, asciidoctor.Attributes{}.SectionNumbers(true).SectNumLevels(2)))
	doc := parseHTMLFile(t, filepath.Join(dir, "multiple_levels.html"))
	for id, check := range map[string]func(string) bool{
		"_level_1": func(s string) bool { return strings.HasPrefix(s, "1.") },
		"_level_2": func(s string) bool { return strings.HasPrefix(s, "1.1.") },
		"_level_3": func(s string) bool { return !strings.HasPrefix(s, "1.1.1.") },
		"_level_4": func(s string) bool { return !strings.HasPrefix(s, "1.1.1.1.") },
	} {
		if got := text(doc.ByID(id)); !check(got) {
			t.Errorf("%s = %q", id, got)
		}
	}
}

func TestAJNoFooter(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), unsafeToDir(dir, asciidoctor.Attributes{}.NoFooter(true)))
	if doc := parseHTMLFile(t, filepath.Join(dir, "rendersample.html")); doc.ByID("footer") != nil {
		t.Error("footer present")
	}
}

func TestAJShowTitle(t *testing.T) {
	for _, tc := range []struct {
		name       string
		attrs      asciidoctor.Attributes
		standalone bool
		want       int
	}{
		{"true embedded", asciidoctor.Attributes{}.ShowTitle(true), false, 1},
		{"false then true embedded", asciidoctor.Attributes{}.ShowTitle(false).ShowTitle(true), false, 1},
		{"false embedded", asciidoctor.Attributes{}.ShowTitle(false), false, 0},
		{"true then false embedded", asciidoctor.Attributes{}.ShowTitle(true).ShowTitle(false), false, 0},
		{"true standalone", asciidoctor.Attributes{}.ShowTitle(true), true, 1},
		{"false standalone", asciidoctor.Attributes{}.ShowTitle(false), true, 0},
		{"true then false standalone", asciidoctor.Attributes{}.ShowTitle(true).ShowTitle(false), true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), &asciidoctor.Options{
				Attributes: tc.attrs, NoOutputFile: true, Standalone: asciidoctor.Bool(tc.standalone),
			})
			h1 := parseHTML(t, out).Select("h1")
			if len(h1) != tc.want {
				t.Fatalf("h1 count = %d, want %d", len(h1), tc.want)
			}
			if tc.want == 1 && text(h1[0]) != "Document Title" {
				t.Errorf("title = %q", text(h1[0]))
			}
		})
	}
}

func TestAJSourceHighlighterAddsLibraries(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), unsafeToDir(dir, asciidoctor.Attributes{}.SourceHighlighter("prettify")))
	links := parseHTMLFile(t, filepath.Join(dir, "rendersample.html")).Select("link")
	if len(links) < 2 || !strings.Contains(attr(links[1], "href"), "prettify.min.css") {
		t.Errorf("links = %d, second href = %q", len(links), attr(links[1], "href"))
	}
}

// assertEmbeddedCSS checks that the stylesheet is embedded, not linked.
func assertEmbeddedCSS(t *testing.T, path string) {
	t.Helper()
	doc := parseHTMLFile(t, path)
	if doc.Select("style").HTML() == "" {
		t.Error("no embedded stylesheet")
	}
	if got := doc.Select("link").HTML(); got != "" {
		t.Errorf("links have content %q", got)
	}
}

func TestAJEmbedsCSSByDefault(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), unsafeToDir(dir, nil))
	assertEmbeddedCSS(t, filepath.Join(dir, "rendersample.html"))
}

func TestAJSkipFrontMatter(t *testing.T) {
	out := mustConvertFile(t, instance(t), ajResource(t, "renderwithfrontmatter.adoc"),
		&asciidoctor.Options{NoOutputFile: true, Attributes: asciidoctor.Attributes{}.SkipFrontMatter(true)})
	if n := len(parseHTML(t, out).Select("hr")); n != 0 {
		t.Errorf("hr count = %d, want 0", n)
	}
}

func TestAJSectAnchors(t *testing.T) {
	out := mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"),
		&asciidoctor.Options{NoOutputFile: true, Attributes: asciidoctor.Attributes{}.SectAnchors(true)})
	if got := attr(parseHTML(t, out).Select(`a[class="anchor"]`).First(), "href"); got != "#_section_a" {
		t.Errorf("anchor href = %q", got)
	}
}

func TestAJIgnoreUndefinedAttributes(t *testing.T) {
	out := mustConvertFile(t, instance(t), ajResource(t, "documentwithundefinedattribute.asciidoc"),
		&asciidoctor.Options{NoOutputFile: true, Attributes: asciidoctor.Attributes{}.IgnoreUndefinedAttributes(true)})
	if !strings.Contains(out, "{bogus-attribute}") {
		t.Errorf("output lacks {bogus-attribute}: %s", out)
	}
}

func TestAJTOC(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "tocsample.asciidoc"), unsafeToDir(dir, asciidoctor.Attributes{}.TOC(true)))
	if !parseHTMLFile(t, filepath.Join(dir, "tocsample.html")).Select("div.toc").HasClass("toc") {
		t.Error("no toc")
	}
}

func TestAJTOCPlacement(t *testing.T) {
	for _, tc := range []struct {
		placement asciidoctor.Placement
		file      string
		selector  string
	}{
		{asciidoctor.PlacementLeft, "tocsample", "body.toc-left > div#header > div#toc"},
		{asciidoctor.PlacementRight, "tocsample", "body.toc-right > div#header > div#toc"},
		{asciidoctor.PlacementPreamble, "tocsample", "body.article > div#content > div#preamble > div#toc"},
		{asciidoctor.PlacementMacro, "tocsamplemacro", "body.article > div#content > div.sect1 > div.sectionbody > div#toc"},
	} {
		t.Run(string(tc.placement), func(t *testing.T) {
			dir := t.TempDir()
			mustConvertFile(t, instance(t), ajResource(t, tc.file+".asciidoc"),
				unsafeToDir(dir, asciidoctor.Attributes{}.TableOfContents(tc.placement)))
			if n := len(parseHTMLFile(t, filepath.Join(dir, tc.file+".html")).Select(tc.selector)); n != 1 {
				t.Errorf("%s matches %d elements, want 1", tc.selector, n)
			}
		})
	}
}

func TestAJAttributeMissingDropLine(t *testing.T) {
	out := mustConvert(t, instance(t), "This is\nblah blah {foobarbaz}\nall there is.",
		&asciidoctor.Options{Attributes: asciidoctor.Attributes{}.AttributeMissing("drop-line")})
	if strings.Contains(out, "{foobarbaz}") {
		t.Errorf("line not dropped: %s", out)
	}
}

func TestAJAttributeUndefinedDrop(t *testing.T) {
	out := mustConvert(t, instance(t), ":foo:\n\n{set:foo!}\n{foo}yes",
		&asciidoctor.Options{Attributes: asciidoctor.Attributes{}.AttributeUndefined("drop")})
	if got := text(parseHTML(t, out).Select("p").First()); got != "{foo}yes" {
		t.Errorf("paragraph = %q", got)
	}
}

func TestAJTOCPlaceableToFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "toc2sample.html")
	mustConvertFile(t, instance(t), ajResource(t, "toc2sample.asciidoc"), &asciidoctor.Options{
		Safe: asciidoctor.SafeModeUnsafe, ToFile: out, Attributes: asciidoctor.Attributes{}.TableOfContents(asciidoctor.PlacementRight),
	})
	classes := strings.Fields(parseHTMLFile(t, out).Select("body").Attr("class"))
	for _, want := range []string{"toc2", "toc-right"} {
		found := false
		for _, c := range classes {
			found = found || c == want
		}
		if !found {
			t.Errorf("body classes %v lack %s", classes, want)
		}
	}
}

func TestAJUnsetLinkCSSArgument(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), unsafeToDir(dir, asciidoctor.Attributes{}.Arguments("linkcss!")))
	assertEmbeddedCSS(t, filepath.Join(dir, "rendersample.html"))
}

func TestAJTOCAndSectnumsFromArguments(t *testing.T) {
	for name, attrs := range map[string]asciidoctor.Attributes{
		"string": asciidoctor.Attributes{}.Arguments("toc sectnums"),
		"array":  asciidoctor.Attributes{}.Arguments("toc", "sectnums"),
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mustConvertFile(t, instance(t), ajResource(t, "tocsample.asciidoc"), unsafeToDir(dir, attrs))
			doc := parseHTMLFile(t, filepath.Join(dir, "tocsample.html"))
			if !doc.Select("div.toc").HasClass("toc") {
				t.Error("no toc")
			}
			if got := text(doc.Select(`a[href="#_first_chapter"]`).First()); !strings.HasPrefix(got, "1.") {
				t.Errorf("toc entry = %q", got)
			}
		})
	}
}

func TestAJTOCDisabled(t *testing.T) {
	out := mustConvertFile(t, instance(t), ajResource(t, "tocsample.asciidoc"),
		&asciidoctor.Options{NoOutputFile: true, Attributes: asciidoctor.Attributes{}.TOC(false)})
	if parseHTML(t, out).Select("div.toc").HasClass("toc") {
		t.Error("toc generated")
	}
}

func TestAJLinkedStylesheet(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs asciidoctor.Attributes
		want  string
	}{
		{"custom name", asciidoctor.Attributes{}.LinkCSS(true).StylesheetName("mycustom.css"), "./mycustom.css"},
		{"unset", asciidoctor.Attributes{}.UnsetStylesheet(), ""},
		{"styles dir", asciidoctor.Attributes{}.StylesDir("./styles").LinkCSS(true).StylesheetName("mycustom.css"), "./styles/mycustom.css"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			o := unsafeToDir(dir, tc.attrs)
			o.MkDirs = true
			mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), o)
			if got := parseHTMLFile(t, filepath.Join(dir, "rendersample.html")).Select("link[href]").Attr("href"); got != tc.want {
				t.Errorf("href = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAJLinkCSSFalseEmbeds(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), unsafeToDir(dir, asciidoctor.Attributes{}.LinkCSS(false)))
	assertEmbeddedCSS(t, filepath.Join(dir, "rendersample.html"))
}

func TestAJLinkCSSLinksDefaultStylesheet(t *testing.T) {
	dir := t.TempDir()
	mustConvertFile(t, instance(t), ajResource(t, "rendersample.asciidoc"), unsafeToDir(dir, asciidoctor.Attributes{}.LinkCSS(true)))
	links := parseHTMLFile(t, filepath.Join(dir, "rendersample.html")).Select("link[href]")
	if len(links) < 2 || attr(links[1], "href") != "./asciidoctor.css" {
		t.Errorf("stylesheet links: %d", len(links))
	}
}

func TestAJCopyCSSInPlace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs asciidoctor.Attributes
		want  bool
	}{
		{"linkcss copycss", asciidoctor.Attributes{}.LinkCSS(true).CopyCSS(true), true},
		{"copycss unset", asciidoctor.Attributes{}.CopyCSS(false), false},
		{"copycss without linkcss", asciidoctor.Attributes{}.CopyCSS(true).LinkCSS(false), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := ajResource(t, "rendersample.asciidoc")
			mustConvertFile(t, instance(t), src, &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, Attributes: tc.attrs})
			if got := fileExists(filepath.Join(filepath.Dir(src), "asciidoctor.css")); got != tc.want {
				t.Errorf("asciidoctor.css exists = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAJCopyCSSToOutputDirectory(t *testing.T) {
	attrs := func() asciidoctor.Attributes { return asciidoctor.Attributes{}.LinkCSS(true).CopyCSS(true) }
	for _, tc := range []struct {
		name    string
		file    bool
		options func(dir string) *asciidoctor.Options
	}{
		{"file to file", true, func(dir string) *asciidoctor.Options {
			return &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, ToFile: filepath.Join(dir, "output.html"), Attributes: attrs()}
		}},
		{"file to dir", true, func(dir string) *asciidoctor.Options { return unsafeToDir(dir, attrs()) }},
		{"string to file", false, func(dir string) *asciidoctor.Options {
			return &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, ToFile: filepath.Join(dir, "output.html"), Attributes: attrs()}
		}},
		{"string to dir", false, func(dir string) *asciidoctor.Options { return unsafeToDir(dir, attrs()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var err error
			if tc.file {
				_, err = instance(t).ConvertFile(ajResource(t, "rendersample.asciidoc"), tc.options(dir))
			} else {
				_, err = instance(t).Convert("This is Asciidoctor", tc.options(dir))
			}
			if err != nil {
				t.Fatal(err)
			}
			if !fileExists(filepath.Join(dir, "asciidoctor.css")) {
				t.Error("asciidoctor.css not copied")
			}
		})
	}
}

func TestAJImageIcons(t *testing.T) {
	src := readFile(t, filepath.Join(ajResourceDir, "documentwithnote.asciidoc"))
	out := mustConvert(t, instance(t), src, &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.Icons(asciidoctor.IconsImage)})
	if len(parseHTML(t, out).XPath("//img[@alt='Note']")) == 0 {
		t.Errorf("no note icon image: %s", out)
	}
}

func TestAJFontIcons(t *testing.T) {
	src := readFile(t, filepath.Join(ajResourceDir, "documentwithnote.asciidoc"))
	out := mustConvert(t, instance(t), src, &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.Icons(asciidoctor.IconsFont)})
	if len(parseHTML(t, out).XPath("//i[@class='fa icon-note']")) == 0 {
		t.Errorf("no note font icon: %s", out)
	}
}

func TestAJIconsDir(t *testing.T) {
	src := readFile(t, filepath.Join(ajResourceDir, "documentwithnote.asciidoc"))
	out := mustConvert(t, instance(t), src, &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.Icons(asciidoctor.IconsImage).IconsDir("icons")})
	if got := parseHTML(t, out).Select("img[src]").Attr("src"); got != "icons/note.png" {
		t.Errorf("icon src = %q", got)
	}
}

func TestAJLinkAttrs(t *testing.T) {
	out := mustConvert(t, instance(t), `https://google.com[Google, window="_blank"]`,
		&asciidoctor.Options{Attributes: asciidoctor.Attributes{}.LinkAttrs(true)})
	if got := parseHTML(t, out).Select("a[target]").Attr("target"); got != "_blank" {
		t.Errorf("target = %q", got)
	}
}

func TestAJExperimental(t *testing.T) {
	out := mustConvert(t, instance(t), "kbd:[F11]", &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.Experimental(true)})
	if got := parseHTML(t, out).Select("kbd").Text(); got != "F11" {
		t.Errorf("kbd = %q", got)
	}
}

func TestAJIconFontCDN(t *testing.T) {
	src := ajResource(t, "documentwithnote.asciidoc")
	mustConvertFile(t, instance(t), src, &asciidoctor.Options{Attributes: asciidoctor.Attributes{}.
		Icons(asciidoctor.IconsFont).IconFontRemote(true).IconFontCDN("http://mycdn/css/font-awesome.min.css")})
	doc := parseHTMLFile(t, filepath.Join(filepath.Dir(src), "documentwithnote.html"))
	if n := len(doc.Select(`link[href="http://mycdn/css/font-awesome.min.css"]`)); n != 1 {
		t.Errorf("CDN links = %d, want 1", n)
	}
}
