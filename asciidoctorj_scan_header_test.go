package asciidoctor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenGlobExpressionIsUsedForScanning,
// WhenDirectoriesWithAsciidocFilesAreScanned and WhenDocumentHeaderIsRequired.

func TestAJGlobScanning(t *testing.T) {
	src := filepath.Join(ajResources(t), "src")
	docs := filepath.Join(src, "documents")
	samples := []string{filepath.Join(docs, "_sample.adoc"), filepath.Join(docs, "sample.adoc")}
	for _, tc := range []struct {
		name, glob string
		want       []string
	}{
		{"current directory", filepath.Join(src, "documents/*.adoc"), samples},
		{"recursive", filepath.Join(src, "**/*.adoc"), samples},
		{"no match", filepath.Join(src, "**/*.a"), nil},
		{"file in root directory", "/this_file_does_not_exist.adoc", []string{filepath.FromSlash("/this_file_does_not_exist.adoc")}},
		{"file in working directory", "this_file_does_not_exist.adoc", []string{"this_file_does_not_exist.adoc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := asciidoctor.GlobDirectoryWalker{Glob: tc.glob}.Scan()
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			eqv(t, "files", got, tc.want)
		})
	}
}

func TestAJDirectoryScanning(t *testing.T) {
	t.Run("only asciidoc files", func(t *testing.T) {
		docs := filepath.Join(ajResources(t), "src", "documents")
		got, err := asciidoctor.AsciiDocDirectoryWalker{BaseDir: docs}.Scan()
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for _, name := range []string{"sample.ad", "sample.adoc", "sample.asc", "sample.asciidoc"} {
			want = append(want, filepath.Join(docs, name))
		}
		eqv(t, "files", got, want)
	})
	t.Run("empty directory", func(t *testing.T) {
		got, err := asciidoctor.AsciiDocDirectoryWalker{BaseDir: t.TempDir()}.Scan()
		if err != nil {
			t.Fatal(err)
		}
		eqv(t, "files", len(got), 0)
	})
	t.Run("missing directory", func(t *testing.T) {
		got, err := asciidoctor.AsciiDocDirectoryWalker{BaseDir: filepath.Join(t.TempDir(), "my_udirectory")}.Scan()
		if err != nil {
			t.Fatal(err)
		}
		eqv(t, "files", len(got), 0)
	})
}

func TestAJDocumentHeader(t *testing.T) {
	path := ajResource(t, "documentheaders.asciidoc")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	d := mustLoadFile(t, path, &asciidoctor.Options{ParseHeaderOnly: true})
	t.Run("title and attributes", func(t *testing.T) {
		eqv(t, "doctitle", d.Doctitle(), "Sample Document")
		eqv(t, "main title", d.StructuredDoctitle(false).Main, "Sample Document")
		attrs := d.Attributes()
		for name, want := range map[string]string{
			"revdate": "2013-05-20", "revnumber": "1.0", "revremark": "First draft",
			"tags": "[document, example]", "author": "Doc Writer", "email": "doc.writer@asciidoc.org",
		} {
			eqv(t, name, attrs[name], any(want))
		}
	})
	t.Run("authors", func(t *testing.T) {
		eqv(t, "authors", d.Authors(), []asciidoctor.Author{
			{Name: "Doc Writer", FirstName: "Doc", LastName: "Writer", Initials: "DW", Email: "doc.writer@asciidoc.org"},
			{Name: "John Smith", FirstName: "John", LastName: "Smith", Initials: "JS", Email: "john.smith@asciidoc.org"},
		})
	})
	t.Run("revision", func(t *testing.T) {
		eqv(t, "revision", d.RevisionInfo(), asciidoctor.RevisionInfo{Date: "2013-05-20", Number: "1.0", Remark: "First draft"})
	})
}
