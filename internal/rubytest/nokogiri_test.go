package rubytest

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	ruby "github.com/go-embedded-ruby/ruby"
	nokogiri "github.com/go-nokogiri/nokogiri"
)

// defineNokogiri binds go-nokogiri into the interpreter: Go implements parsing,
// navigation, serialization and queries on Nokogiri::XML::Node (bound to
// *nokogiri.Node and *nokogiri.Document); support/nokogiri.rb builds the rest
// of the Nokogiri API the test suite uses on top.
func defineNokogiri(in *ruby.Interp) error {
	// rbgo predefines a partial native Nokogiri; replace it entirely.
	if _, err := in.Eval("Object.send :remove_const, :Nokogiri if defined? ::Nokogiri", "nokogiri", 1); err != nil {
		return err
	}
	noko, err := in.DefineModule("Nokogiri")
	if err != nil {
		return err
	}
	xml, err := noko.DefineModule("XML")
	if err != nil {
		return err
	}
	nodeClass, err := xml.DefineClass("Node", nil)
	if err != nil {
		return err
	}
	nodeClass.Bind(&nokogiri.Node{})
	docClass, err := xml.DefineClass("Document", nodeClass)
	if err != nil {
		return err
	}
	docClass.Bind(&nokogiri.Document{})
	html, err := noko.DefineModule("HTML")
	if err != nil {
		return err
	}
	htmlDoc, err := html.DefineClass("Document", docClass)
	if err != nil {
		return err
	}
	// A fragment is a node, not a document: like Nokogiri's, it has no root.
	fragment, err := html.DefineClass("DocumentFragment", nodeClass)
	if err != nil {
		return err
	}
	native, err := noko.DefineModule("Native")
	if err != nil {
		return err
	}

	parse := func(cls *ruby.Module, fn func(string) (*nokogiri.Document, error)) ruby.Func {
		return func(c *ruby.Call) (any, error) {
			d, err := fn(c.Args[0].String())
			if err != nil {
				return nil, ruby.NewError("Nokogiri::XML::SyntaxError", err.Error())
			}
			return cls.Wrap(d), nil
		}
	}
	native.DefineSingletonMethod("parse_xml", parse(docClass, nokogiri.XML))
	native.DefineSingletonMethod("parse_html", parse(htmlDoc, nokogiri.HTML))
	native.DefineSingletonMethod("parse_fragment", parse(fragment, parseFragment))

	self := func(c *ruby.Call) *nokogiri.Node {
		switch v := c.Self.Go().(type) {
		case *nokogiri.Node:
			return v
		case *nokogiri.Document:
			return &v.Node
		}
		return nil
	}
	nodeOrNil := func(n *nokogiri.Node) any {
		if n == nil {
			return nil
		}
		if n.Type == nokogiri.DocumentNode {
			if d := n.Document(); d != nil && &d.Node == n {
				return d
			}
		}
		return n
	}
	nodes := func(s *nokogiri.NodeSet) []any {
		ns := s.Nodes()
		out := make([]any, len(ns))
		for i, n := range ns {
			out[i] = nodeOrNil(n)
		}
		return out
	}
	nsArg := func(c *ruby.Call, i int) map[string]string {
		if len(c.Args) <= i || c.Args[i].IsNil() {
			return nil
		}
		m, _ := c.Args[i].Go().(map[any]any)
		out := map[string]string{}
		for k, v := range m {
			ks, _ := k.(string)
			vs, _ := v.(string)
			if len(ks) > 6 && ks[:6] == "xmlns:" {
				ks = ks[6:]
			}
			out[ks] = vs
		}
		return out
	}
	def := func(name string, fn func(c *ruby.Call, n *nokogiri.Node) (any, error)) {
		nodeClass.DefineMethod(name, func(c *ruby.Call) (any, error) { return fn(c, self(c)) })
	}
	def("native_name", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return n.NodeName(), nil })
	def("native_type", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return int(n.Type), nil })
	def("text", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return n.Text(), nil })
	def("inner_html", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return n.InnerHTML(), nil })
	def("to_html", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return n.ToHTML(), nil })
	def("to_xml", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return n.ToXML(), nil })
	def("to_s", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return n.ToS(), nil })
	def("native_attr", func(c *ruby.Call, n *nokogiri.Node) (any, error) {
		if v, ok := n.Get(c.Args[0].String()); ok {
			return v, nil
		}
		return nil, nil
	})
	def("native_attr_names", func(c *ruby.Call, n *nokogiri.Node) (any, error) {
		names := []string{}
		for _, a := range n.Attrs {
			name := a.Name
			if a.Prefix != "" {
				name = a.Prefix + ":" + a.Name
			}
			names = append(names, name)
		}
		return names, nil
	})
	def("parent", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return nodeOrNil(n.Parent()), nil })
	def("native_children", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return nodes(n.Children()), nil })
	def("native_element_children", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return nodes(n.ElementChildren()), nil })
	def("next_sibling", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return nodeOrNil(n.Next()), nil })
	def("previous_sibling", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return nodeOrNil(n.Previous()), nil })
	def("next_element", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return nodeOrNil(n.NextElement()), nil })
	def("previous_element", func(c *ruby.Call, n *nokogiri.Node) (any, error) { return nodeOrNil(n.PreviousElement()), nil })
	docClass.DefineMethod("root", func(c *ruby.Call) (any, error) { return nodeOrNil(self(c).Root()), nil })
	def("document", func(c *ruby.Call, n *nokogiri.Node) (any, error) {
		if d := n.Document(); d != nil {
			return d, nil
		}
		return nil, nil
	})
	def("namespaces", func(c *ruby.Call, n *nokogiri.Node) (any, error) {
		out := map[string]any{}
		for k, v := range n.Namespaces() {
			out[k] = v
		}
		return out, nil
	})
	def("native_xpath", func(c *ruby.Call, n *nokogiri.Node) (any, error) {
		v, err := evalXPath(n, c.Args[0].String(), nsArg(c, 1))
		if err != nil {
			return nil, ruby.NewError("Nokogiri::XML::XPath::SyntaxError", err.Error())
		}
		switch r := v.(type) {
		case []*nokogiri.Node:
			out := make([]any, len(r))
			for i, x := range r {
				out[i] = nodeOrNil(x)
			}
			return []any{"nodes", out}, nil
		default:
			return []any{"scalar", r}, nil
		}
	})
	def("native_css", func(c *ruby.Call, n *nokogiri.Node) (any, error) {
		selector, preds, err := splitFunctionPredicates(namespacedAttrRx.ReplaceAllString(c.Args[0].String(), "[$1:$2"))
		if err != nil {
			return nil, ruby.NewError("Nokogiri::CSS::SyntaxError", err.Error())
		}
		s, err := n.CSS(selector, nsArg(c, 1))
		if err != nil {
			return nil, ruby.NewError("Nokogiri::CSS::SyntaxError", err.Error())
		}
		matched := s.Nodes()
		for _, p := range preds {
			if matched, err = applyPredicate(matched, p, nsArg(c, 1)); err != nil {
				return nil, ruby.NewError("Nokogiri::CSS::SyntaxError", err.Error())
			}
		}
		out := make([]any, len(matched))
		for i, x := range matched {
			out[i] = nodeOrNil(x)
		}
		return out, nil
	})
	def("native_same?", func(c *ruby.Call, n *nokogiri.Node) (any, error) {
		var other *nokogiri.Node
		switch v := c.Args[0].Go().(type) {
		case *nokogiri.Node:
			other = v
		case *nokogiri.Document:
			other = &v.Node
		}
		return other == n, nil
	})
	def("native_document_order", func(c *ruby.Call, n *nokogiri.Node) (any, error) {
		// position of n among all nodes of its document, for sorting
		root := n
		for root.Parent() != nil {
			root = root.Parent()
		}
		i := 0
		var walk func(*nokogiri.Node) bool
		walk = func(x *nokogiri.Node) bool {
			if x == n {
				return true
			}
			i++
			for ch := x.FirstChild(); ch != nil; ch = ch.Next() {
				if walk(ch) {
					return true
				}
			}
			return false
		}
		walk(root)
		return i, nil
	})
	return nil
}

// defineHostSupport adds what the suite needs from the host that rbgo lacks:
// the interface addresses behind Socket.ip_address_list.
func defineHostSupport(in *ruby.Interp) error {
	mod, err := in.DefineModule("AsciidoctorGoTestHost")
	if err != nil {
		return err
	}
	mod.DefineSingletonMethod("interface_addresses", func(c *ruby.Call) (any, error) {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return nil, err
		}
		out := []string{}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				out = append(out, ipnet.IP.String())
			}
		}
		return out, nil
	})
	return nil
}

// namespacedAttrRx matches a CSS attribute selector with a namespace prefix,
// [prefix|name. go-nokogiri compiles the name as is, which in XPath reads as a
// union; [prefix:name is the form it handles.
var namespacedAttrRx = regexp.MustCompile(`\[\s*([A-Za-z_][\w.-]*)\|([A-Za-z_][\w.-]*)`)

// parseFragment parses markup as Nokogiri::HTML::DocumentFragment does with
// libxml2. go-nokogiri parses fragments with the HTML5 algorithm, which
// rearranges markup that is not HTML (DocBook <title> in <table> is moved out)
// and inserts implied elements (<tbody>); libxml2 keeps the markup as written.
// Well-formed markup, such as DocBook and most of Asciidoctor's HTML, is
// therefore parsed as XML and moved into an HTML fragment; anything else goes
// through the HTML parser.
func parseFragment(src string) (*nokogiri.Document, error) {
	frag, err := nokogiri.HTMLFragment("")
	if err != nil {
		return nil, err
	}
	wrapped, err := nokogiri.XML("<agox-fragment>" + src + "</agox-fragment>")
	if err != nil {
		return nokogiri.HTMLFragment(src)
	}
	root := wrapped.Root()
	var children []*nokogiri.Node
	for ch := root.FirstChild(); ch != nil; ch = ch.Next() {
		children = append(children, ch)
	}
	for _, ch := range children {
		frag.Node.AddChild(ch)
	}
	return frag, nil
}

// functionPredicateRx matches a trailing CSS attribute selector that tests an
// XPath function, such as [text()="x"], which Nokogiri accepts.
var functionPredicateRx = regexp.MustCompile(`\[\s*([a-z-]+\(\)\s*(?:(?:!?=)\s*(?:"[^"]*"|'[^']*'))?)\s*\]\s*$`)

// splitFunctionPredicates removes trailing function predicates from a single
// selector and returns them as XPath predicates, to be applied to the matches.
// go-nokogiri compiles [text()="x"] as an attribute named text(). Function
// predicates elsewhere are reported as unsupported.
func splitFunctionPredicates(selector string) (string, []string, error) {
	var preds []string
	for {
		m := functionPredicateRx.FindStringSubmatchIndex(selector)
		if m == nil {
			break
		}
		preds = append([]string{selector[m[2]:m[3]]}, preds...)
		selector = selector[:m[0]]
	}
	if strings.Contains(selector, "()") {
		return "", nil, fmt.Errorf("unsupported CSS function predicate in %q", selector)
	}
	if len(preds) > 0 && strings.Contains(selector, ",") {
		return "", nil, fmt.Errorf("unsupported CSS function predicate in a selector list: %q", selector)
	}
	return selector, preds, nil
}
