package asciidoctor

import (
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
}

// An Option configures New.
type Option func(*config)

type config struct {
	runtime runtime.Config
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
