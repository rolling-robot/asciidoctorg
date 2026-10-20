package asciidoctor

import (
	"errors"
	"io"
	"os"
	"sync"

	ruby "github.com/go-embedded-ruby/ruby"

	"github.com/asciidoctor-go/asciidoctor/internal/bridge"
	"github.com/asciidoctor-go/asciidoctor/internal/runtime"
)

// Asciidoctor is an Asciidoctor processor: one embedded Ruby interpreter with
// Asciidoctor loaded, together with the extensions, converters and log handlers
// registered on it.
//
// An Asciidoctor is safe for concurrent use; calls are serialized. For parallel
// conversions, create several instances or use a Pool. Nodes, readers and
// other objects handed to an extension or converter callback may be used only
// by the goroutine running that callback, and only until it returns.
type Asciidoctor struct {
	b *bridge.Bridge

	extOnce sync.Once
	ext     *ExtensionRegistry
	logging logging
	// converterHandles are the bridge handles of registered Go converters
	// (accessed with the interpreter held).
	converterHandles map[int64]bool
}

// An Option configures New.
type Option func(*config)

type config struct {
	runtime   runtime.Config
	loadPaths []string
	noSetups  bool
}

// A Setup configures a new instance, for example by registering extensions,
// converters, syntax highlighters or log handlers with it.
type Setup func(*Asciidoctor) error

var (
	setupsMu    sync.Mutex
	setups      = map[int]Setup{}
	setupOrder  []int
	nextSetupID int
)

// RegisterSetup adds a setup that New runs on every instance it creates,
// after the ones registered before it. It is the counterpart of
// AsciidoctorJ's service-loaded extension and converter registries: a package
// providing extensions can call it from an init function, so that importing
// the package enables them. The returned function removes the setup.
func RegisterSetup(s Setup) (remove func()) {
	setupsMu.Lock()
	defer setupsMu.Unlock()
	nextSetupID++
	id := nextSetupID
	setups[id] = s
	setupOrder = append(setupOrder, id)
	return func() {
		setupsMu.Lock()
		defer setupsMu.Unlock()
		delete(setups, id)
	}
}

func registeredSetups() []Setup {
	setupsMu.Lock()
	defer setupsMu.Unlock()
	out := make([]Setup, 0, len(setups))
	for _, id := range setupOrder {
		if s, ok := setups[id]; ok {
			out = append(out, s)
		}
	}
	return out
}

// WithoutRegisteredSetups makes New skip the setups added with RegisterSetup.
func WithoutRegisteredSetups() Option {
	return func(c *config) { c.noSetups = true }
}

// WithRuntimeDir sets the directory below which the embedded Ruby sources are
// written on first use (the default is the user cache directory). The sources
// are written once per version and shared by every instance and process.
func WithRuntimeDir(dir string) Option {
	return func(c *config) { c.runtime.Dir = dir }
}

// WithStdout sets the writer that receives the interpreter's standard output
// (Ruby's $stdout). The default discards it.
func WithStdout(w io.Writer) Option {
	return func(c *config) { c.runtime.Stdout = w }
}

// WithStderr sets the writer that receives the interpreter's standard error
// (Ruby's $stderr), where Asciidoctor's default logger writes warnings. The
// default is os.Stderr.
func WithStderr(w io.Writer) Option {
	return func(c *config) { c.runtime.Stderr = w }
}

// WithLoadPath adds directories to the front of the Ruby load path, where
// RequireLibrary and Ruby's require look for libraries (AsciidoctorJ's
// load path argument of Asciidoctor.Factory.create).
func WithLoadPath(dirs ...string) Option {
	return func(c *config) { c.loadPaths = append(c.loadPaths, dirs...) }
}

// New starts an Asciidoctor instance. Release it with Close.
func New(opts ...Option) (*Asciidoctor, error) {
	cfg := config{runtime: runtime.Config{Stderr: os.Stderr}}
	for _, o := range opts {
		o(&cfg)
	}
	b, err := bridge.New(cfg.runtime)
	if err != nil {
		return nil, wrapError(err)
	}
	a := &Asciidoctor{b: b}
	a.logging.level = SeverityWarn
	if len(cfg.loadPaths) > 0 {
		if err := a.addLoadPath(cfg.loadPaths); err != nil {
			a.Close()
			return nil, err
		}
	}
	if !cfg.noSetups {
		for _, s := range registeredSetups() {
			if err := s(a); err != nil {
				a.Close()
				return nil, err
			}
		}
	}
	return a, nil
}

// Close releases the instance. Later calls fail with ErrClosed.
func (a *Asciidoctor) Close() error {
	return wrapError(a.b.Close())
}

// do runs fn with exclusive access to the interpreter and wraps its error.
func (a *Asciidoctor) do(fn func() error) error {
	return wrapError(a.b.Do(fn))
}

// glue returns the AsciidoctorGo glue module.
func (a *Asciidoctor) glue() (ruby.Value, error) {
	return a.b.Const("AsciidoctorGo")
}

// Convert converts AsciiDoc source and returns the result. If options
// direct the output to a file (ToFile, ToDir) or a stream (Output), the
// result is written there and Convert returns "".
func (a *Asciidoctor) Convert(source string, options *Options) (string, error) {
	return a.convert("convert", source, options)
}

// ConvertFile converts the AsciiDoc file at path. Like Asciidoctor's
// convert_file, it writes the result to a file next to the input (named after
// the input, with the backend's extension) unless ToFile or ToDir says
// otherwise, and returns "". Set NoOutputFile to get the result as a string
// instead, or Output to write it to a stream.
func (a *Asciidoctor) ConvertFile(path string, options *Options) (string, error) {
	return a.convert("convert_file", path, options)
}

func (a *Asciidoctor) convert(method, input string, options *Options) (string, error) {
	var writer *bridge.Writer
	if options != nil && options.Output != nil {
		writer = &bridge.Writer{W: options.Output}
	}
	var out string
	err := a.do(func() error {
		g, err := a.glue()
		if err != nil {
			return err
		}
		v, err := g.Call(method, input, options.rubyOptions(writer))
		if err != nil {
			return err
		}
		if !v.IsNil() {
			out = v.String()
		}
		return nil
	})
	if err == nil && writer != nil && writer.Err != nil {
		err = writer.Err
	}
	return out, err
}

// ConvertDocument converts as Convert does and also returns the converted
// document, for example to read the catalog of the assets it references
// (with Options.CatalogAssets). output is "" when it was written to a file or
// stream.
func (a *Asciidoctor) ConvertDocument(source string, options *Options) (doc *Document, output string, err error) {
	return a.convertDocument("convert", source, options)
}

// ConvertFileDocument converts as ConvertFile does and also returns the
// converted document; see ConvertDocument.
func (a *Asciidoctor) ConvertFileDocument(path string, options *Options) (doc *Document, output string, err error) {
	return a.convertDocument("convert_file", path, options)
}

func (a *Asciidoctor) convertDocument(method, input string, options *Options) (*Document, string, error) {
	var writer *bridge.Writer
	if options != nil && options.Output != nil {
		writer = &bridge.Writer{W: options.Output}
	}
	var docValue ruby.Value
	var out string
	err := a.do(func() error {
		g, err := a.glue()
		if err != nil {
			return err
		}
		v, err := g.Call("convert_document", ruby.Symbol(method), input, options.rubyOptions(writer))
		if err != nil {
			return err
		}
		if docValue, err = v.Call("[]", 0); err != nil {
			return err
		}
		o, err := v.Call("[]", 1)
		if err != nil {
			return err
		}
		if !o.IsNil() {
			out = o.String()
		}
		return nil
	})
	if err == nil && writer != nil && writer.Err != nil {
		err = writer.Err
	}
	if err != nil {
		return nil, "", err
	}
	return a.wrapDocument(docValue), out, nil
}

// ConvertReader converts the AsciiDoc source read from r and writes the result
// to w (overriding options.Output).
func (a *Asciidoctor) ConvertReader(r io.Reader, w io.Writer, options *Options) error {
	if r == nil {
		return errors.New("asciidoctor: ConvertReader: nil reader")
	}
	if w == nil {
		return errors.New("asciidoctor: ConvertReader: nil writer")
	}
	src, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	o := Options{}
	if options != nil {
		o = *options
	}
	o.Output = w
	_, err = a.Convert(string(src), &o)
	return err
}

// ConvertFiles converts each file as ConvertFile does, and returns the
// results in the same order. It stops at the first error, returning the
// results so far.
func (a *Asciidoctor) ConvertFiles(paths []string, options *Options) ([]string, error) {
	results := make([]string, 0, len(paths))
	for _, p := range paths {
		r, err := a.ConvertFile(p, options)
		if err != nil {
			return results, err
		}
		results = append(results, r)
	}
	return results, nil
}

// AsciidoctorVersion is the version of the Asciidoctor implementation, such as
// "2.1.0.alpha.0".
func (a *Asciidoctor) AsciidoctorVersion() (string, error) {
	var v string
	err := a.do(func() error {
		c, err := a.b.Const("Asciidoctor::VERSION")
		if err != nil {
			return err
		}
		v = c.String()
		return nil
	})
	return v, err
}

var (
	defaultOnce     sync.Once
	defaultInstance *Asciidoctor
	defaultErr      error
)

// Default returns the shared instance used by the package-level functions,
// starting it on first use. Do not Close it.
func Default() (*Asciidoctor, error) {
	defaultOnce.Do(func() { defaultInstance, defaultErr = New() })
	return defaultInstance, defaultErr
}

// Convert converts AsciiDoc source with the default instance; see
// Asciidoctor.Convert.
func Convert(source string, options *Options) (string, error) {
	a, err := Default()
	if err != nil {
		return "", err
	}
	return a.Convert(source, options)
}

// ConvertFile converts an AsciiDoc file with the default instance; see
// Asciidoctor.ConvertFile.
func ConvertFile(path string, options *Options) (string, error) {
	a, err := Default()
	if err != nil {
		return "", err
	}
	return a.ConvertFile(path, options)
}
