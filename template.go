package ui

import (
	"fmt"
	"hash/fnv"
	"html"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/a-h/templ"
)

// Markup is HTML with numbered holes, parsed once by Template. Render fills
// the holes and returns the Node: written straight from the precomputed
// static HTML on a plain render, and a frame of statics and dynamics under a
// live session, like JSX.
type Markup struct {
	site   string // where Template was called: file:line, then .2, .3… for more on the line
	file   string
	line   int
	fp     uint64
	shape  string
	nodes  []tnode
	count  [3]int // nodes, attributes, elements
	holes  int
	slots  []tslot // the dynamics, in document order
	direct bool    // slot i is hole i, used as given
	sites  []string
	plan   *ftPlan
	build  func([]any) Node
	static Node // a template without holes: its one node
}

// Template parses src, HTML with holes {{0}}, {{1}}, … that Markup.Render
// fills. Declare it at package level:
//
//	var orderRow = ui.Template(`
//	<tr class="{{0}}">
//	  <td>{{1}}</td>
//	  <td class="tabular-nums">{{2}}</td>
//	</tr>`)
//
//	func OrderRow(o Order) ui.Node {
//		return orderRow.Render(ui.Class{"row": true, "late": o.Late}, o.Ref, money(o.Total))
//	}
//
// A hole takes what its place allows, and a value of another kind fails the
// render with the hole's place in the error:
//
//   - a child, between tags: a Node, []Node (a list, like ui.Each), a string
//     or number (escaped text), an error (its message), nil (nothing), a
//     ui.State (its value, kept current by the browser), a stateful
//     component (&Counter{…}, or ui.Key(k, &Counter{…})), an island's props
//     (Chart{…}, or ui.Mount(Chart{…}, opts…));
//   - text, inside title, textarea or script: a string or number;
//   - an attribute's value, class="{{0}}": what a JSX attribute takes — a
//     string, number, bool (false omits it, true writes it bare), nil, a
//     ui.Class or ui.Style on class and style, a ui.State; an on… attribute
//     takes a handler (a func, ui.On(…), JS commands);
//   - part of a value, href="/orders/{{0}}": a string or number;
//   - an attribute set, <input {{0}}>: a ui.Attr, []ui.Attr or
//     templ.Attributes (ui.BindAttr gives one).
//
// The markup is strict: every element is closed (<br> and the other void
// elements need not be, and <x/> closes any), names are letters, digits and
// - _ : . and holes can't name a tag or an attribute, or sit in a style
// element or a comment. Text follows JSX: lines are trimmed where they meet
// a line break and blank lines dropped, so indentation adds nothing; inside
// pre, textarea, script and style it is kept as written. Entities in text
// and attribute values are decoded and escaped again on output. A mistake is
// a panic when the package initializes, naming the line.
func Template(src string) *Markup {
	m := &Markup{}
	if _, file, line, ok := runtime.Caller(1); ok {
		m.file, m.line = filepath.Base(filepath.Dir(file))+"/"+filepath.Base(file), line
		m.site = m.file + ":" + strconv.Itoa(line)
	}
	if n := siteSeen(m.site); n > 1 {
		m.site += "." + strconv.Itoa(n)
	}
	p := &tparser{m: m, src: src}
	var sh strings.Builder
	m.nodes = p.nodes(&sh, "")
	m.count = counted(m.nodes)
	if p.i < len(src) {
		p.fail("</%s> closes nothing", p.closeName())
	}
	m.shape = sh.String()
	for h := range p.seen {
		if !p.seen[h] {
			p.failAt(len(src), "{{%d}} is missing: holes are numbered from 0", h)
		}
	}
	m.holes, m.slots = len(p.seen), p.slots
	m.direct = len(m.slots) == m.holes
	for i, s := range m.slots {
		m.direct = m.direct && s.kind != kPart && s.hole == i
	}
	for h := range m.holes {
		m.sites = append(m.sites, m.site+"#"+strconv.Itoa(h))
	}
	f := fnv.New64a()
	f.Write([]byte(src))
	m.fp = mix(f.Sum64() ^ 0x74656d706c617465)
	m.build = m.frame
	sample, _ := m.frame(make([]any, len(m.slots))).(*Frame)
	if m.plan = compilePlan(sample, m.shape); m.plan.statics == nil {
		err := Walk(nil, sample, &HTMLRenderer{w: discard{}})
		panic(fmt.Sprintf("ui: template %s: %v", m.site, err))
	}
	if m.holes == 0 {
		m.static = &lazyFrame{fp: m.fp, shape: m.shape, built: sample, p: m.plan}
	}
	return m
}

var (
	sitesMu sync.Mutex
	sites   = map[string]int{}
)

// siteSeen counts the templates declared at site: two on one line are told
// apart by their order.
func siteSeen(site string) int {
	sitesMu.Lock()
	defer sitesMu.Unlock()
	sites[site]++
	return sites[site]
}

// Render fills the holes with args, in order: args[0] fills {{0}}.
func (m *Markup) Render(args ...any) Node {
	if m.static != nil && len(args) == 0 {
		return m.static
	}
	if len(args) != m.holes {
		return Try(nil, fmt.Errorf("ui: template %s has %d holes, Render got %d values", m.site, m.holes, len(args)))
	}
	d := args
	if !m.direct {
		d = make([]any, len(m.slots))
	}
	copied := !m.direct
	for i := range m.slots {
		s := &m.slots[i]
		var v any
		changed := true
		var err error
		switch {
		case s.kind == kPart:
			v, err = m.part(s, args)
		case s.kind < kSet && !s.event && isString(args[s.hole]):
			v, changed = args[s.hole], false
		default:
			v, changed, err = m.check(s, args[s.hole])
		}
		if err != nil {
			return Try(nil, err)
		}
		if !changed && !copied {
			continue
		}
		if !copied {
			d, copied = slices.Clone(args), true
		}
		d[i] = v
	}
	return &lazyFrame{fp: m.fp, shape: m.shape, build: m.build, dyns: d, p: m.plan}
}

func isString(v any) bool {
	_, ok := v.(string)
	return ok
}

// frame is the template's frame with d as its dynamics, as F would build it:
// its nodes, attributes and elements each in one allocation.
func (m *Markup) frame(d []any) Node {
	b := tbuild{d: d, nodes: make([]Node, m.count[0]), attrs: make([]Attr, m.count[1]), elems: make([]Element, m.count[2])}
	return &Frame{FP: m.fp, Shape: m.shape, Kids: b.kids(m.nodes)}
}

type tbuild struct {
	d     []any
	nodes []Node
	attrs []Attr
	elems []Element
}

func (b *tbuild) kids(ns []tnode) []Node {
	out := b.nodes[:len(ns):len(ns)]
	b.nodes = b.nodes[len(ns):]
	for i := range ns {
		n := &ns[i]
		switch {
		case n.static != nil:
			out[i] = n.static
		case n.tag == "":
			out[i] = Child(b.d[n.slot])
		default:
			attrs := b.attrs[:len(n.attrs):len(n.attrs)]
			b.attrs = b.attrs[len(n.attrs):]
			for j, a := range n.attrs {
				attrs[j] = Attr{a.name, a.value}
				if a.slot >= 0 {
					attrs[j].Value = b.d[a.slot]
				}
			}
			e := &b.elems[0]
			b.elems = b.elems[1:]
			e.Tag, e.Attrs = n.tag, attrs
			if len(attrs) == 0 {
				e.Attrs = nil
			}
			if len(n.kids) > 0 {
				e.Children = b.kids(n.kids)
			}
			out[i] = e
		}
	}
	return out
}

// counted is how many nodes, attributes and elements ns holds.
func counted(ns []tnode) (c [3]int) {
	c[0] = len(ns)
	for _, n := range ns {
		if n.static == nil && n.tag != "" {
			k := counted(n.kids)
			c[0] += k[0]
			c[1] += len(n.attrs) + k[1]
			c[2] += 1 + k[2]
		}
	}
	return c
}

// tnode is a parsed node: static text (static), a hole (slot) or an element
// (tag).
type tnode struct {
	static Node
	slot   int
	tag    string
	attrs  []tattr
	kids   []tnode
}

// tattr is an element's attribute: static (slot -1) or a dynamic.
type tattr struct {
	name  string
	value any
	slot  int
}

// What a slot holds, by where it is.
const (
	kChild = iota // between tags
	kText         // inside title, textarea or script
	kValue        // an attribute's whole value
	kPart         // part of an attribute's value
	kSet          // where an attribute goes
)

type tslot struct {
	kind   uint8
	hole   int
	parts  []tpart
	where  string // for errors
	event  bool   // an on… attribute
	class  bool
	styled bool
}

// tpart is a piece of an attribute's value: static text or a hole.
type tpart struct {
	s    string
	hole int // -1: static
}

func (m *Markup) wrong(s *tslot, hole int, v any, why string) error {
	return fmt.Errorf("ui: template %s: {{%d}} is %s; it got %T, %s", m.site, hole, s.where, v, why)
}

const (
	attrsWhy   = "an attribute set: write it where an attribute goes, <div {{n}}>"
	handlerWhy = "an event handler: only an on… attribute takes one"
)

// check is v as slot s's value: converted (changed) where the place gives it
// a meaning — a component's site, a state's binding — or an error when the
// place can't take it.
func (m *Markup) check(s *tslot, v any) (any, bool, error) {
	switch s.kind {
	case kChild:
		switch x := v.(type) {
		case nil, string, int, int64, float64, bool, error, Binding:
			return v, false, nil
		case Attr, []Attr, templ.Attributes:
			return nil, false, m.wrong(s, s.hole, v, attrsWhy)
		case EventHandler, JSChain:
			return nil, false, m.wrong(s, s.hole, v, handlerWhy)
		case Class, Style:
			return nil, false, m.wrong(s, s.hole, v, "an attribute's value")
		case *ComponentNode:
			if x != nil && x.Site == "" {
				cp := *x
				cp.Site = m.sites[s.hole]
				return &cp, true, nil
			}
			return v, false, nil
		case *islandMount:
			return x.node(m.sites[s.hole]), true, nil
		case *list:
			cp := *x
			cp.site = m.sites[s.hole]
			return &cp, true, nil
		case Node:
			return v, false, nil
		case []Node:
			l := listOf(x)
			l.site = m.sites[s.hole]
			return l, true, nil
		case Stateful:
			return &ComponentNode{Site: m.sites[s.hole], Props: x}, true, nil
		case AnyState:
			return bindState(x), true, nil
		}
		t := reflect.TypeOf(v)
		if t.Kind() == reflect.Func {
			return nil, false, m.wrong(s, s.hole, v, "a func: an element's on… attribute takes it")
		}
		if lookupIsland(t) != nil {
			return MountIsland(m.sites[s.hole], v), true, nil
		}
	case kText:
		switch v.(type) {
		case nil, string, int, int64, float64, bool, error, TextNode:
			return v, false, nil
		case Node, Stateful, AnyState, Attr, []Attr, templ.Attributes, EventHandler, JSChain, Class, Style:
			return nil, false, m.wrong(s, s.hole, v, "not text")
		}
		if reflect.TypeOf(v).Kind() == reflect.Func {
			return nil, false, m.wrong(s, s.hole, v, "not text")
		}
	case kValue:
		if s.event {
			switch v.(type) {
			case nil, EventHandler, templ.ComponentScript:
				return v, false, nil
			case JSChain:
				return On(v), true, nil
			}
			if reflect.TypeOf(v).Kind() == reflect.Func {
				return On(v), true, nil
			}
			return nil, false, m.wrong(s, s.hole, v, "not a handler: a func, ui.On(…), JS commands or a templ.ComponentScript")
		}
		switch x := v.(type) {
		case nil, string, int, int64, float64, bool, Binding:
			return v, false, nil
		case Attr, []Attr, templ.Attributes:
			return nil, false, m.wrong(s, s.hole, v, attrsWhy)
		case EventHandler, JSChain:
			return nil, false, m.wrong(s, s.hole, v, handlerWhy)
		case Class:
			if !s.class {
				return nil, false, m.wrong(s, s.hole, v, "a class: only class takes one")
			}
			return v, false, nil
		case Style:
			if !s.styled {
				return nil, false, m.wrong(s, s.hole, v, "a style: only style takes one")
			}
			return v, false, nil
		case Stateful, *ComponentNode, *islandMount:
			return nil, false, m.wrong(s, s.hole, v, "a component: a child hole takes it")
		case AnyState:
			return bindState(x), true, nil
		}
		if reflect.TypeOf(v).Kind() == reflect.Func {
			return nil, false, m.wrong(s, s.hole, v, "a func: only an on… attribute takes one")
		}
	case kSet:
		var set attrSet
		switch x := v.(type) {
		case nil:
		case Attr:
			set = attrSet{x}
		case []Attr:
			set = attrSet(x)
		case templ.Attributes:
			for _, k := range sortedKeys(x) {
				set = append(set, Attr{k, x[k]})
			}
		default:
			return nil, false, m.wrong(s, s.hole, v, "not an attribute set: a ui.Attr, []ui.Attr or templ.Attributes")
		}
		for _, a := range set {
			if a.Name == "key" {
				return nil, false, m.wrong(s, s.hole, v, `holding key: write key="{{n}}" on the element`)
			}
		}
		return set, true, nil
	}
	return v, false, nil
}

// part is the value of an attribute whose value holes are part of: the
// static text and each hole's value as text.
func (m *Markup) part(s *tslot, args []any) (any, error) {
	var b strings.Builder
	for _, p := range s.parts {
		if p.hole < 0 {
			b.WriteString(p.s)
			continue
		}
		switch v := args[p.hole].(type) {
		case nil:
		case string:
			b.WriteString(v)
		case int, int64, float64, templ.SafeURL, error, fmt.Stringer:
			b.WriteString(toString(v))
		case Class, []string, map[string]bool:
			if !s.class {
				return nil, m.wrong(s, p.hole, v, "a class: only class takes one")
			}
			if c, ok := v.(Class); ok {
				v = map[string]bool(c)
			}
			b.WriteString(templ.Classes(v).String())
		case Node:
			return nil, m.wrong(s, p.hole, v, "not text: give the whole value to one hole")
		default:
			if k := reflect.TypeOf(v).Kind(); k >= reflect.Int && k <= reflect.Float64 || k == reflect.String {
				b.WriteString(fmt.Sprint(v))
				continue
			}
			return nil, m.wrong(s, p.hole, v, "not text: give the whole value to one hole")
		}
	}
	return b.String(), nil
}

// attrSet is an attribute set hole's attributes, one attribute of the frame.
type attrSet []Attr

func sortedKeys(a templ.Attributes) []string {
	ks := make([]string, 0, len(a))
	for k := range a {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// bindState is a binding reading s as it is.
func bindState(s AnyState) Binding { return Bind(s.stateOf().v, `{"$":0}`, s) }

// tparser parses a template's source.
type tparser struct {
	m     *Markup
	src   string
	i     int
	slots []tslot
	seen  []bool
	pre   int // inside pre
}

func (p *tparser) fail(format string, args ...any) { p.failAt(p.i, format, args...) }

func (p *tparser) failAt(off int, format string, args ...any) {
	line := p.m.line + strings.Count(p.src[:min(off, len(p.src))], "\n")
	panic(fmt.Sprintf("ui: template %s:%d: %s", p.m.file, line, fmt.Sprintf(format, args...)))
}

func (p *tparser) at(s string) bool { return strings.HasPrefix(p.src[p.i:], s) }

// hole reads {{n}} at p.i, if one is there.
func (p *tparser) hole() (int, bool) {
	s := p.src[p.i:]
	if !strings.HasPrefix(s, "{{") {
		return 0, false
	}
	j := 2
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == 2 || j > 6 || !strings.HasPrefix(s[j:], "}}") {
		return 0, false
	}
	n, _ := strconv.Atoi(s[2:j])
	p.i += j + 2
	for len(p.seen) <= n {
		p.seen = append(p.seen, false)
	}
	p.seen[n] = true
	return n, true
}

func (p *tparser) slot(s tslot) int {
	p.slots = append(p.slots, s)
	return len(p.slots) - 1
}

// tagStart reports whether a tag, a closing tag or a comment starts at p.i.
func (p *tparser) tagStart() bool {
	if p.i+1 >= len(p.src) || p.src[p.i] != '<' {
		return false
	}
	c := p.src[p.i+1]
	return c == '/' || c == '!' || nameByte[c] == nameLetter
}

func (p *tparser) closeName() string {
	j := p.i + 2
	for j < len(p.src) && nameByte[p.src[j]] != 0 {
		j++
	}
	return p.src[p.i+2 : j]
}

// nodes parses nodes up to a closing tag or the end, writing their shape.
func (p *tparser) nodes(sh *strings.Builder, parent string) []tnode {
	var out []tnode
	for p.i < len(p.src) {
		switch {
		case p.at("</"):
			return out
		case p.at("<!--"):
			end := strings.Index(p.src[p.i+4:], "-->")
			if end < 0 {
				p.fail("a comment isn't closed")
			}
			if hasHole(p.src[p.i : p.i+4+end]) {
				p.fail("a hole can't be in a comment")
			}
			p.i += 4 + end + 3
		case p.at("<!"):
			end := strings.IndexByte(p.src[p.i:], '>')
			if end < 0 {
				p.fail("<! isn't closed")
			}
			p.i += end + 1
		case p.tagStart():
			out = append(out, p.element(sh))
		default:
			out = p.text(sh, out, parent)
		}
	}
	return out
}

// hasHole reports whether s holds a hole.
func hasHole(s string) bool {
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			return false
		}
		s = s[i+2:]
		j := 0
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j > 0 && strings.HasPrefix(s[j:], "}}") {
			return true
		}
	}
}

// text parses text up to the next tag, splitting it at holes.
func (p *tparser) text(sh *strings.Builder, out []tnode, parent string) []tnode {
	start := p.i
	flush := func(end int) {
		if t := p.textValue(p.src[start:end]); t != "" {
			out = append(out, tnode{static: TextNode(t)})
			sh.WriteByte('t')
		}
	}
	for p.i < len(p.src) && !p.tagStart() {
		at := p.i
		if n, ok := p.hole(); ok {
			flush(at)
			where := "a child"
			if parent != "" {
				where = "a child of <" + parent + ">"
			}
			out = append(out, tnode{slot: p.slot(tslot{kind: kChild, hole: n, where: where})})
			sh.WriteByte('h')
			start = p.i
			continue
		}
		p.i++
	}
	flush(p.i)
	return out
}

func (p *tparser) textValue(raw string) string {
	if p.pre > 0 {
		return html.UnescapeString(raw)
	}
	return jsxText(raw)
}

// jsxText is text as JSX reads it: lines trimmed where they meet a line
// break, blank lines dropped, the rest joined with one space; entities
// decoded.
func jsxText(raw string) string {
	if !strings.ContainsAny(raw, "\n\t&") {
		return raw
	}
	lines := strings.Split(raw, "\n")
	last := 0
	for i, l := range lines {
		if strings.Trim(l, " \t\r") != "" {
			last = i
		}
	}
	var b strings.Builder
	for i, l := range lines {
		l = strings.ReplaceAll(l, "\t", " ")
		if i > 0 {
			l = strings.TrimLeft(l, " \r")
		}
		if i < len(lines)-1 {
			l = strings.TrimRight(l, " \r")
		}
		if l == "" {
			continue
		}
		if i != last {
			l += " "
		}
		b.WriteString(l)
	}
	return html.UnescapeString(b.String())
}

func (p *tparser) name() string {
	j := p.i
	for j < len(p.src) && nameByte[p.src[j]] != 0 {
		j++
	}
	s := p.src[p.i:j]
	p.i = j
	return s
}

func (p *tparser) space() {
	for p.i < len(p.src) && strings.IndexByte(" \t\r\n", p.src[p.i]) >= 0 {
		p.i++
	}
}

// element parses an element at p.i, its children and its closing tag.
func (p *tparser) element(sh *strings.Builder) tnode {
	start := p.i
	p.i++
	n := tnode{tag: p.name()}
	if !validName(n.tag) {
		p.fail("a tag needs a name")
	}
	var flags strings.Builder
	closed := false
	for {
		p.space()
		if p.i >= len(p.src) {
			p.failAt(start, "<%s isn't closed", n.tag)
		}
		if p.at(">") {
			p.i++
			break
		}
		if p.at("/>") {
			p.i += 2
			closed = true
			break
		}
		if h, ok := p.hole(); ok {
			n.attrs = append(n.attrs, tattr{slot: p.slot(tslot{kind: kSet, hole: h, where: "an attribute set of <" + n.tag + ">"})})
			flags.WriteByte('d')
			continue
		}
		a := p.attr(n.tag)
		n.attrs = append(n.attrs, a)
		flags.WriteByte("sd"[min(a.slot+1, 1)])
	}
	sh.WriteByte('E')
	sh.WriteString(flags.String())
	sh.WriteByte('(')
	defer sh.WriteByte(')')
	if closed || Void(n.tag) {
		return n
	}
	switch n.tag {
	case "script", "style", "textarea", "title":
		n.kids = p.raw(sh, n.tag)
	default:
		if n.tag == "pre" {
			p.pre++
		}
		n.kids = p.nodes(sh, n.tag)
		if n.tag == "pre" {
			p.pre--
		}
	}
	if !p.at("</") {
		p.failAt(start, "<%s> isn't closed", n.tag)
	}
	if name := p.closeName(); name != n.tag {
		p.fail("</%s> closes <%s>", name, n.tag)
	}
	p.i += 2 + len(n.tag)
	p.space()
	if !p.at(">") {
		p.fail("</%s needs a >", n.tag)
	}
	p.i++
	return n
}

// raw parses a text-only element's content up to its closing tag: verbatim
// in script and style, entities decoded in textarea and title.
func (p *tparser) raw(sh *strings.Builder, tag string) []tnode {
	end := strings.Index(p.src[p.i:], "</"+tag)
	if end < 0 {
		p.fail("<%s> isn't closed", tag)
	}
	end += p.i
	var out []tnode
	start := p.i
	flush := func(to int) {
		s := p.src[start:to]
		switch tag {
		case "title":
			s = jsxText(s)
		case "textarea":
			s = html.UnescapeString(s)
		}
		if s == "" {
			return
		}
		if tag == "script" || tag == "style" {
			out = append(out, tnode{static: RawHTML(s)})
		} else {
			out = append(out, tnode{static: TextNode(s)})
		}
		sh.WriteByte('t')
	}
	for p.i < end {
		at := p.i
		if n, ok := p.hole(); ok {
			if tag == "style" {
				p.failAt(at, "a hole can't be in <style>; set a class or a style attribute instead")
			}
			flush(at)
			out = append(out, tnode{slot: p.slot(tslot{kind: kText, hole: n, where: "text in <" + tag + ">"})})
			sh.WriteByte('h')
			start = p.i
			continue
		}
		p.i++
	}
	p.i = end
	flush(end)
	return out
}

// attr parses an attribute: name, name=value, name="…{{n}}…" or name={{n}}.
func (p *tparser) attr(tag string) tattr {
	name := p.name()
	if !validName(name) {
		if p.i < len(p.src) && p.src[p.i] == '{' {
			p.fail("a hole can't name an attribute; a hole where an attribute goes sets attributes: <%s {{n}}>", tag)
		}
		p.fail("<%s> has an attribute with no name, or a name with something other than letters, digits and - _ : .", tag)
	}
	if p.at("{{") {
		p.fail("a hole can't name an attribute; a hole where an attribute goes sets attributes: <%s {{n}}>", tag)
	}
	a := tattr{name: name, value: true, slot: -1}
	save := p.i
	p.space()
	if !p.at("=") {
		p.i = save
		return a
	}
	p.i++
	p.space()
	lower := strings.ToLower(name)
	s := tslot{kind: kValue, where: name + " on <" + tag + ">",
		event: len(lower) > 2 && strings.HasPrefix(lower, "on"),
		class: lower == "class" || name == "className", styled: lower == "style"}
	if h, ok := p.hole(); ok {
		s.hole = h
		a.slot = p.slot(s)
		return a
	}
	if p.i >= len(p.src) {
		p.fail("%s on <%s> has no value", name, tag)
	}
	q := p.src[p.i]
	if q != '"' && q != '\'' {
		j := p.i
		for j < len(p.src) && strings.IndexByte(" \t\r\n>", p.src[j]) < 0 && !strings.HasPrefix(p.src[j:], "/>") {
			j++
		}
		if hasHole(p.src[p.i:j]) {
			p.fail("%s on <%s>: quote a value holes are part of", name, tag)
		}
		a.value = html.UnescapeString(p.src[p.i:j])
		p.i = j
		return a
	}
	p.i++
	end := strings.IndexByte(p.src[p.i:], q)
	if end < 0 {
		p.fail("%s on <%s>: the value's quote isn't closed", name, tag)
	}
	end += p.i
	start := p.i
	for p.i < end {
		at := p.i
		if h, ok := p.hole(); ok {
			if at > start {
				s.parts = append(s.parts, tpart{html.UnescapeString(p.src[start:at]), -1})
			}
			s.parts = append(s.parts, tpart{hole: h})
			start = p.i
			continue
		}
		p.i++
	}
	if end > start {
		s.parts = append(s.parts, tpart{html.UnescapeString(p.src[start:end]), -1})
	}
	p.i = end + 1
	switch {
	case len(s.parts) == 0:
		a.value = ""
	case len(s.parts) == 1 && s.parts[0].hole < 0:
		a.value = s.parts[0].s
	case len(s.parts) == 1:
		s.hole = s.parts[0].hole
		s.parts = nil
		a.slot = p.slot(s)
	default:
		if s.event {
			p.fail("%s on <%s> takes a handler: give its whole value to one hole", name, tag)
		}
		s.kind, s.where = kPart, "part of "+s.where
		a.slot = p.slot(s)
	}
	return a
}
