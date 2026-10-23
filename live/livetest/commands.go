package livetest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

// The client runs JS commands (the live protocol's "JS commands") on a
// model of the page: the server's HTML, parsed, plus what commands changed
// on its elements — classes, attributes, the style's display, values —
// kept across patches while the server renders those attributes as it did,
// as a browser runtime keeps them. An element is known by its place in the
// page. Pushes send events, settling the changes before them as their
// predictions (optimistic.go); calls run the Funcs a test registers;
// debounce and throttle run at once; focus, scrolling and transitions do
// nothing.

// override is an attribute a command set: its value (or removal), and the
// server's value when it did.
type override struct {
	server, val       string
	serverHas, valHas bool
}

func (c *Client) page() *node {
	if c.dom == nil || c.domOf != c.html {
		c.dom, c.domOf = parse(c.html), c.html
	}
	return c.dom
}

// get is an element's attribute as the page shows it.
func (c *Client) get(e *node, name string) (string, bool) {
	if o, ok := c.over[e.path][name]; ok {
		return o.val, o.valHas
	}
	if name == textAttr {
		return c.text(e), true
	}
	return e.attr(name)
}

// set changes an element's attribute, as a command does; has false removes it.
func (c *Client) set(e *node, name, val string, has bool) {
	if e.tag == "#root" {
		return
	}
	if c.over == nil {
		c.over = map[string]map[string]*override{}
	}
	m := c.over[e.path]
	if m == nil {
		m = map[string]*override{}
		c.over[e.path] = m
	}
	if c.acc != nil {
		ch := change{path: e.path, name: name, after: value{val, has}, server: serverValue(e, name)}
		ch.before.s, ch.before.has = c.get(e, name)
		*c.acc = append(*c.acc, ch)
	}
	o := m[name]
	if o == nil {
		o = &override{}
		o.server, o.serverHas = serverAttr(e, name)
		m[name] = o
	}
	o.val, o.valHas = val, has
}

// textAttr is the name commands' text changes are recorded under, as if
// the text were an attribute.
const textAttr = "#text"

// serverAttr is an element's attribute, or its text, as the server rendered it.
func serverAttr(e *node, name string) (string, bool) {
	if name == textAttr {
		return e.textContent(), true
	}
	return e.attr(name)
}

func serverValue(e *node, name string) value {
	v, has := serverAttr(e, name)
	return value{v, has}
}

// text is an element's text as the page shows it after commands.
func (c *Client) text(e *node) string {
	if e.tag == "" {
		return e.text
	}
	if o, ok := c.over[e.path][textAttr]; ok {
		return o.val
	}
	var b strings.Builder
	for _, k := range e.kids {
		b.WriteString(c.text(k))
	}
	return b.String()
}

// reconcile drops what commands changed where the server now renders the
// attribute differently from when they did, or the element is gone.
func (c *Client) reconcile() {
	if len(c.over) == 0 {
		return
	}
	at := map[string]*node{}
	c.page().walk(func(e *node) bool { at[e.path] = e; return true })
	for path, m := range c.over {
		e := at[path]
		for name, o := range m {
			if v, has := "", false; e != nil {
				v, has = serverAttr(e, name)
				if v == o.server && has == o.serverHas {
					continue
				}
			}
			delete(m, name)
		}
		if len(m) == 0 {
			delete(c.over, path)
		}
	}
}

// find is the first element of the page matching a CSS selector.
func (c *Client) find(sel string) *node {
	var found *node
	s := parseSelector(sel)
	c.page().walk(func(e *node) bool {
		if s.matches(e, c.get) {
			found = e
		}
		return found == nil
	})
	return found
}

// Exists reports whether an element matches a CSS selector.
func (c *Client) Exists(sel string) bool { return c.find(sel) != nil }

// Attr is an attribute of the first element matching sel, as the page
// shows it after commands.
func (c *Client) Attr(sel, name string) (string, bool) {
	if e := c.find(sel); e != nil {
		return c.get(e, name)
	}
	return "", false
}

// HasClass reports whether the first element matching sel has class.
func (c *Client) HasClass(sel, class string) bool {
	v, _ := c.Attr(sel, "class")
	return slices.Contains(strings.Fields(v), class)
}

// Visible reports whether the first element matching sel and its ancestors
// are shown: no hidden attribute, no display none in their style. Classes
// aren't interpreted.
func (c *Client) Visible(sel string) bool {
	e := c.find(sel)
	if e == nil {
		return false
	}
	for ; e != nil && e.tag != "#root"; e = e.parent {
		if _, h := c.get(e, "hidden"); h || display(c.style(e)) == "none" {
			return false
		}
	}
	return true
}

// Text is the text of the first element matching sel.
func (c *Client) Text(sel string) string {
	if e := c.find(sel); e != nil {
		return c.text(e)
	}
	return ""
}

func (c *Client) style(e *node) string { v, _ := c.get(e, "style"); return v }

func display(style string) string {
	for _, d := range strings.Split(style, ";") {
		if k, v, ok := strings.Cut(d, ":"); ok && strings.TrimSpace(k) == "display" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func withDisplay(style, v string) string {
	var out []string
	for _, d := range strings.Split(style, ";") {
		if k, _, _ := strings.Cut(d, ":"); strings.TrimSpace(d) != "" && strings.TrimSpace(k) != "display" {
			out = append(out, strings.TrimSpace(d))
		}
	}
	if v != "" {
		out = append(out, "display: "+v)
	}
	return strings.Join(out, "; ")
}

// Click fires a click on the first element matching sel.
func (c *Client) Click(sel string) error { return c.Fire(sel, ui.Event{Type: "click"}) }

// Fire fires the DOM event ev.Type on the first element matching sel, as a
// browser runtime handles it: the nearest element (it or an ancestor)
// carrying data-vk-js-<event> or data-vk-on-<event> runs its commands, or
// sends its handler's event. ev.Value defaults to that element's value (a
// checkbox's checked as "true"/"false"); Key and Form are sent as given.
// The replies to what it sends are applied.
func (c *Client) Fire(sel string, ev ui.Event) error {
	e := c.find(sel)
	if e == nil {
		return fmt.Errorf("livetest: no element %s", sel)
	}
	if err := c.fire(e, ev, true); err != nil {
		return err
	}
	return c.flush()
}

// A click on a ui.Link (or inside one) also follows it, unless the link
// itself takes the click.
func (c *Client) fire(e *node, ev ui.Event, bubbles bool) error {
	if (ev.Type == "click" || ev.Type == "submit") && c.inBusy(e) {
		return nil // ignored while busy
	}
	var link *node
	if ev.Type == "click" && bubbles {
		link = c.linkOf(e)
	}
	for ; e != nil; e = e.parent {
		if js, ok := c.get(e, "data-vk-js-"+ev.Type); ok {
			if e == link {
				link = nil
			}
			err := c.run(e, []byte(js), ev, nil)
			c.follow(link)
			return err
		}
		if h, ok := c.get(e, "data-vk-on-"+ev.Type); ok {
			ev.Target = h
			if ev.Value == "" {
				ev.Value = c.value(e)
			}
			c.queue = append(c.queue, sending{ev: ev})
			if e != link {
				c.follow(link)
			}
			return nil
		}
		if !bubbles {
			return nil
		}
	}
	c.follow(link)
	return nil
}

// flush sends the events commands pushed, applying each reply, unless
// the client holds them.
func (c *Client) flush() error {
	for len(c.queue) > 0 && !c.held {
		q := c.queue[0]
		c.queue = c.queue[1:]
		if q.nav != nil {
			if _, err := c.navigate(*q.nav); err != nil {
				return err
			}
			continue
		}
		m, err := c.send(live.ClientMessage{Type: live.EventMsg, Event: q.ev}, q.p)
		if err != nil {
			return err
		}
		if m.Type == live.Error {
			return fmt.Errorf("livetest: %s", m.Error)
		}
	}
	return nil
}

func (c *Client) value(e *node) string {
	if t, _ := c.get(e, "type"); t == "checkbox" || t == "radio" {
		_, on := c.get(e, "checked")
		if t == "checkbox" {
			if on {
				return "true"
			}
			return "false"
		}
	}
	if e.tag == "textarea" {
		if v, ok := c.get(e, "value"); ok {
			return v
		}
		return c.text(e)
	}
	v, _ := c.get(e, "value")
	return v
}

type op struct {
	name string
	args opArgs
}

type opArgs struct {
	To         json.RawMessage   `json:"to"`
	Names      []string          `json:"names"`
	Name       string            `json:"name"`
	Value      json.RawMessage   `json:"value"`
	Values     []string          `json:"values"`
	Display    string            `json:"display"`
	Event      string            `json:"event"`
	Bubbles    bool              `json:"bubbles"`
	Attr       string            `json:"attr"`
	H          string            `json:"h"`
	Keys       []string          `json:"keys"`
	Msg        string            `json:"msg"`
	Cond       *cond             `json:"cond"`
	Text       string            `json:"text"`
	Transition []string          `json:"transition"`
	OnOK       json.RawMessage   `json:"on_ok"`
	OnError    json.RawMessage   `json:"on_error"`
	Timeout    int64             `json:"timeout"`
	Keep       bool              `json:"keep"`
	Fn         string            `json:"fn"`
	Href       string            `json:"href"`
	Replace    bool              `json:"replace"`
	Args       []json.RawMessage `json:"args"`
}

type cond struct {
	To      json.RawMessage `json:"to"`
	Checked bool            `json:"checked"`
	Value   bool            `json:"value"`
	ValueEq *string         `json:"value_eq"`
	Attr    string          `json:"attr"`
	Is      string          `json:"is"`
	Exists  bool            `json:"exists"`
	Some    bool            `json:"some"`
	And     []*cond         `json:"and"`
	Or      []*cond         `json:"or"`
	Not     *cond           `json:"not"`
}

// joined reports whether k joins other conditions.
func (k *cond) joined() bool { return k.And != nil || k.Or != nil || k.Not != nil }

func parseOps(data []byte) ([]op, error) {
	var raw [][]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("livetest: commands %s: %w", data, err)
	}
	ops := make([]op, len(raw))
	for i, r := range raw {
		if len(r) != 2 || json.Unmarshal(r[0], &ops[i].name) != nil || json.Unmarshal(r[1], &ops[i].args) != nil {
			return nil, fmt.Errorf("livetest: bad command %s", r)
		}
	}
	return ops, nil
}

// targets is the elements a command's to names, from this, in page order.
func (c *Client) targets(this *node, to json.RawMessage) ([]*node, error) {
	if len(to) == 0 {
		return []*node{this}, nil
	}
	var t struct {
		Ref, Sel       oneOrMore
		Closest, Inner *string
		Attr           *attrTarget
	}
	if err := json.Unmarshal(to, &t); err != nil {
		return nil, fmt.Errorf("livetest: target %s: %w", to, err)
	}
	var out []*node
	collect := func(under *node, keep func(*node) bool) {
		if under.tag != "#root" && keep(under) {
			out = append(out, under)
		}
		under.walk(func(e *node) bool {
			if keep(e) {
				out = append(out, e)
			}
			return true
		})
	}
	switch {
	case t.Ref != nil:
		scope := instanceOf(this)
		collect(scope, func(e *node) bool {
			r, ok := c.get(e, "data-vk-ref")
			return ok && slices.Contains(t.Ref, r) && instanceOf(e) == scope
		})
	case t.Attr != nil:
		scope := instanceOf(this)
		if t.Attr.Page {
			scope = c.page()
		}
		collect(scope, func(e *node) bool {
			return t.Attr.matches(c, e) && (t.Attr.Page || instanceOf(e) == scope)
		})
	case t.Sel != nil:
		var sels []selector
		for _, s := range t.Sel {
			sels = append(sels, parseSelector(s))
		}
		collect(c.page(), func(e *node) bool {
			return slices.ContainsFunc(sels, func(s selector) bool { return s.matches(e, c.get) })
		})
	case t.Closest != nil:
		s := parseSelector(*t.Closest)
		for e := this; e != nil; e = e.parent {
			if s.matches(e, c.get) {
				return []*node{e}, nil
			}
		}
	case t.Inner != nil:
		s := parseSelector(*t.Inner)
		this.walk(func(e *node) bool {
			if s.matches(e, c.get) {
				out = append(out, e)
			}
			return true
		})
	}
	return out, nil
}

// oneOrMore is a target's ref or sel: one string, or a list of them.
type oneOrMore []string

func (o *oneOrMore) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*o = oneOrMore{one}
		return nil
	}
	var more []string
	if err := json.Unmarshal(b, &more); err != nil {
		return err
	}
	*o = append(oneOrMore{}, more...)
	return nil
}

// attrTarget is an attr target: the attribute, the values it may have,
// and whether it looks at the whole page.
type attrTarget struct {
	Name string
	Is   []json.RawMessage
	Page bool
}

// matches reports whether e has the attribute with one of the values.
func (a *attrTarget) matches(c *Client, e *node) bool {
	v, ok := c.get(e, a.Name)
	if !ok || e.tag == "#root" {
		return false
	}
	if len(a.Is) == 0 {
		return true
	}
	for _, raw := range a.Is {
		var exact string
		if json.Unmarshal(raw, &exact) == nil {
			if v == exact {
				return true
			}
			continue
		}
		var m map[string]string
		json.Unmarshal(raw, &m)
		for kind, s := range m {
			switch kind {
			case "prefix":
				ok = s != "" && strings.HasPrefix(v, s)
			case "suffix":
				ok = s != "" && strings.HasSuffix(v, s)
			case "contains":
				ok = s != "" && strings.Contains(v, s)
			case "word":
				ok = slices.Contains(strings.Fields(v), s)
			default:
				ok = false
			}
			if ok {
				return true
			}
		}
	}
	return false
}

// instanceOf is the nearest element, e or an ancestor, carrying data-vk-id.
func instanceOf(e *node) *node {
	for ; e.parent != nil; e = e.parent {
		if _, ok := e.attr("data-vk-id"); ok {
			return e
		}
	}
	return e
}

// holds tests a condition: a joined one by its parts; a read by every
// element its target finds, or with some by at least one. A read of no
// element fails, except exists.
func (c *Client) holds(this *node, k *cond) (bool, error) {
	if k == nil {
		return false, fmt.Errorf("livetest: no condition")
	}
	switch {
	case k.Not != nil:
		ok, err := c.holds(this, k.Not)
		return !ok, err
	case k.And != nil || k.Or != nil:
		all, parts := k.And != nil, k.And
		if !all {
			parts = k.Or
		}
		for _, p := range parts {
			ok, err := c.holds(this, p)
			if err != nil || ok != all {
				return ok, err
			}
		}
		return all, nil
	}
	ts, err := c.targets(this, k.To)
	if err != nil {
		return false, err
	}
	if k.Exists || len(ts) == 0 {
		return len(ts) > 0 && k.Exists, nil
	}
	for _, e := range ts {
		ok, err := c.read(e, k)
		if err != nil || ok == k.Some {
			return ok, err
		}
	}
	return !k.Some, nil
}

// read tests a read on one element.
func (c *Client) read(e *node, k *cond) (bool, error) {
	switch {
	case k.Checked:
		_, on := c.get(e, "checked")
		return on, nil
	case k.ValueEq != nil:
		return c.value(e) == *k.ValueEq, nil
	case k.Value:
		return c.value(e) != "", nil
	case k.Attr != "":
		_, ok := c.get(e, k.Attr)
		return ok, nil
	case k.Is != "":
		return parseSelector(k.Is).matches(e, c.get), nil
	}
	return false, fmt.Errorf("livetest: bad condition")
}

// run runs commands with this as This, for the event ev, recording their
// changes in acc, the segment an exec runs in, or in a segment of their own.
func (c *Client) run(this *node, data []byte, ev ui.Event, acc *[]change) error {
	ops, err := parseOps(data)
	if err != nil {
		return err
	}
	if acc == nil {
		acc = new([]change)
	}
	defer func(outer *[]change) { c.acc = outer }(c.acc)
	c.acc = acc
	type level struct{ taken, active, outer bool }
	var levels []level
	var waiting []*busyState // Busy marks for the next push
	var last *pending        // the chain's last push sent
	running := func() bool { return len(levels) == 0 || levels[len(levels)-1].active }
	for i, o := range ops {
		a := &o.args
		switch o.name {
		case "if":
			l := level{taken: true, outer: running()}
			if l.outer {
				ok, err := c.holds(this, a.Cond)
				if err != nil {
					return err
				}
				l.taken, l.active = ok, ok
			}
			levels = append(levels, l)
			continue
		case "else_if", "else", "end":
			if len(levels) == 0 {
				return fmt.Errorf("livetest: %s without an if", o.name)
			}
			l := &levels[len(levels)-1]
			switch o.name {
			case "end":
				levels = levels[:len(levels)-1]
			case "else":
				l.active, l.taken = l.outer && !l.taken, true
			default:
				l.active = false
				if l.outer && !l.taken {
					ok, err := c.holds(this, a.Cond)
					if err != nil {
						return err
					}
					l.taken, l.active = ok, ok
				}
			}
			continue
		}
		if !running() {
			if o.name == "push" {
				*acc = nil // a push that doesn't run predicts nothing
			}
			continue
		}
		switch o.name {
		case "key":
			if !slices.Contains(a.Keys, ev.Key) {
				return nil
			}
			continue
		case "confirm":
			if c.Confirm != nil && !c.Confirm(a.Msg) {
				return nil
			}
			continue
		case "copy_text":
			c.Clipboard = a.Text
			continue
		case "push":
			seg := *acc
			*acc = nil
			if a.H == "" {
				continue
			}
			p := ev
			p.Target = a.H
			if len(a.Value) > 0 {
				var s string
				if json.Unmarshal(a.Value, &s) != nil {
					s = string(a.Value)
				}
				p.Value = s
			} else if p.Value == "" {
				p.Value = c.value(this)
			}
			pd := c.predict(this, a, seg)
			pd.busy, waiting, last = waiting, nil, pd
			c.queue = append(c.queue, sending{ev: p, p: pd})
			continue
		case "busy":
			ts, err := c.targets(this, a.To)
			if err != nil {
				return err
			}
			later := slices.ContainsFunc(ops[i+1:], func(o op) bool { return o.name == "push" && o.args.H != "" })
			for _, t := range ts {
				b := c.makeBusy(t, a.Timeout)
				switch {
				case b == nil:
				case later || last == nil:
					waiting = append(waiting, b)
				default:
					last.busy = append(last.busy, b)
				}
			}
			continue
		case "call":
			ok, err := c.call(this, a, ev)
			if err != nil {
				c.Errors = append(c.Errors, err.Error())
				if err := c.fire(this, ui.Event{Type: "vk:error"}, true); err != nil {
					return err
				}
				return c.failLocally(this, ops[i+1:], acc, waiting)
			}
			if !ok {
				return nil
			}
			continue
		case "pop_focus", "debounce", "throttle":
			continue
		case "patch", "navigate":
			if ui.LoadableURL(a.Href) { // the runtime refuses any other
				c.queue = append(c.queue, sending{nav: &navigation{href: a.Href, patch: o.name == "patch", replace: a.Replace}})
			}
			continue
		}
		ts, err := c.targets(this, a.To)
		if err != nil {
			return err
		}
		for _, t := range ts {
			if err := c.apply(t, o, ev); err != nil {
				return err
			}
		}
	}
	return nil
}

// apply runs one command on one element.
func (c *Client) apply(e *node, o op, ev ui.Event) error {
	a := &o.args
	classes := func(f func(have []string, n string) []string) {
		v, _ := c.get(e, "class")
		have := strings.Fields(v)
		for _, n := range a.Names {
			have = f(have, n)
		}
		c.set(e, "class", strings.Join(have, " "), true)
	}
	without := func(have []string, n string) []string {
		return slices.DeleteFunc(have, func(s string) bool { return s == n })
	}
	show := func() {
		c.set(e, "style", withDisplay(c.style(e), a.Display), true)
		if _, ok := c.get(e, "hidden"); ok {
			c.set(e, "hidden", "", false)
		}
	}
	hide := func() { c.set(e, "style", withDisplay(c.style(e), "none"), true) }
	switch o.name {
	case "add_class":
		classes(func(h []string, n string) []string {
			if slices.Contains(h, n) {
				return h
			}
			return append(h, n)
		})
	case "remove_class":
		classes(without)
	case "toggle_class":
		classes(func(h []string, n string) []string {
			if slices.Contains(h, n) {
				return without(h, n)
			}
			return append(h, n)
		})
	case "set_attr":
		var v string
		json.Unmarshal(a.Value, &v)
		if attrRefused(a.Name, v) {
			return nil
		}
		c.set(e, a.Name, v, true)
	case "remove_attr":
		if attrRefused(a.Name, "") {
			return nil
		}
		c.set(e, a.Name, "", false)
	case "toggle_attr":
		if len(a.Values) == 2 && !attrRefused(a.Name, a.Values[0]) && !attrRefused(a.Name, a.Values[1]) {
			v, _ := c.get(e, a.Name)
			if v == a.Values[0] {
				c.set(e, a.Name, a.Values[1], true)
			} else {
				c.set(e, a.Name, a.Values[0], true)
			}
		}
	case "show":
		show()
	case "hide":
		hide()
	case "toggle":
		if _, h := c.get(e, "hidden"); h || display(c.style(e)) == "none" {
			show()
		} else {
			hide()
		}
	case "set_value":
		var v string
		json.Unmarshal(a.Value, &v)
		if t, _ := c.get(e, "type"); t == "checkbox" || t == "radio" {
			c.set(e, "checked", "", v == "true")
		} else {
			c.set(e, "value", v, true)
		}
		for _, typ := range []string{"input", "change"} {
			if err := c.fire(e, ui.Event{Type: typ}, true); err != nil {
				return err
			}
		}
	case "copy":
		if e.tag == "input" || e.tag == "textarea" || e.tag == "select" {
			c.Clipboard = c.value(e)
		} else {
			c.Clipboard = c.text(e)
		}
	case "dispatch":
		return c.fire(e, ui.Event{Type: a.Event}, a.Bubbles)
	case "exec":
		if js, ok := c.get(e, a.Attr); ok {
			return c.run(e, []byte(js), ev, c.acc)
		}
	case "set_text":
		c.set(e, textAttr, a.Text, true)
	}
	return nil
}

// runPushed runs the commands a server message carries.
func (c *Client) runPushed(js []live.JSPush) error {
	for _, p := range js {
		var root *node
		if p.ID == "c0" {
			root = c.page()
		} else {
			c.page().walk(func(e *node) bool {
				if id, ok := e.attr("data-vk-id"); ok && id == p.ID {
					root = e
				}
				return root == nil
			})
		}
		if root == nil {
			continue
		}
		if err := c.run(root, p.Ops, ui.Event{}, nil); err != nil {
			return err
		}
	}
	return nil
}

// busyState is what one Busy marked on an element and inside it, to put
// back when it is undone.
type busyState struct {
	path     string
	changes  []change
	deadline time.Duration // 0: none
	done     bool
}

// makeBusy marks e busy, as a browser runtime does: aria-busy on it,
// readonly on its fields, aria-disabled and tabindex -1 on its buttons and
// links. Nil when e is busy already.
func (c *Client) makeBusy(e *node, timeout int64) *busyState {
	if e.tag == "#root" || slices.ContainsFunc(c.busy, func(b *busyState) bool { return b.path == e.path }) {
		return nil
	}
	b := &busyState{path: e.path}
	if timeout > 0 {
		b.deadline = c.now + time.Duration(timeout)*time.Millisecond
	}
	defer func(outer *[]change) { c.acc = outer }(c.acc)
	c.acc = &b.changes
	ensure := func(e *node, name, v string) {
		if have, ok := c.get(e, name); !ok || have != v && name != "readonly" {
			c.set(e, name, v, true)
		}
	}
	ensure(e, "aria-busy", "true")
	mark := func(x *node) {
		typ, _ := c.get(x, "type")
		_, href := c.get(x, "href")
		switch {
		case x.tag == "button" || x.tag == "a" && href || x.tag == "input" && slices.Contains([]string{"submit", "button", "reset", "image"}, typ):
			ensure(x, "aria-disabled", "true")
			ensure(x, "tabindex", "-1")
		case x.tag == "input" || x.tag == "textarea" || x.tag == "select":
			ensure(x, "readonly", "")
		}
	}
	mark(e)
	e.walk(func(x *node) bool { mark(x); return true })
	c.busy = append(c.busy, b)
	return b
}

// unbusy puts back what a Busy changed, where the server hasn't rendered
// another value since.
func (c *Client) unbusy(b *busyState) {
	if b.done {
		return
	}
	b.done = true
	c.revert(b.changes)
	c.busy = slices.DeleteFunc(c.busy, func(x *busyState) bool { return x == b })
}

// inBusy reports whether e or an ancestor is marked busy.
func (c *Client) inBusy(e *node) bool {
	for ; e != nil; e = e.parent {
		if slices.ContainsFunc(c.busy, func(b *busyState) bool { return b.path == e.path }) {
			return true
		}
	}
	return false
}

// Busy reports whether the first element matching sel is marked busy by a
// Busy command.
func (c *Client) Busy(sel string) bool {
	e := c.find(sel)
	return e != nil && slices.ContainsFunc(c.busy, func(b *busyState) bool { return b.path == e.path })
}

// PageShow is the browser showing the page again from its back/forward
// cache: every Busy mark is undone.
func (c *Client) PageShow() {
	for _, b := range slices.Clone(c.busy) {
		c.unbusy(b)
	}
}

// attrRefused reports whether the runtime refuses a command changing
// attribute name to value: an event handler attribute (on…) or srcdoc, or
// a URL attribute set to a javascript:, data: or vbscript: URL.
func attrRefused(name, value string) bool {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, "on") || n == "srcdoc" {
		return true
	}
	switch n {
	case "href", "src", "action", "formaction", "xlink:href", "poster", "data":
		v := strings.ToLower(strings.Map(func(r rune) rune {
			if r <= ' ' {
				return -1
			}
			return r
		}, value))
		return strings.HasPrefix(v, "javascript:") || strings.HasPrefix(v, "data:") || strings.HasPrefix(v, "vbscript:")
	}
	return false
}
