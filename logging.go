package asciidoctor

import (
	"context"
	"errors"
	"sync"

	ruby "github.com/go-embedded-ruby/ruby"
)

// Severity is the severity of a log message.
type Severity int

// Severities, with Asciidoctor's numbering.
const (
	SeverityDebug Severity = iota
	SeverityInfo
	SeverityWarn
	SeverityError
	SeverityFatal
	SeverityUnknown
)

func (s Severity) String() string {
	switch s {
	case SeverityDebug:
		return "DEBUG"
	case SeverityInfo:
		return "INFO"
	case SeverityWarn:
		return "WARN"
	case SeverityError:
		return "ERROR"
	case SeverityFatal:
		return "FATAL"
	}
	return "UNKNOWN"
}

// LogRecord is a message logged by Asciidoctor.
type LogRecord struct {
	Severity Severity
	// Message is the text of the message.
	Message string
	// Cursor is the source location the message refers to, if any.
	Cursor *Cursor
}

// LogHandler receives Asciidoctor's log messages.
type LogHandler interface {
	Log(record LogRecord)
}

// ContextLogHandler is a LogHandler that also wants the context of the call
// that logged: the context passed to ConvertContext, LoadContext and the other
// *Context methods (context.Background() for the methods without one). A
// handler that implements it receives LogContext instead of Log, so it can
// pass trace and request values on, as SlogHandler does.
type ContextLogHandler interface {
	LogHandler
	LogContext(ctx context.Context, record LogRecord)
}

// LogHandlerFunc adapts a function to a LogHandler.
type LogHandlerFunc func(record LogRecord)

func (f LogHandlerFunc) Log(record LogRecord) { f(record) }

// logging routes Asciidoctor's logger to the registered Go handlers.
type logging struct {
	mu        sync.Mutex
	handlers  map[int]LogHandler
	order     []int
	nextID    int
	level     Severity
	installed bool
	loggerID  int64
}

// RegisterLogHandler adds a handler for log messages of the instance's log
// level and above (SeverityWarn unless changed with SetLogLevel). While at
// least one handler is registered, messages go to the handlers instead of
// standard error. The returned function unregisters the handler.
func (a *Asciidoctor) RegisterLogHandler(h LogHandler) (unregister func() error, err error) {
	if h == nil {
		return nil, errors.New("asciidoctor: RegisterLogHandler: nil handler")
	}
	var id int
	// Adding the handler and installing the Go logger happen together with
	// the interpreter held, as does removing the last handler and
	// uninstalling the logger, so the two sequences cannot interleave. The
	// interpreter is always taken before logging.mu, which is never held
	// while Ruby runs (Ruby calls the log handlers, which take it).
	err = a.do(func() error {
		l := &a.logging
		l.mu.Lock()
		l.nextID++
		id = l.nextID
		if l.handlers == nil {
			l.handlers = map[int]LogHandler{}
		}
		l.handlers[id] = h
		l.order = append(l.order, id)
		installed, level := l.installed, l.level
		l.mu.Unlock()
		if installed {
			return nil
		}
		if err := a.installLogger(level); err != nil {
			l.mu.Lock()
			l.removeHandler(id)
			l.mu.Unlock()
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() error {
		var err error
		once.Do(func() { err = a.removeLogHandler(id) })
		return err
	}, nil
}

// removeHandler forgets handler id. The caller holds l.mu.
func (l *logging) removeHandler(id int) {
	delete(l.handlers, id)
	for i, x := range l.order {
		if x == id {
			l.order = append(l.order[:i:i], l.order[i+1:]...)
			break
		}
	}
}

func (a *Asciidoctor) removeLogHandler(id int) error {
	return a.do(func() error { // see RegisterLogHandler for the locking
		l := &a.logging
		l.mu.Lock()
		l.removeHandler(id)
		uninstall := len(l.handlers) == 0 && l.installed
		l.mu.Unlock()
		if uninstall {
			return a.uninstallLogger()
		}
		return nil
	})
}

// Log logs a message through Asciidoctor's logger, as Asciidoctor's own
// messages are: to the registered log handlers, or else to standard error,
// if record.Severity is at least the log level. Extensions and converters
// use it to report problems.
func (a *Asciidoctor) Log(record LogRecord) error {
	var cursor any
	if c := record.Cursor; c != nil {
		cursor = []any{nilIfEmpty(c.File), nilIfEmpty(c.Dir), nilIfEmpty(c.Path), c.LineNumber}
	}
	return a.do(func() error {
		g, err := a.glue()
		if err != nil {
			return err
		}
		_, err = g.Call("log", int(record.Severity), record.Message, cursor)
		return err
	})
}

// SetLogLevel sets the lowest severity delivered to log handlers and, while
// none is registered, written to standard error. The default is SeverityWarn.
func (a *Asciidoctor) SetLogLevel(level Severity) error {
	return a.do(func() error {
		l := &a.logging
		l.mu.Lock()
		l.level = level
		l.mu.Unlock()
		return a.setRubyLogLevel(level)
	})
}

// setRubyLogLevel sets the level of Asciidoctor's current logger. The caller
// holds the interpreter.
func (a *Asciidoctor) setRubyLogLevel(level Severity) error {
	mgr, err := a.b.Const("Asciidoctor::LoggerManager")
	if err != nil {
		return err
	}
	logger, err := mgr.Call("logger")
	if err != nil {
		return err
	}
	_, err = logger.Call("level=", int(level))
	return err
}

// installLogger makes a Go-backed logger Asciidoctor's logger. The caller
// holds the interpreter, not l.mu.
func (a *Asciidoctor) installLogger(level Severity) error {
	id := a.b.Register(&logHandler{a: a})
	cls, err := a.b.Const("AsciidoctorGo::Logger")
	if err == nil {
		var logger ruby.Value
		if logger, err = cls.Call("new", id, int(level)); err == nil {
			var mgr ruby.Value
			if mgr, err = a.b.Const("Asciidoctor::LoggerManager"); err == nil {
				_, err = mgr.Call("logger=", logger)
			}
		}
	}
	if err != nil {
		a.b.Unregister(id)
		return err
	}
	l := &a.logging
	l.mu.Lock()
	l.installed = true
	l.loggerID = id
	l.mu.Unlock()
	return nil
}

// uninstallLogger restores Asciidoctor's default logger (on $stderr). The
// caller holds the interpreter, not l.mu.
func (a *Asciidoctor) uninstallLogger() error {
	mgr, err := a.b.Const("Asciidoctor::LoggerManager")
	if err != nil {
		return err
	}
	// Assigning nil installs a default logger on $stderr.
	if _, err := mgr.Call("logger=", nil); err != nil {
		return err
	}
	l := &a.logging
	l.mu.Lock()
	level, id := l.level, l.loggerID
	l.installed = false
	l.loggerID = 0
	l.mu.Unlock()
	a.b.Unregister(id)
	return a.setRubyLogLevel(level)
}

type logHandler struct {
	a *Asciidoctor
}

func (h *logHandler) Dispatch(_ *ruby.Call, method string, args []ruby.Value) (any, error) {
	if method != "log" || len(args) != 3 {
		return nil, unknownMethod(method)
	}
	sev, _ := args[0].Go().(int64)
	rec := LogRecord{Severity: Severity(sev), Message: rubyString(args[1]), Cursor: h.a.cursor(args[2])}
	l := &h.a.logging
	l.mu.Lock()
	handlers := make([]LogHandler, 0, len(l.order))
	for _, id := range l.order {
		handlers = append(handlers, l.handlers[id])
	}
	l.mu.Unlock()
	ctx := h.a.b.Context()
	for _, lh := range handlers {
		if ch, ok := lh.(ContextLogHandler); ok {
			ch.LogContext(ctx, rec)
		} else {
			lh.Log(rec)
		}
	}
	return nil, nil
}
