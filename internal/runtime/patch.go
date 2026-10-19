package runtime

import (
	"fmt"
	"strings"
)

// A sourcePatch rewrites one exact snippet of a vendored Ruby file while the
// tree is materialized. Patches exist only where rbgo cannot run a construct
// and no Ruby-level shim can intervene (for example, unsupported syntax). Each
// one must be semantically equivalent to the original on MRI.
type sourcePatch struct {
	// File is the path within the embedded tree, e.g. "asciidoctor/lib/asciidoctor/parser.rb".
	File string
	// Old must occur exactly Count times in the file; New replaces every occurrence.
	Old, New string
	Count    int
	// Reason documents why the patch is needed and why it is equivalent.
	Reason string
}

// sourcePatches is the complete list of changes made to the vendored source.
var sourcePatches = []sourcePatch{
	{
		File: "asciidoctor/lib/asciidoctor/parser.rb",
		Old: `                break
              end
              redo
            end`,
		New: `                break
              end
              next
            end`,
		Count: 2,
		Reason: "rbgo does not support the redo keyword. Each redo in parse_table " +
			"sits directly in a `while true` loop, where next (re-test a constant " +
			"true condition, then rerun the body) is equivalent to redo.",
	},
	{
		File: "asciidoctor/lib/asciidoctor/parser.rb",
		Old: `              break if (line = post_match).empty?
              redo
            end`,
		New: `              break if (line = post_match).empty?
              next
            end`,
		Count:  1,
		Reason: "See the previous patch; this is the csv branch of the same loop.",
	},
}

// applyPatches returns content with every patch for file applied. It fails if
// a patch no longer matches the vendored source exactly, so that upgrading
// Asciidoctor cannot silently drop a patch.
func applyPatches(file string, content []byte) ([]byte, error) {
	src := string(content)
	for _, p := range sourcePatches {
		if p.File != file {
			continue
		}
		if n := strings.Count(src, p.Old); n != p.Count {
			return nil, fmt.Errorf("asciidoctor-go: source patch for %s expected %d occurrence(s) of snippet, found %d", file, p.Count, n)
		}
		src = strings.ReplaceAll(src, p.Old, p.New)
	}
	return []byte(src), nil
}
