package runtime

import "testing"

// D6: the Go-defined Ruby methods indexed their arguments without checking
// how many they were given. Each call below must raise a Ruby exception
// (ArgumentError or TypeError) instead of panicking.
func TestNativeMethodsCheckArguments(t *testing.T) {
	rt := bootShared(t)
	for _, call := range []string{
		"AsciidoctorGo::RougeNative.lex",
		"AsciidoctorGo::RougeNative.lex('ruby')",
		"AsciidoctorGo::RougeNative.guess",
		"AsciidoctorGo::RougeNative.format_html",
		"AsciidoctorGo::RougeNative.format_html_inline('github')",
		"AsciidoctorGo::RougeNative.theme_styles",
		"AsciidoctorGo::TemplateCompiler.haml('%p')",
		"AsciidoctorGo::TemplateCompiler.slim",
	} {
		t.Run(call, func(t *testing.T) {
			got := eval(t, rt, "begin; "+call+"; 'no error'; rescue ::ArgumentError, ::TypeError => e; e.class.name; end")
			if got != `"ArgumentError"` && got != `"TypeError"` {
				t.Fatalf("%s: got %s, want ArgumentError or TypeError", call, got)
			}
		})
	}
}
