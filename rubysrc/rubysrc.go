// Package rubysrc embeds the Ruby source tree that asciidoctor-go runs: the
// Asciidoctor implementation (the asciidoctor git submodule, used unmodified),
// CodeRay for the coderay source highlighter (the coderay git submodule), the
// shims that adapt them to rbgo, and the Ruby half of the Go bridge.
package rubysrc

import "embed"

// FS holds asciidoctor/{lib,data,LICENSE}, coderay/{lib,MIT-LICENSE}, shims
// and glue.
//
//go:embed asciidoctor/lib asciidoctor/data asciidoctor/LICENSE all:coderay/lib coderay/MIT-LICENSE shims glue
var FS embed.FS
