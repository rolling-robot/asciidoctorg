package asciidoctor

// LoadRubySource evaluates Ruby source in the instance's interpreter, for
// example to define extension classes or to register extensions with
// Asciidoctor::Extensions.register. file names the source in backtraces.
func (a *Asciidoctor) LoadRubySource(source, file string) error {
	if file == "" {
		file = "(ruby source)"
	}
	return a.do(func() error {
		_, err := a.b.Interp().Eval(source, file, 1)
		return err
	})
}

// RequireLibrary requires Ruby libraries (paths or names on the load path)
// into the instance, as AsciidoctorJ's requireLibrary does. Requiring a Ruby
// file that registers extensions makes them apply to the instance.
func (a *Asciidoctor) RequireLibrary(names ...string) error {
	return a.do(func() error {
		for _, name := range names {
			if err := a.b.Interp().Set("__agox_require", name); err != nil {
				return err
			}
			if _, err := a.b.Interp().Eval("require __agox_require", "(asciidoctor-go)", 1); err != nil {
				return err
			}
		}
		return nil
	})
}
