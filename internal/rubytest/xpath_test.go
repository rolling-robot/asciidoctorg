package rubytest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	nokogiri "github.com/go-nokogiri/nokogiri"
)

// go-nokogiri applies the predicate of a filter expression, (expr)[n], to the
// last step of expr instead of to the whole node set, so (//ul)[1]/li selects
// the items of every first-child ul. evalXPath evaluates filter expressions at
// the start of an expression itself:
//
//	( expr ) [pred]... rest
//
// expr is evaluated (recursively, so filter expressions nest), each predicate
// is applied to the whole node set (a position, last(), last()-N, or a
// condition tested on each node), and rest (a relative path starting with "/"
// or "//") is evaluated from each node, with the results in document order.
// Any other filter expression is reported as unsupported rather than
// evaluated wrongly.
func evalXPath(ctx *nokogiri.Node, expr string, ns map[string]string) (any, error) {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "(") {
		if end := matchingClose(expr, 0); end > 0 {
			preds, rest := splitPredicates(expr[end+1:])
			rest = strings.TrimSpace(rest)
			if rest == "" || strings.HasPrefix(rest, "/") {
				set, err := evalNodes(ctx, expr[1:end], ns)
				if err != nil {
					return nil, err
				}
				for _, p := range preds {
					if set, err = applyPredicate(set, p, ns); err != nil {
						return nil, err
					}
				}
				if rest == "" {
					return set, nil
				}
				var out []*nokogiri.Node
				for _, n := range set {
					sub, err := evalNodes(n, "."+rest, ns)
					if err != nil {
						return nil, err
					}
					out = append(out, sub...)
				}
				return docOrder(out), nil
			}
		}
	}
	if hasFilterExpression(expr) {
		return nil, fmt.Errorf("unsupported XPath filter expression: %s", expr)
	}
	v, err := ctx.EvalXPath(expr, ns)
	if err != nil {
		return nil, err
	}
	if s, ok := v.(*nokogiri.NodeSet); ok {
		return s.Nodes(), nil
	}
	return v, nil
}

func evalNodes(ctx *nokogiri.Node, expr string, ns map[string]string) ([]*nokogiri.Node, error) {
	v, err := evalXPath(ctx, expr, ns)
	if err != nil {
		return nil, err
	}
	nodes, ok := v.([]*nokogiri.Node)
	if !ok {
		return nil, fmt.Errorf("XPath %q is not a node set", expr)
	}
	return nodes, nil
}

// applyPredicate filters a node set by one predicate, with positions counted
// over the whole set.
func applyPredicate(set []*nokogiri.Node, pred string, ns map[string]string) ([]*nokogiri.Node, error) {
	pred = strings.TrimSpace(pred)
	pick := func(i int) []*nokogiri.Node {
		if i < 1 || i > len(set) {
			return nil
		}
		return set[i-1 : i]
	}
	if i, err := strconv.Atoi(pred); err == nil {
		return pick(i), nil
	}
	if pred == "last()" {
		return pick(len(set)), nil
	}
	if after, ok := strings.CutPrefix(pred, "last()"); ok {
		if d, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(after), " ", "")); err == nil && d < 0 {
			return pick(len(set) + d), nil
		}
	}
	if strings.Contains(pred, "position()") || strings.Contains(pred, "last()") {
		return nil, fmt.Errorf("unsupported positional predicate on a filter expression: [%s]", pred)
	}
	var out []*nokogiri.Node
	for _, n := range set {
		r, err := n.EvalXPath("self::node()["+pred+"]", ns)
		if err != nil {
			return nil, err
		}
		if s, ok := r.(*nokogiri.NodeSet); ok && s.Length() > 0 {
			out = append(out, n)
		}
	}
	return out, nil
}

// matchingClose returns the index of the parenthesis closing the one at open,
// skipping quoted strings, or -1.
func matchingClose(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch c := s[i]; c {
		case '\'', '"':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return -1
			}
			i += j + 1
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitPredicates splits leading [..] groups off s.
func splitPredicates(s string) (preds []string, rest string) {
	for {
		t := strings.TrimLeft(s, " ")
		if !strings.HasPrefix(t, "[") {
			return preds, s
		}
		depth := 0
		end := -1
	scan:
		for i := 0; i < len(t); i++ {
			switch c := t[i]; c {
			case '\'', '"':
				j := strings.IndexByte(t[i+1:], c)
				if j < 0 {
					return preds, s
				}
				i += j + 1
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					end = i
					break scan
				}
			}
		}
		if end < 0 {
			return preds, s
		}
		preds = append(preds, t[1:end])
		s = t[end+1:]
	}
}

// hasFilterExpression reports whether expr contains a predicate applied to a
// parenthesized expression (other than a node test such as text()[...]).
func hasFilterExpression(expr string) bool {
	for i := 0; i < len(expr); i++ {
		switch c := expr[i]; c {
		case '\'', '"':
			j := strings.IndexByte(expr[i+1:], c)
			if j < 0 {
				return false
			}
			i += j + 1
		case ')':
			if !strings.HasPrefix(strings.TrimLeft(expr[i+1:], " "), "[") {
				continue
			}
			open := matchingOpen(expr, i)
			if open < 0 {
				continue
			}
			name := identBefore(expr, open)
			switch name {
			case "text", "node", "comment", "processing-instruction":
				continue
			}
			return true
		}
	}
	return false
}

func matchingOpen(s string, close int) int {
	depth := 0
	for i := close; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func identBefore(s string, i int) string {
	j := i
	for j > 0 {
		c := s[j-1]
		if c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			j--
			continue
		}
		break
	}
	return s[j:i]
}

// docOrder removes duplicates and sorts nodes in document order.
func docOrder(nodes []*nokogiri.Node) []*nokogiri.Node {
	if len(nodes) == 0 {
		return nodes
	}
	root := nodes[0]
	for root.Parent() != nil {
		root = root.Parent()
	}
	index := map[*nokogiri.Node]int{}
	i := 0
	var walk func(*nokogiri.Node)
	walk = func(n *nokogiri.Node) {
		index[n] = i
		i++
		for ch := n.FirstChild(); ch != nil; ch = ch.Next() {
			walk(ch)
		}
	}
	walk(root)
	seen := map[*nokogiri.Node]bool{}
	var out []*nokogiri.Node
	for _, n := range nodes {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for a := 1; a < len(out); a++ {
		for b := a; b > 0 && index[out[b]] < index[out[b-1]]; b-- {
			out[b], out[b-1] = out[b-1], out[b]
		}
	}
	return out
}

func TestFilterExpressions(t *testing.T) {
	d, err := nokogiri.HTMLFragment(`<div><ul><li>a</li><li>b</li></ul></div><div><ul><li>c</li><li><ul><li>e</li></ul></li></ul></div><ul><li>d</li></ul>`)
	if err != nil {
		t.Fatal(err)
	}
	text := func(v any) string {
		var b strings.Builder
		for _, n := range v.([]*nokogiri.Node) {
			b.WriteString(n.Text())
			b.WriteString(";")
		}
		return b.String()
	}
	for _, tc := range []struct{ expr, want string }{
		{"(.//ul)[1]/li", "a;b;"},
		{"(.//ul)[2]/li", "c;e;"},
		{"(.//ul)[last()]/li", "d;"},
		{"(.//ul)[last()-1]/li", "e;"},
		{"(.//ul)[5]/li", ""},
		{"((.//ul)[2]/li)[2]//li", "e;"},
		{"(((.//ul)[2]/li)[2]//ul)[1]/li", "e;"},
		{"(.//li)[1][text()='a']", "a;"},
		{"(.//li)[2][text()='a']", ""},
		{"(.//li)[text()='c' or text()='d']", "c;d;"},
		{".//ul/li[1]", "a;c;e;d;"},
		{".//li/text()[.='b']", "b;"},
	} {
		v, err := evalXPath(&d.Node, tc.expr, nil)
		if err != nil {
			t.Errorf("%s: %v", tc.expr, err)
			continue
		}
		if got := text(v); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.expr, got, tc.want)
		}
	}
	for _, expr := range []string{"count((.//ul)[1]/li)", "(.//li)[position() > 1]"} {
		if _, err := evalXPath(&d.Node, expr, nil); err == nil {
			t.Errorf("%s: expected an unsupported error", expr)
		}
	}
	if v, err := evalXPath(&d.Node, "count(.//li)", nil); err != nil || v != float64(6) {
		t.Errorf("count: %v, %v", v, err)
	}
}

func TestFragmentParsing(t *testing.T) {
	d, err := parseFragment(`<table frame="all" tabstyle="horizontal"><title>Terms</title><tgroup cols="2"><colspec colwidth="15*"/><colspec colwidth="85*"/></tgroup></table>`)
	if err != nil {
		t.Fatal(err)
	}
	for expr, want := range map[string]float64{"count(./table/title)": 1, "count(./table/tgroup/colspec)": 2} {
		if v, err := evalXPath(&d.Node, expr, nil); err != nil || v != want {
			t.Errorf("%s: %v, %v", expr, v, err)
		}
	}
	h, err := parseFragment(`<div class="hdlist"><table><tr><td>a</td></tr></table></div><br>`)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := evalXPath(&h.Node, "count(.//tbody)", nil); v != float64(1) {
		t.Errorf("markup that is not well-formed should use the HTML parser (tbody implied), got %v", v)
	}
	if got := namespacedAttrRx.ReplaceAllString(`article:root[xml|lang="en"] > [lang|="en"]`, "[$1:$2"); got != `article:root[xml:lang="en"] > [lang|="en"]` {
		t.Errorf("namespace rewrite: %s", got)
	}
}

func TestFunctionPredicates(t *testing.T) {
	sel, preds, err := splitFunctionPredicates(`article > formalpara > para[text()="An open, paragraph."]`)
	if err != nil || sel != "article > formalpara > para" || len(preds) != 1 || preds[0] != `text()="An open, paragraph."` {
		t.Fatalf("got %q %q %v", sel, preds, err)
	}
	if _, _, err := splitFunctionPredicates(`p[text()="x"] > b`); err == nil {
		t.Error("expected an error for a function predicate that is not trailing")
	}
	if sel, preds, err := splitFunctionPredicates(`p.x > b`); err != nil || sel != "p.x > b" || preds != nil {
		t.Errorf("plain selector changed: %q %q %v", sel, preds, err)
	}
}
