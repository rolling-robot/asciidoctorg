// Package rubysrc embeds the Ruby source tree that asciidoctor-go runs: the
// Asciidoctor implementation (the asciidoctor git submodule, used unmodified)
// and the shims that adapt it to rbgo.
package rubysrc

import "embed"

// FS holds asciidoctor/{lib,data,LICENSE} and shims.
//
//go:embed asciidoctor/lib asciidoctor/data asciidoctor/LICENSE shims
var FS embed.FS
