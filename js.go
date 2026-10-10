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

// Commands is a chain of JS commands: DOM changes the browser makes when an
// event fires, without a round trip — show a menu, toggle a class, move
// focus — and, with Push, a call of a handler on the server. A chain is the
// value of an on… attribute:
//
//	<button onClick={ui.JS.RemoveClass(ui.Ref("menu"), "hidden").Push(c.MenuOpened)}>Open</button>
//	<div ref="menu" class="hidden">…</div>
//
// JS is the empty chain; each method returns a new chain with one more step,
// leaving the one it is called on as it was, so a shared start is safe to
// build on. On(target) gives a chain whose commands act on target.
//
// Rendered, a chain is a data-vk-js-<event> attribute (the live protocol's
// "JS commands"), written with or without a session: a static page runs its
// commands too, but its Push steps do nothing.
type Commands struct{ ops []jsOp }

// JS is the empty chain, where chains start.
var JS Commands

// JSChain is a chain of JS commands: Commands or CommandsOn.
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

// Target is the elements a command acts on:
//
//	This            the element the command is on (the default)
//	Ref("menu")     the element with ref="menu" in the same component instance
//	Closest(".card") This's closest ancestor (or itself) matching a selector
//	Inner(".item")  This's descendants matching a selector
//	Sel("#x"), "#x" every element of the document matching a selector
//
// A plain string is a selector, so ui.JS.Hide("#dialog") hides #dialog.
type Target string

// Targets other than selectors are strings no selector can be: a NUL, then
// their kind.
const (
	refTarget     = "\x00ref:"
	closestTarget = "\x00closest:"
	innerTarget   = "\x00inner:"
)

// This is the element a command is on: the one the event's attribute is on,
// or a pushing component's root.
const This Target = ""

// Ref is the element whose ref attribute is name, in the component instance
// the command runs in.
func Ref(name string) Target { return Target(refTarget + name) }

// Closest is This's closest ancestor, or This, matching a CSS selector.
func Closest(sel string) Target { return Target(closestTarget + sel) }

// Inner is This's descendants matching a CSS selector.
func Inner(sel string) Target { return Target(innerTarget + sel) }

// Sel is the elements of the document matching a CSS selector.
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

// Condition is what If and ElseIf test, read from a target when the step is
// reached: t.Checked(), t.Value(), t.Value().Eq(v), t.Attr(name), t.Is(css),
// t.Exists().
type Condition struct {
	to  Target
	key string
	val any
}

// ValueCondition holds when the target's value isn't empty; Eq compares it.
type ValueCondition struct{ c Condition }

// Checked holds when the target is a checked checkbox or radio.
func (t Target) Checked() Condition { return Condition{t, "checked", true} }

// Value holds when the target's value isn't empty.
func (t Target) Value() ValueCondition { return ValueCondition{Condition{t, "value", true}} }

func (c ValueCondition) MarshalJSON() ([]byte, error) { return c.c.MarshalJSON() }

// Eq holds when the target's value is v.
func (c ValueCondition) Eq(v string) Condition { return Condition{c.c.to, "value_eq", v} }

// Attr holds when the target has the attribute name.
func (t Target) Attr(name string) Condition { return Condition{t, "attr", name} }

// Is holds when the target matches a CSS selector.
func (t Target) Is(css string) Condition { return Condition{t, "is", css} }

// Exists holds when the target names an element.
func (t Target) Exists() Condition { return Condition{t, "exists", true} }

// Conditional is a Condition or a ValueCondition.
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

// ShowOption is an option of Show, Hide, Toggle and Transition.
type ShowOption jsArg

// Display is the display Show sets: "flex", "grid", …; else the style's
// display is removed (block when the element would still be hidden).
func Display(v string) ShowOption { return ShowOption{"display", v} }

// Transition animates a change: the classes during and from are added, from
// is swapped for to on the next frame, and all are removed once the
// transition's time (Time, 200ms unless set) has passed. Each is a
// space-separated list of classes.
func Transition(during, from, to string) ShowOption {
	return ShowOption{"transition", [3]string{during, from, to}}
}

// Time is how long a transition runs.
func Time(d time.Duration) ShowOption { return ShowOption{"time", d.Milliseconds()} }

// DispatchOption is an option of Dispatch.
type DispatchOption jsArg

// Detail is the dispatched event's detail, as JSON.
func Detail(v any) DispatchOption { return DispatchOption{"detail", v} }

// NoBubble makes the dispatched event not bubble.
func NoBubble() DispatchOption { return DispatchOption{"bubbles", false} }

// PushOption is an option of Push.
type PushOption func(*push)

// Value is the value the handler gets, in place of the element's: a string
// as is, anything else as its JSON text, parsed by the handler's parameter
// as an element's value would be.
func Value(v any) PushOption { return func(p *push) { p.value, p.has = v, true } }

// Timeout is how long the push waits for the server's reply before it
// counts as failed (see OnError); 10s unless set.
func Timeout(d time.Duration) PushOption { return func(p *push) { p.timeout = d } }

// KeepOnError makes a failed push revert nothing its predictions changed;
// its OnError chain still runs.
func KeepOnError() PushOption { return func(p *push) { p.keep = true } }

func showArgs(t Target, opts []ShowOption) []jsArg {
	args := []jsArg{to(t)}
	for _, o := range opts {
		args = append(args, jsArg(o))
	}
	return args
}

// AddClass adds classes to target.
func (c Commands) AddClass(target Target, names ...string) Commands {
	return c.add("add_class", to(target), jsArg{"names", names})
}

// RemoveClass removes classes from target.
func (c Commands) RemoveClass(target Target, names ...string) Commands {
	return c.add("remove_class", to(target), jsArg{"names", names})
}

// ToggleClass adds each class target lacks, and removes each it has.
func (c Commands) ToggleClass(target Target, names ...string) Commands {
	return c.add("toggle_class", to(target), jsArg{"names", names})
}

// SetAttr sets target's attribute name to value.
func (c Commands) SetAttr(target Target, name, value string) Commands {
	return c.add("set_attr", to(target), jsArg{"name", name}, jsArg{"value", value})
}

// RemoveAttr removes target's attribute name.
func (c Commands) RemoveAttr(target Target, name string) Commands {
	return c.add("remove_attr", to(target), jsArg{"name", name})
}

// ToggleAttr sets target's attribute name to v1, or to v2 when it is v1.
func (c Commands) ToggleAttr(target Target, name, v1, v2 string) Commands {
	return c.add("toggle_attr", to(target), jsArg{"name", name}, jsArg{"values", [2]string{v1, v2}})
}

// Show shows target: sets its style's display (see Display) and removes its
// hidden attribute.
func (c Commands) Show(target Target, opts ...ShowOption) Commands {
	return c.add("show", showArgs(target, opts)...)
}

// Hide hides target: display none.
func (c Commands) Hide(target Target, opts ...ShowOption) Commands {
	return c.add("hide", showArgs(target, opts)...)
}

// Toggle shows target when it is hidden, else hides it.
func (c Commands) Toggle(target Target, opts ...ShowOption) Commands {
	return c.add("toggle", showArgs(target, opts)...)
}

// Transition runs a transition on target (see the Transition option),
// showing or hiding nothing.
func (c Commands) Transition(target Target, during, from, to string, opts ...ShowOption) Commands {
	return c.add("transition", showArgs(target, append([]ShowOption{Transition(during, from, to)}, opts...))...)
}

// Focus focuses target.
func (c Commands) Focus(target Target) Commands { return c.add("focus", to(target)) }

// FocusFirst focuses target's first focusable descendant.
func (c Commands) FocusFirst(target Target) Commands { return c.add("focus_first", to(target)) }

// PushFocus remembers target, or with none the focused element, for PopFocus.
func (c Commands) PushFocus(target ...Target) Commands {
	if len(target) == 0 {
		return c.add("push_focus")
	}
	return c.add("push_focus", to(target[0]))
}

// PopFocus focuses the element PushFocus remembered last, and forgets it.
func (c Commands) PopFocus() Commands { return c.add("pop_focus") }

// Dispatch fires a CustomEvent named event on target, bubbling unless
// NoBubble.
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

// Exec runs the commands held in target's attribute attr, which is a chain
// rendered by another element's on… attribute or a JSON list of commands.
func (c Commands) Exec(target Target, attr string) Commands {
	return c.add("exec", to(target), jsArg{"attr", attr})
}

// Push calls handler on the server, as an on… attribute with handler would:
// handler is any event handler (see CheckHandler), and gets the event's value
// unless Value says otherwise. The steps before it, since the chain's start
// or the Push before it, are its predictions: shown at once, and reverted
// if it fails (see OnError).
func (c Commands) Push(handler any, opts ...PushOption) Commands {
	p := &push{fn: handler}
	for _, o := range opts {
		o(p)
	}
	return c.add("push", jsArg{"h", p})
}

// OnError runs chain when the chain's last Push fails: its handler returns
// an error, the server refuses the event, or no reply comes within its
// Timeout. The steps before that Push, since the chain's start or the Push
// before it, are its predictions: what they changed is reverted first,
// unless KeepOnError, wherever the server hasn't rendered a new value since.
func (c Commands) OnError(chain JSChain) Commands {
	ch := chain.jsChain()
	return c.withPush("OnError", func(p *push) { p.onErr = &ch })
}

// OnSuccess runs chain when the chain's last Push succeeds: the server's
// reply carries no error. Its predictions stay, where the server's render
// doesn't replace them.
func (c Commands) OnSuccess(chain JSChain) Commands {
	ch := chain.jsChain()
	return c.withPush("OnSuccess", func(p *push) { p.onOK = &ch })
}

// Timeout is how long the chain's last Push waits for its reply before it
// fails; 10s unless set.
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

// Call calls a JavaScript function of the page: one registered with
// vuka.register(name, fn), else name as a dotted path on window
// ("Picker.refresh"). It gets a context (the element, the event, the
// instance root) and args: JSON values, or reads of an element made when
// the step is reached — ui.Ref("q").Value(), .Checked(), .Attr("name").
// The chain waits for a returned promise, stops when the function returns
// false, and stops on a throw, which fails the Push it predicts for. Call
// panics unless name is an identifier or a dotted path of them.
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

// Debounce runs the steps after it once events have stopped for d.
func (c Commands) Debounce(d time.Duration) Commands {
	return c.add("debounce", jsArg{"ms", d.Milliseconds()})
}

// Throttle runs the steps after it at most once per d.
func (c Commands) Throttle(d time.Duration) Commands {
	return c.add("throttle", jsArg{"ms", d.Milliseconds()})
}

// Key stops the chain unless the event is a key press of one of keys
// ("Enter", "Escape", "a", …: KeyboardEvent.key).
func (c Commands) Key(keys ...string) Commands { return c.add("key", jsArg{"keys", keys}) }

// Confirm asks msg, and stops the chain unless the user confirms.
func (c Commands) Confirm(msg string) Commands { return c.add("confirm", jsArg{"msg", msg}) }

// If runs the steps after it, up to its ElseIf, Else or End, when cond holds.
func (c Commands) If(cond Conditional) Commands {
	return c.add("if", jsArg{"cond", cond.cond()})
}

// ElseIf is a branch of the If before it, taken when no branch before it
// was and cond holds.
func (c Commands) ElseIf(cond Conditional) Commands {
	c.branch("ElseIf")
	return c.add("else_if", jsArg{"cond", cond.cond()})
}

// Else is the If's branch taken when none before it was.
func (c Commands) Else() Commands {
	c.branch("Else")
	return c.add("else")
}

// End ends the If before it; the steps after it run whichever branch ran.
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

// Copy copies target's value (an input, textarea or select) or text.
func (c Commands) Copy(target Target) Commands { return c.add("copy", to(target)) }

// CopyText copies text.
func (c Commands) CopyText(text string) Commands { return c.add("copy_text", jsArg{"text", text}) }

// ScrollTo scrolls target into view.
func (c Commands) ScrollTo(target Target) Commands { return c.add("scroll_to", to(target)) }

// SetValue sets target's value (a checkbox's checked, from "true"), firing
// input and change on it.
func (c Commands) SetValue(target Target, v string) Commands {
	return c.add("set_value", to(target), jsArg{"value", v})
}

// SetText sets target's text, replacing its content.
func (c Commands) SetText(target Target, text string) Commands {
	return c.add("set_text", to(target), jsArg{"text", text})
}

// On is the chain with target as the target of the commands added to it.
func (c Commands) On(target Target) CommandsOn { return CommandsOn{c, target} }

// CommandsOn is a chain whose commands act on one target, which they don't
// name: ui.JS.On(ui.Ref("menu")).RemoveClass("hidden").Focus(). It is a
// chain like Commands, and immutable like it.
type CommandsOn struct {
	c  Commands
	to Target
}

func (o CommandsOn) jsChain() Commands { return o.c }

func (o CommandsOn) with(c Commands) CommandsOn { return CommandsOn{c, o.to} }

// On is the chain with target as the commands' target from here on.
func (o CommandsOn) On(target Target) CommandsOn { return CommandsOn{o.c, target} }

// Commands is the chain, for adding commands with a target of their own.
func (o CommandsOn) Commands() Commands { return o.c }

func (o CommandsOn) AddClass(names ...string) CommandsOn {
	return o.with(o.c.AddClass(o.to, names...))
}
func (o CommandsOn) RemoveClass(names ...string) CommandsOn {
	return o.with(o.c.RemoveClass(o.to, names...))
}
func (o CommandsOn) ToggleClass(names ...string) CommandsOn {
	return o.with(o.c.ToggleClass(o.to, names...))
}
func (o CommandsOn) SetAttr(name, value string) CommandsOn {
	return o.with(o.c.SetAttr(o.to, name, value))
}
func (o CommandsOn) RemoveAttr(name string) CommandsOn { return o.with(o.c.RemoveAttr(o.to, name)) }
func (o CommandsOn) ToggleAttr(name, v1, v2 string) CommandsOn {
	return o.with(o.c.ToggleAttr(o.to, name, v1, v2))
}
func (o CommandsOn) Show(opts ...ShowOption) CommandsOn   { return o.with(o.c.Show(o.to, opts...)) }
func (o CommandsOn) Hide(opts ...ShowOption) CommandsOn   { return o.with(o.c.Hide(o.to, opts...)) }
func (o CommandsOn) Toggle(opts ...ShowOption) CommandsOn { return o.with(o.c.Toggle(o.to, opts...)) }
func (o CommandsOn) Transition(during, from, to string, opts ...ShowOption) CommandsOn {
	return o.with(o.c.Transition(o.to, during, from, to, opts...))
}
func (o CommandsOn) Focus() CommandsOn      { return o.with(o.c.Focus(o.to)) }
func (o CommandsOn) FocusFirst() CommandsOn { return o.with(o.c.FocusFirst(o.to)) }
func (o CommandsOn) PushFocus() CommandsOn  { return o.with(o.c.PushFocus(o.to)) }
func (o CommandsOn) PopFocus() CommandsOn   { return o.with(o.c.PopFocus()) }
func (o CommandsOn) Dispatch(event string, opts ...DispatchOption) CommandsOn {
	return o.with(o.c.Dispatch(o.to, event, opts...))
}
func (o CommandsOn) Exec(attr string) CommandsOn { return o.with(o.c.Exec(o.to, attr)) }
func (o CommandsOn) Push(handler any, opts ...PushOption) CommandsOn {
	return o.with(o.c.Push(handler, opts...))
}
func (o CommandsOn) Debounce(d time.Duration) CommandsOn { return o.with(o.c.Debounce(d)) }
func (o CommandsOn) Throttle(d time.Duration) CommandsOn { return o.with(o.c.Throttle(d)) }
func (o CommandsOn) Key(keys ...string) CommandsOn       { return o.with(o.c.Key(keys...)) }
func (o CommandsOn) Confirm(msg string) CommandsOn       { return o.with(o.c.Confirm(msg)) }
func (o CommandsOn) If(cond Conditional) CommandsOn      { return o.with(o.c.If(cond)) }
func (o CommandsOn) ElseIf(cond Conditional) CommandsOn  { return o.with(o.c.ElseIf(cond)) }
func (o CommandsOn) Else() CommandsOn                    { return o.with(o.c.Else()) }
func (o CommandsOn) End() CommandsOn                     { return o.with(o.c.End()) }
func (o CommandsOn) Copy() CommandsOn                    { return o.with(o.c.Copy(o.to)) }
func (o CommandsOn) CopyText(text string) CommandsOn     { return o.with(o.c.CopyText(text)) }
func (o CommandsOn) ScrollTo() CommandsOn                { return o.with(o.c.ScrollTo(o.to)) }
func (o CommandsOn) SetValue(v string) CommandsOn        { return o.with(o.c.SetValue(o.to, v)) }
func (o CommandsOn) SetText(text string) CommandsOn      { return o.with(o.c.SetText(o.to, text)) }
func (o CommandsOn) OnError(chain JSChain) CommandsOn    { return o.with(o.c.OnError(chain)) }
func (o CommandsOn) OnSuccess(chain JSChain) CommandsOn  { return o.with(o.c.OnSuccess(chain)) }
func (o CommandsOn) Timeout(d time.Duration) CommandsOn  { return o.with(o.c.Timeout(d)) }

// Call calls a JavaScript function (see Commands.Call) with the target as
// its context's element.
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
