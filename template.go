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
	"unsafe"

	"github.com/a-h/templ"
)

// HTML is src, markup with labelled holes, filled with args: one value for
// each label, in the order the labels first appear. A label is the Go
// expression its value is written as, so the call reads like the markup:
//
//	func OrderRow(o Order) ui.Node {
//		return ui.HTML(`
//	<tr key={o.ID} class="row" class:late={o.Late}>
//	  <td>{o.Ref}</td>
//	  <td class="tabular-nums">{o.Total | money}</td>
//	</tr>`, o.ID, o.Late, o.Ref, o.Total)
//	}
//
// src is parsed once, on its first render, and kept: write it as a
// constant. A hole takes what its place allows, and a value of another kind
// fails the render naming the hole and its line:
//
//   - a child, between tags, {x}: a Node, []Node (a list, like ui.Each), a
//     string or number (escaped text), an error (its message), nil
//     (nothing), a ui.State (its value, kept current by the browser), a
//     stateful component (&Counter{…}, or ui.Key(k, &Counter{…})), an
//     island's props (Chart{…}, or ui.Mount(Chart{…}, opts…));
//   - text, inside title or textarea: a string or number;
//   - an attribute's value, class={x} or class="{x}": what a JSX attribute
//     takes — a string, number, bool (false omits it, true writes it
//     bare), nil, a ui.Class, ui.Classes or ui.Style on class and style, a
//     ui.State (an expression of states too: open.Not()); an on… attribute
//     takes a handler (a func, ui.On(…), JS commands);
//   - part of a value, href="/orders/{id}": a string or number;
//   - an attribute set, <input {attrs...}>: a ui.Attr, []ui.Attr or
//     templ.Attributes (ui.BindAttr gives one);
//   - a toggled class, class:active={on}: a bool or a ui.State[bool].
//
// A label appearing twice is one value. Blocks are Go producing nodes —
// ui.Each, ui.If, a []ui.Node — and components their calls, each in a
// hole: {rows}, {ui.Card{Title: t}}. {/* … */} is a comment.
//
// The markup is strict: every element is closed (<br> and the other void
// elements need not be, and <x/> closes any), names are letters, digits and
// - _ : . and holes can't name a tag or an attribute, or sit in a comment.
// {{ is a literal {; inside script and style braces are text. Text follows JSX: lines are trimmed where they meet a line break
// and blank lines dropped, so indentation adds nothing; inside pre and
// textarea it is kept as written. Entities in text and attribute values are
// decoded and escaped again on output. A mistake fails the render, naming
// the line.
func HTML(src string, args ...any) Node {
	t, err := htmlTemplate(src)
	if err != nil {
		return Try(nil, err)
	}
	return t.renderArgs(args)
}

type htmlEntry struct {
	src string
	t   *tmpl
	err error
}

// Templates parsed by HTML, by their source's data pointer (a constant's is
// fixed) and, for a string built at run time, by its text.
var (
	htmlByPtr  sync.Map // unsafe.Pointer → *htmlEntry
	htmlMu     sync.Mutex
	htmlBySrc  = map[string]*htmlEntry{}
	htmlPtrs   int
	htmlCached = 10000
)

func htmlTemplate(src string) (*tmpl, error) {
	if e, ok := htmlByPtr.Load(unsafe.Pointer(unsafe.StringData(src))); ok {
		if e := e.(*htmlEntry); e.src == src {
			return e.t, e.err
		}
	}
	return htmlParse(src)
}

func htmlParse(src string) (*tmpl, error) {
	htmlMu.Lock()
	defer htmlMu.Unlock()
	e := htmlBySrc[src]
	if e == nil {
		e = &htmlEntry{src: src}
		file, place, line := caller(4)
		e.t, e.err = parseTemplate(src, file, place, line)
		if len(htmlBySrc) < htmlCached {
			htmlBySrc[src] = e
		}
	}
	if htmlPtrs < htmlCached {
		htmlPtrs++
		htmlByPtr.Store(unsafe.Pointer(unsafe.StringData(src)), &htmlEntry{src, e.t, e.err})
	}
	return e.t, e.err
}

// caller is where the function skip frames up was called: its directory
// and file, for messages; its package and file, the place its templates'
// sites name, the same wherever the module is checked out; and its line.
func caller(skip int) (file, place string, line int) {
	pc, path, line, ok := runtime.Caller(skip)
	if !ok {
		return "template", "template", 1
	}
	base := filepath.Base(path)
	place = base
	if fn := runtime.FuncForPC(pc); fn != nil {
		name := fn.Name()
		slash := strings.LastIndexByte(name, '/')
		if dot := strings.IndexByte(name[slash+1:], '.'); dot >= 0 {
			place = name[:slash+1+dot] + "/" + base
		}
	}
	return filepath.Base(filepath.Dir(path)) + "/" + base, place, line
}

var (
	sitesMu sync.Mutex
	sites   = map[string]int{}
)

// siteOf is a template's site, place:line, with .2, .3… for the templates
// after the first declared on the line.
func siteOf(place string, line int) string {
	site := place + ":" + strconv.Itoa(line)
	sitesMu.Lock()
	defer sitesMu.Unlock()
	sites[site]++
	if n := sites[site]; n > 1 {
		site += "." + strconv.Itoa(n)
	}
	return site
}

// tmpl is parsed markup: a block's body or a component's children are
// templates of their own, read with the same values.
type tmpl struct {
	site   string
	fp     uint64
	shape  string
	nodes  []tnode
	count  [3]int // nodes, attributes, elements
	slots  []tslot
	sites  []string
	empty  []*list // each child slot's empty list, an empty []Node's value
	direct bool    // slot i is value i, used as given
	pass   bool    // the template is one child hole: {nodes}
	plan   *ftPlan
	build  func([]any) Node
	static Node     // a template without holes: its one node
	labels []string // the positional values' labels, at the root
}

// blockAt is the block a []Node in child slot i is: the slot's own empty
// block, or the nodes, placed at the slot.
func (t *tmpl) blockAt(i int, ns []Node) *list {
	if len(ns) == 0 {
		return t.empty[i]
	}
	return &list{site: t.sites[i], nodes: ns}
}

// renderArgs fills an HTML template's holes with args.
func (t *tmpl) renderArgs(args []any) Node {
	if len(args) != len(t.labels) {
		return Try(nil, fmt.Errorf("ui: template %s has %d values (%s), it got %d", t.site, len(t.labels), labelList(t.labels), len(args)))
	}
	if t.static != nil {
		return t.static
	}
	if t.pass {
		// A template that is one hole of nodes, {nodes}, is those nodes:
		// the block a statement at a template's top level builds.
		if x, ok := args[0].([]Node); ok {
			if len(x) == 0 {
				return t.empty[0]
			}
			return &list{site: t.sites[0], nodes: x}
		}
	}
	if !t.direct {
		return t.render(args)
	}
	d := args
	copied := false
	for i := range t.slots {
		s := &t.slots[i]
		if s.kind < kSet && !s.event && isString(args[i]) {
			continue
		}
		if s.kind == kChild {
			if _, ok := args[i].([]Node); ok {
				continue // a block: the frame's build makes it the slot's (blockAt)
			}
		}
		v, changed, err := t.check(s, i, args[i])
		if err != nil {
			return Try(nil, err)
		}
		if !changed {
			continue
		}
		if !copied {
			d, copied = slices.Clone(args), true
		}
		d[i] = v
	}
	return &lazyFrame{fp: t.fp, shape: t.shape, build: t.build, dyns: d, p: t.plan}
}

func labelList(ls []string) string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = "{" + l + "}"
	}
	return strings.Join(out, ", ")
}

// render fills the holes from args.
func (t *tmpl) render(args []any) Node {
	if t.static != nil {
		return t.static
	}
	d := make([]any, len(t.slots))
	for i := range t.slots {
		v, err := t.value(i, args)
		if err != nil {
			return Try(nil, err)
		}
		d[i] = v
	}
	return &lazyFrame{fp: t.fp, shape: t.shape, build: t.build, dyns: d, p: t.plan}
}

// value is slot i's dynamic.
func (t *tmpl) value(i int, args []any) (any, error) {
	s := &t.slots[i]
	switch s.kind {
	case kPart:
		return t.part(s, args)
	case kClass:
		v, err := t.classes(s, args)
		if err != nil {
			return nil, err
		}
		v, _, err = t.check(s, i, v)
		return v, err
	}
	v := args[s.op.arg]
	if s.kind < kSet && !s.event && isString(v) {
		return v, nil
	}
	v, _, err := t.check(s, i, v)
	return v, err
}

func isString(v any) bool {
	_, ok := v.(string)
	return ok
}

// frame is the template's frame with d as its dynamics, as F would build it:
// its nodes, attributes and elements each in one allocation.
func (t *tmpl) frame(d []any) Node {
	b := tbuild{t: t, d: d, nodes: make([]Node, t.count[0]), attrs: make([]Attr, t.count[1]), elems: make([]Element, t.count[2])}
	return &Frame{FP: t.fp, Shape: t.shape, Kids: b.kids(t.nodes)}
}

type tbuild struct {
	t     *tmpl
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
			if ns, ok := b.d[n.slot].([]Node); ok {
				out[i] = b.t.blockAt(n.slot, ns)
			} else {
				out[i] = Child(b.d[n.slot])
			}
		default:
			attrs := b.attrs[:len(n.attrs):len(n.attrs)]
			b.attrs = b.attrs[len(n.attrs):]
			for j, a := range n.attrs {
				attrs[j] = Attr{a.name, a.value}
				if a.slot >= 0 {
					v := b.d[a.slot]
					if x, ok := v.([]Attr); ok && b.t.slots[a.slot].kind == kSet {
						v = attrSet(x)
					}
					attrs[j].Value = v
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
	kText         // inside title or textarea
	kValue        // an attribute's whole value
	kPart         // part of an attribute's value
	kClass        // class with toggled names
	kSet          // where an attribute goes
)

type tslot struct {
	kind    uint8
	op      *hole
	parts   []tpart   // kPart; kClass: the class attribute's value
	toggles []ttoggle // kClass
	where   string    // for errors
	line    string    // file:line
	event   bool      // an on… attribute
	class   bool
	styled  bool
}

// tpart is a piece of an attribute's value: static text or a hole.
type tpart struct {
	s  string
	op *hole // nil: static
}

// ttoggle is class:name={on}.
type ttoggle struct {
	name string
	op   *hole
}

func (t *tmpl) wrong(s *tslot, op *hole, v any, why string) error {
	return fmt.Errorf("ui: template %s: {%s} is %s; it got %T, %s", s.line, op.label, s.where, v, why)
}

const (
	attrsWhy   = "an attribute set: write it where an attribute goes, <div {attrs...}>"
	handlerWhy = "an event handler: only an on… attribute takes one"
)

// check is v as slot s's value: converted (changed) where the place gives it
// a meaning — a component's site, a state's binding — or an error when the
// place can't take it.
func (t *tmpl) check(s *tslot, i int, v any) (any, bool, error) {
	op := s.op
	if op == nil {
		op = &hole{label: "…"}
	}
	switch s.kind {
	case kChild:
		switch x := v.(type) {
		case nil, string, int, int64, float64, bool, error, Binding:
			return v, false, nil
		case Attr, []Attr, templ.Attributes:
			return nil, false, t.wrong(s, op, v, attrsWhy)
		case EventHandler, JSChain:
			return nil, false, t.wrong(s, op, v, handlerWhy)
		case Class, Classes, Style:
			return nil, false, t.wrong(s, op, v, "an attribute's value")
		case *ComponentNode:
			if x != nil && x.Site == "" {
				cp := *x
				cp.Site = t.sites[i]
				return &cp, true, nil
			}
			return v, false, nil
		case *islandMount:
			return x.node(t.sites[i]), true, nil
		case *list:
			cp := *x
			cp.site = t.sites[i]
			return &cp, true, nil
		case Node:
			return v, false, nil
		case []Node:
			if len(x) == 0 {
				return t.empty[i], true, nil
			}
			l := listOf(x)
			l.site = t.sites[i]
			return l, true, nil
		case Stateful:
			return &ComponentNode{Site: t.sites[i], Props: x}, true, nil
		case AnyState:
			return bindState(x), true, nil
		}
		rt := reflect.TypeOf(v)
		if rt.Kind() == reflect.Func {
			return nil, false, t.wrong(s, op, v, "a func: an element's on… attribute takes it")
		}
		if lookupIsland(rt) != nil {
			return MountIsland(t.sites[i], v), true, nil
		}
	case kText:
		switch v.(type) {
		case nil, string, int, int64, float64, bool, error, TextNode:
			return v, false, nil
		case Node, Stateful, AnyState, Attr, []Attr, templ.Attributes, EventHandler, JSChain, Class, Classes, Style:
			return nil, false, t.wrong(s, op, v, "not text")
		}
		if reflect.TypeOf(v).Kind() == reflect.Func {
			return nil, false, t.wrong(s, op, v, "not text")
		}
	case kValue, kClass:
		if s.event {
			switch v.(type) {
			case nil, EventHandler, templ.ComponentScript, JSChain:
				return v, false, nil // the renderer takes a func or commands as a handler
			}
			if reflect.TypeOf(v).Kind() == reflect.Func {
				return v, false, nil
			}
			return nil, false, t.wrong(s, op, v, "not a handler: a func, ui.On(…), JS commands or a templ.ComponentScript")
		}
		switch x := v.(type) {
		case nil, string, int, int64, float64, bool, Binding:
			return v, false, nil
		case Attr, []Attr, templ.Attributes:
			return nil, false, t.wrong(s, op, v, attrsWhy)
		case EventHandler, JSChain:
			return nil, false, t.wrong(s, op, v, handlerWhy)
		case Class:
			if !s.class {
				return nil, false, t.wrong(s, op, v, "a class: only class takes one")
			}
			return v, false, nil
		case Classes:
			if !s.class {
				return nil, false, t.wrong(s, op, v, "a class: only class takes one")
			}
			return x.binding(), true, nil
		case Style:
			if !s.styled {
				return nil, false, t.wrong(s, op, v, "a style: only style takes one")
			}
			return v, false, nil
		case Stateful, *ComponentNode, *islandMount:
			return nil, false, t.wrong(s, op, v, "a component: a child hole takes it")
		case AnyState:
			return bindState(x), true, nil
		}
		if reflect.TypeOf(v).Kind() == reflect.Func {
			return nil, false, t.wrong(s, op, v, "a func: only an on… attribute takes one")
		}
	case kSet:
		var set attrSet
		switch x := v.(type) {
		case nil:
		case Attr:
			set = attrSet{x}
		case []Attr:
			// kept as given: the frame's build and the fast path read it as
			// the slot's set
			for _, a := range x {
				if a.Name == "key" {
					return nil, false, t.wrong(s, op, v, `holding key: write key={k} on the element`)
				}
			}
			return v, false, nil
		case templ.Attributes:
			for _, k := range sortedKeys(x) {
				set = append(set, Attr{k, x[k]})
			}
		default:
			return nil, false, t.wrong(s, op, v, "not an attribute set: a ui.Attr, []ui.Attr or templ.Attributes")
		}
		for _, a := range set {
			if a.Name == "key" {
				return nil, false, t.wrong(s, op, v, `holding key: write key={k} on the element`)
			}
		}
		return set, true, nil
	}
	return v, false, nil
}

// part is the value of an attribute whose value holes are part of: the
// static text and each hole's value as text.
func (t *tmpl) part(s *tslot, args []any) (any, error) {
	var b strings.Builder
	for _, p := range s.parts {
		if p.op == nil {
			b.WriteString(p.s)
			continue
		}
		v := args[p.op.arg]
		switch x := v.(type) {
		case nil:
		case string:
			b.WriteString(x)
		case int, int64, float64, templ.SafeURL, error, fmt.Stringer:
			b.WriteString(toString(x))
		case Class, []string, map[string]bool:
			if !s.class {
				return nil, t.wrong(s, p.op, v, "a class: only class takes one")
			}
			if c, ok := x.(Class); ok {
				v = map[string]bool(c)
			}
			b.WriteString(templ.Classes(v).String())
		case Node:
			return nil, t.wrong(s, p.op, v, "not text: give the whole value to one hole")
		default:
			if k := reflect.TypeOf(v).Kind(); k >= reflect.Int && k <= reflect.Float64 || k == reflect.String {
				b.WriteString(fmt.Sprint(v))
				continue
			}
			return nil, t.wrong(s, p.op, v, "not text: give the whole value to one hole")
		}
	}
	return b.String(), nil
}

// classes is a class attribute with toggled names: a Class, or Classes
// when a state switches one.
func (t *tmpl) classes(s *tslot, args []any) (any, error) {
	c := Classes{}
	stated := false
	var base any = ""
	if len(s.parts) == 1 && s.parts[0].op != nil {
		base = args[s.parts[0].op.arg]
	} else if len(s.parts) > 0 {
		v, err := t.part(s, args)
		if err != nil {
			return nil, err
		}
		base = v
	}
	if b, ok := base.(string); ok || base == nil {
		if cl, ok := t.plainClass(s, b, args); ok {
			return cl, nil
		}
	}
	switch b := base.(type) {
	case nil:
	case string:
		for _, n := range strings.Fields(b) {
			c[n] = true
		}
	case Class:
		for k, v := range b {
			c[k] = v
		}
	case Classes:
		for k, v := range b {
			c[k] = v
			_, isState := v.(AnyState)
			stated = stated || isState
		}
	default:
		return nil, t.wrong(s, s.parts[0].op, base, "not a class: a string, ui.Class or ui.Classes")
	}
	for _, g := range s.toggles {
		v := args[g.op.arg]
		switch x := v.(type) {
		case bool:
			c[g.name] = x
		case AnyState:
			c[g.name], stated = x, true
		default:
			return nil, t.wrong(s, g.op, v, "not a bool or a ui.State[bool]")
		}
	}
	if stated {
		return c, nil
	}
	cl := make(Class, len(c))
	for k, v := range c {
		cl[k] = v.(bool)
	}
	return cl, nil
}

// plainClass is a class of a string and toggles none of which is a state:
// one Class, built at once.
func (t *tmpl) plainClass(s *tslot, base string, args []any) (Class, bool) {
	for _, g := range s.toggles {
		if _, ok := args[g.op.arg].(bool); !ok {
			return nil, false
		}
	}
	names := strings.Fields(base)
	cl := make(Class, len(names)+len(s.toggles))
	for _, n := range names {
		cl[n] = true
	}
	for _, g := range s.toggles {
		cl[g.name] = args[g.op.arg].(bool)
	}
	return cl, true
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
func bindState(s AnyState) Binding { return BindExpr(s.stateOf().v, `{"$":0}`, s) }

// tmplError is a template's parse error.
type tmplError string

func (e tmplError) Error() string { return string(e) }

// hole is a hole's value: the positional one its label names.
type hole struct {
	label string
	arg   int
}

// tparser parses a template's source.
type tparser struct {
	src    string
	file   string
	line   int
	i      int
	labels []string
	pre    int // inside pre
	t      *tmpl
	sh     *strings.Builder
}

// parseTemplate parses src, which starts on line of file, its site the
// line of place.
func parseTemplate(src, file, place string, line int) (t *tmpl, err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(tmplError)
			if !ok {
				panic(r)
			}
			t, err = nil, e
		}
	}()
	p := &tparser{src: src, file: file, line: line}
	site := siteOf(place, line)
	t = p.template(site, func() []tnode {
		ns := p.nodes("")
		if p.i < len(p.src) {
			p.fail("</%s> closes nothing", p.closeName())
		}
		return ns
	})
	t.labels = p.labels
	t.direct = len(t.slots) == len(t.labels)
	for i, s := range t.slots {
		t.direct = t.direct && s.kind != kPart && s.kind != kClass && s.op != nil && s.op.arg == i
	}
	return t, nil
}

// template parses a template with body, its holes the template's own.
func (p *tparser) template(site string, body func() []tnode) *tmpl {
	t := &tmpl{site: site}
	outerT, outerSh := p.t, p.sh
	var sh strings.Builder
	p.t, p.sh = t, &sh
	t.nodes = body()
	p.t, p.sh = outerT, outerSh
	t.shape = sh.String()
	t.count = counted(t.nodes)
	t.build = t.frame
	t.empty = make([]*list, len(t.slots))
	for i := range t.slots {
		t.sites = append(t.sites, site+"#"+strconv.Itoa(i))
		if t.slots[i].kind == kChild {
			t.empty[i] = &list{site: t.sites[i]}
		}
	}
	f := fnv.New64a()
	f.Write([]byte(p.src))
	f.Write([]byte(site[strings.LastIndexByte(site, ':')+1:]))
	t.fp = mix(f.Sum64() ^ 0x74656d706c617465)
	sample, _ := t.frame(make([]any, len(t.slots))).(*Frame)
	if t.plan = compilePlan(sample, t.shape); t.plan.statics == nil {
		err := Walk(nil, sample, &HTMLRenderer{w: discard{}})
		panic(tmplError(fmt.Sprintf("ui: template %s: %v", site, err)))
	}
	t.plan.sites = t.sites
	for i := range t.slots {
		if t.slots[i].kind == kSet {
			if t.plan.sets == nil {
				t.plan.sets = make([]bool, len(t.slots))
			}
			t.plan.sets[i] = true
		}
	}
	t.pass = len(t.nodes) == 1 && t.nodes[0].static == nil && t.nodes[0].tag == "" && len(t.slots) == 1 && t.slots[0].kind == kChild
	if len(t.slots) == 0 {
		t.static = &lazyFrame{fp: t.fp, shape: t.shape, built: sample, p: t.plan}
	}
	return t
}

func (p *tparser) fail(format string, args ...any) { p.failAt(p.i, format, args...) }

func (p *tparser) failAt(off int, format string, args ...any) {
	panic(tmplError(fmt.Sprintf("ui: template %s: %s", p.lineAt(off), fmt.Sprintf(format, args...))))
}

// lineAt is file:line of src's offset off.
func (p *tparser) lineAt(off int) string {
	return p.file + ":" + strconv.Itoa(p.line+strings.Count(p.src[:min(off, len(p.src))], "\n"))
}

func (p *tparser) at(s string) bool { return strings.HasPrefix(p.src[p.i:], s) }

func (p *tparser) slot(s tslot, off int) int {
	s.line = p.lineAt(off)
	p.t.slots = append(p.t.slots, s)
	return len(p.t.slots) - 1
}

// holeAt reports whether a hole starts at p.i: a { that isn't {{.
func (p *tparser) holeAt() bool {
	return p.i < len(p.src) && p.src[p.i] == '{' && !p.at("{{")
}

// label reads a hole at p.i, {label} or {label...}, and returns its label.
func (p *tparser) label() (label string, spread bool) {
	start := p.i
	end := goEnd(p.src, p.i+1, '}')
	if end < 0 {
		p.failAt(start, "a hole isn't closed: {%s", firstLine(p.src[start+1:]))
	}
	label = strings.TrimSpace(p.src[start+1 : end])
	p.i = end + 1
	if label == "" {
		p.failAt(start, "a hole needs a label: {name}")
	}
	if s, ok := strings.CutSuffix(label, "..."); ok {
		label, spread = strings.TrimSpace(s), true
	}
	return label, spread
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

// goEnd is the offset of the close byte ending the Go text at s[i:], past
// brackets and string and rune literals, or -1.
func goEnd(s string, i int, close byte) int {
	depth := 0
	for ; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\'', '`':
			j := i + 1
			for j < len(s) && s[j] != c {
				if s[j] == '\\' && c != '`' {
					j++
				}
				j++
			}
			if j >= len(s) {
				return -1
			}
			i = j
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				if c == close {
					return i
				}
				return -1
			}
			depth--
		}
	}
	return -1
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
func (p *tparser) nodes(parent string) []tnode {
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
			p.i += 4 + end + 3
		case p.at("<!"):
			end := strings.IndexByte(p.src[p.i:], '>')
			if end < 0 {
				p.fail("<! isn't closed")
			}
			p.i += end + 1
		case p.tagStart():
			out = append(out, p.element())
		default:
			out = p.text(out, parent)
		}
	}
	return out
}

// text parses text up to the next tag, splitting it at holes.
func (p *tparser) text(out []tnode, parent string) []tnode {
	var raw strings.Builder
	flush := func() {
		if t := p.textValue(raw.String()); t != "" {
			out = append(out, tnode{static: TextNode(t)})
			p.sh.WriteByte('t')
		}
		raw.Reset()
	}
	for p.i < len(p.src) && !p.tagStart() {
		switch {
		case p.at("{{"):
			raw.WriteByte('{')
			p.i += 2
		case p.at("{/*"):
			end := strings.Index(p.src[p.i:], "*/}")
			if end < 0 {
				p.fail("a comment {/* isn't closed: */}")
			}
			p.i += end + 3
		case p.holeAt():
			flush()
			at := p.i
			label, spread := p.label()
			if spread {
				p.failAt(at, "{%s...} is an attribute set: it goes where an attribute does", label)
			}
			where := "a child"
			if parent != "" {
				where = "a child of <" + parent + ">"
			}
			out = append(out, tnode{slot: p.slot(tslot{kind: kChild, op: p.hole(label, at), where: where}, at)})
			p.sh.WriteByte('h')
		default:
			raw.WriteByte(p.src[p.i])
			p.i++
		}
	}
	flush()
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

// rawAttr is an attribute as written, before its slot is placed.
type rawAttr struct {
	name   string
	value  any // static
	op     *hole
	parts  []tpart
	spread bool
	at     int
}

// attrs parses an element's attributes up to > or />, reporting whether the
// tag closed itself.
func (p *tparser) attrs(tag string, start int) (as []rawAttr, closed bool) {
	for {
		p.space()
		if p.i >= len(p.src) {
			p.failAt(start, "<%s isn't closed", tag)
		}
		if p.at(">") {
			p.i++
			return as, false
		}
		if p.at("/>") {
			p.i += 2
			return as, true
		}
		at := p.i
		if p.holeAt() {
			label, spread := p.label()
			if !spread {
				p.failAt(at, "a hole where an attribute goes is an attribute set: <%s {%s...}>", tag, label)
			}
			as = append(as, rawAttr{op: p.hole(label, at), spread: true, at: at})
			continue
		}
		as = append(as, p.attr(tag))
	}
}

// attr parses an attribute: name, name=value, name="…{x}…" or name={x}.
func (p *tparser) attr(tag string) rawAttr {
	at := p.i
	name := p.name()
	if !validName(name) {
		p.fail("<%s> has an attribute with no name, or a name with something other than letters, digits and - _ : .", tag)
	}
	if p.at("{") {
		p.fail("a hole can't name an attribute; a hole where an attribute goes sets attributes: <%s {attrs...}>", tag)
	}
	a := rawAttr{name: name, value: true, at: at}
	save := p.i
	p.space()
	if !p.at("=") {
		p.i = save
		return a
	}
	p.i++
	p.space()
	if p.holeAt() {
		vat := p.i
		label, spread := p.label()
		if spread {
			p.failAt(vat, "{%s...} is an attribute set: it goes where an attribute does", label)
		}
		a.op = p.hole(label, vat)
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
		if strings.Contains(p.src[p.i:j], "{") {
			p.fail("%s on <%s>: quote a value holes are part of", name, tag)
		}
		a.value = html.UnescapeString(p.src[p.i:j])
		p.i = j
		return a
	}
	p.i++
	var s strings.Builder
	text := func() {
		if s.Len() > 0 {
			a.parts = append(a.parts, tpart{s: html.UnescapeString(s.String())})
			s.Reset()
		}
	}
	for {
		if p.i >= len(p.src) {
			p.failAt(at, "%s on <%s>: the value's quote isn't closed", name, tag)
		}
		switch {
		case p.src[p.i] == q:
			p.i++
			text()
			switch {
			case len(a.parts) == 0:
				a.value = ""
			case len(a.parts) == 1 && a.parts[0].op == nil:
				a.value = a.parts[0].s
			case len(a.parts) == 1:
				a.op = a.parts[0].op
			default:
				return a
			}
			a.parts = nil
			return a
		case p.at("{{"):
			s.WriteByte('{')
			p.i += 2
		case p.holeAt():
			text()
			vat := p.i
			label, spread := p.label()
			if spread {
				p.failAt(vat, "{%s...} is an attribute set: it goes where an attribute does", label)
			}
			a.parts = append(a.parts, tpart{op: p.hole(label, vat)})
		default:
			s.WriteByte(p.src[p.i])
			p.i++
		}
	}
}

// element parses an element at p.i, its children and its closing tag.
func (p *tparser) element() tnode {
	start := p.i
	p.i++
	tag := p.name()
	if !validName(tag) {
		p.fail("a tag needs a name")
	}
	if c := tag[0]; 'A' <= c && c <= 'Z' || strings.HasPrefix(tag, "slot:") {
		p.failAt(start, "<%s> is a component's: in ui.HTML a component is a hole, {%s(…)}", tag, strings.TrimPrefix(tag, "slot:"))
	}
	n := tnode{tag: tag}
	raw, closed := p.attrs(tag, start)
	var flags strings.Builder
	for _, a := range p.placeAttrs(tag, raw) {
		n.attrs = append(n.attrs, a)
		flags.WriteByte("sd"[min(a.slot+1, 1)])
	}
	p.sh.WriteByte('E')
	p.sh.WriteString(flags.String())
	p.sh.WriteByte('(')
	defer p.sh.WriteByte(')')
	if closed || Void(tag) {
		return n
	}
	switch tag {
	case "script", "style", "textarea", "title":
		n.kids = p.raw(tag)
	default:
		if tag == "pre" {
			p.pre++
		}
		n.kids = p.nodes(tag)
		if tag == "pre" {
			p.pre--
		}
	}
	p.closeTag(tag, start)
	return n
}

// closeTag reads </tag>.
func (p *tparser) closeTag(tag string, start int) {
	if !p.at("</") {
		p.failAt(start, "<%s> isn't closed", tag)
	}
	if name := p.closeName(); name != tag {
		p.fail("</%s> closes <%s>", name, tag)
	}
	p.i += 2 + len(tag)
	p.space()
	if !p.at(">") {
		p.fail("</%s needs a >", tag)
	}
	p.i++
}

// placeAttrs gives an element's attributes their slots, in order: class
// and its class:name toggles become one class attribute where the first of
// them is.
func (p *tparser) placeAttrs(tag string, raw []rawAttr) []tattr {
	var out []tattr
	var toggles []ttoggle
	cls := -1
	for _, a := range raw {
		if name, ok := strings.CutPrefix(a.name, "class:"); ok {
			if a.op == nil {
				p.failAt(a.at, "%s on <%s> takes a hole: %s={on}", a.name, tag, a.name)
			}
			toggles = append(toggles, ttoggle{name, a.op})
		}
	}
	for _, a := range raw {
		if strings.HasPrefix(a.name, "class:") {
			if cls >= 0 {
				continue
			}
			a = rawAttr{name: "class", value: "", at: a.at}
			for _, b := range raw {
				if b.name == "class" || b.name == "className" {
					a = b
				}
			}
		} else if (a.name == "class" || a.name == "className") && toggles != nil {
			if cls >= 0 {
				continue
			}
		}
		lower := strings.ToLower(a.name)
		s := tslot{kind: kValue, op: a.op, where: a.name + " on <" + tag + ">",
			event: len(lower) > 2 && strings.HasPrefix(lower, "on"),
			class: lower == "class" || a.name == "className", styled: lower == "style"}
		switch {
		case a.spread:
			s.kind, s.where = kSet, "an attribute set of <"+tag+">"
		case s.class && toggles != nil:
			s.kind, s.toggles = kClass, toggles
			s.parts, s.op = a.parts, nil
			if a.op != nil {
				s.parts = []tpart{{op: a.op}}
			} else if v, ok := a.value.(string); ok && a.parts == nil && v != "" {
				s.parts = []tpart{{s: v}}
			}
			cls = len(out)
			out = append(out, tattr{name: "class", slot: p.slot(s, a.at)})
			continue
		case a.parts != nil:
			if s.event {
				p.failAt(a.at, "%s on <%s> takes a handler: give its whole value to one hole", a.name, tag)
			}
			s.kind, s.parts, s.where = kPart, a.parts, "part of "+s.where
		case a.op == nil:
			out = append(out, tattr{name: a.name, value: a.value, slot: -1})
			continue
		}
		out = append(out, tattr{name: a.name, slot: p.slot(s, a.at)})
	}
	return out
}

// raw parses a text-only element's content up to its closing tag: verbatim
// in script and style, entities decoded and holes text in textarea and
// title.
func (p *tparser) raw(tag string) []tnode {
	end := strings.Index(p.src[p.i:], "</"+tag)
	if end < 0 {
		p.fail("<%s> isn't closed", tag)
	}
	end += p.i
	if tag == "script" || tag == "style" {
		s := p.src[p.i:end]
		p.i = end
		if s == "" {
			return nil
		}
		p.sh.WriteByte('t')
		return []tnode{{static: RawHTML(s)}}
	}
	var out []tnode
	var raw strings.Builder
	flush := func() {
		s := raw.String()
		raw.Reset()
		if tag == "title" {
			s = jsxText(s)
		} else {
			s = html.UnescapeString(s)
		}
		if s != "" {
			out = append(out, tnode{static: TextNode(s)})
			p.sh.WriteByte('t')
		}
	}
	for p.i < end {
		switch {
		case p.at("{{"):
			raw.WriteByte('{')
			p.i += 2
		case p.holeAt():
			flush()
			at := p.i
			label, _ := p.label()
			out = append(out, tnode{slot: p.slot(tslot{kind: kText, op: p.hole(label, at), where: "text in <" + tag + ">"}, at)})
			p.sh.WriteByte('h')
		default:
			raw.WriteByte(p.src[p.i])
			p.i++
		}
	}
	p.i = end
	flush()
	return out
}

// operand is the value labelled label: its first appearance makes it the
// next positional one.
func (p *tparser) hole(label string, at int) *hole {
	if i := slices.Index(p.labels, label); i >= 0 {
		return &hole{label, i}
	}
	p.labels = append(p.labels, label)
	return &hole{label, len(p.labels) - 1}
}
