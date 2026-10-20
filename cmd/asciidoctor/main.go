// Command asciidoctor converts AsciiDoc documents, as Asciidoctor's own
// asciidoctor command does: it runs that command, with the same options, on
// the Asciidoctor embedded in asciidoctor-go. Run asciidoctor --help for the
// options.
package main

import (
	"fmt"
	"os"

	"github.com/asciidoctor-go/asciidoctor"
)

func main() {
	a, err := asciidoctor.New(asciidoctor.WithStdout(os.Stdout), asciidoctor.WithStderr(os.Stderr))
	if err != nil {
		fmt.Fprintln(os.Stderr, "asciidoctor:", err)
		os.Exit(1)
	}
	code, err := a.RunCLI(os.Args[1:], os.Stdin)
	a.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "asciidoctor:", err)
		os.Exit(1)
	}
	os.Exit(code)
}
