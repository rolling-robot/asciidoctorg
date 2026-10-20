package runtime

import (
	ruby "github.com/go-embedded-ruby/ruby"
	haml "github.com/go-ruby-haml/haml"
	slim "github.com/go-ruby-slim/slim"
)

// defineTemplateCompiler exposes the Haml and Slim compilers to the Tilt shim
// as AsciidoctorGo::TemplateCompiler.haml(source, format, escape_fn) and
// .slim(source, escape_fn). rbgo's own Haml and Slim classes always compile
// with default options; Asciidoctor's template converter needs the format and
// escaping it asks for. Each returns Ruby source that evaluates to the output.
func defineTemplateCompiler(in *ruby.Interp) error {
	mod, err := in.DefineModule("AsciidoctorGo::TemplateCompiler")
	if err != nil {
		return err
	}
	mod.DefineSingletonMethod("haml", func(c *ruby.Call) (any, error) {
		if len(c.Args) != 3 {
			return nil, ruby.NewError("ArgumentError", "haml(source, format, escape_fn)")
		}
		src, err := haml.Compile(c.Args[0].String(), haml.Options{Format: optString(c.Args[1]), EscapeFn: optString(c.Args[2])})
		if err != nil {
			return nil, ruby.NewError("Haml::SyntaxError", err.Error())
		}
		return src, nil
	})
	mod.DefineSingletonMethod("slim", func(c *ruby.Call) (any, error) {
		if len(c.Args) != 2 {
			return nil, ruby.NewError("ArgumentError", "slim(source, escape_fn)")
		}
		src, err := slim.Compile(c.Args[0].String(), slim.Options{EscapeFn: optString(c.Args[1])})
		if err != nil {
			return nil, ruby.NewError("Slim::Parser::SyntaxError", err.Error())
		}
		return src, nil
	})
	return nil
}

func optString(v ruby.Value) string {
	if v.IsNil() {
		return ""
	}
	return v.String()
}
