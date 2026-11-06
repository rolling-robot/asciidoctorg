package asciidoctor

import (
	"os"
	"path/filepath"
	"testing"
)

// D7: Default cached the error of its first New, so a transient failure (an
// unusable cache directory, say) broke the package-level functions for the
// life of the process. Default must retry until an instance starts.
//
// This test must run before anything else in the package uses Default; the
// examples run after the tests.
func TestDefaultRetriesAfterFailure(t *testing.T) {
	// The instance Default finally starts outlives the test (the examples
	// use it), so it must use the real cache directory, not a temporary one.
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache directory: %v", err)
	}
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", notADir)
	t.Setenv("HOME", notADir)
	if _, err := Default(); err == nil {
		t.Fatal("Default succeeded with an unusable cache directory")
	}
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	a, err := Default()
	if err != nil {
		t.Fatalf("Default after the cache directory became usable: %v", err)
	}
	if b, _ := Default(); b != a {
		t.Fatal("Default returned a different instance on a later call")
	}
}
