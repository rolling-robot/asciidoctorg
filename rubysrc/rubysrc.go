// Package rubysrc embeds the Ruby source tree that asciidoctor-go runs: the
// Asciidoctor implementation (the asciidoctor git submodule, used unmodified),
// the shims that adapt it to rbgo, and the Ruby half of the Go bridge.
package rubysrc

import "embed"

// FS holds asciidoctor/{lib,data,LICENSE}, shims and glue.
//
//go:embed asciidoctor/lib asciidoctor/data asciidoctor/LICENSE shims glue
var FS embed.FS
