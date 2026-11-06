package apitest

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/rolling-robot/asciidoctorg"
)

// duplicateID logs one warning: "id assigned to section already in use: a",
// at line 5 of <stdin>.
const duplicateID = "[[a]]\n== A\n\n[[a]]\n== B\n"

func TestSlogHandlerWritesRecords(t *testing.T) {
	a := newInstance(t)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	unregister, err := a.RegisterLogHandler(asciidoctor.SlogHandler{Logger: logger})
	check(t, err)
	defer unregister()
	mustConvert(t, a, duplicateID, nil)
	var rec struct {
		Level    string `json:"level"`
		Msg      string `json:"msg"`
		Asciidoc struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		} `json:"asciidoc"`
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	eq(t, "records", len(lines), 1)
	check(t, json.Unmarshal([]byte(lines[0]), &rec))
	eq(t, "record", []any{rec.Level, rec.Msg, rec.Asciidoc.Path, rec.Asciidoc.Line},
		[]any{"WARN", "id assigned to section already in use: a", "<stdin>", 5})
}

func TestSlogHandlerHonoursLoggerLevel(t *testing.T) {
	a := newInstance(t)
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	unregister, err := a.RegisterLogHandler(asciidoctor.SlogHandler{Logger: logger})
	check(t, err)
	defer unregister()
	mustConvert(t, a, duplicateID, nil)
	eq(t, "output below the logger's level", buf.String(), "")
}

type ctxKey struct{}

// ctxCapture is a slog.Handler that records the context of each record.
type ctxCapture struct {
	mu   *sync.Mutex
	vals *[]any
}

func (h ctxCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h ctxCapture) Handle(ctx context.Context, _ slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.vals = append(*h.vals, ctx.Value(ctxKey{}))
	return nil
}
func (h ctxCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h ctxCapture) WithGroup(string) slog.Handler      { return h }

// The context passed to ConvertContext reaches context-aware log handlers.
func TestContextLogHandlerReceivesCallContext(t *testing.T) {
	a := newInstance(t)
	var vals []any
	logger := slog.New(ctxCapture{mu: &sync.Mutex{}, vals: &vals})
	unregister, err := a.RegisterLogHandler(asciidoctor.SlogHandler{Logger: logger})
	check(t, err)
	defer unregister()
	ctx := context.WithValue(context.Background(), ctxKey{}, "request-42")
	if _, err := a.ConvertContext(ctx, duplicateID, nil); err != nil {
		t.Fatal(err)
	}
	mustConvert(t, a, duplicateID, nil) // no context: Background
	eq(t, "context values", vals, []any{"request-42", nil})
}

func TestSeveritySlogLevel(t *testing.T) {
	for _, tc := range []struct {
		s    asciidoctor.Severity
		want slog.Level
	}{
		{asciidoctor.SeverityDebug, slog.LevelDebug},
		{asciidoctor.SeverityInfo, slog.LevelInfo},
		{asciidoctor.SeverityWarn, slog.LevelWarn},
		{asciidoctor.SeverityError, slog.LevelError},
		{asciidoctor.SeverityFatal, slog.LevelError + 4},
		{asciidoctor.SeverityUnknown, slog.LevelError + 8},
	} {
		eq(t, tc.s.String(), tc.s.SlogLevel(), tc.want)
	}
}
