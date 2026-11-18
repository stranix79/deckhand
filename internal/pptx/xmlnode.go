package pptx

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// node is a minimal XML tree. A slide mixes shapes, pictures and tables in an
// order that matters, which encoding/xml structs cannot keep, so the parts are
// loaded into this tree and walked by element name. Namespaces are dropped on
// elements (p:sp and a:sp never collide in practice); attributes keep theirs
// so that r:id can be told apart from id.
type node struct {
	name     string
	attrs    []xml.Attr
	children []*node
	text     string
}

// relNS is the fragment shared by every "relationships" namespace
// (transitional and strict OOXML use different prefixes).
const relNS = "officeDocument/2006/relationships"

func parseXML(r io.Reader) (*node, error) {
	dec := xml.NewDecoder(r)
	var stack []*node
	var root *node
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name.Local, attrs: t.Attr}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	if root == nil {
		return nil, errors.New("empty XML document")
	}
	return root, nil
}

// child returns the first direct child with that name, or nil.
func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

// path follows a chain of first children: n.path("txBody", "bodyPr").
func (n *node) path(names ...string) *node {
	cur := n
	for _, name := range names {
		cur = cur.child(name)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// all returns the direct children with that name, in document order.
func (n *node) all(name string) []*node {
	if n == nil {
		return nil
	}
	var out []*node
	for _, c := range n.children {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}

// find returns the first descendant with that name, depth first, or nil.
func (n *node) find(name string) *node {
	if n == nil {
		return nil
	}
	for _, c := range n.children {
		if c.name == name {
			return c
		}
		if d := c.find(name); d != nil {
			return d
		}
	}
	return nil
}

// attr returns an attribute that is not in a relationships namespace.
func (n *node) attr(name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.attrs {
		if a.Name.Local == name && !strings.Contains(a.Name.Space, relNS) {
			return a.Value
		}
	}
	return ""
}

// rattr returns a relationship attribute (r:id, r:embed, r:link).
func (n *node) rattr(name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.attrs {
		if a.Name.Local == name && strings.Contains(a.Name.Space, relNS) {
			return a.Value
		}
	}
	return ""
}
