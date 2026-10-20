// Package apitest tests the public API of package asciidoctor.
package apitest

import (
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
	"github.com/asciidoctor-go/asciidoctor/internal/testutil"
)

// The helpers shared with the other test packages, from internal/testutil.

func TestMain(m *testing.M) { testutil.Main(m) }

func instance(t *testing.T) *asciidoctor.Asciidoctor { t.Helper(); return testutil.Instance(t) }

func newInstance(t *testing.T, opts ...asciidoctor.Option) *asciidoctor.Asciidoctor {
	t.Helper()
	return testutil.NewInstance(t, opts...)
}

func mustConvert(t *testing.T, a *asciidoctor.Asciidoctor, src string, o *asciidoctor.Options) string {
	t.Helper()
	return testutil.MustConvert(t, a, src, o)
}

func writeFile(t *testing.T, path, content string) { t.Helper(); testutil.WriteFile(t, path, content) }

func readFile(t *testing.T, path string) string { t.Helper(); return testutil.ReadFile(t, path) }

func eqv[T any](t *testing.T, what string, got, want T) {
	t.Helper()
	testutil.Equal(t, what, got, want)
}
