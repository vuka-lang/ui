package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
)

// Commands is a chain of JS commands: changes the browser makes at once when
// an event fires, such as showing a menu, toggling a class or moving focus,
// with no round trip. A Push step also calls a handler on the server.
//
// A chain is the value of an on… attribute. Start from JS and add steps;
// each method returns a new chain and leaves the old one as it was, so a
// shared start is safe to build on. On(target) gives a chain whose commands
// all act on one target.
//
// A chain renders the same with or without a live session; without one,
// its Push steps do nothing.
//
// Example:
//
//	<button onClick={ui.JS.RemoveClass(ui.Ref("menu"), "hidden").Push(c.MenuOpened)}>Open</button>
//	<ul ref="menu" class="hidden">…</ul>
type Commands struct{ ops []jsOp }

// JS is the empty chain: every chain of commands starts here.
//
// Example:
//
//	<button onClick={ui.JS.Toggle(ui.Ref("menu"))}>Menu</button>
var JS Commands

// JSChain is either kind of chain, Commands or CommandsOn: what an on…
// attribute, OnError and OnSuccess take.
type JSChain interface{ jsChain() Commands }

func (c Commands) jsChain() Commands { return c }

type jsOp struct {
	name string
	args []jsArg
	when int // on the end closing a When: 1, or 2 once it has its Else
}

type jsArg struct {
	key string
	val any
}

// push is a Push step's handler and options, resolved when the chain
// renders; an Options step's too (options set).
type push struct {
	fn      any
	options bool
	args    callArgs // an Options step's arguments
	value   any
	has     bool
	onOK    *Commands
	onErr   *Commands
	timeout time.Duration
	keep    bool
}

// callArgs is a Call step's arguments, element reads among them.
type callArgs []any

func (a callArgs) MarshalJSON() ([]byte, error) {
	out := make([]any, len(a))
	for i, v := range a {
		out[i] = v
		if r, ok := v.(Conditional); ok {
			out[i] = map[string]Condition{"$read": r.cond()}
		}
	}
	return marshal(out)
}

func (c Commands) add(name string, args ...jsArg) Commands {
	return Commands{append(slices.Clip(c.ops), jsOp{name: name, args: args})}
}

func to(t Target) jsArg { return jsArg{"to", t} }

// Target is the element or elements a command acts on:
//
//	ui.This                  the element the attribute is on (the default)
//	ui.Ref("menu")           the element with ref="menu" in the same component
//	ui.Ref("user", "pass")   the elements with either ref, in page order
//	ui.GetAttr("name", "q")  the component's elements whose name attribute is "q"
//	ui.Closest(".card")      the nearest of This and its ancestors matching a selector
//	ui.Inner(".item")        the elements inside This matching a selector
//	ui.Sel("#cart")          every element of the page matching a selector
//
// A plain string is a selector too: ui.JS.Hide("#cart") hides #cart. A
// target of several parts matches every element any part matches, in page
// order, each once.
//
// Example:
//
//	<button onClick={ui.JS.Hide(ui.Closest("li"))}>Dismiss</button>
type Target string

// Targets other than selectors are strings no selector can be: a NUL, then
// their kind.
const (
	refTarget     = "\x00ref:"
	selsTarget    = "\x00sels:"
	attrTarget    = "\x00attr:"
	closestTarget = "\x00closest:"
	innerTarget   = "\x00inner:"
)

// This is the element the on… attribute is on, and a command's target when
// it names none. In a chain the server pushes with PushJS, it is the
// component's root element.
//
// Example:
//
//	<button onClick={ui.JS.ToggleClass(ui.This, "active")}>Bold</button>
const This Target = ""

// Ref is the element marked ref="name" in the same component instance, or
// with several names, every element marked with one of them, in page order.
// Each instance has its own refs, so ten Dropdowns each open their own
// menu; the compiler checks that the component's JSX has each ref. Focus,
// ScrollTo and Copy act on the first element a target finds.
//
// Example:
//
//	<button onClick={ui.JS.Toggle(ui.Ref("menu"))}>Menu</button>
//	<ul ref="menu" style="display: none">…</ul>
//	<button onClick={ui.JS.SetAttr(ui.Ref("username", "password"), "readonly", "")}>Lock</button>
//	<input ref="username" /><input ref="password" type="password" />
func Ref(names ...string) Target {
	if len(names) == 0 {
		panic("ui: Ref needs a name")
	}
	for _, n := range names {
		if strings.ContainsRune(n, 0) {
			panic(fmt.Sprintf("ui: Ref %q: a name holds no NUL", n))
		}
	}
	return Target(refTarget + strings.Join(names, "\x00"))
}

// Closest is the nearest element matching a CSS selector, looking at This
// first and then up its ancestors.
//
// Example:
//
//	<li class="todo">{todo.Title} <button onClick={ui.JS.Hide(ui.Closest("li"))}>Dismiss</button></li>
func Closest(sel string) Target { return Target(closestTarget + sel) }

// Inner is every element inside This matching a CSS selector.
//
// Example:
//
//	<li onClick={ui.JS.Show(ui.Inner(".actions"))}>{post.Title} <span class="actions" style="display: none">Edit</span></li>
func Inner(sel string) Target { return Target(innerTarget + sel) }

// Sel is every element of the page matching a CSS selector, or any of
// several, in page order; a plain string is the same target. Inside a
// component prefer Ref or GetAttr, which can't reach into another
// component.
//
// Example:
//
//	<button onClick={ui.JS.Show(ui.Sel("#cart"))}>Cart</button>
//	<button onClick={ui.JS.Hide(ui.Sel("#cart", "#wishlist"))}>Close</button>
func Sel(css ...string) Target {
	switch {
	case len(css) == 0:
		panic("ui: Sel needs a selector")
	case len(css) == 1:
		return Target(css[0])
	}
	for _, c := range css {
		if strings.ContainsRune(c, 0) {
			panic(fmt.Sprintf("ui: Sel %q: a selector holds no NUL", c))
		}
	}
	return Target(selsTarget + strings.Join(css, "\x00"))
}

// attrSpec is a GetAttr target: the attribute, the values it may have
// (strings, or matchers as {"prefix": s}…), and whether it looks at the
// whole page.
type attrSpec struct {
	Name string `json:"name"`
	Is   []any  `json:"is,omitempty"`
	Page bool   `json:"page,omitempty"`
}

// Matchers are strings no attribute value is: a NUL, then their kind.
const matcherMark = "\x00match:"

func matcher(kind, s string) string {
	if s == "" {
		panic("ui: " + kind + "(\"\") matches no value; GetAttr(name) with no values matches the attribute's presence")
	}
	return matcherMark + kind + ":" + s
}

// GetAttr is the elements of the component instance that have the
// attribute name: any value with no values given, else one of values.
// A plain value is an exact match; Prefix, Suffix, Contains and Word match
// part of one. Page() widens it to the whole page. GetAttr picks elements;
// a target's Attr reads one.
//
// When the component's JSX writes the attribute literally, the compiler
// checks each literal value against those it has. Panics when name is
// empty or not an attribute name.
//
// Example:
//
//	<button onClick={ui.JS.SetAttr(ui.GetAttr("name", "username", "password"), "readonly", "")}>Lock</button>
//	<input name="username" /><input name="password" type="password" />
//	<button onClick={ui.JS.Hide(ui.GetAttr("data-step"))}>Skip</button>
func GetAttr(name string, values ...string) Target {
	if name == "" || strings.ContainsAny(name, " \t\n\f\r\"'>/=\x00") {
		panic(fmt.Sprintf("ui: GetAttr %q: not an attribute name", name))
	}
	spec := attrSpec{Name: name}
	for _, v := range values {
		var is any = v
		if rest, ok := strings.CutPrefix(v, matcherMark); ok {
			kind, s, _ := strings.Cut(rest, ":")
			is = map[string]string{kind: s}
		} else if strings.ContainsRune(v, 0) {
			panic(fmt.Sprintf("ui: GetAttr %q: a value holds no NUL", v))
		}
		spec.Is = append(spec.Is, is)
	}
	return spec.target()
}

func (s attrSpec) target() Target {
	j, err := marshal(s)
	if err != nil {
		panic(err)
	}
	return Target(attrTarget + string(j))
}

// Prefix is a GetAttr value matching any value that starts with s.
//
// Example:
//
//	<button onClick={ui.JS.Hide(ui.GetAttr("id", ui.Prefix("step-")))}>Hide steps</button>
func Prefix(s string) string { return matcher("prefix", s) }

// Suffix is a GetAttr value matching any value that ends with s.
//
// Example:
//
//	<button onClick={ui.JS.Show(ui.GetAttr("href", ui.Suffix(".pdf")))}>Show PDFs</button>
func Suffix(s string) string { return matcher("suffix", s) }

// Contains is a GetAttr value matching any value that holds s.
//
// Example:
//
//	<button onClick={ui.JS.AddClass(ui.GetAttr("title", ui.Contains("draft")), "italic")}>Mark drafts</button>
func Contains(s string) string { return matcher("contains", s) }

// Word is a GetAttr value matching any space-separated list of words that
// has s among them, as class lists are.
//
// Example:
//
//	<button onClick={ui.JS.Hide(ui.GetAttr("data-tags", ui.Word("old")))}>Hide old</button>
func Word(s string) string { return matcher("word", s) }

// Page widens a GetAttr target from the component instance to the whole
// page. Panics on any other target.
//
// Example:
//
//	<button onClick={ui.JS.Hide(ui.GetAttr("role", "alert").Page())}>Dismiss all</button>
func (t Target) Page() Target {
	j, ok := strings.CutPrefix(string(t), attrTarget)
	if !ok {
		panic("ui: Page widens a GetAttr target, not " + t.String())
	}
	var s attrSpec
	if err := json.Unmarshal([]byte(j), &s); err != nil {
		panic(err)
	}
	s.Page = true
	return s.target()
}

// String is the target as its JSON shows it.
func (t Target) String() string {
	j, err := t.MarshalJSON()
	if err != nil {
		return string(t)
	}
	return string(j)
}

func (t Target) MarshalJSON() ([]byte, error) {
	s := string(t)
	if j, ok := strings.CutPrefix(s, attrTarget); ok {
		return []byte(`{"attr":` + j + `}`), nil
	}
	for _, k := range [...]struct{ prefix, key string }{{refTarget, "ref"}, {selsTarget, "sel"}, {closestTarget, "closest"}, {innerTarget, "inner"}} {
		v, ok := strings.CutPrefix(s, k.prefix)
		if !ok {
			continue
		}
		if parts := strings.Split(v, "\x00"); len(parts) > 1 || k.prefix == selsTarget {
			return marshal(map[string][]string{k.key: parts})
		}
		return marshal(map[string]string{k.key: v})
	}
	return marshal(map[string]string{"sel": s})
}

// Condition is a test for If, ElseIf and When, made from a target:
// t.Checked(), t.Value(), t.Value().Eq(v), t.Attr(name), t.Is(css) or
// t.Exists(), or conditions joined with And, Or, Not, All and Any. It is
// read in the browser when it is reached, not when the chain is built.
//
// A read of a target that finds several elements holds when every one of
// them passes it, or with Some when at least one does; a target that finds
// none fails every read but Exists.
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.If(ui.Ref("all").Checked()).Show(ui.Ref("bulk")).Else().Hide(ui.Ref("bulk")).End()} />
type Condition struct {
	to   Target
	key  string
	val  any
	some bool
	op   string // "and", "or" or "not" for a joined condition
	of   []Condition
}

// ValueCondition is t.Value(): it holds when the target's value isn't
// empty, and Eq compares it. As an argument of Call it is the value itself.
type ValueCondition struct{ c Condition }

// Checked holds when the target is a checked checkbox or radio button. As
// an argument of Call it is true or false.
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.If(ui.Ref("all").Checked()).Show(ui.Ref("bulk")).Else().Hide(ui.Ref("bulk")).End()} />
func (t Target) Checked() Condition { return Condition{to: t, key: "checked", val: true} }

// Value holds when the target's value isn't empty; .Eq(v) compares it. As
// an argument of Call it is the value itself, read when the step runs.
//
// Example:
//
//	<input ref="title" onInput={ui.JS.If(ui.Ref("title").Value()).RemoveAttr(ui.Ref("save"), "disabled").Else().SetAttr(ui.Ref("save"), "disabled", "").End()} />
func (t Target) Value() ValueCondition {
	return ValueCondition{Condition{to: t, key: "value", val: true}}
}

func (c ValueCondition) MarshalJSON() ([]byte, error) { return c.c.MarshalJSON() }

// Eq holds when the target's value is exactly v.
//
// Example:
//
//	<select ref="kind" onChange={ui.JS.If(ui.Ref("kind").Value().Eq("other")).Show(ui.Ref("other")).Else().Hide(ui.Ref("other")).End()}>…</select>
func (c ValueCondition) Eq(v string) Condition {
	return Condition{to: c.c.to, key: "value_eq", val: v, some: c.c.some}
}

// Attr holds when the target has the attribute name, whatever its value. As
// an argument of Call it is the attribute's value, or null.
//
// Example:
//
//	<button onClick={ui.JS.If(ui.Ref("menu").Attr("hidden")).Show(ui.Ref("menu")).Else().Hide(ui.Ref("menu")).End()}>Menu</button>
func (t Target) Attr(name string) Condition { return Condition{to: t, key: "attr", val: name} }

// Is holds when the target matches a CSS selector.
//
// Example:
//
//	<button onClick={ui.JS.If(ui.This.Is(".active")).Push(c.Unfollow).Else().Push(c.Follow).End()}>Follow</button>
func (t Target) Is(css string) Condition { return Condition{to: t, key: "is", val: css} }

// Exists holds when the target finds an element at all.
//
// Example:
//
//	<button onClick={ui.JS.If(ui.Sel("#cart").Exists()).Show("#cart").Else().Navigate("/cart").End()}>Cart</button>
func (t Target) Exists() Condition { return Condition{to: t, key: "exists", val: true} }

// Conditional is what If, ElseIf and When test and what Call reads from an
// element: a Condition or a ValueCondition.
type Conditional interface{ cond() Condition }

func (c Condition) cond() Condition      { return c }
func (c ValueCondition) cond() Condition { return c.c }

// Some makes a read hold when at least one of the elements its target
// finds passes it, rather than every one. Panics on a joined condition.
//
// Example:
//
//	<button onClick={ui.JS.If(ui.Inner("input").Value().Some()).Show(ui.Ref("clear")).End()}>Check</button>
func (c Condition) Some() Condition {
	if c.op != "" {
		panic("ui: Some applies to a read of a target, not to a joined condition")
	}
	c.some = true
	return c
}

// Some holds when at least one of the elements the target finds has a
// value, rather than every one; .Eq still compares.
//
// Example:
//
//	<form onInput={ui.JS.If(ui.GetAttr("name", "q", "tag").Value().Some()).Show(ui.Ref("clear")).Else().Hide(ui.Ref("clear")).End()}>…</form>
func (c ValueCondition) Some() ValueCondition {
	c.c.some = true
	return c
}

// join is a condition of op over conds, flattening those of the same op,
// so a.And(b).And(c) is one list.
func join(op string, conds ...Conditional) Condition {
	j := Condition{op: op}
	for _, c := range conds {
		k := c.cond()
		if k.op == op {
			j.of = append(j.of, k.of...)
		} else {
			j.of = append(j.of, k)
		}
	}
	return j
}

// And holds when c and every one of more hold. Joining nests as written:
// a.And(b).Or(d) holds when a and b hold, or d does.
//
// Example:
//
//	<input ref="email" onInput={ui.JS.If(ui.Ref("email").Value().And(ui.Ref("terms").Checked())).RemoveAttr(ui.Ref("send"), "disabled").End()} />
func (c Condition) And(more ...Conditional) Condition {
	return join("and", append([]Conditional{c}, more...)...)
}

// Or holds when c or one of more holds.
//
// Example:
//
//	<select ref="plan" onChange={ui.JS.If(ui.Ref("plan").Value().Eq("team").Or(ui.Ref("plan").Value().Eq("business"))).Show(ui.Ref("seats")).End()}>…</select>
func (c Condition) Or(more ...Conditional) Condition {
	return join("or", append([]Conditional{c}, more...)...)
}

// Not holds when c doesn't.
//
// Example:
//
//	<button onClick={ui.JS.If(ui.Ref("menu").Attr("hidden").Not()).Hide(ui.Ref("menu")).End()}>Close</button>
func (c Condition) Not() Condition { return Condition{op: "not", of: []Condition{c}} }

// And holds when the target has a value and every one of more holds.
//
// Example:
//
//	<input ref="email" onInput={ui.JS.When(ui.Ref("email").Value().And(ui.Ref("terms").Checked()), ui.JS.RemoveAttr(ui.Ref("send"), "disabled"))} />
func (c ValueCondition) And(more ...Conditional) Condition { return c.c.And(more...) }

// Or holds when the target has a value or one of more holds.
//
// Example:
//
//	<input ref="phone" onInput={ui.JS.When(ui.Ref("phone").Value().Or(ui.Ref("email").Value()), ui.JS.Hide(ui.Ref("contact-error")))} />
func (c ValueCondition) Or(more ...Conditional) Condition { return c.c.Or(more...) }

// Not holds when the target's value is empty.
//
// Example:
//
//	<input ref="title" onInput={ui.JS.When(ui.Ref("title").Value().Not(), ui.JS.SetAttr(ui.Ref("save"), "disabled", ""))} />
func (c ValueCondition) Not() Condition { return c.c.Not() }

// All holds when every one of conds holds; with none, it holds.
//
// Example:
//
//	<input ref="name" onInput={ui.JS.When(ui.All(ui.Ref("name").Value(), ui.Ref("email").Value()), ui.JS.RemoveAttr(ui.Ref("send"), "disabled"))} />
func All(conds ...Conditional) Condition { return join("and", conds...) }

// Any holds when one of conds holds; with none, it doesn't.
//
// Example:
//
//	<input ref="phone" onInput={ui.JS.When(ui.Any(ui.Ref("phone").Value(), ui.Ref("email").Value()), ui.JS.Hide(ui.Ref("contact-error")))} />
func Any(conds ...Conditional) Condition { return join("or", conds...) }

func (c Condition) MarshalJSON() ([]byte, error) {
	switch c.op {
	case "and", "or":
		of := c.of
		if of == nil {
			of = []Condition{}
		}
		return marshal(map[string][]Condition{c.op: of})
	case "not":
		return marshal(map[string]Condition{"not": c.of[0]})
	}
	var b bytes.Buffer
	b.WriteByte('{')
	if c.to != This {
		if err := writeKV(&b, "to", c.to); err != nil {
			return nil, err
		}
		b.WriteByte(',')
	}
	if err := writeKV(&b, c.key, c.val); err != nil {
		return nil, err
	}
	if c.some {
		b.WriteString(`,"some":true`)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// ShowOption changes how Show, Hide, Toggle and Transition work: Display,
// Transition, Time.
type ShowOption jsArg

// Display is the display Show sets, such as "flex" or "grid". Without it,
// Show removes the element's display style, and sets block if the element
// would still be hidden.
//
// Example:
//
//	<button onClick={ui.JS.Show(ui.Ref("dialog"), ui.Display("flex"))}>Edit</button>
func Display(v string) ShowOption { return ShowOption{"display", v} }

// Transition animates Show, Hide or Toggle with CSS classes: during and from
// are added, from is swapped for to on the next frame, and all of them are
// removed once Time (200ms) has passed. Each is a space-separated list of
// classes.
//
// Example:
//
//	<button onClick={ui.JS.Show(ui.Ref("menu"), ui.Transition("transition duration-200", "opacity-0", "opacity-100"))}>Menu</button>
func Transition(during, from, to string) ShowOption {
	return ShowOption{"transition", [3]string{during, from, to}}
}

// Time is how long a transition runs before its classes are removed; 200ms
// unless set.
//
// Example:
//
//	<button onClick={ui.JS.Hide(ui.Ref("toast"), ui.Transition("transition-opacity", "opacity-100", "opacity-0"), ui.Time(300*time.Millisecond))}>Dismiss</button>
func Time(d time.Duration) ShowOption { return ShowOption{"time", d.Milliseconds()} }

// DispatchOption changes the event Dispatch fires: Detail, NoBubble.
type DispatchOption jsArg

// Detail is the dispatched event's detail (event.detail in a listener),
// sent as JSON.
//
// Example:
//
//	<button onClick={ui.JS.Dispatch(ui.This, "pet-picked", ui.Detail(pet.ID))}>Pick</button>
func Detail(v any) DispatchOption { return DispatchOption{"detail", v} }

// NoBubble keeps the dispatched event on its target: it doesn't bubble up
// to the target's ancestors.
//
// Example:
//
//	<button onClick={ui.JS.Dispatch(ui.Ref("chart"), "refresh", ui.NoBubble())}>Refresh</button>
func NoBubble() DispatchOption { return DispatchOption{"bubbles", false} }

// PushOption changes a Push: Value, Timeout, KeepOnError.
type PushOption func(*push)

// Value sends v to the handler in place of the element's value. The
// handler's parameter parses it as it would the element's value: a string
// as is, anything else as its JSON.
//
// Example, for func (c *Pets) Pick(id int):
//
//	<button onClick={ui.JS.Push(c.Pick, ui.Value(pet.ID))}>Pick</button>
func Value(v any) PushOption { return func(p *push) { p.value, p.has = v, true } }

// Timeout is how long a Push waits for the server's reply before it counts
// as failed and its OnError runs; 10s unless set. A reply that comes later
// is still applied.
//
// Example:
//
//	<button onClick={ui.JS.AddClass(ui.This, "liked").Push(c.Like, ui.Timeout(3*time.Second))}>Like</button>
func Timeout(d time.Duration) PushOption { return func(p *push) { p.timeout = d } }

// KeepOnError keeps a Push's predictions when it fails, instead of taking
// them back; OnError still runs. For a change the user should see either
// way, such as a "not sent" mark.
//
// Example:
//
//	<button onClick={ui.JS.SetText(ui.Ref("status"), "Sending…").Push(c.Send, ui.KeepOnError()).OnError(ui.JS.SetText(ui.Ref("status"), "Not sent"))}>Send</button>
func KeepOnError() PushOption { return func(p *push) { p.keep = true } }

func showArgs(t Target, opts []ShowOption) []jsArg {
	args := []jsArg{to(t)}
	for _, o := range opts {
		args = append(args, jsArg(o))
	}
	return args
}

// AddClass adds CSS classes to target.
//
// Example:
//
//	<button onClick={ui.JS.AddClass(ui.Ref("menu"), "open", "shadow")}>Menu</button>
func (c Commands) AddClass(target Target, names ...string) Commands {
	return c.add("add_class", to(target), jsArg{"names", names})
}

// RemoveClass removes CSS classes from target.
//
// Example:
//
//	<button onClick={ui.JS.RemoveClass(ui.Ref("menu"), "hidden")}>Menu</button>
func (c Commands) RemoveClass(target Target, names ...string) Commands {
	return c.add("remove_class", to(target), jsArg{"names", names})
}

// ToggleClass adds each class target doesn't have, and removes each it has.
//
// Example:
//
//	<button onClick={ui.JS.ToggleClass(ui.Ref("menu"), "hidden")}>Menu</button>
func (c Commands) ToggleClass(target Target, names ...string) Commands {
	return c.add("toggle_class", to(target), jsArg{"names", names})
}

// SetAttr sets target's attribute name to value. A URL attribute's value
// (href, src, action, formaction, xlink:href, poster, data, …) is sanitized
// as an href written in markup is: a javascript: URL, say, becomes
// about:invalid#TemplFailedSanitizationURL. Name may not be an event
// handler attribute (on…) or srcdoc: SetAttr panics on one, and the
// runtime refuses one in commands from elsewhere.
//
// Example:
//
//	<button onClick={ui.JS.SetAttr(ui.This, "aria-pressed", "true")}>Bold</button>
func (c Commands) SetAttr(target Target, name, value string) Commands {
	jsAttrName("SetAttr", name)
	return c.add("set_attr", to(target), jsArg{"name", name}, jsArg{"value", jsAttrValue(name, value)})
}

// RemoveAttr removes target's attribute name, which, as SetAttr's, may not
// be an event handler attribute or srcdoc.
//
// Example:
//
//	<button onClick={ui.JS.RemoveAttr(ui.Ref("save"), "disabled")}>Edit</button>
func (c Commands) RemoveAttr(target Target, name string) Commands {
	jsAttrName("RemoveAttr", name)
	return c.add("remove_attr", to(target), jsArg{"name", name})
}

// ToggleAttr flips target's attribute name between two values: it sets v1,
// or v2 when the attribute already is v1. Name and values are checked and
// sanitized as SetAttr's.
//
// Example:
//
//	<button onClick={ui.JS.ToggleAttr(ui.This, "aria-expanded", "true", "false").Toggle(ui.Ref("menu"))}>Menu</button>
func (c Commands) ToggleAttr(target Target, name, v1, v2 string) Commands {
	jsAttrName("ToggleAttr", name)
	return c.add("toggle_attr", to(target), jsArg{"name", name}, jsArg{"values", [2]string{jsAttrValue(name, v1), jsAttrValue(name, v2)}})
}

// Show makes target visible: it removes its hidden attribute and sets its
// display (see Display). Options: Display, Transition, Time.
//
// Example:
//
//	<button onClick={ui.JS.Show(ui.Ref("dialog"))}>Edit</button>
func (c Commands) Show(target Target, opts ...ShowOption) Commands {
	return c.add("show", showArgs(target, opts)...)
}

// Hide hides target (display: none). Options: Transition, Time.
//
// Example:
//
//	<button onClick={ui.JS.Hide(ui.Ref("dialog"))}>Close</button>
func (c Commands) Hide(target Target, opts ...ShowOption) Commands {
	return c.add("hide", showArgs(target, opts)...)
}

// Toggle shows target when it is hidden, and hides it otherwise. Options:
// Display, Transition, Time.
//
// Example:
//
//	<button onClick={ui.JS.Toggle(ui.Ref("menu"))}>Menu</button>
func (c Commands) Toggle(target Target, opts ...ShowOption) Commands {
	return c.add("toggle", showArgs(target, opts)...)
}

// Transition runs a class transition on target without showing or hiding
// it: during and from are added, from is swapped for to on the next frame,
// and all are removed after Time (200ms).
//
// Example:
//
//	<button onClick={ui.JS.Transition(ui.Ref("cart"), "transition duration-300", "scale-110", "scale-100")}>Add to cart</button>
func (c Commands) Transition(target Target, during, from, to string, opts ...ShowOption) Commands {
	return c.add("transition", showArgs(target, append([]ShowOption{Transition(during, from, to)}, opts...))...)
}

// Focus moves the keyboard focus to target.
//
// Example:
//
//	<button onClick={ui.JS.Show(ui.Ref("search")).Focus(ui.Ref("search"))}>Search</button>
func (c Commands) Focus(target Target) Commands { return c.add("focus", to(target)) }

// FocusFirst focuses the first focusable element inside target, such as a
// dialog's first field.
//
// Example:
//
//	<button onClick={ui.JS.Show(ui.Ref("dialog")).FocusFirst(ui.Ref("dialog"))}>Edit</button>
func (c Commands) FocusFirst(target Target) Commands { return c.add("focus_first", to(target)) }

// PushFocus remembers an element for PopFocus to focus again: target, or
// with none the element that has the focus now. Use it before opening a
// dialog, so closing it gives the focus back.
//
// Example:
//
//	<button onClick={ui.JS.PushFocus().Show(ui.Ref("dialog")).FocusFirst(ui.Ref("dialog"))}>Edit</button>
func (c Commands) PushFocus(target ...Target) Commands {
	if len(target) == 0 {
		return c.add("push_focus")
	}
	return c.add("push_focus", to(target[0]))
}

// PopFocus focuses the element the last PushFocus remembered, and forgets
// it.
//
// Example:
//
//	<button onClick={ui.JS.Hide(ui.Ref("dialog")).PopFocus()}>Cancel</button>
func (c Commands) PopFocus() Commands { return c.add("pop_focus") }

// Dispatch fires a CustomEvent named event on target, for the page's
// scripts to listen to. It bubbles unless NoBubble; Detail sets its detail.
//
// Example:
//
//	<button onClick={ui.JS.Dispatch(ui.This, "pet-picked", ui.Detail(pet.ID))}>Pick</button>
func (c Commands) Dispatch(target Target, event string, opts ...DispatchOption) Commands {
	args := []jsArg{to(target), {"event", event}, {"bubbles", true}}
	for _, o := range opts {
		if o.key == "bubbles" {
			args[2] = jsArg(o)
		} else {
			args = append(args, jsArg(o))
		}
	}
	return c.add("dispatch", args...)
}

// Exec runs the commands held in target's attribute attr: another
// element's chain, rendered as data-vk-js-<event>, or a JSON list of
// commands. Inside them, This is target.
//
// Example: Escape runs the Cancel button's click.
//
//	<div ref="dialog" onKeyDown={ui.JS.Key("Escape").Exec(ui.Ref("cancel"), "data-vk-js-click")}>
//		<button ref="cancel" onClick={ui.JS.Hide(ui.Ref("dialog")).PopFocus()}>Cancel</button>
//	</div>
func (c Commands) Exec(target Target, attr string) Commands {
	return c.add("exec", to(target), jsArg{"attr", attr})
}

// Push calls handler on the server, as onClick={handler} would: a method
// value, a closure or a func prop, getting the element's value unless
// ui.Value gives another. Without a live session it does nothing.
//
// The steps before it, since the chain's start or the previous Push, are
// its predictions: they show at once and are taken back if the push fails.
// OnError, OnSuccess and Timeout after it apply to it.
//
// Example:
//
//	<input name="q" onInput={ui.JS.Debounce(150*time.Millisecond).Push(c.Search)} />
//	<button onClick={ui.JS.RemoveClass(ui.Ref("menu"), "hidden").Push(c.MenuOpened)}>Open</button>
func (c Commands) Push(handler any, opts ...PushOption) Commands {
	p := &push{fn: handler}
	for _, o := range opts {
		o(p)
	}
	return c.add("push", jsArg{"h", p})
}

// OnError runs chain when the last Push before it fails: its handler
// returns an error, the server refuses the event, or no reply comes in time
// (see Timeout). That Push's predictions are taken back first, unless
// KeepOnError, except where the server has rendered a new value since.
// Panics with no Push before it.
//
// Example: a like shown at once, taken back if it fails.
//
//	<button onClick={ui.JS.AddClass(ui.This, "liked").Push(c.Like).OnError(ui.JS.Show(ui.Ref("like-error")))}>Like</button>
//	<p ref="like-error" style="display: none">Couldn't like</p>
func (c Commands) OnError(chain JSChain) Commands {
	ch := chain.jsChain()
	return c.withPush("OnError", func(p *push) { p.onErr = &ch })
}

// OnSuccess runs chain when the last Push before it succeeds: the reply
// carries no error. Its predictions stay, unless the server renders
// something else. Panics with no Push before it.
//
// Example:
//
//	<button onClick={ui.JS.Push(c.Save).OnSuccess(ui.JS.Show(ui.Ref("saved")))}>Save</button>
func (c Commands) OnSuccess(chain JSChain) Commands {
	ch := chain.jsChain()
	return c.withPush("OnSuccess", func(p *push) { p.onOK = &ch })
}

// Timeout is how long the last Push before it waits for its reply before
// it fails; 10s unless set. The same as Push's ui.Timeout option. Panics
// with no Push before it.
//
// Example:
//
//	<button onClick={ui.JS.AddClass(ui.This, "liked").Push(c.Like).Timeout(3*time.Second)}>Like</button>
func (c Commands) Timeout(d time.Duration) Commands {
	return c.withPush("Timeout", func(p *push) { p.timeout = d })
}

// withPush is the chain with its last Push (or Options) changed by f, a
// copy of it.
func (c Commands) withPush(step string, f func(*push)) Commands {
	for i := len(c.ops) - 1; i >= 0; i-- {
		if c.ops[i].name != "push" && c.ops[i].name != "options" {
			continue
		}
		args := slices.Clone(c.ops[i].args)
		for j, a := range args {
			if x, ok := a.val.(*push); ok {
				p := *x
				f(&p)
				args[j] = jsArg{a.key, &p}
			}
		}
		ops := slices.Clone(c.ops)
		ops[i] = jsOp{name: c.ops[i].name, args: args}
		return Commands{ops}
	}
	panic("ui: JS " + step + " without a Push or Options before it")
}

// Options loads the options of target, a select, from handler: a method
// of the page or of a component on it, called with args — values, or
// element reads such as ui.This.Value(), made when the step runs. The
// call goes to the page's own URL over HTTP, so it needs no connection:
// it works on a page that isn't live as on one that is.
//
//	func (p *Checkout) CitiesOf(ctx context.Context, country string, geo *Geo) ([]ui.Option, error)
//
// The method returns []Option, or []OptionGroup for <optgroup>s, and maybe
// an error; it takes a context.Context first if it needs one, then the
// arguments (the parameters that aren't pointers or interfaces), then its
// dependencies (pointers and interfaces, injected). Vuka's compiler checks
// the arguments against them, and only methods an Options command names
// can be called. The call runs the page's guards and the app's
// middleware, not the page's Load: the method is called on a fresh page
// (its path and query fields bound) or component, not on what the page
// rendered, so it takes what it needs as arguments and dependencies.
//
// The reply replaces target's options — a leading option whose value is
// "" (a placeholder) stays — and keeps the value if it is still offered,
// else clears it; then target fires change, so a select loading another's
// options from its own chains on. A failure (the method's error, a reply
// that doesn't come in time) runs OnError after it, else is logged and
// leaves the options as they were; OnSuccess and Timeout apply to it as to
// a Push.
//
// Example:
//
//	<select name="country" onChange={ui.JS.Options(ui.Sel("#city"), c.CitiesOf, ui.This.Value())}>…</select>
//	<select name="country" onChange={ui.JS.Options(ui.Ref("city"), c.CitiesOf, ui.This.Value()).OnError(ui.JS.Show(ui.Ref("cities-error")))}>…</select>
func (c Commands) Options(target Target, handler any, args ...any) Commands {
	p := &push{fn: handler, options: true}
	if len(args) > 0 {
		p.args = callArgs(slices.Clone(args))
	}
	return c.add("options", to(target), jsArg{"m", p})
}

// Call runs a JavaScript function of the page: one registered with
// vuka.register(name, fn), else a dotted path on window ("Chart.refresh").
// The function gets a context (ctx.el, ctx.event, ctx.root), then args:
// JSON values, or element reads such as ui.Ref("q").Value(), made when the
// step runs.
//
// The chain waits for a returned promise. Returning false stops the chain;
// a throw stops it too, and fails the Push it predicts for. Call panics
// unless name is an identifier or a dotted path of them.
//
// Example:
//
//	<button onClick={ui.JS.Call("confetti", 120)}>Celebrate</button>
//	<input ref="q" onInput={ui.JS.Debounce(200*time.Millisecond).Call("Search.preview", ui.Ref("q").Value())} />
func (c Commands) Call(name string, args ...any) Commands {
	return c.callOn(This, name, args)
}

func (c Commands) callOn(t Target, name string, args []any) Commands {
	if !fnName.MatchString(name) {
		panic(fmt.Sprintf("ui: JS Call %q: not a function name or a dotted path of them", name))
	}
	op := []jsArg{{"fn", name}, to(t)}
	if len(args) > 0 {
		op = append(op, jsArg{"args", callArgs(slices.Clone(args))})
	}
	return c.add("call", op...)
}

var fnName = regexp.MustCompile(`^[A-Za-z_$][\w$]*(\.[A-Za-z_$][\w$]*)*$`)

// Debounce waits until the events have stopped for d, then runs the steps
// after it. Only the steps after it wait; the steps before it run on every
// event.
//
// Example: search once the user pauses typing.
//
//	<input name="q" onInput={ui.JS.Debounce(150*time.Millisecond).Push(c.Search)} />
func (c Commands) Debounce(d time.Duration) Commands {
	return c.add("debounce", jsArg{"ms", d.Milliseconds()})
}

// Throttle runs the steps after it at most once every d; for input events
// the last one still runs when the time is up. The steps before it run on
// every event.
//
// Example:
//
//	<input type="range" name="volume" onInput={ui.JS.Throttle(100*time.Millisecond).Push(c.SetVolume)} />
func (c Commands) Throttle(d time.Duration) Commands {
	return c.add("throttle", jsArg{"ms", d.Milliseconds()})
}

// Key stops the chain unless the event is a press of one of keys, named as
// KeyboardEvent.key names them: "Enter", "Escape", "a". Put it first: the
// steps before it run on every key.
//
// Example:
//
//	<div ref="dialog" onKeyDown={ui.JS.Key("Escape").Hide(ui.Ref("dialog"))}>…</div>
func (c Commands) Key(keys ...string) Commands { return c.add("key", jsArg{"keys", keys}) }

// Confirm asks msg in the browser's confirm dialog, and stops the chain
// unless the user accepts.
//
// Example:
//
//	<button onClick={ui.JS.Confirm("Delete " + pet.Name + "?").Push(func() { c.Delete(pet.ID) })}>Delete</button>
func (c Commands) Confirm(msg string) Commands { return c.add("confirm", jsArg{"msg", msg}) }

// If runs the steps after it, up to its ElseIf, Else or End, only when cond
// holds; cond is read when the If is reached. Ifs nest. Without End, the
// branch runs to the end of the chain.
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.If(ui.Ref("all").Checked()).Show(ui.Ref("bulk")).Else().Hide(ui.Ref("bulk")).End()} />
func (c Commands) If(cond Conditional) Commands {
	return c.add("if", jsArg{"cond", cond.cond()})
}

// ElseIf is another branch of the open If: its steps run when no branch
// before it ran and cond holds. Panics with no open If, or after its Else.
//
// Example:
//
//	<select ref="plan" onChange={ui.JS.If(ui.Ref("plan").Value().Eq("team")).Show(ui.Ref("seats")).ElseIf(ui.Ref("plan").Value().Eq("free")).Hide(ui.Ref("seats")).End()}>…</select>
func (c Commands) ElseIf(cond Conditional) Commands {
	c.branch("ElseIf")
	return c.add("else_if", jsArg{"cond", cond.cond()})
}

// Else is the open If's last branch: its steps run when no branch before it
// ran. Panics with no open If, or after another Else.
//
// Right after a When, Else(chain) is that When's other branch: chain runs
// when the condition doesn't hold, and no End is needed.
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.If(ui.Ref("all").Checked()).Show(ui.Ref("bulk")).Else().Hide(ui.Ref("bulk")).End()} />
//	<input type="checkbox" ref="all" onChange={ui.JS.When(ui.Ref("all").Checked(), ui.JS.Show(ui.Ref("bulk"))).Else(ui.JS.Hide(ui.Ref("bulk")))} />
func (c Commands) Else(chain ...JSChain) Commands {
	switch len(chain) {
	case 0:
		c.branch("Else")
		return c.add("else")
	case 1:
	default:
		panic("ui: JS Else takes one chain at most")
	}
	n := len(c.ops)
	if n == 0 || c.ops[n-1].when != 1 {
		panic("ui: JS Else(chain) goes right after a When, once")
	}
	ops := append(slices.Clone(c.ops[:n-1]), jsOp{name: "else"})
	ops = append(ops, closed(chain[0].jsChain().ops)...)
	return Commands{append(ops, jsOp{name: "end", when: 2})}
}

// When runs chain only when cond holds, read when the step is reached; the
// steps after it run either way. Else(chain) right after it gives the
// other branch. It needs no End: an If whose branch is chain.
//
// Example:
//
//	<input ref="title" onInput={ui.JS.When(ui.Ref("title").Value(), ui.JS.RemoveAttr(ui.Ref("save"), "disabled")).Else(ui.JS.SetAttr(ui.Ref("save"), "disabled", ""))} />
func (c Commands) When(cond Conditional, chain JSChain) Commands {
	ops := append(slices.Clone(c.ops), jsOp{name: "if", args: []jsArg{{"cond", cond.cond()}}})
	ops = append(ops, closed(chain.jsChain().ops)...)
	return Commands{append(ops, jsOp{name: "end", when: 1})}
}

// closed is ops with an End for each If they leave open, so a chain put in
// a branch can't take over the branch's End.
func closed(ops []jsOp) []jsOp {
	open := 0
	for _, o := range ops {
		switch o.name {
		case "if":
			open++
		case "end":
			open--
		}
	}
	out := slices.Clone(ops)
	if n := len(out); n > 0 && out[n-1].when != 0 {
		out[n-1].when = 0 // a When ending the inner chain takes no Else from outside it
	}
	for ; open > 0; open-- {
		out = append(out, jsOp{name: "end"})
	}
	return out
}

// End closes the open If: the steps after it run whichever branch ran.
// Panics with no open If.
//
// Example: the Push runs whether the box was checked or not.
//
//	<input type="checkbox" ref="done" onChange={ui.JS.If(ui.Ref("done").Checked()).AddClass(ui.Closest("li"), "line-through").Else().RemoveClass(ui.Closest("li"), "line-through").End().Push(c.ToggleDone)} />
func (c Commands) End() Commands {
	c.branch("End")
	return c.add("end")
}

// branch panics unless an If is open for step: one with no End, and no Else
// unless step is End.
func (c Commands) branch(step string) {
	var open []bool // per open If: whether it had its Else
	for _, o := range c.ops {
		switch o.name {
		case "if":
			open = append(open, false)
		case "else":
			open[len(open)-1] = true
		case "end":
			open = open[:len(open)-1]
		}
	}
	switch {
	case len(open) == 0:
		panic("ui: JS " + step + " without an If before it")
	case open[len(open)-1] && step != "End":
		panic("ui: JS " + step + " after an Else; an If has one Else, last")
	}
}

// BusyArg is what Busy takes: the targets to mark busy, and ui.Timeout.
type BusyArg interface{ busyArg() }

func (Target) busyArg()     {}
func (PushOption) busyArg() {}

// Busy marks target busy — This when none is given — while what the event
// started is under way: a form posting, a Push waiting for its reply.
//
// It sets aria-busy="true" on the target; makes the inputs, textareas and
// selects inside it readonly (never disabled, so their values are still
// submitted); gives its buttons and links aria-disabled="true" and
// tabindex="-1"; and ignores further clicks and submits on it, so a form
// can't be sent twice. Style it with Tailwind's aria-busy: variants, or
// group-aria-busy: inside an element with class="group".
//
// It is undone, each attribute put back as it was, by the reply to a Push
// of the same chain (success or failure, before OnSuccess or OnError run),
// by ui.Timeout(d), or when the browser shows the page again from its
// back/forward cache. A plain form post that loads the next page needs
// none of them.
//
// Example:
//
//	<form method="post" class="group" onSubmit={ui.JS.Busy()}><input name="username" /><button class="group-aria-busy:opacity-50">Sign in</button></form>
//	<button onClick={ui.JS.Busy(ui.Ref("form"), ui.Timeout(5*time.Second)).Push(c.Save)}>Save</button>
func (c Commands) Busy(args ...BusyArg) Commands {
	var ts []Target
	var ms int64
	for _, a := range args {
		switch a := a.(type) {
		case Target:
			ts = append(ts, a)
		case PushOption:
			var p push
			a(&p)
			if p.has || p.keep || p.timeout <= 0 {
				panic("ui: JS Busy takes targets and ui.Timeout, no other option")
			}
			ms = p.timeout.Milliseconds()
		}
	}
	if len(ts) == 0 {
		ts = []Target{This}
	}
	for _, t := range ts {
		op := []jsArg{to(t)}
		if ms > 0 {
			op = append(op, jsArg{"timeout", ms})
		}
		c = c.add("busy", op...)
	}
	return c
}

// Copy copies target's value (an input, textarea or select), or else its
// text, to the clipboard.
//
// Example:
//
//	<input ref="token" value={key.Token} readonly />
//	<button onClick={ui.JS.Copy(ui.Ref("token")).AddClass(ui.This, "copied")}>Copy</button>
func (c Commands) Copy(target Target) Commands { return c.add("copy", to(target)) }

// CopyText copies text to the clipboard.
//
// Example:
//
//	<button onClick={ui.JS.CopyText(post.URL)}>Copy link</button>
func (c Commands) CopyText(text string) Commands { return c.add("copy_text", jsArg{"text", text}) }

// ScrollTo scrolls the page until target is in view.
//
// Example:
//
//	<button onClick={ui.JS.ScrollTo(ui.Ref("comments"))}>Comments</button>
func (c Commands) ScrollTo(target Target) Commands { return c.add("scroll_to", to(target)) }

// SetValue sets a form field's value and fires input and change on it, so
// its own on… attributes run. The value is a string, a bool (a checkbox's
// checked state; "true" checks one too), or a read of another element,
// made when the step runs: ui.Sel("#bill-city").Value(), ui.This.Checked().
// A select takes the option of that value, a radio or checkbox is checked
// when the value is true (or its own value).
//
// Example:
//
//	<button onClick={ui.JS.SetValue(ui.Ref("q"), "").Focus(ui.Ref("q"))}>Clear</button>
//	<input type="checkbox" onChange={ui.JS.If(ui.This.Checked()).SetValue(ui.Sel("#ship-city"), ui.Sel("#bill-city").Value()).End()} />
func (c Commands) SetValue(target Target, v any) Commands {
	return c.add("set_value", to(target), jsArg{"value", setValueArg(v)})
}

// setValueArg is a SetValue's value as the step carries it: a string, a
// bool, or an element read ({"$read": …}). Panics on anything else.
func setValueArg(v any) any {
	switch x := v.(type) {
	case string, bool:
		return x
	case Conditional:
		return map[string]Condition{"$read": x.cond()}
	}
	if t := reflect.TypeOf(v); t != nil && (t.Kind() == reflect.String || t.Kind() == reflect.Bool) {
		return v
	}
	panic(fmt.Sprintf("ui: JS SetValue takes a string, a bool or a read (ui.Sel(\"#x\").Value()), not %T", v))
}

// SetText replaces target's content with text. Before a Push it predicts
// what the server will render, such as a new count.
//
// Example:
//
//	<button onClick={ui.JS.SetText(ui.Ref("likes"), strconv.Itoa(c.likes+1)).Push(c.Like)}>♥ <span ref="likes">{c.likes}</span></button>
func (c Commands) SetText(target Target, text string) Commands {
	return c.add("set_text", to(target), jsArg{"text", text})
}

// Patch moves a live page to href on the same route, with other path
// parameters or query, and the address bar changes: a component whose
// Update takes the new URL stays mounted and gets the new props. On a page with no connection, or for
// another origin, it loads href. Href is a path, or an http or https URL
// (LoadableURL): any other, such as a javascript: URL, becomes
// about:invalid#TemplFailedSanitizationURL, which the runtime refuses to
// load. The server decides what stays mounted (see live.Session.Navigate):
// a component without such an Update mounts again.
//
// Example:
//
//	<button onClick={ui.JS.Patch("/pets?page=2")}>Next</button>
func (c Commands) Patch(href string) Commands { return c.add("patch", jsArg{"href", navURL(href)}) }

// PatchReplace is Patch taking the current history entry's place instead of
// adding one: what a write of the page's URL state (an @ui.URL field, web's
// Query) does, so that back leaves the page rather than stepping through
// its tabs.
func (c Commands) PatchReplace(href string) Commands {
	return c.add("patch", jsArg{"href", navURL(href)}, jsArg{"replace", true})
}

// Navigate moves a live page to another page, href, over its connection:
// the new page's components mount, and the address bar changes. On a page
// with no connection, for another origin, or when href's route isn't a
// page, it loads href. Href is sanitized as Patch's.
//
// Example:
//
//	<button onClick={ui.JS.Navigate("/pets/" + strconv.Itoa(pet.ID))}>Open</button>
func (c Commands) Navigate(href string) Commands {
	return c.add("navigate", jsArg{"href", navURL(href)})
}

// On returns a chain whose next commands all act on target, so they don't
// name it. On again switches the target; Commands() goes back to commands
// naming their own.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("menu")).RemoveClass("hidden").AddClass("opacity-100").Push(c.MenuOpened)}>Open</button>
func (c Commands) On(target Target) CommandsOn { return CommandsOn{c, target} }

// CommandsOn is a chain whose commands act on one target, set by On, which
// they don't name. It is a chain like Commands, and as immutable.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("menu")).RemoveClass("hidden").Focus()}>Menu</button>
type CommandsOn struct {
	c  Commands
	to Target
}

func (o CommandsOn) jsChain() Commands { return o.c }

func (o CommandsOn) with(c Commands) CommandsOn { return CommandsOn{c, o.to} }

// On switches the target of the commands added from here on.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("dialog")).Hide().On(ui.Ref("saved")).Show()}>Done</button>
func (o CommandsOn) On(target Target) CommandsOn { return CommandsOn{o.c, target} }

// Commands goes back to a plain chain, whose commands name their own
// target.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("search")).Show().Commands().AddClass(ui.This, "active")}>Search</button>
func (o CommandsOn) Commands() Commands { return o.c }

// AddClass adds CSS classes to the target.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("menu")).AddClass("open", "shadow")}>Menu</button>
func (o CommandsOn) AddClass(names ...string) CommandsOn {
	return o.with(o.c.AddClass(o.to, names...))
}

// RemoveClass removes CSS classes from the target.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("menu")).RemoveClass("hidden")}>Menu</button>
func (o CommandsOn) RemoveClass(names ...string) CommandsOn {
	return o.with(o.c.RemoveClass(o.to, names...))
}

// ToggleClass adds each class the target doesn't have, and removes each it
// has.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("menu")).ToggleClass("hidden")}>Menu</button>
func (o CommandsOn) ToggleClass(names ...string) CommandsOn {
	return o.with(o.c.ToggleClass(o.to, names...))
}

// SetAttr sets the target's attribute name to value.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.This).SetAttr("aria-pressed", "true")}>Bold</button>
func (o CommandsOn) SetAttr(name, value string) CommandsOn {
	return o.with(o.c.SetAttr(o.to, name, value))
}

// RemoveAttr removes the target's attribute name.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("save")).RemoveAttr("disabled")}>Edit</button>
func (o CommandsOn) RemoveAttr(name string) CommandsOn { return o.with(o.c.RemoveAttr(o.to, name)) }

// ToggleAttr flips the target's attribute name between two values: it sets
// v1, or v2 when the attribute already is v1.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.This).ToggleAttr("aria-expanded", "true", "false")}>Menu</button>
func (o CommandsOn) ToggleAttr(name, v1, v2 string) CommandsOn {
	return o.with(o.c.ToggleAttr(o.to, name, v1, v2))
}

// Show makes the target visible: it removes its hidden attribute and sets
// its display (see Display). Options: Display, Transition, Time.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("dialog")).Show(ui.Display("flex")).FocusFirst()}>Edit</button>
func (o CommandsOn) Show(opts ...ShowOption) CommandsOn { return o.with(o.c.Show(o.to, opts...)) }

// Hide hides the target (display: none). Options: Transition, Time.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("dialog")).Hide()}>Close</button>
func (o CommandsOn) Hide(opts ...ShowOption) CommandsOn { return o.with(o.c.Hide(o.to, opts...)) }

// Toggle shows the target when it is hidden, and hides it otherwise.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("menu")).Toggle()}>Menu</button>
func (o CommandsOn) Toggle(opts ...ShowOption) CommandsOn { return o.with(o.c.Toggle(o.to, opts...)) }

// Transition runs a class transition on the target without showing or
// hiding it (see Commands.Transition).
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("cart")).Transition("transition duration-300", "scale-110", "scale-100")}>Add to cart</button>
func (o CommandsOn) Transition(during, from, to string, opts ...ShowOption) CommandsOn {
	return o.with(o.c.Transition(o.to, during, from, to, opts...))
}

// Focus moves the keyboard focus to the target.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("search")).Show().Focus()}>Search</button>
func (o CommandsOn) Focus() CommandsOn { return o.with(o.c.Focus(o.to)) }

// FocusFirst focuses the first focusable element inside the target.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("dialog")).Show().FocusFirst()}>Edit</button>
func (o CommandsOn) FocusFirst() CommandsOn { return o.with(o.c.FocusFirst(o.to)) }

// PushFocus remembers the target for PopFocus to focus again.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.This).PushFocus().On(ui.Ref("dialog")).Show().FocusFirst()}>Edit</button>
func (o CommandsOn) PushFocus() CommandsOn { return o.with(o.c.PushFocus(o.to)) }

// PopFocus focuses the element the last PushFocus remembered, and forgets
// it; the target plays no part.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("dialog")).Hide().PopFocus()}>Cancel</button>
func (o CommandsOn) PopFocus() CommandsOn { return o.with(o.c.PopFocus()) }

// Dispatch fires a CustomEvent named event on the target. It bubbles unless
// NoBubble; Detail sets its detail.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("chart")).Dispatch("refresh", ui.NoBubble())}>Refresh</button>
func (o CommandsOn) Dispatch(event string, opts ...DispatchOption) CommandsOn {
	return o.with(o.c.Dispatch(o.to, event, opts...))
}

// Exec runs the commands held in the target's attribute attr, such as
// another element's chain rendered as data-vk-js-<event>.
//
// Example:
//
//	<div onKeyDown={ui.JS.Key("Escape").On(ui.Ref("cancel")).Exec("data-vk-js-click")}>…</div>
func (o CommandsOn) Exec(attr string) CommandsOn { return o.with(o.c.Exec(o.to, attr)) }

// Push calls handler on the server; the steps before it are its
// predictions (see Commands.Push). The target plays no part.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("menu")).RemoveClass("hidden").Push(c.MenuOpened)}>Open</button>
func (o CommandsOn) Push(handler any, opts ...PushOption) CommandsOn {
	return o.with(o.c.Push(handler, opts...))
}

// Debounce waits until the events have stopped for d, then runs the steps
// after it; the steps before it run on every event.
//
// Example:
//
//	<input name="q" onInput={ui.JS.On(ui.Ref("spinner")).Show().Debounce(150*time.Millisecond).Push(c.Search)} />
func (o CommandsOn) Debounce(d time.Duration) CommandsOn { return o.with(o.c.Debounce(d)) }

// Throttle runs the steps after it at most once every d; the steps before
// it run on every event.
//
// Example:
//
//	<input type="range" ref="volume" onInput={ui.JS.On(ui.Ref("volume")).Throttle(100*time.Millisecond).AddClass("busy").Push(c.SetVolume)} />
func (o CommandsOn) Throttle(d time.Duration) CommandsOn { return o.with(o.c.Throttle(d)) }

// Key stops the chain unless the event is a press of one of keys
// ("Enter", "Escape", …). Put it first.
//
// Example:
//
//	<div onKeyDown={ui.JS.Key("Escape").On(ui.Ref("dialog")).Hide()}>…</div>
func (o CommandsOn) Key(keys ...string) CommandsOn { return o.with(o.c.Key(keys...)) }

// Confirm asks msg in the browser's confirm dialog, and stops the chain
// unless the user accepts.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Closest("li")).Confirm("Delete this todo?").Hide().Push(c.DeleteTodo)}>Delete</button>
func (o CommandsOn) Confirm(msg string) CommandsOn { return o.with(o.c.Confirm(msg)) }

// If runs the steps after it, up to its ElseIf, Else or End, only when cond
// holds (see Commands.If).
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.On(ui.Ref("bulk")).If(ui.Ref("all").Checked()).Show().Else().Hide().End()} />
func (o CommandsOn) If(cond Conditional) CommandsOn { return o.with(o.c.If(cond)) }

// ElseIf is another branch of the open If, taken when no branch before it
// was and cond holds.
//
// Example:
//
//	<select ref="plan" onChange={ui.JS.On(ui.Ref("seats")).If(ui.Ref("plan").Value().Eq("team")).Show().ElseIf(ui.Ref("plan").Value().Eq("free")).Hide().End()}>…</select>
func (o CommandsOn) ElseIf(cond Conditional) CommandsOn { return o.with(o.c.ElseIf(cond)) }

// Else is the open If's last branch, taken when no branch before it was;
// right after a When, Else(chain) is its other branch (see Commands.Else).
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.On(ui.Ref("bulk")).If(ui.Ref("all").Checked()).Show().Else().Hide().End()} />
//	<input type="checkbox" ref="all" onChange={ui.JS.On(ui.Ref("bulk")).When(ui.Ref("all").Checked(), ui.JS.Show(ui.Ref("bulk"))).Else(ui.JS.Hide(ui.Ref("bulk")))} />
func (o CommandsOn) Else(chain ...JSChain) CommandsOn { return o.with(o.c.Else(chain...)) }

// When runs chain only when cond holds (see Commands.When). The chain names
// its own targets.
//
// Example:
//
//	<input ref="title" onInput={ui.JS.On(ui.Ref("save")).When(ui.Ref("title").Value().Not(), ui.JS.SetAttr(ui.Ref("save"), "disabled", "")).AddClass("checked")} />
func (o CommandsOn) When(cond Conditional, chain JSChain) CommandsOn {
	return o.with(o.c.When(cond, chain))
}

// Busy marks the target busy until a Push of the chain is answered, the
// timeout given with ui.Timeout passes, or the page comes back from the
// browser's back/forward cache (see Commands.Busy).
//
// Example:
//
//	<form method="post" ref="login" onSubmit={ui.JS.On(ui.Ref("login")).Busy(ui.Timeout(30*time.Second))}>…</form>
func (o CommandsOn) Busy(opts ...PushOption) CommandsOn {
	args := []BusyArg{o.to}
	for _, op := range opts {
		args = append(args, op)
	}
	return o.with(o.c.Busy(args...))
}

// End closes the open If: the steps after it run whichever branch ran.
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.On(ui.Ref("bulk")).If(ui.Ref("all").Checked()).Show().Else().Hide().End().Push(c.SelectAll)} />
func (o CommandsOn) End() CommandsOn { return o.with(o.c.End()) }

// Copy copies the target's value (an input, textarea or select), or else
// its text, to the clipboard.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("token")).Copy()}>Copy</button>
func (o CommandsOn) Copy() CommandsOn { return o.with(o.c.Copy(o.to)) }

// CopyText copies text to the clipboard; the target plays no part.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.This).CopyText(post.URL).AddClass("copied")}>Copy link</button>
func (o CommandsOn) CopyText(text string) CommandsOn { return o.with(o.c.CopyText(text)) }

// ScrollTo scrolls the page until the target is in view.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("comments")).Show().ScrollTo()}>Comments</button>
func (o CommandsOn) ScrollTo() CommandsOn { return o.with(o.c.ScrollTo(o.to)) }

// SetValue sets the target field's value (a string, a bool or a read, as
// Commands.SetValue takes) and fires input and change on it.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("q")).SetValue("").Focus()}>Clear</button>
func (o CommandsOn) SetValue(v any) CommandsOn { return o.with(o.c.SetValue(o.to, v)) }

// Options loads the target select's options from handler, a method of the
// page or of a component on it (see Commands.Options).
//
// Example:
//
//	<select name="country" onChange={ui.JS.On(ui.Ref("city")).Options(c.CitiesOf, ui.This.Value())}>…</select>
func (o CommandsOn) Options(handler any, args ...any) CommandsOn {
	return o.with(o.c.Options(o.to, handler, args...))
}

// SetText replaces the target's content with text.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("status")).SetText("Saving…").Push(c.Save)}>Save</button>
func (o CommandsOn) SetText(text string) CommandsOn { return o.with(o.c.SetText(o.to, text)) }

// OnError runs chain when the last Push before it fails, after its
// predictions are taken back (see Commands.OnError).
//
// Example:
//
//	<button onClick={ui.JS.On(ui.This).AddClass("liked").Push(c.Like).OnError(ui.JS.Show(ui.Ref("like-error")))}>Like</button>
func (o CommandsOn) OnError(chain JSChain) CommandsOn { return o.with(o.c.OnError(chain)) }

// OnSuccess runs chain when the last Push before it succeeds.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("saved")).Hide().Push(c.Save).OnSuccess(ui.JS.Show(ui.Ref("saved")))}>Save</button>
func (o CommandsOn) OnSuccess(chain JSChain) CommandsOn { return o.with(o.c.OnSuccess(chain)) }

// Timeout is how long the last Push before it waits for its reply before
// it fails; 10s unless set.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.This).AddClass("liked").Push(c.Like).Timeout(3*time.Second)}>Like</button>
func (o CommandsOn) Timeout(d time.Duration) CommandsOn { return o.with(o.c.Timeout(d)) }

// Patch moves a live page to href on the same route (see Commands.Patch).
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("list")).AddClass("opacity-50").Patch("/pets?page=2")}>Next</button>
func (o CommandsOn) Patch(href string) CommandsOn { return o.with(o.c.Patch(href)) }

// Navigate moves a live page to another page, href (see Commands.Navigate).
//
// Example:
//
//	<button onClick={ui.JS.On(ui.This).AddClass("opacity-50").Navigate("/pets/new")}>New pet</button>
func (o CommandsOn) Navigate(href string) CommandsOn { return o.with(o.c.Navigate(href)) }

// Call runs a JavaScript function of the page (see Commands.Call), with the
// target as its context's element, ctx.el.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("chart")).Call("Chart.refresh")}>Refresh</button>
func (o CommandsOn) Call(name string, args ...any) CommandsOn {
	return o.with(o.c.callOn(o.to, name, args))
}

// EncodeJS is a chain's JSON, the value of its data-vk-js-<event> attribute:
// each Push's handler registered with ctx's live host, which gives its id,
// or left without one when ctx has none.
func EncodeJS(ctx context.Context, ch JSChain) ([]byte, error) {
	host := liveHost(ctx)
	var b bytes.Buffer
	b.WriteByte('[')
	for i, o := range ch.jsChain().ops {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`["` + o.name + `",{`)
		sep := ""
		for _, a := range o.args {
			kvs := []jsArg{a}
			switch x := a.val.(type) {
			case Target:
				if x == This {
					continue
				}
			case stateView:
				r, err := x.ref(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", o.name, err)
				}
				m := r.(map[string]any)
				kvs = []jsArg{{"s", m["s"]}, {"i", m["i"]}}
				if p, ok := m["p"]; ok {
					kvs = append(kvs, jsArg{"p", p})
				}
			case *push:
				if x.options {
					om, err := optionsOf(x.fn)
					if err != nil {
						return nil, err
					}
					kvs[0].val = optionsRef(ctx, om)
					if len(x.args) > 0 {
						if len(x.args) != len(om.args) {
							return nil, fmt.Errorf("ui: JS Options: %s takes %d arguments, not %d", om.name, len(om.args), len(x.args))
						}
						kvs = append(kvs, jsArg{"args", x.args})
					} else if len(om.args) > 0 {
						return nil, fmt.Errorf("ui: JS Options: %s takes %d arguments, not 0", om.name, len(om.args))
					}
				} else if err := CheckHandler(x.fn); err != nil {
					return nil, fmt.Errorf("ui: JS Push %w", err)
				}
				if !x.options {
					if host == nil {
						continue
					}
					kvs[0].val = host.Handler(ctx, x.fn)
				}
				if x.has {
					kvs = append(kvs, jsArg{"value", x.value})
				}
				for _, h := range [...]struct {
					key string
					ch  *Commands
				}{{"on_ok", x.onOK}, {"on_error", x.onErr}} {
					if h.ch != nil {
						j, err := EncodeJS(ctx, *h.ch)
						if err != nil {
							return nil, err
						}
						kvs = append(kvs, jsArg{h.key, json.RawMessage(j)})
					}
				}
				if x.timeout > 0 {
					kvs = append(kvs, jsArg{"timeout", x.timeout.Milliseconds()})
				}
				if x.keep {
					kvs = append(kvs, jsArg{"keep", true})
				}
			}
			for _, kv := range kvs {
				b.WriteString(sep)
				sep = ","
				if err := writeKV(&b, kv.key, kv.val); err != nil {
					return nil, err
				}
			}
		}
		b.WriteString("}]")
	}
	b.WriteByte(']')
	return b.Bytes(), nil
}

func writeKV(b *bytes.Buffer, k string, v any) error {
	j, err := marshal(v)
	if err != nil {
		return fmt.Errorf("ui: JS %s: %w", k, err)
	}
	b.WriteString(`"` + k + `":`)
	b.Write(j)
	return nil
}

// marshal is v's JSON, compact, without HTML escaping.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// jsAttr writes a chain's data-vk-js-<event> attribute.
func (h *HTMLRenderer) jsAttr(name string, ch JSChain) error {
	j, err := EncodeJS(h.ctx, ch)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if p := h.probe(); p != nil {
		if p.info.JS = true; p.info.Live {
			return errPageLive
		}
	}
	h.write(` data-vk-js-` + EventName(name) + `="`)
	h.write(templ.EscapeString(string(j)))
	h.write(`"`)
	return nil
}
