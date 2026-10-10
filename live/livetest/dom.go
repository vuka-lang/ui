package livetest

import (
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
)

// node is an element, or text (tag ""), of the page as parsed from its HTML.
type node struct {
	tag    string
	attrs  [][2]string
	kids   []*node
	parent *node
	text   string
	path   string // the element's place: its index among its parent's elements, from the root
}

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.attrs {
		if a[0] == name {
			return a[1], true
		}
	}
	return "", false
}

func (n *node) elems() []*node {
	var out []*node
	for _, k := range n.kids {
		if k.tag != "" {
			out = append(out, k)
		}
	}
	return out
}

// walk calls f for each element under n, in document order, until f is false.
func (n *node) walk(f func(*node) bool) bool {
	for _, k := range n.kids {
		if k.tag != "" && (!f(k) || !k.walk(f)) {
			return false
		}
	}
	return true
}

// parse parses the server's HTML into a tree under a root standing for the
// page's container, instance c0.
func parse(src string) *node {
	root := &node{tag: "#root", attrs: [][2]string{{"data-vk-id", live.RootID}}}
	cur := root
	add := func(n *node) {
		n.parent = cur
		if n.tag != "" {
			n.path = cur.path + "/" + strconv.Itoa(len(cur.elems()))
		}
		cur.kids = append(cur.kids, n)
	}
	for i := 0; i < len(src); {
		lt := strings.IndexByte(src[i:], '<')
		if lt != 0 {
			end := len(src)
			if lt > 0 {
				end = i + lt
			}
			add(&node{text: html.UnescapeString(src[i:end])})
			i = end
			continue
		}
		switch {
		case strings.HasPrefix(src[i:], "<!--"):
			if j := strings.Index(src[i:], "-->"); j >= 0 {
				i += j + 3
			} else {
				i = len(src)
			}
		case strings.HasPrefix(src[i:], "<!"):
			i += strings.IndexByte(src[i:], '>') + 1
		case strings.HasPrefix(src[i:], "</"):
			j := strings.IndexByte(src[i:], '>')
			tag := strings.ToLower(src[i+2 : i+j])
			for n := cur; n != root; n = n.parent {
				if n.tag == tag {
					cur = n.parent
					break
				}
			}
			i += j + 1
		default:
			n, next := openTag(src, i)
			add(n)
			i = next
			switch {
			case ui.Void(n.tag):
			case n.tag == "script" || n.tag == "style":
				end := strings.Index(src[i:], "</"+n.tag)
				if end < 0 {
					end = len(src) - i
				}
				n.kids = []*node{{text: src[i : i+end], parent: n}}
				i += end
				i += strings.IndexByte(src[i:], '>') + 1
			default:
				cur = n
			}
		}
	}
	return root
}

func openTag(src string, i int) (*node, int) {
	i++
	j := i
	for j < len(src) && !strings.ContainsRune(" \t\n/>", rune(src[j])) {
		j++
	}
	n := &node{tag: strings.ToLower(src[i:j])}
	for i = j; i < len(src); {
		for i < len(src) && strings.ContainsRune(" \t\n/", rune(src[i])) {
			i++
		}
		if i >= len(src) || src[i] == '>' {
			return n, i + 1
		}
		j = i
		for j < len(src) && !strings.ContainsRune(" \t\n/>=", rune(src[j])) {
			j++
		}
		name, val := src[i:j], ""
		i = j
		if i < len(src) && src[i] == '=' {
			i++
			if q := src[i]; q == '"' || q == '\'' {
				end := strings.IndexByte(src[i+1:], q)
				val, i = src[i+1:i+1+end], i+end+2
			} else {
				j = i
				for j < len(src) && !strings.ContainsRune(" \t\n>", rune(src[j])) {
					j++
				}
				val, i = src[i:j], j
			}
		}
		n.attrs = append(n.attrs, [2]string{strings.ToLower(name), html.UnescapeString(val)})
	}
	return n, i
}

// textContent is n's text.
func (n *node) textContent() string {
	if n.tag == "" {
		return n.text
	}
	var b strings.Builder
	for _, k := range n.kids {
		b.WriteString(k.textContent())
	}
	return b.String()
}

// A CSS selector, the subset tests need: groups (a, b), descendant and
// child combinators, and compounds of a tag or *, #id, .class, [attr],
// [attr=value].
type selector [][]step // groups of compound steps, last compound first

type step struct {
	comb  byte // ' ' or '>' to the compound before it
	tag   string
	id    string
	class []string
	attrs [][3]string // name, "=" or "", value
}

func parseSelector(s string) selector {
	var sel selector
	for _, g := range strings.Split(s, ",") {
		var steps []step
		comb := byte(' ')
		for _, tok := range tokens(strings.TrimSpace(g)) {
			if tok == ">" {
				comb = '>'
				continue
			}
			st := compound(tok)
			st.comb = comb
			steps = append(steps, st)
			comb = ' '
		}
		slices.Reverse(steps)
		sel = append(sel, steps)
	}
	return sel
}

// tokens splits a selector at whitespace and >, outside brackets.
func tokens(s string) []string {
	var out []string
	cur, depth := "", 0
	flush := func() {
		if cur != "" {
			out = append(out, cur)
			cur = ""
		}
	}
	for _, r := range s {
		switch {
		case r == '[':
			depth++
			cur += string(r)
		case r == ']':
			depth--
			cur += string(r)
		case depth == 0 && (r == ' ' || r == '\t' || r == '\n'):
			flush()
		case depth == 0 && r == '>':
			flush()
			out = append(out, ">")
		default:
			cur += string(r)
		}
	}
	flush()
	return out
}

func compound(s string) step {
	var st step
	for s != "" {
		k := strings.IndexAny(s[1:], "#.[")
		part := s
		if k >= 0 {
			part, s = s[:k+1], s[k+1:]
		} else {
			s = ""
		}
		if part[0] == '[' { // an attribute selector may hold # or .
			end := strings.IndexByte(part+s, ']')
			part, s = (part + s)[:end+1], (part + s)[end+1:]
		}
		switch part[0] {
		case '#':
			st.id = part[1:]
		case '.':
			st.class = append(st.class, part[1:])
		case '[':
			name, val, eq := strings.Cut(part[1:len(part)-1], "=")
			op := ""
			if eq {
				op = "="
			}
			st.attrs = append(st.attrs, [3]string{strings.TrimSpace(name), op, strings.Trim(strings.TrimSpace(val), `"'`)})
		default:
			if part != "*" {
				st.tag = strings.ToLower(part)
			}
		}
	}
	return st
}

// matches reports whether e matches sel, attributes read through get.
func (sel selector) matches(e *node, get func(*node, string) (string, bool)) bool {
	for _, steps := range sel {
		if matchSteps(steps, e, get) {
			return true
		}
	}
	return false
}

func matchSteps(steps []step, e *node, get func(*node, string) (string, bool)) bool {
	if len(steps) == 0 || e == nil || e.tag == "#root" || !steps[0].match(e, get) {
		return false
	}
	if len(steps) == 1 {
		return true
	}
	if steps[0].comb == '>' {
		return matchSteps(steps[1:], e.parent, get)
	}
	for p := e.parent; p != nil; p = p.parent {
		if matchSteps(steps[1:], p, get) {
			return true
		}
	}
	return false
}

func (st step) match(e *node, get func(*node, string) (string, bool)) bool {
	if st.tag != "" && st.tag != e.tag {
		return false
	}
	if st.id != "" {
		if v, _ := get(e, "id"); v != st.id {
			return false
		}
	}
	cls, _ := get(e, "class")
	have := strings.Fields(cls)
	for _, c := range st.class {
		if !slices.Contains(have, c) {
			return false
		}
	}
	for _, a := range st.attrs {
		v, ok := get(e, a[0])
		if !ok || a[1] == "=" && v != a[2] {
			return false
		}
	}
	return true
}
