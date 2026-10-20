package asciidoctor_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Ported from AsciidoctorJ's WhenAsciidoctorLogsToConsole and
// WhenTwoAsciidoctorsLogToConsole.

const ajRemoteRenderSample = "https://raw.githubusercontent.com/asciidoctor/asciidoctorj/e47a2c01c563fa624ca61e1bc58e264a114f7fb5/asciidoctorj-core/src/test/resources/rendersample.asciidoc"

// notExistingFileDocument copies documentwithnotexistingfile.adoc with its
// remote include pointed at a local server that serves the same file
// (rendersample.asciidoc), so the test needs no network.
func notExistingFileDocument(t *testing.T) string {
	t.Helper()
	dir := ajResources(t)
	sample := readFile(t, filepath.Join(dir, "rendersample.asciidoc"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sample)) }))
	t.Cleanup(srv.Close)
	path := filepath.Join(dir, "documentwithnotexistingfile.adoc")
	src := readFile(t, path)
	if !strings.Contains(src, ajRemoteRenderSample) {
		t.Fatal("fixture does not include the remote sample")
	}
	if err := os.WriteFile(path, []byte(strings.Replace(src, ajRemoteRenderSample, srv.URL+"/rendersample.asciidoc", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func notExistingFileOptions() *asciidoctor.Options {
	return &asciidoctor.Options{Safe: asciidoctor.SafeModeServer, Attributes: asciidoctor.Attributes{}.AllowURIRead(true)}
}

// assertNotExistingFileRecords checks the four messages of converting
// documentwithnotexistingfile.adoc: three missing includes and the level 0
// section of the remote include.
func assertNotExistingFileRecords(t *testing.T, recs []asciidoctor.LogRecord, path string) {
	t.Helper()
	if len(recs) != 4 {
		for _, r := range recs {
			t.Logf("%s: %s", r.Severity, r.Message)
		}
		t.Fatalf("records = %d, want 4", len(recs))
	}
	if !strings.Contains(recs[0].Message, "include file not found") {
		t.Errorf("first message = %q", recs[0].Message)
	}
	c := recs[0].Cursor
	if c == nil {
		t.Fatal("first record has no cursor")
	}
	eqv(t, "dir", filepath.ToSlash(c.Dir), filepath.ToSlash(filepath.Dir(path)))
	eqv(t, "file", filepath.Base(c.File), filepath.Base(path))
	eqv(t, "line", c.LineNumber, 3)
	for _, r := range recs {
		if r.Cursor == nil || r.Cursor.File == "" || r.Cursor.Dir == "" {
			t.Errorf("record %q has an incomplete cursor %+v", r.Message, r.Cursor)
		}
	}
}

func TestAJLogHandlerNotified(t *testing.T) {
	a := newInstance(t)
	logs := recordLogs(t, a)
	path := notExistingFileDocument(t)
	mustConvertFile(t, a, path, notExistingFileOptions())
	assertNotExistingFileRecords(t, logs.Records(), path)
}

// AsciidoctorJ formats the message of a record with a cursor for
// java.util.logging; the Go record keeps them apart, and Cursor.String
// gives the same "file: line N" context.
func TestAJLogRecordContext(t *testing.T) {
	a := newInstance(t)
	logs := recordLogs(t, a)
	path := notExistingFileDocument(t)
	mustConvertFile(t, a, path, notExistingFileOptions())
	recs := logs.Records()
	if len(recs) == 0 {
		t.Fatal("no records")
	}
	if got := recs[0].Cursor.String(); !strings.Contains(got, "documentwithnotexistingfile.adoc: line 3") {
		t.Errorf("cursor = %q", got)
	}
}

// "possible invalid reference" is an INFO message; AsciidoctorJ passes INFO
// to its handlers, while the Go default level is WARN, as in Asciidoctor.
func TestAJLogInvalidRefs(t *testing.T) {
	a := newInstance(t)
	logs := recordLogs(t, a)
	if err := a.SetLogLevel(asciidoctor.SeverityInfo); err != nil {
		t.Fatal(err)
	}
	mustConvertFile(t, a, ajResource(t, "documentwithinvalidrefs.adoc"), &asciidoctor.Options{
		Safe: asciidoctor.SafeModeServer, NoOutputFile: true, Attributes: asciidoctor.Attributes{}.AllowURIRead(true),
	})
	recs := logs.Records()
	if len(recs) != 1 {
		t.Fatalf("records = %+v, want 1", recs)
	}
	if !strings.Contains(recs[0].Message, "invalid reference: invalidref") {
		t.Errorf("message = %q", recs[0].Message)
	}
	if recs[0].Cursor != nil {
		t.Errorf("cursor = %+v, want nil", recs[0].Cursor)
	}
}

func TestAJLogOnlyFromRegisteredInstance(t *testing.T) {
	a, other := newInstance(t), newInstance(t, asciidoctor.WithStderr(io.Discard))
	logs := recordLogs(t, a)
	path := notExistingFileDocument(t)
	mustConvertFile(t, other, path, notExistingFileOptions())
	eqv(t, "records from the other instance", len(logs.Records()), 0)
	mustConvertFile(t, a, path, notExistingFileOptions())
	assertNotExistingFileRecords(t, logs.Records(), path)
}

func TestAJLogHandlerUnregistered(t *testing.T) {
	a := newInstance(t, asciidoctor.WithStderr(io.Discard))
	var count int
	unregister, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(asciidoctor.LogRecord) { count++ }))
	if err != nil {
		t.Fatal(err)
	}
	path := notExistingFileDocument(t)
	mustConvertFile(t, a, path, notExistingFileOptions())
	eqv(t, "records", count, 4)
	count = 0
	if err := unregister(); err != nil {
		t.Fatal(err)
	}
	mustConvertFile(t, a, path, notExistingFileOptions())
	eqv(t, "records after unregister", count, 0)
}

// AsciidoctorJ discovers log handlers with Java's service loader; Go packages
// register a setup.
func TestAJLogHandlerFromSetup(t *testing.T) {
	var recs []asciidoctor.LogRecord
	var instances atomic.Int32
	remove := asciidoctor.RegisterSetup(func(a *asciidoctor.Asciidoctor) error {
		instances.Add(1)
		_, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(r asciidoctor.LogRecord) { recs = append(recs, r) }))
		return err
	})
	defer remove()
	a := newInstance(t)
	path := notExistingFileDocument(t)
	mustConvertFile(t, a, path, notExistingFileOptions())
	assertNotExistingFileRecords(t, recs, path)
	// WhenTwoAsciidoctorsLogToConsole: each instance gets its own handler.
	mustConvert(t, newInstance(t), "Test", nil)
	eqv(t, "setups run", instances.Load(), int32(2))
}

func TestAJExtensionLogs(t *testing.T) {
	a := newInstance(t)
	logs := recordLogs(t, a)
	if err := a.SetLogLevel(asciidoctor.SeverityInfo); err != nil {
		t.Fatal(err)
	}
	mustRegister(t)(a.Extensions().Block("big", asciidoctor.BlockProcessorFunc(func(parent asciidoctor.StructuralNode, reader *asciidoctor.Reader, attrs map[string]any) (asciidoctor.StructuralNode, error) {
		if err := parent.Asciidoctor().Log(asciidoctor.LogRecord{Severity: asciidoctor.SeverityInfo, Cursor: parent.SourceLocation(), Message: "Hello Log"}); err != nil {
			return nil, err
		}
		lines := reader.ReadLines()
		for i, l := range lines {
			lines[i] = strings.ToUpper(l)
		}
		return asciidoctor.CreateParagraph(parent, lines, nil, nil)
	}), nil))
	out := mustConvert(t, a, "= Test\n\n== Something different\n\n[big]\nHello World", &asciidoctor.Options{SourceMap: true})
	recs := logs.Records()
	if len(recs) != 1 {
		t.Fatalf("records = %+v, want 1", recs)
	}
	eqv(t, "message", recs[0].Message, "Hello Log")
	if recs[0].Cursor == nil {
		t.Fatal("no cursor")
	}
	eqv(t, "line", recs[0].Cursor.LineNumber, 3)
	if !strings.Contains(out, "HELLO WORLD") {
		t.Errorf("output = %s", out)
	}
}

// A LogHandler cannot return an error; one that panics aborts the
// conversion, which fails with the panic as the cause.
func TestAJPanickingLogHandlerFailsConversion(t *testing.T) {
	a := newInstance(t)
	abort := errors.New("Conversion aborted by LogHandler")
	if _, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(asciidoctor.LogRecord) { panic(abort) })); err != nil {
		t.Fatal(err)
	}
	if err := a.SetLogLevel(asciidoctor.SeverityInfo); err != nil {
		t.Fatal(err)
	}
	_, err := a.Convert("= Test\n\nSee <<missing>>.", nil)
	if err == nil {
		t.Fatal("conversion succeeded")
	}
	if !errors.Is(err, abort) && !strings.Contains(err.Error(), abort.Error()) {
		t.Errorf("err = %v", err)
	}
}
