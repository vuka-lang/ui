package ui

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"

	"github.com/a-h/templ"
)

// VukaJSX marks the package as a JSX target implementing version 2 of Vuka's
// contract: a .vuka file importing ui lowers each piece of its JSX to a
// Template and its Render, blocks to Each and If, tags naming a Stateful type
// to &T{…} (Key with a key), islands to their props (Mount with options),
// ui.State to UseState, and reads of states to State's methods; children of
// components that read them from their context (templ components) go
// through WithChildren.
const VukaJSX = 2

// Node is anything that renders HTML: what a JSX expression evaluates to. It is
// templ.Component itself, so Vuka components are templ components and templ
// components are Vuka children, with nothing in between.
//
// The Nodes built here (*Element, TextNode, RawHTML, Group, Builder, TryNode,
// Boundary, *Frame) are also a tree any Renderer can Walk; any other Node — a templ
// component, a NodeFunc — is opaque: it can only render HTML. Nodes are
// immutable values; rendering one concurrently is safe.
type Node = templ.Component

// NodeFunc is a function rendering HTML: templ.ComponentFunc.
type NodeFunc = templ.ComponentFunc

// Attr is one attribute of an element. In HTML it renders the way templ
// renders the same attribute written as an expression:
//   - nil and false omit it, true writes it bare;
//   - class takes a string or anything templ.Classes does ([]string,
//     map[string]bool, templ.KV, …);
//   - style takes a Style or anything templ's style attributes do, sanitized
//     by templ (an unsafe property or value becomes templ's placeholder);
//   - URL attributes are sanitized: href on any element (a, link, area,
//     base, SVG's), xlink:href, action on form, data on object, src on
//     iframe, frame, embed, source, track, video, audio, img, input and
//     script, poster on video and formaction on button and input. templ.URL
//     keeps relative URLs and http, https, mailto, tel, ftp and ftps, and
//     turns anything else into about:invalid#TemplFailedSanitizationURL;
//     a media source also keeps a data: URL of its kind (data:image/… on
//     img src). A templ.SafeURL passes as is;
//   - a templ.ComponentScript writes its Call; a Node is rendered and escaped;
//     anything else goes through fmt.Sprint and is escaped
//     (templ.ResolveAttributeValue).
//
// className and htmlFor name class and for; ref names the element for JS
// commands (Ref) and renders as data-vk-ref. Names must be letters, digits,
// - _ : and . (templ only ever sees literal names; here a dynamic one could
// smuggle in another attribute), else Render fails.
type Attr struct {
	Name  string
	Value any
}

// Style is an inline style: style={ui.Style{"color": "red"}}, rendered by
// templ as "color:red;" sorted by property, each property and value sanitized.
type Style map[string]string

// Element is an element: <Tag Attrs>Children</Tag>. A void element (br, img,
// input, …; see Void) has no closing tag, and children on one are an error; in
// HTML, html is preceded by the doctype.
//
// Text inside script follows templ's Go expressions in a script: it is
// JSON-encoded (templ's runtime.ScriptContentOutsideStringLiteral), so it
// arrives as a quoted JS string with < > & escaped. templ has no expressions
// inside style; here text in a style element is written as is with "</"
// written "<\/", so it can't end the element.
type Element struct {
	Tag      string
	Attrs    []Attr
	Children []Node
}

// TextNode is text, escaped when rendered.
type TextNode string

// RawHTML is trusted HTML, written as is.
type RawHTML string

// Group is children with no element around them.
type Group []Node

// Builder produces children as it runs: a loop, if or match block. It runs
// once per Render or Walk, each added child rendered as it is added.
type Builder func(add func(Node))

// TryNode is a (Node, error) component's result as a child: Err fails the
// render, else Node renders.
type TryNode struct {
	Node Node
	Err  error
}

// Boundary renders Children or, when one of them fails, Fallback(err) in place
// of everything they produced.
type Boundary struct {
	Fallback func(err error) Node
	Children []Node
}

// El is an element.
func El(tag string, attrs []Attr, children ...Node) Node {
	return newElement(tag, attrs, children)
}

// Text is text: v formatted like fmt.Sprint, nil as nothing.
func Text(v any) Node {
	if _, ok := v.(string); ok {
		return textOf(v)
	}
	return TextNode(toString(v))
}

// Child is a {expr} child: a Node as is, nil as nothing, an error as its
// message, a []Node or []any as each element in turn, anything else as Text.
func Child(x any) Node {
	switch v := x.(type) {
	case nil:
		return Group(nil)
	case Binding:
		return v.node()
	case Node:
		return v
	case string:
		return textOf(x)
	case []Node:
		return Group(v)
	case []any:
		ns := make(Group, len(v))
		for i, x := range v {
			ns[i] = Child(x)
		}
		return ns
	}
	return TextNode(toString(x))
}

// textOf is the string x holds as a TextNode, sharing the string x boxes:
// a TextNode is a string, so the interface's data is the same.
func textOf(x any) Node {
	var n Node
	(*iface)(unsafe.Pointer(&n)).tab = textNodeTab
	(*iface)(unsafe.Pointer(&n)).data = (*iface)(unsafe.Pointer(&x)).data
	return n
}

type iface struct{ tab, data unsafe.Pointer }

var textNodeTab = func() unsafe.Pointer {
	var n Node = TextNode("")
	return (*iface)(unsafe.Pointer(&n)).tab
}()

// Fragment renders children in order, with no element around them.
func Fragment(children ...Node) Node { return Group(children) }

// Nodes is the children a loop, if or match block produces.
func Nodes(build func(add func(Node))) Node { return Builder(build) }

// Each is a list of what fn makes of each item, as a {for} block is: under a
// live session its items diff by their keys (a key attribute on an item's
// first element, or ui.Key on a component).
//
//	ui.Each(orders, OrderRow)
func Each[T any](items []T, fn func(T) Node) Node {
	return &list{items: func(add func(Node)) {
		for _, it := range items {
			add(fn(it))
		}
	}}
}

// If is n when cond holds, else nothing, as an {if} block is. n is built
// either way; when it can only be built while cond holds, use a Go if.
func If(cond bool, n Node) Node {
	if !cond || n == nil {
		return &list{items: func(func(Node)) {}}
	}
	return &list{items: func(add func(Node)) { add(n) }}
}

// list is a block: Each's, If's or a []Node in a template's hole. Its
// components and islands with no site of their own take the hole's.
type list struct {
	site  string
	items func(add func(Node))
}

func listOf(ns []Node) *list {
	return &list{items: func(add func(Node)) {
		for _, n := range ns {
			add(n)
		}
	}}
}

func (n *list) Render(ctx context.Context, w io.Writer) error { return renderHTML(ctx, w, n) }

func (n *list) placed(c Node) Node {
	switch c := c.(type) {
	case *ComponentNode:
		if c != nil && c.Site == "" && n.site != "" {
			cp := *c
			cp.Site = n.site
			return &cp
		}
	case *islandMount:
		return c.node(n.site)
	}
	return c
}

// item is c as an item of the block: a frame of its own, as each of a {for}
// block's bodies is, so a live render tree keeps the items apart.
func (n *list) item(c Node) Node {
	switch c := n.placed(c).(type) {
	case *Frame, *lazyFrame:
		return c
	default:
		return newFrame(itemFP, "h", []Node{c})
	}
}

const itemFP = 0x6974656d

// WithChildren renders c with children as its templ children
// (templ.WithChildren), so a templ component's { children... } renders them:
// <Card>{…}</Card> on a templ Card.
func WithChildren(c Node, children Node) Node {
	if children == nil {
		children = templ.NopComponent
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if c == nil {
			return nil
		}
		return c.Render(templ.WithChildren(ctx, children), w)
	})
}

// Safe is trusted HTML, written as is. Never pass it user input.
func Safe(html string) Node { return RawHTML(html) }

// Try is a (Node, error) component's result as a child.
func Try(n Node, err error) Node { return TryNode{n, err} }

// ErrorBoundary renders children, or — when one of them fails — fallback(err)
// in place of everything they wrote.
func ErrorBoundary(fallback func(err error) Node, children ...Node) Node {
	return Boundary{fallback, children}
}

func (n *Element) Render(ctx context.Context, w io.Writer) error { return renderHTML(ctx, w, n) }
func (n TextNode) Render(ctx context.Context, w io.Writer) error { return renderHTML(ctx, w, n) }
func (n RawHTML) Render(ctx context.Context, w io.Writer) error  { return renderHTML(ctx, w, n) }
func (n Group) Render(ctx context.Context, w io.Writer) error    { return renderHTML(ctx, w, n) }
func (n Builder) Render(ctx context.Context, w io.Writer) error  { return renderHTML(ctx, w, n) }
func (n TryNode) Render(ctx context.Context, w io.Writer) error  { return renderHTML(ctx, w, n) }
func (n Boundary) Render(ctx context.Context, w io.Writer) error { return renderHTML(ctx, w, n) }

// String renders n as HTML into a string.
func String(ctx context.Context, n Node) (string, error) {
	if n == nil {
		return "", nil
	}
	var b strings.Builder
	err := n.Render(ctx, &b)
	return b.String(), err
}

// Void reports whether tag is a void element: no children, no closing tag.
func Void(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr":
		return true
	}
	return false
}

// toString formats v like fmt.Sprint, with nil as "".
func toString(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case error:
		return v.Error()
	case fmt.Stringer:
		return v.String()
	}
	return fmt.Sprint(v)
}
