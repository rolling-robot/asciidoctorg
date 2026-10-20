// Package runtime boots an rbgo interpreter with Asciidoctor loaded.
package runtime

import (
	"io"
	"path/filepath"

	ruby "github.com/go-embedded-ruby/ruby"
)

// Config configures Boot.
type Config struct {
	// Dir is the parent directory for the materialized Ruby tree. Empty means
	// DefaultCacheDir.
	Dir string
	// Stdout and Stderr receive the interpreter's $stdout and $stderr. Nil
	// discards the output.
	Stdout, Stderr io.Writer
	// AsciidoctorDir, if set, is an Asciidoctor source tree (with lib/ and
	// data/) to load instead of the embedded one, for example a full checkout
	// prepared with PatchTree. The shims and glue still come from the
	// embedded tree.
	AsciidoctorDir string
}

// Runtime is an interpreter with Asciidoctor loaded. It is not safe for
// concurrent use.
type Runtime struct {
	Interp *ruby.Interp
	// Root is the materialized Ruby tree.
	Root string
}

// Boot materializes the Ruby tree and loads Asciidoctor into a new interpreter.
func Boot(cfg Config) (*Runtime, error) {
	parent := cfg.Dir
	if parent == "" {
		var err error
		if parent, err = DefaultCacheDir(); err != nil {
			return nil, err
		}
	}
	root, err := Materialize(parent)
	if err != nil {
		return nil, err
	}
	stdout, stderr := cfg.Stdout, cfg.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	in := ruby.New(ruby.WithStdout(stdout), ruby.WithStderr(stderr))
	if err := in.Set("__agox_shim_dir", shimDir(root)); err != nil {
		return nil, err
	}
	lib := libDir(root)
	if cfg.AsciidoctorDir != "" {
		lib = filepath.Join(cfg.AsciidoctorDir, "lib")
	}
	if err := in.Set("__agox_lib_dir", lib); err != nil {
		return nil, err
	}
	if err := in.Set("__agox_glue_dir", glueDir(root)); err != nil {
		return nil, err
	}
	if err := in.Set("__agox_coderay_dir", coderayLibDir(root)); err != nil {
		return nil, err
	}
	if err := defineTemplateCompiler(in); err != nil {
		return nil, err
	}
	if err := defineRouge(in); err != nil {
		return nil, err
	}
	if _, err := in.Eval(bootScript, "(asciidoctor-go boot)", 1); err != nil {
		return nil, err
	}
	return &Runtime{Interp: in, Root: root}, nil
}

const bootScript = `
$LOAD_PATH.unshift __agox_shim_dir
require 'asciidoctor_go_prelude'
$LOAD_PATH.unshift __agox_lib_dir
$LOAD_PATH.push __agox_glue_dir
$LOAD_PATH.push __agox_coderay_dir
require 'asciidoctor'
require 'asciidoctor/extensions'
require 'asciidoctor_go_frame_local_match'
nil
`
