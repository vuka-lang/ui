// Package term renders a ui Node tree as terminal text: headings, paragraphs
// word-wrapped at a width, lists, tables, quotes, code, links, and inline
// bold/italic/underline as ANSI styles when Color is on.
//
// It walks the tree (ui.Walk), so it sees elements and text. A component
// that only renders HTML — a templ component, a NodeFunc — and Safe content
// arrive as HTML, which is parsed into the same tree, so their headings,
// tables and lists lay out as if written in JSX.
//
// The package is pure: whether to use color (NO_COLOR, a tty) is the
// caller's decision. The page's text never carries terminal commands: its
// control characters, but newlines and tabs, are dropped (ANSI escapes,
// OSC sequences, carriage returns, C1 controls), so text a visitor wrote
// can't restyle, retitle or clear the terminal.
package term

import (
	"context"
	"html"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/vuka-lang/ui/v2"
)

// Options shape the output.
type Options struct {
	Width int  // wrap paragraphs at this many columns; 0 doesn't wrap
	Color bool // ANSI styles: bold, italic, underline, dim, inverse
}

// Render writes n as terminal text to w, ending with a newline unless empty.
func Render(ctx context.Context, w io.Writer, n ui.Node, o Options) error {
	s, err := String(ctx, n, o)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s)
	return err
}

// String renders n as terminal text.
func String(ctx context.Context, n ui.Node, o Options) (string, error) {
	b := builder{}
	b.stack = []*node{&b.root}
	if err := ui.Walk(ctx, n, &b); err != nil {
		return "", err
	}
	l := layout{o: o}
	lines := l.blocks(b.root.kids, o.Width, nil, false)
	if len(lines) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for _, ln := range lines {
		sb.WriteString(strings.TrimRight(ln.s, " "))
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// node is the tree the layout works on: an element or, with tag "", text.
type node struct {
	tag   string
	attrs []ui.Attr
	text  string
	kids  []*node
}

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.attrs {
		an := a.Name
		if an == "className" {
			an = "class"
		}
		if an != name {
			continue
		}
		switch v := a.Value.(type) {
		case nil:
			return "", false
		case bool:
			return "", v
		case string:
			return clean(v), true
		}
		s, _ := ui.String(context.Background(), ui.Text(a.Value))
		return clean(html.UnescapeString(s)), true
	}
	return "", false
}

// builder is the ui.Renderer building the node tree.
type builder struct {
	root  node
	stack []*node
}

func (b *builder) top() *node { return b.stack[len(b.stack)-1] }

func (b *builder) Open(e *ui.Element) error {
	b.open(strings.ToLower(e.Tag), e.Attrs, false)
	return nil
}

func (b *builder) open(tag string, attrs []ui.Attr, empty bool) {
	n := &node{tag: tag, attrs: attrs}
	t := b.top()
	t.kids = append(t.kids, n)
	if !empty && !ui.Void(tag) {
		b.stack = append(b.stack, n)
	}
}

func (b *builder) Close(*ui.Element) error {
	b.stack = b.stack[:len(b.stack)-1]
	return nil
}

func (b *builder) Text(s string) error {
	t := b.top()
	t.kids = append(t.kids, &node{text: clean(s)})
	return nil
}

// clean drops the control characters of text the page holds, but newlines
// and tabs: C0 (ESC among them), DEL and C1 — what a terminal reads as
// commands (colors, the cursor, the window's title, a cleared screen)
// rather than as text; invalid UTF-8 becomes U+FFFD. The styles the
// output carries are its own.
func clean(s string) string {
	if utf8.ValidString(s) && !strings.ContainsFunc(s, control) {
		return s
	}
	// an invalid byte (a raw 0x9b, 8-bit CSI) arrives as U+FFFD
	return strings.Map(func(r rune) rune {
		if control(r) {
			return -1
		}
		return r
	}, s)
}

func control(r rune) bool {
	return r < 0x20 && r != '\n' && r != '\t' || r == 0x7f || r >= 0x80 && r <= 0x9f
}

func (b *builder) Opaque(ctx context.Context, n ui.Node) error {
	var sb strings.Builder
	if err := n.Render(ctx, &sb); err != nil {
		return err
	}
	return b.Raw(sb.String())
}

func isBlock(tag string) bool {
	switch tag {
	case "p", "div", "section", "header", "footer", "article", "main", "form", "nav", "aside",
		"h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "pre", "blockquote", "table",
		"thead", "tbody", "tfoot", "tr", "hr", "figure", "figcaption", "details", "summary",
		"dl", "dt", "dd", "body", "html", "address", "fieldset", "legend",
		"script", "style", "head", "title", "template", "noscript":
		return true
	}
	return false
}

// skipped elements render nothing.
func skipped(tag string) bool {
	switch tag {
	case "script", "style", "head", "title", "template", "noscript":
		return true
	}
	return false
}
