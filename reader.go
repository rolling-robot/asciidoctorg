package asciidoctor

import (
	ruby "github.com/go-embedded-ruby/ruby"
)

// Reader reads the lines of a source, as Asciidoctor's parser does. Block
// processors receive the lines of the block; preprocessors and include
// processors receive the document's PreprocessorReader.
type Reader struct {
	a *Asciidoctor
	v ruby.Value
}

func (r *Reader) call(method string, args ...any) ruby.Value {
	v, err := r.a.callValue(r.v, method, args...)
	if err != nil {
		panic(err)
	}
	return v
}

// Value is the underlying Ruby object.
func (r *Reader) Value() ruby.Value { return r.v }

// HasMoreLines reports whether lines remain.
func (r *Reader) HasMoreLines() bool { return rubyBool(r.call("has_more_lines?")) }

// IsNextLineEmpty reports whether the next line is empty (or there is none).
func (r *Reader) IsNextLineEmpty() bool { return rubyBool(r.call("next_line_empty?")) }

// PeekLine returns the next line without consuming it; ok is false at the end.
func (r *Reader) PeekLine() (line string, ok bool) {
	v := r.call("peek_line")
	return rubyString(v), !v.IsNil()
}

// PeekLines returns up to n next lines without consuming them.
func (r *Reader) PeekLines(n int) []string { return rubyStrings(r.call("peek_lines", n)) }

// ReadLine consumes and returns the next line; ok is false at the end.
func (r *Reader) ReadLine() (line string, ok bool) {
	v := r.call("read_line")
	return rubyString(v), !v.IsNil()
}

// ReadLines consumes and returns the remaining lines.
func (r *Reader) ReadLines() []string { return rubyStrings(r.call("read_lines")) }

// Read consumes the remaining lines and returns them joined by newlines.
func (r *Reader) Read() string { return rubyString(r.call("read")) }

// Lines returns a copy of the remaining lines, without consuming them.
func (r *Reader) Lines() []string { return rubyStrings(r.call("lines")) }

// String returns the remaining lines joined by newlines, without consuming
// them.
func (r *Reader) String() string { return rubyString(r.call("string")) }

// Source returns all the source lines given to the reader, joined.
func (r *Reader) Source() string { return rubyString(r.call("source")) }

// Advance skips the next line; it reports whether there was one.
func (r *Reader) Advance() bool { return rubyBool(r.call("advance")) }

// RestoreLine puts a line back at the front of the reader.
func (r *Reader) RestoreLine(line string) { r.call("unshift_line", line) }

// RestoreLines puts lines back at the front of the reader, in order.
func (r *Reader) RestoreLines(lines []string) {
	r.call("unshift_lines", append([]string{}, lines...))
}

// ReplaceNextLine replaces the next line.
func (r *Reader) ReplaceNextLine(line string) { r.call("replace_next_line", line) }

// SkipBlankLines skips blank lines and returns how many were skipped.
func (r *Reader) SkipBlankLines() int { return rubyInt(r.call("skip_blank_lines")) }

// SkipCommentLines skips comment lines and blocks.
func (r *Reader) SkipCommentLines() { r.call("skip_comment_lines") }

// Terminate consumes all remaining lines.
func (r *Reader) Terminate() { r.call("terminate") }

// LineNumber is the line number of the next line.
func (r *Reader) LineNumber() int { return rubyInt(r.call("lineno")) }

// File is the file being read ("" for a string source).
func (r *Reader) File() string { return rubyString(r.call("file")) }

// Dir is the directory of the file being read.
func (r *Reader) Dir() string { return rubyString(r.call("dir")) }

// Path is the path of the file as given, or "<stdin>".
func (r *Reader) Path() string { return rubyString(r.call("path")) }

// Cursor is the position of the next line.
func (r *Reader) Cursor() *Cursor { return r.a.cursor(r.call("cursor")) }

// ReadUntilOptions control ReadLinesUntil.
type ReadUntilOptions struct {
	// Terminator stops reading at a line equal to it.
	Terminator string
	// BreakOnBlankLines stops at a blank line.
	BreakOnBlankLines bool
	// BreakOnListContinuation stops at a list continuation ("+").
	BreakOnListContinuation bool
	// SkipFirstLine skips the first line before reading.
	SkipFirstLine bool
	// PreserveLastLine leaves the line that stopped reading in the reader.
	PreserveLastLine bool
	// ReadLastLine includes the line that stopped reading in the result.
	ReadLastLine bool
	// SkipLineComments drops line comments.
	SkipLineComments bool
	// SkipProcessing reads lines without preprocessing them.
	SkipProcessing bool
}

// ReadLinesUntil reads lines until a stop condition in opts or until stop, if
// non-nil, returns true for a line.
func (r *Reader) ReadLinesUntil(opts ReadUntilOptions, stop func(line string) bool) []string {
	o := map[ruby.Symbol]any{}
	if opts.Terminator != "" {
		o["terminator"] = opts.Terminator
	}
	set := func(k ruby.Symbol, b bool) {
		if b {
			o[k] = true
		}
	}
	set("break_on_blank_lines", opts.BreakOnBlankLines)
	set("break_on_list_continuation", opts.BreakOnListContinuation)
	set("skip_first_line", opts.SkipFirstLine)
	set("preserve_last_line", opts.PreserveLastLine)
	set("read_last_line", opts.ReadLastLine)
	set("skip_line_comments", opts.SkipLineComments)
	set("skip_processing", opts.SkipProcessing)
	var handle any
	if stop != nil {
		id := r.a.b.Register(lineHandler(stop))
		defer r.a.b.Unregister(id)
		handle = id
	}
	g, err := r.a.glueValue()
	if err != nil {
		panic(err)
	}
	v, err := r.a.callValue(g, "read_lines_until", r.v, o, handle)
	if err != nil {
		panic(err)
	}
	return rubyStrings(v)
}

type lineHandler func(string) bool

func (h lineHandler) Dispatch(_ *ruby.Call, _ string, args []ruby.Value) (any, error) {
	return h(rubyString(args[0])), nil
}

// PreprocessorReader is the reader of a whole document, which processes
// preprocessor directives (includes and conditionals) as lines are read.
type PreprocessorReader struct {
	*Reader
}

// PushInclude pushes source lines onto the reader as though they were
// included from file (absolute path) shown as path, starting at lineNumber.
// The attributes are those of the include directive.
func (r *PreprocessorReader) PushInclude(data []string, file, path string, lineNumber int, attributes map[string]any) {
	if attributes == nil {
		attributes = map[string]any{}
	}
	r.call("push_include", append([]string{}, data...), nilIfEmpty(file), nilIfEmpty(path), lineNumber, attributes)
}

// IncludeDepth is the depth of nested includes being read.
func (r *PreprocessorReader) IncludeDepth() int { return rubyInt(r.call("include_depth")) }

// Document is the document being read.
func (r *PreprocessorReader) Document() *Document {
	return r.a.wrapDocument(r.call("instance_variable_get", ruby.Symbol("@document")))
}

// NewReader creates a reader over lines, for example to return a replacement
// reader from a Preprocessor or to parse content with ParseContent. cursor,
// if non-nil, sets the reported file and starting line.
func (a *Asciidoctor) NewReader(lines []string, cursor *Cursor) (*Reader, error) {
	var v ruby.Value
	err := a.do(func() error {
		readerClass, err := a.b.Const("Asciidoctor::Reader")
		if err != nil {
			return err
		}
		var c any
		if cursor != nil {
			cursorClass, err := a.b.Const("Asciidoctor::Reader::Cursor")
			if err != nil {
				return err
			}
			cv, err := cursorClass.Call("new", nilIfEmpty(cursor.File), nilIfEmpty(cursor.Dir), nilIfEmpty(cursor.Path), max(cursor.LineNumber, 1))
			if err != nil {
				return err
			}
			c = cv
		}
		v, err = readerClass.Call("new", append([]string{}, lines...), c)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Reader{a: a, v: v}, nil
}
