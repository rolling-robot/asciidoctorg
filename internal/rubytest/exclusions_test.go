package rubytest

// The reasons a test of Asciidoctor's suite cannot run here.
const (
	spawnsRuby = "spawns the ruby executable (Gem.ruby) to run bin/asciidoctor or a script; " +
		"an embedded runtime has no Ruby executable"
	needsOpenURICached = "requires the open-uri-cached gem (cache-uri attribute), which is not available"
	needsMonokai       = "uses Rouge's monokai theme; go-rouge bundles only the base16, github and " +
		"thankful_eyes themes"
	fifoOpenBlocks = "rbgo holds the interpreter lock while opening a FIFO for reading, so the " +
		"writer thread never runs (docs/rbgo-gaps.md)"
)

// exclusions lists the tests of Asciidoctor's suite that do not run here, by
// test file, then "Class#test_method" ("Class#*" for every test of a class,
// "*" for the whole file), with the
// reason. A test listed here is reported as skipped with its reason; every
// other test must pass.
var exclusions = map[string]map[string]string{
	"blocks_test": {
		"Minitest::Test::TestBlocks::TestImages#test_should_cache_remote_SVG_when_allow_uri_read_cache_uri_and_inline_option_are_set": needsOpenURICached,
		"Minitest::Test::TestBlocks::TestImages#test_should_cache_remote_image_when_allow_uri_read_cache_uri_and_data_uri_are_set":    needsOpenURICached,
	},
	"invoker_test": {
		"Minitest::Test::TestInvoker#test_should_accept_input_from_named_pipe_and_output_to_stdout":                      fifoOpenBlocks,
		"Minitest::Test::TestInvoker#test_should_force_default_external_encoding_to_UTF_8":                               spawnsRuby,
		"Minitest::Test::TestInvoker#test_should_force_stdio_encoding_to_UTF_8":                                          spawnsRuby,
		"Minitest::Test::TestInvoker#test_should_not_emit_any_unexpected_warnings":                                       spawnsRuby,
		"Minitest::Test::TestInvoker#test_should_not_fail_to_load_if_call_to_Dir_home_fails":                             spawnsRuby,
		"Minitest::Test::TestInvoker#test_should_show_backtrace_when_trace_option_is_specified_and_program_raises_error": spawnsRuby,
		"Minitest::Test::TestInvoker#test_should_show_timezone_as_UTC_if_system_TZ_is_set_to_UTC":                        spawnsRuby,
		"Minitest::Test::TestInvoker#test_should_show_timezone_as_offset_if_system_TZ_is_not_set_to_UTC":                 spawnsRuby,
	},
	"logger_test": {
		"Minitest::Test::TestLogger::TestLogger#test_should_extend_Logger_from_logger_gem_if_gem_is_loaded": spawnsRuby,
	},
	"syntax_highlighter_test": {
		"Minitest::Test::TestSyntaxHighlighter::TestRouge#test_should_not_fail_to_load_rouge_if_the_Asciidoctor_module_is_included_into_the_global_namespace": spawnsRuby,
		"Minitest::Test::TestSyntaxHighlighter::TestRouge#test_should_read_stylesheet_for_specified_style":                                                    needsMonokai,
	},
	"substitutions_test": {},
}

func excluded(file, class, method string) (string, bool) {
	if m, ok := exclusions[file]; ok {
		if reason, ok := m[class+"#"+method]; ok {
			return reason, true
		}
		if reason, ok := m[class+"#*"]; ok {
			return reason, true
		}
		if reason, ok := m["*"]; ok {
			return reason, true
		}
	}
	return "", false
}

// fileExcluded reports whether a whole test file is excluded.
func fileExcluded(file string) (string, bool) {
	reason, ok := exclusions[file]["*"]
	return reason, ok
}
