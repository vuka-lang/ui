package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
}

type jsArg struct {
	key string
	val any
}

// push is a Push step's handler and options, resolved when the chain renders.
type push struct {
	fn      any
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
	return Commands{append(slices.Clip(c.ops), jsOp{name, args})}
}

func to(t Target) jsArg { return jsArg{"to", t} }

// Target is the element or elements a command acts on:
//
//	ui.This            the element the attribute is on (the default)
//	ui.Ref("menu")     the element with ref="menu" in the same component
//	ui.Closest(".card") the nearest of This and its ancestors matching a selector
//	ui.Inner(".item")  the elements inside This matching a selector
//	ui.Sel("#cart")    every element of the page matching a selector
//
// A plain string is a selector too: ui.JS.Hide("#cart") hides #cart.
//
// Example:
//
//	<button onClick={ui.JS.Hide(ui.Closest("li"))}>Dismiss</button>
type Target string

// Targets other than selectors are strings no selector can be: a NUL, then
// their kind.
const (
	refTarget     = "\x00ref:"
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

// Ref is the element marked ref="name" in the same component instance. Each
// instance has its own refs, so ten Dropdowns each open their own menu; the
// compiler checks that the component's JSX has the ref.
//
// Example:
//
//	<button onClick={ui.JS.Toggle(ui.Ref("menu"))}>Menu</button>
//	<ul ref="menu" style="display: none">…</ul>
func Ref(name string) Target { return Target(refTarget + name) }

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

// Sel is every element of the page matching a CSS selector; a plain string
// is the same target. Inside a component prefer Ref, which can't reach into
// another component.
//
// Example:
//
//	<button onClick={ui.JS.Show(ui.Sel("#cart"))}>Cart</button>
func Sel(css string) Target { return Target(css) }

func (t Target) MarshalJSON() ([]byte, error) {
	s := string(t)
	for _, k := range [...]struct{ prefix, key string }{{refTarget, "ref"}, {closestTarget, "closest"}, {innerTarget, "inner"}} {
		if v, ok := strings.CutPrefix(s, k.prefix); ok {
			return marshal(map[string]string{k.key: v})
		}
	}
	return marshal(map[string]string{"sel": s})
}

// Condition is a test for If and ElseIf, made from a target: t.Checked(),
// t.Value(), t.Value().Eq(v), t.Attr(name), t.Is(css) or t.Exists(). It is
// read in the browser when the If is reached, not when the chain is built.
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.If(ui.Ref("all").Checked()).Show(ui.Ref("bulk")).Else().Hide(ui.Ref("bulk")).End()} />
type Condition struct {
	to  Target
	key string
	val any
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
func (t Target) Checked() Condition { return Condition{t, "checked", true} }

// Value holds when the target's value isn't empty; .Eq(v) compares it. As
// an argument of Call it is the value itself, read when the step runs.
//
// Example:
//
//	<input ref="title" onInput={ui.JS.If(ui.Ref("title").Value()).RemoveAttr(ui.Ref("save"), "disabled").Else().SetAttr(ui.Ref("save"), "disabled", "").End()} />
func (t Target) Value() ValueCondition { return ValueCondition{Condition{t, "value", true}} }

func (c ValueCondition) MarshalJSON() ([]byte, error) { return c.c.MarshalJSON() }

// Eq holds when the target's value is exactly v.
//
// Example:
//
//	<select ref="kind" onChange={ui.JS.If(ui.Ref("kind").Value().Eq("other")).Show(ui.Ref("other")).Else().Hide(ui.Ref("other")).End()}>…</select>
func (c ValueCondition) Eq(v string) Condition { return Condition{c.c.to, "value_eq", v} }

// Attr holds when the target has the attribute name, whatever its value. As
// an argument of Call it is the attribute's value, or null.
//
// Example:
//
//	<button onClick={ui.JS.If(ui.Ref("menu").Attr("hidden")).Show(ui.Ref("menu")).Else().Hide(ui.Ref("menu")).End()}>Menu</button>
func (t Target) Attr(name string) Condition { return Condition{t, "attr", name} }

// Is holds when the target matches a CSS selector.
//
// Example:
//
//	<button onClick={ui.JS.If(ui.This.Is(".active")).Push(c.Unfollow).Else().Push(c.Follow).End()}>Follow</button>
func (t Target) Is(css string) Condition { return Condition{t, "is", css} }

// Exists holds when the target finds an element at all.
//
// Example:
//
//	<button onClick={ui.JS.If(ui.Sel("#cart").Exists()).Show("#cart").Else().Navigate("/cart").End()}>Cart</button>
func (t Target) Exists() Condition { return Condition{t, "exists", true} }

// Conditional is what If and ElseIf test and what Call reads from an
// element: a Condition or a ValueCondition.
type Conditional interface{ cond() Condition }

func (c Condition) cond() Condition      { return c }
func (c ValueCondition) cond() Condition { return c.c }

func (c Condition) MarshalJSON() ([]byte, error) {
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

// SetAttr sets target's attribute name to value.
//
// Example:
//
//	<button onClick={ui.JS.SetAttr(ui.This, "aria-pressed", "true")}>Bold</button>
func (c Commands) SetAttr(target Target, name, value string) Commands {
	return c.add("set_attr", to(target), jsArg{"name", name}, jsArg{"value", value})
}

// RemoveAttr removes target's attribute name.
//
// Example:
//
//	<button onClick={ui.JS.RemoveAttr(ui.Ref("save"), "disabled")}>Edit</button>
func (c Commands) RemoveAttr(target Target, name string) Commands {
	return c.add("remove_attr", to(target), jsArg{"name", name})
}

// ToggleAttr flips target's attribute name between two values: it sets v1,
// or v2 when the attribute already is v1.
//
// Example:
//
//	<button onClick={ui.JS.ToggleAttr(ui.This, "aria-expanded", "true", "false").Toggle(ui.Ref("menu"))}>Menu</button>
func (c Commands) ToggleAttr(target Target, name, v1, v2 string) Commands {
	return c.add("toggle_attr", to(target), jsArg{"name", name}, jsArg{"values", [2]string{v1, v2}})
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

// withPush is the chain with its last Push changed by f, a copy of it.
func (c Commands) withPush(step string, f func(*push)) Commands {
	for i := len(c.ops) - 1; i >= 0; i-- {
		if c.ops[i].name != "push" {
			continue
		}
		p := *c.ops[i].args[0].val.(*push)
		f(&p)
		ops := slices.Clone(c.ops)
		ops[i] = jsOp{"push", []jsArg{{"h", &p}}}
		return Commands{ops}
	}
	panic("ui: JS " + step + " without a Push before it")
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
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.If(ui.Ref("all").Checked()).Show(ui.Ref("bulk")).Else().Hide(ui.Ref("bulk")).End()} />
func (c Commands) Else() Commands {
	c.branch("Else")
	return c.add("else")
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

// SetValue sets a form field's value ("true" checks a checkbox) and fires
// input and change on it, so its own on… attributes run.
//
// Example:
//
//	<button onClick={ui.JS.SetValue(ui.Ref("q"), "").Focus(ui.Ref("q"))}>Clear</button>
func (c Commands) SetValue(target Target, v string) Commands {
	return c.add("set_value", to(target), jsArg{"value", v})
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
// parameters or query: the components stay mounted and get the new props,
// and the address bar changes. On a page with no connection, or for
// another origin, it loads href.
//
// Example:
//
//	<button onClick={ui.JS.Patch("/pets?page=2")}>Next</button>
func (c Commands) Patch(href string) Commands { return c.add("patch", jsArg{"href", href}) }

// Navigate moves a live page to another page, href, over its connection:
// the new page's components mount, and the address bar changes. On a page
// with no connection, for another origin, or when href's route isn't a
// page, it loads href.
//
// Example:
//
//	<button onClick={ui.JS.Navigate("/pets/" + strconv.Itoa(pet.ID))}>Open</button>
func (c Commands) Navigate(href string) Commands { return c.add("navigate", jsArg{"href", href}) }

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

// Else is the open If's last branch, taken when no branch before it was.
//
// Example:
//
//	<input type="checkbox" ref="all" onChange={ui.JS.On(ui.Ref("bulk")).If(ui.Ref("all").Checked()).Show().Else().Hide().End()} />
func (o CommandsOn) Else() CommandsOn { return o.with(o.c.Else()) }

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

// SetValue sets the target field's value ("true" checks a checkbox) and
// fires input and change on it.
//
// Example:
//
//	<button onClick={ui.JS.On(ui.Ref("q")).SetValue("").Focus()}>Clear</button>
func (o CommandsOn) SetValue(v string) CommandsOn { return o.with(o.c.SetValue(o.to, v)) }

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
			case *push:
				if err := CheckHandler(x.fn); err != nil {
					return nil, fmt.Errorf("ui: JS Push %w", err)
				}
				if host == nil {
					continue
				}
				kvs[0].val = host.Handler(ctx, x.fn)
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
	h.write(` data-vk-js-` + EventName(name) + `="`)
	h.write(templ.EscapeString(string(j)))
	h.write(`"`)
	return nil
}
