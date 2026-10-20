package asciidoctor

import (
	"io"

	ruby "github.com/go-embedded-ruby/ruby"
)

// RunCLI runs Asciidoctor's command line (the asciidoctor command) with the
// given arguments, without the program name, and returns its exit code. The
// command writes to the instance's standard output and error (see WithStdout
// and WithStderr) and, given the input file "-", reads the document from
// stdin. Options that configure the interpreter itself, such as -r, -I and
// -t, apply to this instance.
func (a *Asciidoctor) RunCLI(args []string, stdin io.Reader) (int, error) {
	h := &stdinHandler{r: stdin}
	code := 0
	err := a.do(func() error {
		g, err := a.glue()
		if err != nil {
			return err
		}
		id := a.b.Register(h)
		defer a.b.Unregister(id)
		v, err := g.Call("cli", id, append([]string{}, args...))
		if err != nil {
			return err
		}
		code = rubyInt(v)
		return nil
	})
	return code, err
}

// stdinHandler supplies the command line's standard input.
type stdinHandler struct {
	r io.Reader
}

func (h *stdinHandler) Dispatch(_ *ruby.Call, method string, _ []ruby.Value) (any, error) {
	if method != "stdin" {
		return nil, unknownMethod(method)
	}
	if h.r == nil {
		return "", nil
	}
	b, err := io.ReadAll(h.r)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
