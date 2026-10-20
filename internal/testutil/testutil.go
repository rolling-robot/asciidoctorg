// Package testutil holds what the test packages of the public API share:
// the runtime directory and instances of a test binary, file helpers, and
// paths in the repository.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

var (
	runtimeDir string
	sharedOnce sync.Once
	shared     *asciidoctor.Asciidoctor
	sharedErr  error
)

// Main runs a test binary's tests (call it from TestMain) with a temporary
// runtime directory shared by its instances, removed afterwards.
func Main(m *testing.M) {
	dir, err := os.MkdirTemp("", "asciidoctor-go-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runtimeDir = dir
	code := m.Run()
	if shared != nil {
		shared.Close()
	}
	os.RemoveAll(dir)
	os.Exit(code)
}

// RuntimeDir is the runtime directory set up by Main.
func RuntimeDir() string { return runtimeDir }

// Instance returns an instance shared by the tests that do not change its
// configuration.
func Instance(t testing.TB) *asciidoctor.Asciidoctor {
	t.Helper()
	sharedOnce.Do(func() { shared, sharedErr = asciidoctor.New(asciidoctor.WithRuntimeDir(runtimeDir)) })
	if sharedErr != nil {
		t.Fatal(sharedErr)
	}
	return shared
}

// NewInstance returns a fresh instance, closed when the test ends.
func NewInstance(t testing.TB, opts ...asciidoctor.Option) *asciidoctor.Asciidoctor {
	t.Helper()
	a, err := asciidoctor.New(append([]asciidoctor.Option{asciidoctor.WithRuntimeDir(runtimeDir)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

// MustConvert converts src, failing the test on error.
func MustConvert(t testing.TB, a *asciidoctor.Asciidoctor, src string, o *asciidoctor.Options) string {
	t.Helper()
	out, err := a.Convert(src, o)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// WriteFile writes content to path, creating its directory.
func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ReadFile returns the content of path.
func ReadFile(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Equal reports got if it differs from want.
func Equal[T any](t testing.TB, what string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

// RepoRoot is the root directory of the repository.
func RepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// Path is the absolute path of a slash-separated path relative to the
// repository root.
func Path(rel string) string {
	return filepath.Join(RepoRoot(), filepath.FromSlash(rel))
}
