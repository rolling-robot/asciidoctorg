// Package rubysrc embeds the Ruby source tree that asciidoctor-go runs: the
// vendored Asciidoctor implementation and the shims that adapt it to rbgo.
package rubysrc

import "embed"

// FS holds the asciidoctor and shims directories.
//
//go:embed asciidoctor shims
var FS embed.FS
