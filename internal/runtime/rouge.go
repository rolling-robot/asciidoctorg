package runtime

import (
	ruby "github.com/go-embedded-ruby/ruby"
	rouge "github.com/go-rouge/rouge"
)

// rougeTokens are Rouge's token names (Rouge::Token::Tokens), parents before
// children, in the order the gem declares them. go-rouge defines the same
// hierarchy but does not export a list.
var rougeTokens = []string{
	"Text", "Text.Whitespace",
	"Escape", "Error", "Other",
	"Keyword", "Keyword.Constant", "Keyword.Declaration", "Keyword.Namespace", "Keyword.Pseudo",
	"Keyword.Reserved", "Keyword.Type", "Keyword.Variable",
	"Name", "Name.Attribute", "Name.Builtin", "Name.Builtin.Pseudo", "Name.Class", "Name.Constant",
	"Name.Decorator", "Name.Entity", "Name.Exception", "Name.Function", "Name.Function.Magic",
	"Name.Property", "Name.Label", "Name.Namespace", "Name.Other", "Name.Tag", "Name.Variable",
	"Name.Variable.Class", "Name.Variable.Global", "Name.Variable.Instance", "Name.Variable.Magic",
	"Literal", "Literal.Date",
	"Literal.String", "Literal.String.Affix", "Literal.String.Backtick", "Literal.String.Char",
	"Literal.String.Delimiter", "Literal.String.Doc", "Literal.String.Double", "Literal.String.Escape",
	"Literal.String.Heredoc", "Literal.String.Interpol", "Literal.String.Other", "Literal.String.Regex",
	"Literal.String.Single", "Literal.String.Symbol",
	"Literal.Number", "Literal.Number.Bin", "Literal.Number.Float", "Literal.Number.Hex",
	"Literal.Number.Integer", "Literal.Number.Integer.Long", "Literal.Number.Oct", "Literal.Number.Other",
	"Operator", "Operator.Word",
	"Punctuation", "Punctuation.Indicator",
	"Comment", "Comment.Hashbang", "Comment.Doc", "Comment.Multiline", "Comment.Preproc",
	"Comment.PreprocFile", "Comment.Single", "Comment.Special",
	"Generic", "Generic.Deleted", "Generic.Emph", "Generic.Error", "Generic.Heading", "Generic.Inserted",
	"Generic.Output", "Generic.Prompt", "Generic.Strong", "Generic.Subheading", "Generic.Traceback",
	"Generic.Lineno", "Generic.EmphStrong",
}

// rougeLexerTags are the lexers go-rouge registers.
var rougeLexerTags = []string{
	"plaintext", "json", "diff", "go", "sql", "css", "shell", "yaml", "python",
	"javascript", "html", "markdown", "ruby", "php", "tex",
}

// rougeThemes are the themes go-rouge bundles.
var rougeThemes = []string{"base16", "github", "thankful_eyes"}

// defineRouge exposes go-rouge to shims/rouge.rb as AsciidoctorGo::RougeNative:
// tokens, lexers, lexing, the HTML formatters and theme styles.
func defineRouge(in *ruby.Interp) error {
	mod, err := in.DefineModule("AsciidoctorGo::RougeNative")
	if err != nil {
		return err
	}
	mod.DefineSingletonMethod("tokens", func(c *ruby.Call) (any, error) {
		out := []any{}
		for _, q := range rougeTokens {
			if t := rouge.TokenByName(q); t != nil {
				out = append(out, []any{q, t.Shortname})
			}
		}
		return out, nil
	})
	mod.DefineSingletonMethod("lexers", func(c *ruby.Call) (any, error) {
		out := []any{}
		for _, tag := range rougeLexerTags {
			if l := rouge.FindLexer(tag); l != nil {
				out = append(out, []any{l.Tag(), l.Title(), append([]string{}, l.Aliases()...)})
			}
		}
		return out, nil
	})
	// lex(spec, source): spec is a lexer tag with optional cgi-style options
	// ("php?start_inline=true"). Returns [[token qualname, value], ...].
	mod.DefineSingletonMethod("lex", func(c *ruby.Call) (any, error) {
		if len(c.Args) != 2 {
			return nil, ruby.NewError("ArgumentError", "lex(spec, source)")
		}
		l := rouge.FindFancy(c.Args[0].String())
		if l == nil {
			return nil, ruby.NewError("ArgumentError", "unknown lexer: "+c.Args[0].String())
		}
		toks := l.Lex(c.Args[1].String())
		out := make([]any, len(toks))
		for i, tv := range toks {
			out[i] = []any{tv.Token.Qualname(), tv.Value}
		}
		return out, nil
	})
	// guess(source) returns the tag of the lexer go-rouge detects.
	mod.DefineSingletonMethod("guess", func(c *ruby.Call) (any, error) {
		return rouge.Guess(c.Args[0].String()).Tag(), nil
	})
	tokenValues := func(v ruby.Value) ([]rouge.TokenValue, error) {
		items, _ := v.Go().([]any)
		out := make([]rouge.TokenValue, 0, len(items))
		for _, it := range items {
			pair, _ := it.([]any)
			if len(pair) != 2 {
				return nil, ruby.NewError("ArgumentError", "tokens must be [qualname, value] pairs")
			}
			q, _ := pair[0].(string)
			val, _ := pair[1].(string)
			t := rouge.TokenByName(q)
			if t == nil {
				return nil, ruby.NewError("ArgumentError", "unknown token: "+q)
			}
			out = append(out, rouge.TokenValue{Token: t, Value: val})
		}
		return out, nil
	}
	mod.DefineSingletonMethod("format_html", func(c *ruby.Call) (any, error) {
		toks, err := tokenValues(c.Args[0])
		if err != nil {
			return nil, err
		}
		return rouge.HTMLFormatter{}.Format(toks), nil
	})
	mod.DefineSingletonMethod("format_html_inline", func(c *ruby.Call) (any, error) {
		theme := rouge.FindTheme(c.Args[0].String())
		if theme == nil {
			return nil, ruby.NewError("ArgumentError", "unknown theme: "+c.Args[0].String())
		}
		toks, err := tokenValues(c.Args[1])
		if err != nil {
			return nil, err
		}
		return rouge.HTMLInlineFormatter{Theme: theme}.Format(toks), nil
	})
	mod.DefineSingletonMethod("themes", func(c *ruby.Call) (any, error) {
		return append([]string{}, rougeThemes...), nil
	})
	// theme_styles(name) returns [[qualname, declarations], ...] for the
	// tokens the theme styles itself (a style different from the parent's),
	// in token order.
	mod.DefineSingletonMethod("theme_styles", func(c *ruby.Call) (any, error) {
		theme := rouge.FindTheme(c.Args[0].String())
		if theme == nil {
			return nil, ruby.NewError("ArgumentError", "unknown theme: "+c.Args[0].String())
		}
		out := []any{}
		for _, q := range rougeTokens {
			t := rouge.TokenByName(q)
			if t == nil {
				continue
			}
			style := theme.StyleFor(t)
			if t.Parent == nil || style != theme.StyleFor(t.Parent) {
				if style != "" {
					out = append(out, []any{q, style})
				}
			}
		}
		return out, nil
	})
	return nil
}
