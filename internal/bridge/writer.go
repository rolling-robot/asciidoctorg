package bridge

import (
	"io"

	ruby "github.com/go-embedded-ruby/ruby"
)

// Writer adapts an io.Writer to the IO-like target Asciidoctor accepts for
// :to_file (it calls #write). It crosses into Ruby as AsciidoctorGo::Writer.
type Writer struct {
	W io.Writer
	// Err is the first write error; Asciidoctor sees it raised as an IOError.
	Err error
}

func defineWriter(in *ruby.Interp) error {
	cls, err := in.DefineClass("AsciidoctorGo::Writer", nil)
	if err != nil {
		return err
	}
	cls.Bind(&Writer{})
	write := func(c *ruby.Call) (any, error) {
		w := c.Self.Go().(*Writer)
		n := 0
		for _, a := range c.Args {
			s := a.String()
			m, err := io.WriteString(w.W, s)
			n += m
			if err != nil {
				if w.Err == nil {
					w.Err = err
				}
				return nil, ruby.NewError("IOError", err.Error())
			}
		}
		return n, nil
	}
	cls.DefineMethod("write", write)
	cls.DefineMethod("<<", func(c *ruby.Call) (any, error) {
		if _, err := write(c); err != nil {
			return nil, err
		}
		return c.Self, nil
	})
	cls.DefineMethod("flush", func(c *ruby.Call) (any, error) {
		if f, ok := c.Self.Go().(*Writer).W.(interface{ Flush() error }); ok {
			if err := f.Flush(); err != nil {
				return nil, ruby.NewError("IOError", err.Error())
			}
		}
		return c.Self, nil
	})
	return nil
}
