package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// State is a value the browser keeps: a menu's open flag, the tab picked,
// a filter's text. Commands change it at once, with no round trip, and the
// parts of the page that read it follow.
//
// In a .vuka file, ui.State(v) declares one; the compiler gives each
// declaration its place, so two components using the same variable name
// never share a state. Declared in a component's function it is that
// instance's own (a component rendered ten times has ten); declared at
// package level it is the page's, one value every component reads.
//
//	open := ui.State(false)
//	<button onClick={open.Toggle()} aria-expanded={open}>Menu</button>
//	<ul hidden={!open}>…</ul>
//
// Writes are commands, chained with the rest of ui.JS: Set, Toggle (a
// State[bool]), Inc and Dec (numbers), Reset. Reads are expressions in JSX
// attributes, text and ui.Class that the compiler turns into ones the
// browser evaluates (see Bind); the server renders them with the initial
// value, so the page is right before any script runs.
//
// A struct state's fields are states of their own: f.Q.Set("x") changes
// one field, and what reads f.Q follows only f.Q. A state passed as a prop
// is the same state; Get passes its value.
//
// The server knows a state's initial value only (Get); what the browser
// changed stays in the browser. A component pushes changes with PushJS:
// c.PushJS(tab.Set("env")).
type State[T any] struct {
	c    *stateCell
	path []string // a struct state's field, by Go field names
	v    T        // the value it renders with
}

// stateCell is a state's identity: where it is declared, its initial value,
// and the key the browser knows it by, given on its first render.
type stateCell struct {
	site    string
	page    bool
	init    any
	typ     reflect.Type
	derived *Binding // a Derived's expression
	mu      sync.Mutex
	key     string
}

// NewState is a state declared at site, a name unique in the program: what
// ui.State(initial) in a .vuka file becomes. Each value NewState returns is
// a state of its own; the browser knows it by site, the component instance
// rendering it and how many of site's states that instance rendered before.
func NewState[T any](site string, initial T) State[T] {
	return State[T]{c: &stateCell{site: site, init: initial, typ: reflect.TypeFor[T]()}, v: initial}
}

// PageState is a state the whole page shares, declared at site: what a
// package-level ui.State(initial) becomes. Every component reading it reads
// one value.
func PageState[T any](site string, initial T) State[T] {
	return State[T]{c: &stateCell{site: site, page: true, init: initial, typ: reflect.TypeFor[T](), key: "g:" + site}, v: initial}
}

// Derived names an expression of states for reuse, read wherever a state
// is: the compiler turns ui.Derived(!open && tab == "env") into NewDerived.
// Without the compiler it is a state that never changes.
func Derived[T any](value T) State[T] { return NewDerived(value, `{"$":0}`, value) }

// NewDerived is a derived state: value is what it renders with, expr and
// deps its expression, as Bind takes them.
func NewDerived[T any](value T, expr string, deps ...any) State[T] {
	return State[T]{c: &stateCell{init: value, typ: reflect.TypeFor[T](), derived: &Binding{value, expr, deps}}, v: value}
}

// StateField is the field name of a struct state: what f.Q becomes outside
// JSX. sample is the field's value as rendered (f.Get().Q), which also gives
// its type.
func StateField[S, T any](s State[S], name string, sample T) State[T] {
	if s.c != nil && s.c.derived != nil {
		panic("ui: a field of a ui.Derived isn't a state")
	}
	return State[T]{c: s.c, path: append(s.path[:len(s.path):len(s.path)], name), v: sample}
}

// Get is the state's value as the server renders it: its initial value.
// Passed as a prop, it gives the child the value rather than the state.
func (s State[T]) Get() T { return s.v }

// Set is the command setting the state to v.
//
// Example:
//
//	<button onClick={tab.Set("env")}>Environment</button>
func (s State[T]) Set(v T) Commands { return JS.Set(s, v) }

// Toggle is the command flipping a State[bool]. The compiler refuses it on
// any other; built at run time it panics.
//
// Example:
//
//	<button onClick={open.Toggle()}>Menu</button>
func (s State[T]) Toggle() Commands {
	return JS.add("state_toggle", jsArg{"$state", s.view("Toggle", "bool")})
}

// Inc is the command adding one to a number state; Dec subtracts one. The
// compiler refuses them on any other state.
//
// Example:
//
//	<button onClick={n.Inc()}>+</button>
func (s State[T]) Inc() Commands { return s.inc("Inc", 1) }

// Dec is the command subtracting one from a number state.
func (s State[T]) Dec() Commands { return s.inc("Dec", -1) }

func (s State[T]) inc(name string, by int) Commands {
	return JS.add("state_inc", jsArg{"$state", s.view(name, "number")}, jsArg{"by", by})
}

// Reset is the command setting the state back to its initial value.
//
// Example:
//
//	<button onClick={f.Reset()}>Clear filters</button>
func (s State[T]) Reset() Commands { return JS.Set(s, s.v) }

// AnyState is a State of any type: what Commands.Set takes.
type AnyState interface{ stateOf() stateView }

// stateView is a state with its type erased: a command's or a binding's
// reference to it.
type stateView struct {
	c    *stateCell
	path []string
	v    any
	t    reflect.Type
}

func (s State[T]) stateOf() stateView {
	return stateView{s.c, s.path, s.v, reflect.TypeFor[T]()}
}

// view is s for a write named op; kind ("bool", "number" or "") is what
// the state must be.
func (s State[T]) view(op, kind string) stateView {
	v := s.stateOf()
	v.writable(op)
	k := v.t.Kind()
	switch {
	case kind == "bool" && k != reflect.Bool, kind == "number" && !numeric(k):
		panic(fmt.Sprintf("ui: %s needs a ui.State of a %s, not ui.State[%s]", op, kind, v.t))
	}
	return v
}

func (v stateView) writable(op string) {
	switch {
	case v.c == nil:
		panic("ui: " + op + " on a zero ui.State; declare it with ui.State(v)")
	case v.c.derived != nil:
		panic("ui: " + op + " on a ui.Derived; set the states it reads")
	}
}

func numeric(k reflect.Kind) bool { return k >= reflect.Int && k <= reflect.Float64 }

// Set is the command setting state s to v, which must be of its type (a
// number converts to a number state's type).
//
// Example:
//
//	<button onClick={tab.Set("env").Set(open, false)}>Environment</button>
func (c Commands) Set(s AnyState, v any) Commands {
	sv := s.stateOf()
	sv.writable("Set")
	rv := reflect.ValueOf(v)
	switch {
	case !rv.IsValid():
		switch sv.t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		default:
			panic(fmt.Sprintf("ui: Set: nil isn't a %s", sv.t))
		}
	case rv.Type().AssignableTo(sv.t):
	case numeric(rv.Kind()) && numeric(sv.t.Kind()):
		v = rv.Convert(sv.t).Interface()
	default:
		panic(fmt.Sprintf("ui: Set: a ui.State[%s] takes no %T", sv.t, v))
	}
	return c.add("state_set", jsArg{"$state", sv}, jsArg{"v", v})
}

// Set is the command setting state s to v (see Commands.Set); the target
// plays no part.
func (o CommandsOn) Set(s AnyState, v any) CommandsOn { return o.with(o.c.Set(s, v)) }

// StateHost names the states rendered under a live host: package live's
// scope keys them by the instance rendering them. A host that isn't one
// keys them by site and a counter.
type StateHost interface {
	StateKey(ctx context.Context, site string) string
}

var staticStates atomic.Uint64

// keyIn is the cell's key, given the first time it is rendered under ctx.
func (c *stateCell) keyIn(ctx context.Context) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key == "" {
		if sh, ok := liveHost(ctx).(StateHost); ok {
			c.key = sh.StateKey(ctx, c.site)
		} else {
			c.key = c.site + ":" + strconv.FormatUint(staticStates.Add(1), 36)
		}
	}
	return c.key
}

// ref is the state as an expression's or a command's reference to it, at
// path (Go field names) under its own: its key, its whole initial value, and
// the path in the value's JSON. A derived state is its expression.
func (v stateView) ref(ctx context.Context, path []string) (any, error) {
	if v.c == nil {
		return nil, fmt.Errorf("ui: a zero ui.State[%s]; declare it with ui.State(v)", v.t)
	}
	full := append(v.path[:len(v.path):len(v.path)], path...)
	if d := v.c.derived; d != nil {
		if len(full) > 0 {
			return nil, fmt.Errorf("ui: a field of a ui.Derived isn't a state")
		}
		return d.expr(ctx)
	}
	r := map[string]any{"s": v.c.keyIn(ctx), "i": v.c.init}
	if len(full) > 0 {
		p, err := jsonPath(v.c.typ, full)
		if err != nil {
			return nil, err
		}
		r["p"] = p
	}
	return r, nil
}

// jsonPath is a path of Go field names as the keys of the value's JSON.
func jsonPath(t reflect.Type, path []string) ([]string, error) {
	out := make([]string, len(path))
	for i, name := range path {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		f, ok := t.FieldByName(name)
		if t.Kind() != reflect.Struct || !ok {
			return nil, fmt.Errorf("ui: %s has no field %s", t, name)
		}
		key := name
		if tag, _, _ := strings.Cut(f.Tag.Get("json"), ","); tag != "" && tag != "-" {
			key = tag
		}
		out[i], t = key, f.Type
	}
	return out, nil
}

// Binding is a JSX expression that reads states: an attribute's value
// (hidden={!open}), a child ({count}), a ui.Show's when. The compiler
// writes it as Bind; the page renders it with the value Init, and the
// browser evaluates it again whenever a state it reads changes.
type Binding struct {
	Init any    // its value, as rendered
	Expr string // its expression: JSON, with deps as {"$": i}
	Deps []any  // the states it reads, and the server's values it uses
}

// Bind is a binding of an expression of states, as the compiler writes
// hidden={!open}: Bind(!open.Get(), `["!",{"$":0}]`, open).
//
// The expression is JSON: a literal (a string, number, bool or null), a
// dep {"$": i} — a state, with "p" naming a field path, or a value of the
// server's — or an operation ["op", operands…]: "!", "neg", "&&", "||",
// "==", "!=", "<", "<=", ">", ">=", "+", "-", "len", or "class" over an
// object of class names and expressions.
func Bind(init any, expr string, deps ...any) Binding { return Binding{init, expr, deps} }

var parsedExprs sync.Map // expression text → its parsed JSON

// expr is the binding's expression for the browser: each dep resolved, a
// state as its reference (or a derived state's expression), anything else
// as a literal.
func (b Binding) expr(ctx context.Context) (any, error) {
	parsed, ok := parsedExprs.Load(b.Expr)
	if !ok {
		var x any
		if err := json.Unmarshal([]byte(b.Expr), &x); err != nil {
			return nil, fmt.Errorf("ui: bad binding %s: %v", b.Expr, err)
		}
		parsed, _ = parsedExprs.LoadOrStore(b.Expr, x)
	}
	return b.resolve(ctx, parsed)
}

func (b Binding) resolve(ctx context.Context, x any) (any, error) {
	switch x := x.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			r, err := b.resolve(ctx, e)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case map[string]any:
		i, ok := x["$"].(float64)
		if !ok {
			out := make(map[string]any, len(x))
			for k, e := range x {
				r, err := b.resolve(ctx, e)
				if err != nil {
					return nil, err
				}
				out[k] = r
			}
			return out, nil
		}
		if int(i) < 0 || int(i) >= len(b.Deps) {
			return nil, fmt.Errorf("ui: binding %s has no dep %d", b.Expr, int(i))
		}
		var path []string
		if p, ok := x["p"].([]any); ok {
			for _, s := range p {
				path = append(path, fmt.Sprint(s))
			}
		}
		switch d := b.Deps[int(i)].(type) {
		case AnyState:
			return d.stateOf().ref(ctx, path)
		case Binding:
			return d.expr(ctx)
		default:
			return literal(d)
		}
	}
	return x, nil
}

// literal is a server's value in an expression: a string, number, bool or
// null as itself, anything else as {"v": value}.
func literal(v any) (any, error) {
	j, err := marshal(v)
	if err != nil {
		return nil, err
	}
	if len(j) > 0 && (j[0] == '{' || j[0] == '[') {
		return map[string]any{"v": json.RawMessage(j)}, nil
	}
	return json.RawMessage(j), nil
}

// json is the binding's expression as the attribute holding it writes it.
func (b Binding) json(ctx context.Context) (string, error) {
	x, err := b.expr(ctx)
	if err != nil {
		return "", err
	}
	if p, ok := liveHost(ctx).(*pageProbe); ok {
		p.info.JS = true
	}
	j, err := marshal(x)
	return string(j), err
}

// Render writes the binding as a child: its value as text, in a <vk-t> the
// browser keeps up to date.
func (b Binding) Render(ctx context.Context, w io.Writer) error {
	return renderHTML(ctx, w, b.node())
}

func (b Binding) node() Node {
	return El("vk-t", []Attr{{Name: "data-vk-b", Value: bindJSON{b}}}, Text(b.Init))
}

// bindJSON is a binding's expression as an attribute value, written once
// the render's context is known.
type bindJSON struct{ b Binding }

func (j bindJSON) Render(ctx context.Context, w io.Writer) error {
	s, err := j.b.json(ctx)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s)
	return err
}

// bindAttr writes an attribute bound to an expression: its value as
// rendered, then data-vk-b-<name> holding the expression. An aria-*
// attribute writes a bool as "true" or "false".
func (h *HTMLRenderer) bindAttr(tag string, e *Element, name string, b Binding) error {
	switch name {
	case "key", "ref":
		return fmt.Errorf("ui: %s can't read a ui.State", name)
	case "className":
		name = "class"
	case "htmlFor":
		name = "for"
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "on") || lower == "srcdoc" {
		return fmt.Errorf("ui: %s can't read a ui.State", name)
	}
	v := b.Init
	if bv, ok := v.(bool); ok && strings.HasPrefix(lower, "aria-") {
		v = strconv.FormatBool(bv)
	}
	if err := h.attrNamed(tag, e, Attr{Name: name, Value: v}); err != nil {
		return err
	}
	j, err := b.json(h.ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	h.write(` data-vk-b-` + lower + `="`)
	h.writeEscaped(j)
	h.writeByte('"')
	return h.err
}

// Class is a class attribute of names switched on and off: each name (or
// space-separated names) is there when its value is true. Its values may
// read states: class={ui.Class{"bg-brand": tab == "env"}}.
type Class map[string]bool

// ShowProps are what Show takes: When, a bool, a State[bool] or an
// expression of states (the compiler writes a Binding), and the children it
// shows.
type ShowProps struct {
	When     any
	Children Node
}

// Show renders its children always, and shows them in the browser only
// while When holds, as Vue's v-show does. They are in a <vk-show> element
// (display: contents), hidden while When doesn't hold.
//
// Transitions are the children's CSS: while hidden they have data-closed;
// appearing, data-enter; disappearing, data-leave; and they are hidden once
// their transitions or animations end. With Tailwind:
//
//	<ui.Show when={open}>
//		<div class="transition duration-150 data-closed:opacity-0 data-leave:duration-100">…</div>
//	</ui.Show>
//
// A child with the popover attribute is shown with showPopover, in the top
// layer, and hidden with hidePopover, with the same phases.
func Show(p ShowProps) Node {
	open, attr, err := whenOf(p.When)
	if err != nil {
		return Try(nil, err)
	}
	attrs := make([]Attr, 0, 2)
	if attr != nil {
		attrs = append(attrs, Attr{Name: "data-vk-show", Value: attr})
	}
	if open {
		attrs = append(attrs, Attr{Name: "style", Value: "display: contents"})
	} else {
		attrs = append(attrs, Attr{Name: "hidden", Value: true})
	}
	return El("vk-show", attrs, p.Children)
}

func whenOf(w any) (open bool, attr Node, err error) {
	switch w := w.(type) {
	case nil:
		return false, nil, nil
	case bool:
		return w, nil, nil
	case Binding:
		b, _ := w.Init.(bool)
		return b, bindJSON{w}, nil
	case AnyState:
		sv := w.stateOf()
		if sv.t.Kind() != reflect.Bool {
			return false, nil, fmt.Errorf("ui: Show's when is a ui.State[%s], not a bool", sv.t)
		}
		b, _ := sv.v.(bool)
		return b, bindJSON{Bind(b, `{"$":0}`, w)}, nil
	}
	return false, nil, fmt.Errorf("ui: Show's when is %T; it takes a bool, a ui.State[bool] or an expression of states", w)
}

// KeyStates keys the states among a stateful component's props in the
// render ctx is of, its parent's: a state the parent passes is the parent's,
// whether or not the parent's own markup reads it first. Package ui calls
// it before a live host renders a component's tag.
func KeyStates(ctx context.Context, props Stateful) {
	v := reflect.ValueOf(props)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return
	}
	v = v.Elem()
	for _, i := range propStates(v.Type()) {
		if s, ok := v.Field(i).Interface().(AnyState); ok {
			if c := s.stateOf().c; c != nil {
				c.keyIn(ctx)
			}
		}
	}
}

var propStatesOf sync.Map // reflect.Type → []int

// propStates is the indexes of t's exported fields that are states.
func propStates(t reflect.Type) []int {
	if f, ok := propStatesOf.Load(t); ok {
		return f.([]int)
	}
	var out []int
	any := reflect.TypeFor[AnyState]()
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); f.IsExported() && f.Type.Implements(any) {
			out = append(out, i)
		}
	}
	propStatesOf.Store(t, out)
	return out
}
