package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"

	"github.com/vuka-lang/vuka/v2/diag"
)

// Live, embedded by value in a struct whose pointer has a Render() Node method,
// makes the struct a stateful component:
//
//	type Counter struct {
//		ui.Live
//		Start int // exported fields are props, set from the tag's attributes
//		n     int // unexported fields are state
//	}
//
// Rendered without a live session (ui.String, Handler, …) a stateful
// component mounts and renders once, as plain HTML. Under a session (package
// live) it is an instance that keeps its state between renders and answers
// the events of the elements it renders.
type Live struct {
	inst  LiveInstance
	loads *Loads // a page backed by its island: its deferred fields
}

// LiveInstance is a session's side of a component instance. Package live
// implements it.
type LiveInstance interface {
	ID() string
	Subscribe(topics ...string)
	PushJS(c Commands)
}

// ID is the instance's id in its session, the value of its root element's
// data-vk-id; "" when rendered without one.
func (l *Live) ID() string {
	if l.inst == nil {
		return ""
	}
	return l.inst.ID()
}

// Subscribe asks the session for the messages published on topics, delivered
// to the component's Update methods. Without a session it does nothing.
func (l *Live) Subscribe(topics ...string) {
	if l.inst != nil {
		l.inst.Subscribe(topics...)
	}
}

// PushJS runs commands in the browser once the reply to the current event
// (or message, or the join, from Mount) is applied, This being the
// component's root element and refs its own. Without a session it does
// nothing.
func (l *Live) PushJS(ch JSChain) {
	if l.inst != nil {
		l.inst.PushJS(ch.jsChain())
	}
}

// PushPatch moves the page to href once the reply to the current event (or
// message) is applied, as JS.Patch does: the same live route with other
// parameters, the page staying mounted when an Update of its takes the URL
// (see live.Session.Navigate).
func (l *Live) PushPatch(href string) { l.PushJS(JS.Patch(href)) }

// PushNavigate moves the page to href once the reply to the current event
// (or message) is applied, as JS.Navigate does.
func (l *Live) PushNavigate(href string) { l.PushJS(JS.Navigate(href)) }

// PushEvent hands payload to the component's scripts once the reply to the
// current event (or message, or the join) is applied: a hook's
// ctx.on(name, fn) gets fn(payload), payload as JSON. In the browser it is a
// "vk:event" CustomEvent fired on the component's root, not bubbling, its
// detail {name, payload}.
func (l *Live) PushEvent(name string, payload any) {
	l.PushJS(JS.Dispatch(This, "vk:event", Detail(map[string]any{"name": name, "payload": payload}), NoBubble()))
}

func (l *Live) vukaLive() *Live { return l }

// Stateful is a stateful component: a pointer to a struct embedding Live, with
// a Render method.
type Stateful interface {
	Render() Node
	vukaLive() *Live
}

// AttachLive binds a component to its session instance. Package live calls it.
func AttachLive(c Stateful, inst LiveInstance) { c.vukaLive().inst = inst }

// ComponentNode is a stateful component's tag, <Counter Start={5} />: Props is
// a fresh value carrying the attributes, never mounted itself. Site (the tag's
// place in the source), Key and the instance rendering it identify the
// instance a session keeps.
//
// A tag sets its props on the instance each time it renders: a parent owns
// the exported fields of the components it renders. A routed page's tag
// (Routed, what web.Live renders) has no parent: the page owns its exported
// fields once mounted, so the tag rendered again sets none of them, and a new
// tag in its place, a patch to another URL of the route, sets only URL, the
// fields its URL binds. URLKey (a tag's urlKey attribute, ui.URLKey) is the
// name the instance keeps its @ui.URL fields' parameters under.
type ComponentNode struct {
	Site   string
	Key    any
	Props  Stateful
	Routed bool
	URL    []string
	URLKey string
}

// Component is a stateful component's tag.
func Component(site string, key any, props Stateful) Node {
	return &ComponentNode{Site: site, Key: key, Props: props}
}

// Key is a stateful component with a key, as a key attribute on its tag is:
// the instances in one place are told apart by their keys, unkeyed ones by
// their order.
//
//	ui.Each(items, func(it Item) ui.Node { return ui.Key(it.ID, &Counter{Label: it.Name}) })
func Key(key any, c Stateful) Node { return &ComponentNode{Key: key, Props: c} }

func (n *ComponentNode) Render(ctx context.Context, w io.Writer) error {
	if h := liveHost(ctx); h != nil {
		KeyStates(ctx, n.Props)
		return h.Component(ctx, n, w)
	}
	c, err := n.static(ctx)
	if err != nil || c == nil {
		return err
	}
	if r := c.Render(); r != nil {
		return r.Render(ctx, w)
	}
	return nil
}

// static is the instance a render without a session uses: a copy of Props,
// mounted.
func (n *ComponentNode) static(ctx context.Context) (Stateful, error) {
	if n.Props == nil {
		return nil, nil
	}
	if err := CheckStateful(n.Props); err != nil {
		return nil, err
	}
	c := CopyLive(n.Props)
	fillStatic(ctx, c, n.Props, n.URLKey)
	return c, MountLive(ctx, c)
}

// CheckStateful reports a stateful component that embeds *Live rather than
// Live, which has no instance to attach.
func CheckStateful(c Stateful) error {
	if c.vukaLive() == nil {
		return diag.New("E0329", "%T embeds *ui.Live", c).WithHelp("embed ui.Live by value")
	}
	return nil
}

// CopyLive is a shallow copy of c.
func CopyLive(c Stateful) Stateful {
	v := reflect.ValueOf(c)
	cp := reflect.New(v.Type().Elem())
	cp.Elem().Set(v.Elem())
	return cp.Interface().(Stateful)
}

// MountLive gives c's zero ui.State fields their states, then runs c's
// mount hook, the one its LiveSchema sets (@ui.Mount), after checking its
// hooks as MountPage does.
func MountLive(ctx context.Context, c Stateful) error {
	initStates(reflect.ValueOf(c))
	return MountPage(ctx, c)
}

// EventHandler is an on… attribute's expression on an element: onClick={c.Inc}.
// Under a live session it renders as data-vk-on-<event>="<handler id>" and the
// session calls Fn when the browser reports the event; without one it renders
// nothing. A chain of JS commands (Commands, CommandsOn) renders as
// data-vk-js-<event>, with or without a session; a templ.ComponentScript as
// templ renders it.
type EventHandler struct{ Fn any }

// On is an on… attribute's expression.
func On(fn any) EventHandler { return EventHandler{fn} }

// EventName is the DOM event an on… attribute names: onClick is click,
// onKeyDown keydown.
func EventName(attr string) string { return strings.ToLower(strings.TrimPrefix(attr, "on")) }

// Event is what the browser reports of an event, and what a handler taking a
// ui.Event receives. Target is the handler's id; Type the DOM event; Value
// the target element's value (for a checkbox, "true" or "false"); Key the key
// of a keyboard event; Form the fields of the form a submit event comes from.
type Event struct {
	Target string          `json:"target"`
	Type   string          `json:"event,omitempty"`
	Value  string          `json:"value,omitempty"`
	Key    string          `json:"key,omitempty"`
	Form   url.Values      `json:"form,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"` // an island emit's payload (arbitrary JSON)
	// Args are a Push step's arguments, in order, when it passes any
	// (JSON values; a read's text); Form then carries the enclosing
	// form's fields when the handler's struct takes them.
	Args []json.RawMessage `json:"args,omitempty"`
}

// LiveHost renders stateful components and registers event handlers for a
// live session. Package live implements it; ctx carries it.
type LiveHost interface {
	Component(ctx context.Context, n *ComponentNode, w io.Writer) error
	Handler(ctx context.Context, fn any) string
	Keyed(ctx context.Context, key any) context.Context
}

type hostKey struct{}

// WithLiveHost is ctx carrying h.
func WithLiveHost(ctx context.Context, h LiveHost) context.Context {
	return context.WithValue(ctx, hostKey{}, h)
}

func liveHost(ctx context.Context) LiveHost {
	if ctx == nil {
		return nil
	}
	h, _ := ctx.Value(hostKey{}).(LiveHost)
	return h
}

// elementKey is an element's key attribute, which scopes the instances of the
// stateful components inside it.
func elementKey(e *Element) (any, bool) {
	for _, a := range e.Attrs {
		if a.Name == "key" {
			return a.Value, true
		}
	}
	return nil, false
}

// handlerAttr writes an event handler's attribute, or nothing without a session.
func (h *HTMLRenderer) handlerAttr(name string, eh EventHandler) error {
	if eh.Fn == nil {
		return nil
	}
	if err := CheckHandler(eh.Fn); err != nil {
		return fmt.Errorf("ui: %s %w", name, err)
	}
	host := liveHost(h.ctx)
	if host == nil {
		return nil
	}
	id := host.Handler(h.ctx, eh.Fn)
	if _, probe := host.(*pageProbe); probe {
		return errPageLive
	}
	h.write(` data-vk-on-` + EventName(name) + `="`)
	h.write(id)
	h.write(`"`)
	return nil
}

var (
	ctxType = reflect.TypeFor[context.Context]()
	errType = reflect.TypeFor[error]()
)

// CheckPush reports a value a Push step can't take: a func whose
// parameters are a context.Context (first), the step's arguments (one
// struct at most) and what the app injects (pointers and interfaces),
// returning nothing, an error, or a result and maybe an error (see
// Commands.Push).
func CheckPush(fn any) error {
	_, err := handlerShape(fn)
	return err
}

// CheckHandler reports a value an on… attribute can't take: a handler is a
// func returning nothing or an error, its parameters filled by type (see
// Injector): a context.Context, the event's payload — its value (a
// string, number or bool), its form (a struct or pointer to one, or
// url.Values) or the Event — once, and what the app injects (a pointer or
// interface: the request, a service). An app checks its pages' handlers'
// injected parameters when it starts (CheckParams); a call reports one
// nothing fills.
func CheckHandler(fn any) error {
	t := reflect.TypeOf(fn)
	ok := t != nil && t.Kind() == reflect.Func && !t.IsVariadic() && t.NumOut() <= 1 && (t.NumOut() == 0 || t.Out(0) == errType)
	payloads := 0
	for i := 0; ok && i < t.NumIn(); i++ {
		switch pt := t.In(i); {
		case pt == ctxType || pt.Kind() == reflect.Pointer || pt.Kind() == reflect.Interface:
		case payloadType(pt):
			payloads++
		default:
			ok = false
		}
	}
	if ok && payloads <= 1 {
		return nil
	}
	return fmt.Errorf("takes a func returning nothing or an error, taking the event's value (a string, number or bool), a form (a struct, or url.Values) or a ui.Event once, a context.Context, and what the app injects; this is %T", fn)
}
