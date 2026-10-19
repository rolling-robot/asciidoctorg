// Package asciidoctor converts AsciiDoc documents with Asciidoctor, from Go.
//
// It runs the original Asciidoctor implementation, unmodified, in an embedded
// pure-Go Ruby interpreter (rbgo), in the way AsciidoctorJ runs it on JRuby.
// Output is therefore Asciidoctor's own, and Go code can extend the processor
// with the same extension points Ruby code has.
//
//	html, err := asciidoctor.Convert("= Hello\n\nA *bold* paragraph.", nil)
//
// An Asciidoctor instance holds one interpreter. Calls on an instance are
// serialized; create several instances, or a Pool, to convert in parallel.
package asciidoctor

// Version is the version of this module.
const Version = "0.1.0-dev"
