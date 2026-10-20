// Package rubytest runs Asciidoctor's own test suite (the Ruby files in the
// asciidoctor submodule's test directory) on rbgo, one Go subtest per Ruby
// test. Tests that cannot apply to an embedded runtime are listed in
// exclusions_test.go with the reason.
package rubytest

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/asciidoctor-go/asciidoctor/internal/runtime"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// testFiles are the Ruby test files to run: every *_test.rb, sorted.
func testFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_test.rb"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatalf("no test files in %s (is the asciidoctor submodule checked out?)", dir)
	}
	return files
}

// copyTree copies the asciidoctor submodule (without .git) to dest.
func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func TestAsciidoctorSuite(t *testing.T) {
	root := repoRoot(t)
	// The suite expects the whole repository (Asciidoctor::ROOT_DIR contains
	// test/, bin/, ...) and writes into test/fixtures, so it runs on a patched
	// copy of the submodule.
	tree := filepath.Join(t.TempDir(), "asciidoctor")
	if err := copyTree(filepath.Join(root, "rubysrc", "asciidoctor"), tree); err != nil {
		t.Fatal(err)
	}
	if err := runtime.PatchTree(tree); err != nil {
		t.Fatal(err)
	}
	testDir := filepath.Join(tree, "test")
	supportDir, err := filepath.Abs("support")
	if err != nil {
		t.Fatal(err)
	}
	runtimeDir := t.TempDir()
	cwd, _ := os.Getwd()
	// The suite's helper changes the working directory; restore it.
	t.Cleanup(func() { os.Chdir(cwd) })
	only := os.Getenv("RUBYTEST_FILE")
	for _, file := range testFiles(t, testDir) {
		base := filepath.Base(file)
		if only != "" && !strings.Contains(base, only) {
			continue
		}
		t.Run(strings.TrimSuffix(base, ".rb"), func(t *testing.T) {
			runFile(t, runtimeDir, tree, supportDir, file)
		})
	}
}

func runFile(t *testing.T, runtimeDir, tree, supportDir, file string) {
	if reason, ok := fileExcluded(strings.TrimSuffix(filepath.Base(file), ".rb")); ok {
		t.Skip(reason)
	}
	var out bytes.Buffer
	rt, err := runtime.Boot(runtime.Config{Dir: runtimeDir, AsciidoctorDir: tree, Stdout: &out, Stderr: &out})
	if err != nil {
		t.Fatal(err)
	}
	in := rt.Interp
	if err := defineNokogiri(in); err != nil {
		t.Fatal(err)
	}
	if err := defineHostSupport(in); err != nil {
		t.Fatal(err)
	}
	set := func(name string, v any) {
		if err := in.Set(name, v); err != nil {
			t.Fatal(err)
		}
	}
	set("__agox_support_dir", supportDir)
	set("__agox_lib_dir", filepath.Join(tree, "lib"))
	if _, err := in.Eval("$agox_support_dir, $agox_lib_dir = __agox_support_dir, __agox_lib_dir; load File.join($agox_support_dir, 'harness.rb')", "harness", 1); err != nil {
		t.Fatalf("loading harness: %v", err)
	}
	set("__agox_test_file", file)
	if _, err := in.Eval("load __agox_test_file", "loader", 1); err != nil {
		t.Fatalf("loading %s: %v\n%s", filepath.Base(file), err, tail(out.String()))
	}
	list, err := in.Eval("AsciidoctorGoTest.tests", "lister", 1)
	if err != nil {
		t.Fatal(err)
	}
	tests, _ := list.Go().([]any)
	if len(tests) == 0 {
		t.Fatalf("no tests found in %s", filepath.Base(file))
	}
	fileKey := strings.TrimSuffix(filepath.Base(file), ".rb")
	for _, entry := range tests {
		pair := entry.([]any)
		class, method := pair[0].(string), pair[1].(string)
		name := class + "/" + strings.TrimPrefix(method, "test_")
		t.Run(name, func(t *testing.T) {
			if reason, ok := excluded(fileKey, class, method); ok {
				t.Skip(reason)
			}
			set("__agox_class", class)
			set("__agox_method", method)
			out.Reset()
			res, err := in.Eval("AsciidoctorGoTest.run __agox_class, __agox_method", "runner", 1)
			if err != nil {
				t.Fatalf("runner: %v", err)
			}
			r := res.Go().([]any)
			status, msg := r[0].(string), r[1].(string)
			switch status {
			case "pass":
			case "skip":
				t.Skip("skipped by the suite: " + msg)
			default:
				t.Errorf("%s: %s\noutput:\n%s", status, msg, tail(out.String()))
			}
		})
	}
}

// tail returns the end of the captured interpreter output.
func tail(s string) string {
	if len(s) > 2000 {
		return "..." + s[len(s)-2000:]
	}
	return s
}
