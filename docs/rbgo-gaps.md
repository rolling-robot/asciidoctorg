# rbgo gaps found while running Asciidoctor

Every divergence from MRI Ruby that asciidoctor-go works around, with a
minimal reproduction against rbgo v0.8.0 and where the workaround lives.
Expected results are MRI Ruby 3.4.10. These are candidates for upstream
reports; when rbgo fixes one, its workaround can be removed (each workaround
has a regression test in `internal/runtime`).

| # | Gap | Minimal reproduction (rbgo → MRI) | Workaround |
|---|---|---|---|
| 1 | No `cgi/escape` feature | `require 'cgi/escape'` → `LoadError` (MRI: loads) | `rubysrc/shims/cgi/escape.rb` requires `cgi` |
| 2 | `Gem.loaded_specs` undefined | `Gem.loaded_specs` → `NoMethodError` | prelude defines it |
| 3 | `StringScanner#get_byte` undefined | `StringScanner.new('a').get_byte` → `NoMethodError` | prelude adds it |
| 4 | `StringScanner` cannot match from inside a character | `s = StringScanner.new '¦x'; s.pos = 1; s.scan(/[^,]+/)` → `nil` (MRI: `"\xA6x"`) | `get_byte` scans a multibyte character whole |
| 5 | `File#mtime`, `#atime`, `#ctime` undefined | `File.open(f).mtime` → `NoMethodError` | prelude delegates to `File#stat` |
| 6 | `Module#===` ignores modules added with `extend` | `m = Module.new; m === ''.dup.extend(m)` → `false` (MRI: `true`; `is_a?` is correct) | prelude gives extended modules a correct `===` |
| 7 | `redo` keyword unsupported | `i = 0; while true; i += 1; redo if i < 2; break; end` → `NameError: undefined local variable or method 'redo'` | source patch (`internal/runtime/patch.go`) |
| 8 | Escapes in character literals mis-parsed | `?\u007f` → `"u"`; `?\x41` → `"\u0000"`; `?\C-a` → `"C"` (MRI: `"\u007F"`, `"A"`, `"\u0001"`) | source patches to double-quoted strings |
| 9 | Native `Logger` cannot be subclassed | `require 'logger'; class L < Logger; end; L.new(nil).class` → `Logger` (MRI: `L`); `Logger.allocate` + `initialize` fails | `rubysrc/shims/logger.rb` replaces it |
| 10 | Native `Rouge` lacks the gem's API | `Rouge::Lexer.find('ruby').new` → `NoMethodError` | reported as not installed (prelude) |
| 11 | Native libraries shadow `$LOAD_PATH` | a `logger.rb` earlier on `$LOAD_PATH` is ignored by `require 'logger'` | prelude intercepts `require` for replaced libraries |
| 12 | Native libraries' constants are predefined | `defined?(::Rouge)` → `"constant"` before any `require` | prelude removes replaced constants |
| 13 | `private_constant` still listed by `constants` | `module M; X = 1; private_constant :X; end; M.constants` → `[:X]` (MRI: `[]`) | avoided in shims |
| 14 | `TracePoint.new(:raise)` never fires | block is not called when an exception is raised | none needed (debugging aid only) |
| 15 | Parser accepts a command call in a rescue modifier | `x = 1 rescue p $!` parses (MRI: `SyntaxError`) | none needed |
| 16 | `NotImplementedError` inherits from `StandardError` | `NotImplementedError.superclass` → `StandardError` (MRI: `ScriptError`), so `rescue => e` catches it | prelude redefines it under `ScriptError` |
