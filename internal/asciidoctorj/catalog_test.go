package asciidoctorj

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenReadingImagesFromCatalogAsset,
// WhenReadingLinksFromCatalogAsset and
// WhenReadingImagesFromCatalogAssetFromConverter.

var (
	ajBlockImages = []asciidoctor.ImageReference{{Target: "images/block-image.jpg"}}
	ajAllImages   = []asciidoctor.ImageReference{{Target: "images/block-image.jpg"}, {Target: "images/inline-image.png"}}
	ajAllLinks    = []string{"https://docs.asciidoctor.org", "https://github.com", "https://some.rangomsite.org", "downloads/report.pdf"}
)

func sortedImages(images []asciidoctor.ImageReference) []asciidoctor.ImageReference {
	out := append([]asciidoctor.ImageReference(nil), images...)
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}

func sortedStrings(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func catalogOptions() *asciidoctor.Options { return &asciidoctor.Options{CatalogAssets: true} }

func mustConvertDocument(t *testing.T, a *asciidoctor.Asciidoctor, src string, o *asciidoctor.Options) *asciidoctor.Document {
	t.Helper()
	d, _, err := a.ConvertDocument(src, o)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mustConvertFileDocument(t *testing.T, a *asciidoctor.Asciidoctor, path string, o *asciidoctor.Options) *asciidoctor.Document {
	t.Helper()
	d, _, err := a.ConvertFileDocument(path, o)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAJCatalogImages(t *testing.T) {
	images := readFile(t, filepath.Join(ajResourceDir, "sample-with-images.adoc"))
	t.Run("none", func(t *testing.T) {
		eqv(t, "images", len(mustLoad(t, "= Hello", catalogOptions()).Catalog().Images), 0)
	})
	t.Run("block images when loaded", func(t *testing.T) {
		eqv(t, "images", mustLoad(t, images, catalogOptions()).Catalog().Images, ajBlockImages)
	})
	t.Run("imagesdir", func(t *testing.T) {
		o := catalogOptions()
		o.Attributes = asciidoctor.Attributes{}.ImagesDir("some-path")
		eqv(t, "images", mustLoad(t, images, o).Catalog().Images,
			[]asciidoctor.ImageReference{{Target: "images/block-image.jpg", Imagesdir: "some-path"}})
	})
	t.Run("inline images after content", func(t *testing.T) {
		d := mustLoad(t, images, catalogOptions())
		if _, err := d.Content(); err != nil {
			t.Fatal(err)
		}
		eqv(t, "images", sortedImages(d.Catalog().Images), ajAllImages)
	})
	t.Run("catalog assets off", func(t *testing.T) {
		d := mustLoad(t, images, &asciidoctor.Options{})
		if _, err := d.Content(); err != nil {
			t.Fatal(err)
		}
		eqv(t, "images", len(d.Catalog().Images), 0)
	})
	t.Run("convert file", func(t *testing.T) {
		o := catalogOptions()
		o.Safe, o.ToFile = asciidoctor.SafeModeUnsafe, filepath.Join(t.TempDir(), "output")
		d := mustConvertFileDocument(t, newInstance(t), ajResource(t, "sample-with-images.adoc"), o)
		eqv(t, "images", sortedImages(d.Catalog().Images), ajAllImages)
	})
	t.Run("convert", func(t *testing.T) {
		o := catalogOptions()
		o.Safe, o.ToFile = asciidoctor.SafeModeUnsafe, filepath.Join(t.TempDir(), "output")
		eqv(t, "images", sortedImages(mustConvertDocument(t, newInstance(t), images, o).Catalog().Images), ajAllImages)
	})
	t.Run("convert to string", func(t *testing.T) {
		d, out, err := newInstance(t).ConvertDocument(images, catalogOptions())
		if err != nil {
			t.Fatal(err)
		}
		if out == "" {
			t.Error("no output")
		}
		eqv(t, "images", sortedImages(d.Catalog().Images), ajAllImages)
	})
}

func TestAJCatalogLinks(t *testing.T) {
	links := readFile(t, filepath.Join(ajResourceDir, "sample-with-links.adoc"))
	t.Run("none", func(t *testing.T) {
		eqv(t, "links", len(mustLoad(t, "= Hello", catalogOptions()).Catalog().Links), 0)
	})
	t.Run("not converted", func(t *testing.T) {
		eqv(t, "links", len(mustLoad(t, links, catalogOptions()).Catalog().Links), 0)
	})
	t.Run("catalog assets off", func(t *testing.T) {
		d := mustConvertFileDocument(t, newInstance(t), ajResource(t, "sample-with-links.adoc"), &asciidoctor.Options{})
		eqv(t, "links", len(d.Catalog().Links), 0)
	})
	t.Run("convert file", func(t *testing.T) {
		d := mustConvertFileDocument(t, newInstance(t), ajResource(t, "sample-with-links.adoc"), catalogOptions())
		eqv(t, "links", sortedStrings(d.Catalog().Links), sortedStrings(ajAllLinks))
	})
	t.Run("convert", func(t *testing.T) {
		o := catalogOptions()
		o.Safe, o.ToFile = asciidoctor.SafeModeUnsafe, filepath.Join(t.TempDir(), "output")
		eqv(t, "links", sortedStrings(mustConvertDocument(t, newInstance(t), links, o).Catalog().Links), sortedStrings(ajAllLinks))
	})
}

func TestAJCatalogImagesFromConverter(t *testing.T) {
	images := readFile(t, filepath.Join(ajResourceDir, "sample-with-images.adoc"))
	setup := func(t *testing.T) (*asciidoctor.Asciidoctor, *[]asciidoctor.ImageReference, *[]asciidoctor.ImageReference, *bool) {
		a := newInstance(t)
		var before, after []asciidoctor.ImageReference
		called := false
		mustRegisterConverter(t, a, asciidoctor.ConverterFunc(func(node asciidoctor.Node, _ string, _ map[string]any) (string, error) {
			var content any
			var err error
			switch n := node.(type) {
			case *asciidoctor.Document:
				called = true
				before = n.Catalog().Images
				content, err = n.Content()
				after = n.Catalog().Images
			case asciidoctor.StructuralNode:
				content, err = n.Content()
			}
			s, _ := content.(string)
			return s, err
		}), nil, "custom-backend")
		return a, &before, &after, &called
	}
	o := &asciidoctor.Options{CatalogAssets: true, Backend: "custom-backend"}
	t.Run("no images", func(t *testing.T) {
		a, before, after, _ := setup(t)
		mustConvert(t, a, "", o)
		eqv(t, "before", len(*before), 0)
		eqv(t, "after", len(*after), 0)
	})
	t.Run("load does not convert", func(t *testing.T) {
		a, _, _, called := setup(t)
		if _, err := a.Load(images, o); err != nil {
			t.Fatal(err)
		}
		eqv(t, "converter called", *called, false)
	})
	t.Run("all images", func(t *testing.T) {
		a, before, after, _ := setup(t)
		mustConvert(t, a, images, o)
		eqv(t, "before", *before, ajBlockImages)
		eqv(t, "after", *after, ajAllImages)
	})
}
