package asciidoctor_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/asciidoctor-go/asciidoctor"
)

// Go versions of the extensions in AsciidoctorJ's test suite
// (org.asciidoctor.extension), shared by the asciidoctorj_*extension*_test.go
// files.

// ajRubyClassURL is the URI the sample-with-uri-include.ad fixture includes,
// and ajRubyClassPrefix the start of the file served at it.
const (
	ajRubyClassURL    = "http://example.com/asciidoctorclass.rb"
	ajRubyClassPrefix = "module AsciidoctorJ    include_package 'org.asciidoctor'"
)

// ajProxy is an HTTP proxy that serves the files in routes (absolute URL to
// local path), like AsciidoctorJ's TestHttpServer.
func ajProxy(t *testing.T, routes map[string]string) *http.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ok := routes[r.URL.String()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	proxy, _ := url.Parse(srv.URL)
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}
}

// ajRubyClassProxy serves AsciidoctorJ's asciidoctorclass.rb at ajRubyClassURL.
func ajRubyClassProxy(t *testing.T) *http.Client {
	return ajProxy(t, map[string]string{
		ajRubyClassURL: "third_party/asciidoctorj/asciidoctorj-core/src/main/resources/org/asciidoctor/jruby/internal/asciidoctorclass.rb",
	})
}

// uriIncludeProcessor includes http(s) targets fetched through client, with
// line breaks removed as UriIncludeProcessor does.
func uriIncludeProcessor(client *http.Client) asciidoctor.IncludeProcessor {
	return asciidoctor.IncludeProcessorFuncs{
		HandlesFunc: func(_ *asciidoctor.Document, target string) bool {
			return strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")
		},
		ProcessFunc: func(_ *asciidoctor.Document, reader *asciidoctor.PreprocessorReader, target string, attrs map[string]any) error {
			resp, err := client.Get(target)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("GET %s: %s", target, resp.Status)
			}
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			content := strings.ReplaceAll(strings.ReplaceAll(string(b), "\r", ""), "\n", "")
			reader.PushInclude([]string{content}, target, target, 1, attrs)
			return nil
		},
	}
}

// positionalAttrsIncludeProcessor includes the include's attribute values,
// sorted by name and joined by commas.
var positionalAttrsIncludeProcessor = asciidoctor.IncludeProcessorFuncs{
	HandlesFunc: func(*asciidoctor.Document, string) bool { return true },
	ProcessFunc: func(_ *asciidoctor.Document, reader *asciidoctor.PreprocessorReader, target string, attrs map[string]any) error {
		names := make([]string, 0, len(attrs))
		for k := range attrs {
			names = append(names, k)
		}
		sort.Strings(names)
		values := make([]string, len(names))
		for i, k := range names {
			values[i] = fmt.Sprint(attrs[k])
		}
		reader.PushInclude([]string{strings.Join(values, ",")}, target, target, 1, attrs)
		return nil
	},
}

// metaRobotsDocinfo adds a robots meta element.
var metaRobotsDocinfo = asciidoctor.DocinfoProcessorFunc(func(*asciidoctor.Document) (string, error) {
	return `<meta name="robots" content="index,follow">`, nil
})

// changeAttributeValuePreprocessor sets the content attribute to Alex.
var changeAttributeValuePreprocessor = asciidoctor.PreprocessorFunc(func(doc *asciidoctor.Document, reader *asciidoctor.PreprocessorReader) (*asciidoctor.Reader, error) {
	doc.SetAttr("content", "Alex", true)
	return nil, nil
})

const ajCopyright = "Copyright Acme, Inc."

// customFooterPostprocessor appends a copyright notice to the footer text of
// HTML output.
var customFooterPostprocessor = asciidoctor.PostprocessorFunc(func(doc *asciidoctor.Document, output string) (string, error) {
	if !doc.IsBasebackend("html") {
		return output, nil
	}
	start := strings.Index(output, `<div id="footer-text">`)
	if start < 0 {
		return output, nil
	}
	end := start + strings.Index(output[start:], "</div>")
	return output[:end] + ajCopyright + output[end:], nil
})

// terminalCommandTreeprocessor replaces paragraphs that start with "$" by
// listings with the commands marked up.
var terminalCommandTreeprocessor = asciidoctor.TreeprocessorFunc(func(doc *asciidoctor.Document) (*asciidoctor.Document, error) {
	var process func(node asciidoctor.StructuralNode) error
	process = func(node asciidoctor.StructuralNode) error {
		for i, child := range node.Blocks() {
			if child.Context() != "paragraph" {
				if err := process(child); err != nil {
					return err
				}
				continue
			}
			lines := child.(*asciidoctor.Block).Lines()
			if len(lines) == 0 || !strings.HasPrefix(lines[0], "$") {
				continue
			}
			attrs := asciidoctor.Attributes(child.Attributes())
			attrs["role"] = "terminal"
			var out strings.Builder
			for _, line := range lines {
				if strings.HasPrefix(line, "$") {
					out.WriteString(`<span class="command">` + line[2:] + "</span>")
				} else {
					out.WriteString(line)
				}
			}
			listing, err := asciidoctor.CreateBlock(doc, "listing", []string{out.String()}, attrs, nil)
			if err != nil {
				return err
			}
			node.SetBlock(i, listing)
		}
		return nil
	}
	return doc, process(doc)
})

// gistMacro embeds a gist.
var gistMacro = asciidoctor.BlockMacroProcessorFunc(func(parent asciidoctor.StructuralNode, target string, attrs map[string]any) (asciidoctor.StructuralNode, error) {
	content := "<div class=\"content\">\n<script src=\"https://gist.github.com/" + target + ".js\"></script>\n</div>"
	return asciidoctor.CreatePassBlock(parent, []string{content}, attrs, nil)
})

// manpageMacro links to the HTML page of a man page.
var manpageMacro = asciidoctor.InlineMacroProcessorFunc(func(parent asciidoctor.Node, target string, attrs map[string]any) (*asciidoctor.PhraseNode, error) {
	return asciidoctor.CreateAnchor(parent, target, attrs, map[string]any{"type": "link", "target": target + ".html"})
})

// sayMacro emphasizes its target strongly, through the normal substitutions.
var sayMacro = asciidoctor.InlineMacroProcessorFunc(func(parent asciidoctor.Node, target string, _ map[string]any) (*asciidoctor.PhraseNode, error) {
	return asciidoctor.CreatePhraseNode(parent, "quoted", "*"+target+"*", asciidoctor.Attributes{"subs": "normal"}, nil)
})

// yellBlock converts the block's lines to upper case in a paragraph.
var yellBlock = asciidoctor.BlockProcessorFunc(func(parent asciidoctor.StructuralNode, reader *asciidoctor.Reader, attrs map[string]any) (asciidoctor.StructuralNode, error) {
	return asciidoctor.CreateParagraph(parent, []string{strings.ToUpper(strings.Join(reader.ReadLines(), "\n"))}, attrs, nil)
})

func yellOptions(context string) *asciidoctor.BlockOptions {
	return &asciidoctor.BlockOptions{Contexts: []string{context}, ContentModel: asciidoctor.ContentModelSimple}
}

const ajYelled = "THE TIME IS NOW. GET A MOVE ON."

// ajConvertResource converts a copy of a resource to a string.
func ajConvertResource(t *testing.T, a *asciidoctor.Asciidoctor, name string, o *asciidoctor.Options) string {
	t.Helper()
	opts := asciidoctor.Options{}
	if o != nil {
		opts = *o
	}
	opts.NoOutputFile = true
	return mustConvertFile(t, a, ajResource(t, name), &opts)
}

// ajConvertToFile converts a copy of a resource to an HTML file and returns
// the parsed file.
func ajConvertToFile(t *testing.T, a *asciidoctor.Asciidoctor, name string) *htmlDoc {
	t.Helper()
	out := filepath.Join(t.TempDir(), strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))+".html")
	mustConvertFile(t, a, ajResource(t, name), &asciidoctor.Options{Safe: asciidoctor.SafeModeUnsafe, ToFile: out})
	return parseHTMLFile(t, out)
}

// logRecorder collects an instance's log records.
type logRecorder struct {
	mu      sync.Mutex
	records []asciidoctor.LogRecord
}

func recordLogs(t *testing.T, a *asciidoctor.Asciidoctor) *logRecorder {
	t.Helper()
	r := &logRecorder{}
	unregister, err := a.RegisterLogHandler(asciidoctor.LogHandlerFunc(func(rec asciidoctor.LogRecord) {
		r.mu.Lock()
		r.records = append(r.records, rec)
		r.mu.Unlock()
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unregister() })
	return r
}

func (r *logRecorder) Records() []asciidoctor.LogRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]asciidoctor.LogRecord(nil), r.records...)
}

// mustRegister checks the result of a registration:
// mustRegister(t)(a.Extensions().Block(...)).
func mustRegister(t *testing.T) func(*asciidoctor.ExtensionGroup, error) *asciidoctor.ExtensionGroup {
	t.Helper()
	return func(g *asciidoctor.ExtensionGroup, err error) *asciidoctor.ExtensionGroup {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
}
