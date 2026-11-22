package ui

import (
	"strings"
	"sync"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Sanitize is s with only what DefaultPolicy allows: HTML from users,
// a comment's or an article's body, safe to render as RawHTML. s is parsed
// as the content of a <div>, filtered and written again, so the result is
// well-formed whatever s was.
func Sanitize(s string) RawHTML {
	defaultOnce.Do(func() { defaultRules = DefaultPolicy().rules() })
	return defaultRules.sanitize(s)
}

// SanitizeWith is s with only what p allows.
func SanitizeWith(s string, p Policy) RawHTML { return p.rules().sanitize(s) }

// Policy is what Sanitize keeps: elements, their attributes, and the URL
// schemes their links may name. An element it doesn't name is dropped and
// its content kept; an attribute it doesn't name, dropped.
//
// Whatever a Policy says, script, style and the elements whose content is
// not markup (iframe, noscript, textarea, title, template, xmp, …) go with
// their content, as do SVG and MathML; on… and style attributes go; and a
// URL attribute (href, src, …) naming a scheme Schemes doesn't goes.
type Policy struct {
	Elements     []string            // elements kept: "p", "a"
	Attrs        []string            // attributes kept on every element kept
	ElementAttrs map[string][]string // attributes kept on one element: "a": {"href"}
	Schemes      []string            // URL schemes a URL attribute may name; a relative URL names none
}

// DefaultPolicy is Sanitize's: text formatting, headings, lists, quotes,
// code, tables, links and images. Attributes are href on a, src and alt on
// img, colspan and rowspan on cells, and title; URLs are http, https,
// mailto or relative. class and id are not kept: content written by
// users can't take the page's styles or ids. The Policy is a copy: add to
// it and pass it to SanitizeWith.
func DefaultPolicy() Policy {
	return Policy{
		Elements: strings.Fields(`p br hr b strong i em u s code pre blockquote h1 h2 h3 h4 h5 h6 ul ol li
			a img span div table caption colgroup col thead tbody tfoot tr th td`),
		Attrs: []string{"title"},
		ElementAttrs: map[string][]string{
			"a":   {"href"},
			"img": {"src", "alt"},
			"td":  {"colspan", "rowspan"},
			"th":  {"colspan", "rowspan"},
		},
		Schemes: []string{"http", "https", "mailto"},
	}
}

var (
	defaultOnce  sync.Once
	defaultRules *sanitizeRules
)

type sanitizeRules struct {
	elements map[string]bool
	attrs    map[string]bool            // every element's
	elAttrs  map[string]map[string]bool // one element's
	schemes  map[string]bool
}

func (p Policy) rules() *sanitizeRules {
	r := &sanitizeRules{elements: map[string]bool{}, attrs: map[string]bool{}, elAttrs: map[string]map[string]bool{}, schemes: map[string]bool{}}
	for _, e := range p.Elements {
		r.elements[strings.ToLower(e)] = true
	}
	for _, a := range p.Attrs {
		r.attrs[strings.ToLower(a)] = true
	}
	for e, as := range p.ElementAttrs {
		m := map[string]bool{}
		for _, a := range as {
			m[strings.ToLower(a)] = true
		}
		r.elAttrs[strings.ToLower(e)] = m
	}
	for _, s := range p.Schemes {
		r.schemes[strings.ToLower(s)] = true
	}
	return r
}

// dropped are the elements that go with their content, whatever a Policy
// says: scripts and styles, and the elements whose content a browser reads
// as text or out of the page.
var dropped = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Template: true, atom.Noscript: true,
	atom.Iframe: true, atom.Frame: true, atom.Frameset: true, atom.Object: true,
	atom.Embed: true, atom.Applet: true, atom.Xmp: true, atom.Noembed: true,
	atom.Noframes: true, atom.Plaintext: true, atom.Textarea: true, atom.Title: true,
	atom.Select: true, atom.Svg: true, atom.Math: true, atom.Base: true,
	atom.Link: true, atom.Meta: true, atom.Head: true,
}

// urlAttrs are the attributes holding a URL, checked against the schemes.
var urlAttrs = map[string]bool{
	"href": true, "src": true, "cite": true, "action": true, "formaction": true,
	"poster": true, "background": true, "longdesc": true, "usemap": true,
	"data": true, "codebase": true, "ping": true, "srcset": true, "xlink:href": true,
}

var divContext = &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}

func (r *sanitizeRules) sanitize(s string) RawHTML {
	if s == "" {
		return ""
	}
	nodes, err := html.ParseFragment(strings.NewReader(s), divContext)
	if err != nil {
		return RawHTML(html.EscapeString(s))
	}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		r.keep(root, n)
	}
	var b strings.Builder
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			return ""
		}
	}
	return RawHTML(b.String())
}

// keep adds to parent what of n the rules keep.
func (r *sanitizeRules) keep(parent, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		parent.AppendChild(&html.Node{Type: html.TextNode, Data: n.Data})
		return
	case html.ElementNode:
	default: // comments, doctypes
		return
	}
	if n.Namespace != "" || dropped[n.DataAtom] {
		return
	}
	into := parent
	if r.elements[n.Data] {
		into = &html.Node{Type: html.ElementNode, Data: n.Data, DataAtom: n.DataAtom, Attr: r.attributes(n)}
		parent.AppendChild(into)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.keep(into, c)
	}
}

func (r *sanitizeRules) attributes(n *html.Node) []html.Attribute {
	var out []html.Attribute
	for _, a := range n.Attr {
		name := a.Key
		if a.Namespace != "" {
			name = a.Namespace + ":" + a.Key
		}
		if strings.HasPrefix(name, "on") || name == "style" || !r.attrs[name] && !r.elAttrs[n.Data][name] {
			continue
		}
		val := a.Val
		switch {
		case name == "srcset":
			if !r.srcset(val) {
				continue
			}
		case urlAttrs[name]:
			var ok bool
			if val, ok = r.url(val); !ok {
				continue
			}
		case name == "colspan" || name == "rowspan":
			if !spanValue(val) {
				continue
			}
		}
		out = append(out, html.Attribute{Key: name, Val: val})
	}
	return out
}

// url is v as a URL attribute's value, if its scheme is one of the rules':
// what a browser reads, tabs, line breaks and the controls around it gone.
func (r *sanitizeRules) url(v string) (string, bool) {
	v = strings.Trim(v, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\t\n\v\f\r\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f ")
	v = strings.Map(func(c rune) rune {
		if c == '\t' || c == '\n' || c == '\r' {
			return -1
		}
		return c
	}, v)
	scheme, ok := urlScheme(v)
	if ok && !r.schemes[strings.ToLower(scheme)] {
		return "", false
	}
	return v, true
}

// urlScheme is what v names before a : that comes before any /, ? or #:
// its scheme, or something no scheme of a Policy's is either.
func urlScheme(v string) (string, bool) {
	i := strings.IndexAny(v, ":/?#")
	if i < 0 || v[i] != ':' {
		return "", false
	}
	return v[:i], true
}

// srcset reports whether each of a srcset's URLs is one the rules keep.
func (r *sanitizeRules) srcset(v string) bool {
	for _, c := range strings.Split(v, ",") {
		f := strings.Fields(c)
		if len(f) == 0 {
			continue
		}
		if u, ok := r.url(f[0]); !ok || u != f[0] {
			return false
		}
	}
	return true
}

func spanValue(s string) bool {
	if s == "" || len(s) > 4 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
