package runtime

import (
	"fmt"
	"os"
	"path/filepath"
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
	{
		File: "asciidoctor/lib/asciidoctor/extensions.rb",
		Old: `        super
        option :content_model, :attributes`,
		New: `        SyntaxProcessorDsl.instance_method(:resolve_attributes).bind_call self, *args
        option :content_model, :attributes`,
		Count: 1,
		Reason: "rbgo cannot resolve super in a module method when the receiver is a class " +
			"(a processor class using the DSL extends it). In MacroProcessorDsl#resolve_attributes " +
			"the next definition is always SyntaxProcessorDsl#resolve_attributes, which the " +
			"rewrite calls explicitly with the same arguments.",
	},
	{
		File:  "asciidoctor/lib/asciidoctor/document.rb",
		Old:   "(::Time.at Integer ::ENV['SOURCE_DATE_EPOCH'])",
		New:   "(::Time.at Integer(::ENV['SOURCE_DATE_EPOCH']))",
		Count: 1,
		Reason: "rbgo parses `Integer ::ENV[...]` (a call of Kernel#Integer) as the scoped " +
			"constant Integer::ENV; parentheses make the call explicit.",
	},
	{
		File:  "asciidoctor/lib/asciidoctor/reader.rb",
		Old:   "(@dir = file.dup).path = (dir = ::File.dirname file.path) == '/' ? '' : dir",
		New:   "@dir = ::AsciidoctorGoURI.with_path file, ((dir = ::File.dirname file.path) == '/' ? '' : dir)",
		Count: 1,
		Reason: "rbgo's URI objects have no path= setter and dup shares their state with the " +
			"original. The replacement builds a new URI from the original's components with " +
			"the new path, which is what duplicating it and assigning the path produces.",
	},
	{
		File:  "asciidoctor/lib/asciidoctor/rx.rb",
		Old:   `\]|\2([^\s]+?)&gt;|([^\s\[\]<]*([^\s,.?!\[\]<\)]))))m`,
		New:   `\]|(?<=&lt;https://|&lt;\\https://|&lt;http://|&lt;\\http://|&lt;file://|&lt;\\file://|&lt;ftp://|&lt;\\ftp://|&lt;irc://|&lt;\\irc://)([^\s]+?)&gt;|([^\s\[\]<]*([^\s,.?!\[\]<\)]))))m`,
		Count: 1,
		Reason: "rbgo's regexp engine lets a backreference to a group that did not participate " +
			"match the empty string; Onigmo fails it. InlineLinkRx uses \\2 (an empty group that " +
			"participates only after &lt;) as a condition. The lookbehind tests the same " +
			"condition: &lt; (optionally followed by a backslash) and the scheme precede.",
	},
	{
		File:  "asciidoctor/lib/asciidoctor/rx.rb",
		Old:   `(?=\\\+))(?:\2(x-|[^\[\]]+ x-)\]|`,
		New:   `(?=\\\+))(?:(?<!\\)\[(x-|[^\[\]]+ x-)\]|`,
		Count: 1,
		Reason: "See the previous patch. In InlinePassRx, \\2 consumes the [ that group 2 captured " +
			"in a lookahead, which happens only when group 1 is not the escaped form; (?<!\\\\)\\[ " +
			"consumes a [ not preceded by a backslash, which is the same condition.",
	},
	{
		File:  "asciidoctor/lib/asciidoctor/converter/manpage.rb",
		Old:   "%(.so #{basename}), mode: FILE_WRITE_MODE",
		New:   "%(.so #{basename}), mode: ::Asciidoctor::FILE_WRITE_MODE",
		Count: 1,
		Reason: "In a class method of a class declared with a compound path (class " +
			"Converter::ManPageConverter inside module Asciidoctor), rbgo's constant lookup " +
			"skips the enclosing module. FILE_WRITE_MODE resolves to Asciidoctor::FILE_WRITE_MODE " +
			"on MRI; the patch names it in full. It is the only such reference in Asciidoctor.",
	},
	{
		File:  "coderay/lib/coderay/scanners/java.rb",
		Old:   `Regexp.new('[[[:alpha:]]_][[[:alnum:]]_]*')`,
		New:   `Regexp.new('[[:alpha:]_][[:alnum:]_]*')`,
		Count: 1,
		Reason: "rbgo's regexp engine matches nothing with a nested character class ([a[bc]]). The " +
			"class [[[:alpha:]]_] is the union of [[:alpha:]] and _, which [[:alpha:]_] states flat.",
	},
	coderayIdentPatch("coderay/lib/coderay/scanners/ruby/patterns.rb"),
	coderayIdentPatch("coderay/lib/coderay/scanners/php.rb"),
	{
		File: "coderay/lib/coderay/tokens.rb",
		Old: `        when String
          if size && part_size + content.size > size  # token must be cut`,
		New: `        when String
          while true
          if size && part_size + content.size > size  # token must be cut`,
		Count: 1,
		Reason: "rbgo does not support the redo keyword. Tokens#split_into_parts redoes the body of " +
			"its for loop to cut the rest of a String token; this patch and the next wrap that branch in " +
			"a while loop that next repeats, which is equivalent: content is still a String, so the " +
			"redone body would take the same branch.",
	},
	{
		File: "coderay/lib/coderay/tokens.rb",
		Old: `            redo unless content.empty?
          else
            part << content << item
            part_size += content.size
          end
          content = nil`,
		New: `            next unless content.empty?
          else
            part << content << item
            part_size += content.size
          end
          break
          end
          content = nil`,
		Count:  1,
		Reason: "See the previous patch.",
	},
	charLiteralPatch("asciidoctor/lib/asciidoctor/substitutors.rb", "CAN", `\u0018`),
	charLiteralPatch("asciidoctor/lib/asciidoctor/substitutors.rb", "DEL", `\u007f`),
	charLiteralPatch("asciidoctor/lib/asciidoctor/substitutors.rb", "PASS_START", `\u0096`),
	charLiteralPatch("asciidoctor/lib/asciidoctor/substitutors.rb", "PASS_END", `\u0097`),
	charLiteralPatch("asciidoctor/lib/asciidoctor/converter/manpage.rb", "ESC", `\u001b`),
}

// coderayIdentPatch rewrites CodeRay's identifier pattern, which allows
// non-ASCII characters through a nested negated class, [[:alpha:]_[^\0-\177]].
// rbgo's regexp engine matches nothing with a nested class, and rbgo's
// StringScanner mis-reads octal escapes in a class; the alternation of the
// flat class and the hex-escaped negated class matches the same characters.
func coderayIdentPatch(file string) sourcePatch {
	return sourcePatch{
		File:   file,
		Old:    `Regexp.new('[[:alpha:]_[^\0-\177]][[:alnum:]_[^\0-\177]]*')`,
		New:    `Regexp.new('(?:[[:alpha:]_]|[^\x00-\x7f])(?:[[:alnum:]_]|[^\x00-\x7f])*')`,
		Count:  1,
		Reason: "rbgo cannot match nested character classes or octal escapes in a StringScanner class; see coderayIdentPatch.",
	}
}

// charLiteralPatch rewrites the constant assignment `NAME = ?<escape>` to the
// double-quoted string `NAME = "<escape>"`. rbgo's parser mis-reads escape
// sequences in character literals (?\u007f yields "u"), while the same escape
// in a double-quoted string is correct. A character literal is a String, and
// both forms are frozen under the file's frozen_string_literal comment, so the
// rewrite is equivalent.
func charLiteralPatch(file, name, escape string) sourcePatch {
	return sourcePatch{
		File:   file,
		Old:    name + " = ?" + escape,
		New:    name + ` = "` + escape + `"`,
		Count:  1,
		Reason: "rbgo mis-parses escapes in character literals; use the equivalent double-quoted string.",
	}
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

// PatchTree applies the source patches for Asciidoctor to an Asciidoctor
// source tree on disk (a directory with lib/), such as a copy of the
// asciidoctor submodule. Patches to other embedded trees (CodeRay) do not
// apply to it.
func PatchTree(asciidoctorDir string) error {
	done := map[string]bool{}
	for _, p := range sourcePatches {
		if done[p.File] {
			continue
		}
		done[p.File] = true
		rel, ok := strings.CutPrefix(p.File, "asciidoctor/")
		if !ok {
			continue
		}
		path := filepath.Join(asciidoctorDir, filepath.FromSlash(rel))
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		patched, err := applyPatches(p.File, src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, patched, 0o644); err != nil {
			return err
		}
	}
	return nil
}
