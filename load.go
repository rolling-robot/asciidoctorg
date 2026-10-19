package asciidoctor

import ruby "github.com/go-embedded-ruby/ruby"

// Load parses AsciiDoc source into a Document without converting it. Output
// options (ToFile, ToDir, Output) do not apply.
func (a *Asciidoctor) Load(source string, options *Options) (*Document, error) {
	return a.load("load", source, options)
}

// LoadFile parses the AsciiDoc file at path into a Document.
func (a *Asciidoctor) LoadFile(path string, options *Options) (*Document, error) {
	return a.load("load_file", path, options)
}

func (a *Asciidoctor) load(method, input string, options *Options) (*Document, error) {
	var v ruby.Value
	err := a.do(func() error {
		opts := options.rubyOptions(nil)
		delete(opts, "to_file")
		delete(opts, "to_dir")
		delete(opts, "mkdirs")
		var err error
		v, err = a.b.Asciidoctor().Call(method, input, opts)
		return err
	})
	if err != nil {
		return nil, err
	}
	return a.wrapDocument(v), nil
}

// Load parses AsciiDoc source with the default instance; see
// Asciidoctor.Load.
func Load(source string, options *Options) (*Document, error) {
	a, err := Default()
	if err != nil {
		return nil, err
	}
	return a.Load(source, options)
}

// LoadFile parses an AsciiDoc file with the default instance; see
// Asciidoctor.LoadFile.
func LoadFile(path string, options *Options) (*Document, error) {
	a, err := Default()
	if err != nil {
		return nil, err
	}
	return a.LoadFile(path, options)
}
