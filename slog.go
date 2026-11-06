package asciidoctor

import (
	"context"
	"log/slog"
)

// SlogLevel maps a severity to the log/slog level used by SlogHandler:
// Debug, Info, Warn and Error map to the slog levels of the same name, Fatal
// to slog.LevelError+4 and Unknown to slog.LevelError+8.
func (s Severity) SlogLevel() slog.Level {
	switch s {
	case SeverityDebug:
		return slog.LevelDebug
	case SeverityInfo:
		return slog.LevelInfo
	case SeverityWarn:
		return slog.LevelWarn
	case SeverityError:
		return slog.LevelError
	case SeverityFatal:
		return slog.LevelError + 4
	}
	return slog.LevelError + 8
}

// SlogHandler is a log handler that writes Asciidoctor's messages to a
// *slog.Logger. Register it with RegisterLogHandler:
//
//	unregister, err := a.RegisterLogHandler(asciidoctor.SlogHandler{Logger: slog.Default()})
//
// Each message becomes a record at Severity.SlogLevel, with the message as
// its text and, when Asciidoctor reports a source location, an "asciidoc" group
// holding "path", "file", "dir" and "line" (empty values left out; the group
// is not called "source", which slog uses for the Go caller). The
// record carries the context of the conversion that logged (see
// ContextLogHandler), so a context-aware slog.Handler sees its trace values.
//
// The instance's log level (SetLogLevel) decides which messages reach the
// handler; the logger's own level then applies.
type SlogHandler struct {
	// Logger receives the messages; nil means slog.Default().
	Logger *slog.Logger
}

var _ ContextLogHandler = SlogHandler{}

// Log writes record with context.Background().
func (h SlogHandler) Log(record LogRecord) { h.LogContext(context.Background(), record) }

// LogContext writes record to the logger, with ctx.
func (h SlogHandler) LogContext(ctx context.Context, record LogRecord) {
	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}
	level := record.Severity.SlogLevel()
	if !logger.Enabled(ctx, level) {
		return
	}
	var attrs []slog.Attr
	if c := record.Cursor; c != nil {
		var src []any
		if c.Path != "" {
			src = append(src, slog.String("path", c.Path))
		}
		if c.File != "" {
			src = append(src, slog.String("file", c.File))
		}
		if c.Dir != "" {
			src = append(src, slog.String("dir", c.Dir))
		}
		if c.LineNumber > 0 {
			src = append(src, slog.Int("line", c.LineNumber))
		}
		if len(src) > 0 {
			attrs = append(attrs, slog.Group("asciidoc", src...))
		}
	}
	logger.LogAttrs(ctx, level, record.Message, attrs...)
}
