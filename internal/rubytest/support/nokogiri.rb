# frozen_string_literal: true

# The Nokogiri API used by Asciidoctor's test suite, on top of go-nokogiri.
# The Go harness defines Nokogiri::XML::Node (and the document classes) with
# native_* primitives before loading this file.
module Nokogiri
  class SyntaxError < ::StandardError; end

  module XML
    class SyntaxError < ::Nokogiri::SyntaxError; end

    module XPath
      class SyntaxError < ::Nokogiri::XML::SyntaxError; end
    end

    module ParseOptions
      STRICT = 0
      RECOVER = 1
      NONET = 2048
    end

    # An attribute of an element.
    class Attr
      attr_reader :name, :value, :parent

      def initialize parent, name, value
        @parent, @name, @value = parent, name, value
      end

      alias content value
      alias text value
      alias to_s value
      alias node_name name

      def == other
        Attr === other && other.parent == @parent && other.name == @name
      end
    end

    # An ordered collection of nodes.
    class NodeSet
      include ::Enumerable

      attr_reader :document

      def initialize document = nil, nodes = []
        @document = document
        @nodes = nodes
      end

      def each(&block)
        return to_enum :each unless block
        @nodes.each(&block)
        self
      end

      def size
        @nodes.size
      end

      alias length size

      def empty?
        @nodes.empty?
      end

      def first n = nil
        n ? @nodes.first(n) : @nodes.first
      end

      def last n = nil
        n ? @nodes.last(n) : @nodes.last
      end

      def [] *args
        r = @nodes[*args]
        ::Array === r ? (NodeSet.new @document, r) : r
      end

      alias slice []

      def to_a
        @nodes.dup
      end

      alias to_ary to_a

      def text
        @nodes.map(&:text).join
      end

      alias inner_text text
      alias content text

      def inner_html
        @nodes.map(&:inner_html).join
      end

      def to_s
        @nodes.map(&:to_s).join
      end

      alias to_html to_s
      alias to_xml to_s

      def xpath path, namespaces = nil
        NodeSet.new @document, (uniq_nodes @nodes.flat_map {|n| (n.xpath path, namespaces).to_a })
      end

      def css selector
        NodeSet.new @document, (uniq_nodes @nodes.flat_map {|n| (n.css selector).to_a })
      end

      alias search css

      def at path
        (path.start_with?('/') || path.start_with?('.') ? (xpath path) : (css path)).first
      end

      def attr name
        @nodes.first&.attr name
      end

      def include? node
        @nodes.any? {|n| n == node }
      end

      def + other
        NodeSet.new @document, (uniq_nodes @nodes + other.to_a)
      end

      def == other
        (NodeSet === other || ::Array === other) && to_a.size == other.to_a.size &&
          (to_a.zip other.to_a).all? {|a, b| a == b }
      end

      def inspect
        %([#{@nodes.map(&:inspect).join ', '}])
      end

      private

      def uniq_nodes nodes
        nodes.each_with_object [] do |n, acc|
          acc << n unless acc.any? {|m| m == n }
        end
      end
    end

    class Node
      ELEMENT_NODE = 1
      ATTRIBUTE_NODE = 2
      TEXT_NODE = 3
      CDATA_SECTION_NODE = 4
      PI_NODE = 7
      COMMENT_NODE = 8
      DOCUMENT_NODE = 9
      DOCUMENT_TYPE_NODE = 10

      def name
        native_name
      end

      alias node_name name

      def type
        native_type
      end

      alias node_type type

      def element?
        native_type == ELEMENT_NODE
      end

      alias elem? element?

      def text?
        native_type == TEXT_NODE
      end

      def cdata?
        native_type == CDATA_SECTION_NODE
      end

      def comment?
        native_type == COMMENT_NODE
      end

      def document?
        native_type == DOCUMENT_NODE
      end

      def blank?
        text? && text.strip.empty?
      end

      alias content text
      alias inner_text text

      def [] name
        native_attr name.to_s
      end

      alias get []
      alias attr []

      def key? name
        !(native_attr name.to_s).nil?
      end

      alias has_attribute? key?

      def keys
        native_attr_names
      end

      def values
        keys.map {|k| native_attr k }
      end

      def attribute name
        (value = native_attr name.to_s) ? (Attr.new self, name.to_s, value) : nil
      end

      def attributes
        keys.each_with_object({}) {|k, acc| acc[k] = Attr.new self, k, (native_attr k) }
      end

      def attribute_nodes
        attributes.values
      end

      def children
        NodeSet.new document, native_children
      end

      def element_children
        NodeSet.new document, native_element_children
      end

      alias elements element_children

      def first_element_child
        native_element_children.first
      end

      def last_element_child
        native_element_children.last
      end

      alias next next_sibling
      alias previous previous_sibling

      def ancestors
        result, node = [], parent
        while node
          result << node
          node = node.parent
        end
        NodeSet.new document, result
      end

      def xpath *paths
        namespaces = ::Hash === paths.last ? paths.pop : nil
        results = paths.map do |path|
          kind, value = native_xpath path, namespaces
          kind == 'nodes' ? value : (return value)
        end
        NodeSet.new document, results.flatten(1)
      end

      def css *selectors
        NodeSet.new document, (selectors.flat_map {|s| native_css s })
      end

      def search *paths
        paths.flat_map {|p| (p.start_with?('/') || p.start_with?('.')) ? (xpath p).to_a : (css p).to_a }.then {|n| NodeSet.new document, n }
      end

      def at_xpath *args
        r = xpath(*args)
        NodeSet === r ? r.first : r
      end

      def at_css *args
        css(*args).first
      end

      def at path
        (path.start_with?('/') || path.start_with?('.')) ? (at_xpath path) : (at_css path)
      end

      alias % at
      alias / search

      def path
        segments, node = [], self
        while node && node.element?
          siblings = node.parent ? (node.parent.native_element_children.select {|s| s.name == node.name }) : [node]
          segments.unshift(siblings.size > 1 ? %(#{node.name}[#{(siblings.index {|s| s == node }) + 1}]) : node.name)
          node = node.parent
        end
        %(/#{segments.join '/'})
      end

      def == other
        Node === other && (native_same? other)
      end

      alias eql? ==

      def hash
        native_document_order.hash
      end

      def <=> other
        native_document_order <=> other.native_document_order
      end

      def inspect
        element? ? %(#<#{self.class} name="#{name}">) : %(#<#{self.class} #{text.inspect}>)
      end
    end

    class Document
      def self.parse string, _url = nil, _encoding = nil, _options = nil
        Native.parse_xml string.to_s
      end
    end
  end

  module HTML
    class Document
      def self.parse string, _url = nil, _encoding = nil, _options = nil
        Native.parse_html string.to_s
      end
    end

    class DocumentFragment
      def self.parse string, _encoding = nil
        Native.parse_fragment string.to_s
      end
    end

    def self.parse(*args)
      Document.parse(*args)
    end

    def self.fragment string
      DocumentFragment.parse string
    end
  end

  HTML4 = HTML

  def self.HTML(*args)
    HTML::Document.parse(*args)
  end

  def self.XML(*args)
    XML::Document.parse(*args)
  end
end
