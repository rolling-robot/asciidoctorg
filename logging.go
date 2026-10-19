package asciidoctor

import (
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
	l := &a.logging
	l.mu.Lock()
	l.nextID++
	id := l.nextID
	if l.handlers == nil {
		l.handlers = map[int]LogHandler{}
	}
	l.handlers[id] = h
	l.order = append(l.order, id)
	needInstall := !l.installed
	l.mu.Unlock()
	if needInstall {
		if err := a.installLogger(); err != nil {
			a.removeLogHandler(id)
			return nil, err
		}
	}
	var once sync.Once
	return func() error {
		var err error
		once.Do(func() { err = a.removeLogHandler(id) })
		return err
	}, nil
}

func (a *Asciidoctor) removeLogHandler(id int) error {
	l := &a.logging
	l.mu.Lock()
	delete(l.handlers, id)
	for i, x := range l.order {
		if x == id {
			l.order = append(l.order[:i:i], l.order[i+1:]...)
			break
		}
	}
	uninstall := len(l.handlers) == 0 && l.installed
	l.mu.Unlock()
	if uninstall {
		return a.uninstallLogger()
	}
	return nil
}

// SetLogLevel sets the lowest severity delivered to log handlers and, while
// none is registered, written to standard error. The default is SeverityWarn.
func (a *Asciidoctor) SetLogLevel(level Severity) error {
	l := &a.logging
	l.mu.Lock()
	l.level = level
	l.mu.Unlock()
	return a.do(func() error {
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
	})
}

func (a *Asciidoctor) installLogger() error {
	l := &a.logging
	return a.do(func() error {
		l.mu.Lock()
		level := l.level
		l.mu.Unlock()
		id := a.b.Register(&logHandler{a: a})
		cls, err := a.b.Const("AsciidoctorGo::Logger")
		if err != nil {
			return err
		}
		logger, err := cls.Call("new", id, int(level))
		if err != nil {
			a.b.Unregister(id)
			return err
		}
		mgr, err := a.b.Const("Asciidoctor::LoggerManager")
		if err != nil {
			return err
		}
		if _, err := mgr.Call("logger=", logger); err != nil {
			return err
		}
		l.mu.Lock()
		l.installed = true
		l.loggerID = id
		l.mu.Unlock()
		return nil
	})
}

func (a *Asciidoctor) uninstallLogger() error {
	l := &a.logging
	return a.do(func() error {
		mgr, err := a.b.Const("Asciidoctor::LoggerManager")
		if err != nil {
			return err
		}
		// Assigning nil installs a default logger on $stderr.
		if _, err := mgr.Call("logger=", nil); err != nil {
			return err
		}
		l.mu.Lock()
		level, id := l.level, l.loggerID
		l.installed = false
		l.mu.Unlock()
		a.b.Unregister(id)
		logger, err := mgr.Call("logger")
		if err != nil {
			return err
		}
		_, err = logger.Call("level=", int(level))
		return err
	})
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
	for _, lh := range handlers {
		lh.Log(rec)
	}
	return nil, nil
}
