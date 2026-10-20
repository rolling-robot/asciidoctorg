package asciidoctor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// A DirectoryWalker finds the files ConvertDirectory converts.
type DirectoryWalker interface {
	Scan() ([]string, error)
}

// asciidocFileRx matches AsciiDoc file names: .adoc, .asciidoc, .asc and .ad
// files whose name does not start with "_" or ".".
var asciidocFileRx = regexp.MustCompile(`^[^_.].*\.a((sc(iidoc)?)|d(oc)?)$`)

// AsciiDocDirectoryWalker finds AsciiDoc files (.adoc, .asciidoc, .asc, .ad)
// below a base directory, recursively. Files and directories whose name starts
// with "_" or "." are skipped (partials and hidden files, by convention).
type AsciiDocDirectoryWalker struct {
	BaseDir string
}

// Scan returns the AsciiDoc files below BaseDir, sorted. A BaseDir that does
// not exist has none.
func (w AsciiDocDirectoryWalker) Scan() ([]string, error) {
	var files []string
	err := filepath.WalkDir(w.BaseDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if p == w.BaseDir && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != w.BaseDir && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if asciidocFileRx.MatchString(name) {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// GlobDirectoryWalker finds the files matching a glob expression. Within a
// path segment, "*" matches any run of characters and "?" any one character;
// a "**" segment matches any number of directories. The part of the
// expression before the first segment with a wildcard must name an existing
// directory, and the results are relative to it as written ("docs/**/*.adoc"
// yields paths such as "docs/guide/intro.adoc").
type GlobDirectoryWalker struct {
	Glob string
}

// Scan returns the matching files and directories, sorted. An expression
// without wildcards yields its path, whether or not the file exists.
func (w GlobDirectoryWalker) Scan() ([]string, error) {
	expr := filepath.ToSlash(w.Glob)
	root, pattern := splitGlob(expr)
	if root != "" {
		fi, err := os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("asciidoctor: directory does not exist: %s", root)
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("asciidoctor: file must be a directory: %s", root)
		}
	}
	// An expression without wildcards names a file, which need not exist.
	if !strings.ContainsAny(pattern, "*?[") {
		return []string{filepath.FromSlash(path.Join(root, pattern))}, nil
	}
	// Normalize "**x" and "x**" into separate segments, as AsciidoctorJ does.
	pattern = regexp.MustCompile(`\*\*([^/])`).ReplaceAllString(pattern, "**/*$1")
	pattern = regexp.MustCompile(`([^/])\*\*`).ReplaceAllString(pattern, "$1*/**")
	segments := strings.Split(pattern, "/")
	for _, s := range segments {
		if _, err := path.Match(s, ""); err != nil {
			return nil, fmt.Errorf("asciidoctor: invalid glob %q: %w", w.Glob, err)
		}
	}
	dir := root
	if dir == "" {
		dir = "."
	}
	seen := map[string]bool{}
	var matches []string
	var walk func(rel string, segs []string)
	walk = func(rel string, segs []string) {
		if len(segs) == 0 {
			return
		}
		seg := segs[0]
		if seg == "**" {
			if len(segs) == 1 {
				// a trailing ** matches everything below
				walkAll(dir, rel, func(p string) {
					if !seen[p] {
						seen[p] = true
						matches = append(matches, p)
					}
				})
				return
			}
			// ** matches zero directories...
			walk(rel, segs[1:])
			// ...or one more, staying on the ** segment
			for _, e := range readDir(filepath.Join(dir, rel)) {
				if e.IsDir() {
					walk(path.Join(rel, e.Name()), segs)
				}
			}
			return
		}
		for _, e := range readDir(filepath.Join(dir, rel)) {
			if ok, _ := path.Match(seg, e.Name()); !ok {
				continue
			}
			p := path.Join(rel, e.Name())
			if len(segs) == 1 {
				if !seen[p] {
					seen[p] = true
					matches = append(matches, p)
				}
			} else if e.IsDir() {
				walk(p, segs[1:])
			}
		}
	}
	walk("", segments)
	out := make([]string, len(matches))
	for i, m := range matches {
		if root != "" {
			m = path.Join(root, m)
		}
		out[i] = filepath.FromSlash(m)
	}
	sort.Strings(out)
	return out, nil
}

// splitGlob splits a glob into the leading directory without wildcards and
// the remaining pattern. Without a wildcard, the last segment is the pattern.
func splitGlob(expr string) (root, pattern string) {
	last := -1
	for i := 0; i < len(expr) && expr[i] != '*' && expr[i] != '?'; i++ {
		if expr[i] == '/' {
			last = i
		}
	}
	switch {
	case last < 0:
		return "", expr
	case last == 0:
		return "/", expr[1:]
	default:
		return expr[:last], expr[last+1:]
	}
}

func readDir(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return entries
}

// walkAll reports every file and directory below dir/rel (relative to dir).
func walkAll(dir, rel string, report func(string)) {
	for _, e := range readDir(filepath.Join(dir, rel)) {
		p := path.Join(rel, e.Name())
		report(p)
		if e.IsDir() {
			walkAll(dir, p, report)
		}
	}
}

// ConvertDirectory converts every file the walker finds, as ConvertFiles
// does.
func (a *Asciidoctor) ConvertDirectory(walker DirectoryWalker, options *Options) ([]string, error) {
	files, err := walker.Scan()
	if err != nil {
		return nil, err
	}
	return a.ConvertFiles(files, options)
}
