package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"

	"github.com/asciidoctor-go/asciidoctor/rubysrc"
)

// completeMarker is written last; its presence means the tree is usable.
const completeMarker = ".complete"

var (
	treeMu    sync.Mutex
	treeCache = map[string]string{}
)

// treeFiles returns the embedded files (sorted) with patches applied.
func treeFiles() (map[string][]byte, []string, error) {
	files := map[string][]byte{}
	var names []string
	err := fs.WalkDir(rubysrc.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := rubysrc.FS.ReadFile(p)
		if err != nil {
			return err
		}
		if data, err = applyPatches(p, data); err != nil {
			return err
		}
		files[p] = data
		names = append(names, p)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(names)
	return files, names, nil
}

// treeHash identifies the content of the materialized tree.
func treeHash(files map[string][]byte, names []string) string {
	h := sha256.New()
	for _, n := range names {
		fmt.Fprintf(h, "%s\x00%d\x00", n, len(files[n]))
		h.Write(files[n])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// DefaultCacheDir is the parent directory used when no directory is configured.
func DefaultCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("asciidoctor-go: no user cache directory (set a runtime directory explicitly): %w", err)
	}
	return filepath.Join(dir, "asciidoctor-go"), nil
}

// Materialize writes the embedded Ruby tree (with patches applied) below
// parent and returns the tree root. The root is named after the content hash,
// so a tree is written once and reused by every later call and process. It is
// assembled in a temporary sibling directory and renamed into place, so a
// concurrent or interrupted writer never exposes a partial tree.
func Materialize(parent string) (string, error) {
	treeMu.Lock()
	defer treeMu.Unlock()
	if root, ok := treeCache[parent]; ok {
		return root, nil
	}
	files, names, err := treeFiles()
	if err != nil {
		return "", err
	}
	root := filepath.Join(parent, treeHash(files, names))
	if _, err := os.Stat(filepath.Join(root, completeMarker)); err == nil {
		treeCache[parent] = root
		return root, nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("asciidoctor-go: create runtime directory: %w", err)
	}
	tmp, err := os.MkdirTemp(parent, ".tmp-")
	if err != nil {
		return "", fmt.Errorf("asciidoctor-go: create runtime directory: %w", err)
	}
	defer os.RemoveAll(tmp)
	for _, n := range names {
		dest := filepath.Join(tmp, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", fmt.Errorf("asciidoctor-go: write runtime tree: %w", err)
		}
		if err := os.WriteFile(dest, files[n], 0o644); err != nil {
			return "", fmt.Errorf("asciidoctor-go: write runtime tree: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, completeMarker), nil, 0o644); err != nil {
		return "", fmt.Errorf("asciidoctor-go: write runtime tree: %w", err)
	}
	if err := os.Rename(tmp, root); err != nil {
		// Another process may have won the race; accept its complete tree.
		if _, statErr := os.Stat(filepath.Join(root, completeMarker)); statErr != nil {
			return "", fmt.Errorf("asciidoctor-go: install runtime tree: %w", errors.Join(err, statErr))
		}
	}
	treeCache[parent] = root
	return root, nil
}

// libDir, shimDir and glueDir locate the load path entries within a materialized root.
func libDir(root string) string {
	return filepath.Join(root, filepath.FromSlash(path.Join("asciidoctor", "lib")))
}
func shimDir(root string) string { return filepath.Join(root, "shims") }
func glueDir(root string) string { return filepath.Join(root, "glue") }
