package ui

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"
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
type Live struct{ inst LiveInstance }

// LiveInstance is a session's side of a component instance. Package live
// implements it.
type LiveInstance interface {
	ID() string
	Subscribe(topics ...string)
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
type ComponentNode struct {
	Site  string
	Key   any
	Props Stateful
}

// Component is a stateful component's tag.
func Component(site string, key any, props Stateful) Node {
	return &ComponentNode{Site: site, Key: key, Props: props}
}

func (n *ComponentNode) Render(ctx context.Context, w io.Writer) error {
	if h := liveHost(ctx); h != nil {
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
	return c, MountLive(ctx, c)
}

// CheckStateful reports a stateful component that embeds *Live rather than
// Live, which has no instance to attach.
func CheckStateful(c Stateful) error {
	if c.vukaLive() == nil {
		return fmt.Errorf("ui: %T embeds *ui.Live; embed ui.Live by value", c)
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

// MountLive runs c's Mount method, if it has one: Mount(), Mount() error,
// Mount(context.Context) or Mount(context.Context) error.
func MountLive(ctx context.Context, c Stateful) error {
	switch m := c.(type) {
	case interface{ Mount(context.Context) error }:
		return m.Mount(ctx)
	case interface{ Mount(context.Context) }:
		m.Mount(ctx)
	case interface{ Mount() error }:
		return m.Mount()
	case interface{ Mount() }:
		m.Mount()
	default:
		if m := reflect.ValueOf(c).MethodByName("Mount"); m.IsValid() {
			return fmt.Errorf("ui: %T's Mount is %s; it takes nothing or a context.Context, and returns nothing or an error", c, m.Type())
		}
	}
	return nil
}

// EventHandler is an on… attribute's expression on an element: onClick={c.Inc}.
// Under a live session it renders as data-vk-on-<event>="<handler id>" and the
// session calls Fn when the browser reports the event; without one it renders
// nothing. A templ.ComponentScript renders as templ renders it.
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
	Target string     `json:"target"`
	Type   string     `json:"event,omitempty"`
	Value  string     `json:"value,omitempty"`
	Key    string     `json:"key,omitempty"`
	Form   url.Values `json:"form,omitempty"`
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
	h.write(` data-vk-on-` + EventName(name) + `="`)
	h.write(host.Handler(h.ctx, eh.Fn))
	h.write(`"`)
	return nil
}

var (
	ctxType = reflect.TypeFor[context.Context]()
	errType = reflect.TypeFor[error]()
)

// CheckHandler reports a value an on… attribute can't take: a handler is a
// func(), or a func taking the event's value (a string, number or bool), a
// form (a struct or pointer to one, or url.Values) or an Event, optionally
// after a context.Context, returning nothing or an error.
func CheckHandler(fn any) error {
	t := reflect.TypeOf(fn)
	ok := t != nil && t.Kind() == reflect.Func && !t.IsVariadic() && t.NumOut() <= 1 && (t.NumOut() == 0 || t.Out(0) == errType)
	if ok {
		i := 0
		if t.NumIn() > 0 && t.In(0) == ctxType {
			i = 1
		}
		ok = t.NumIn()-i == 0 || t.NumIn()-i == 1 && payloadOK(t.In(i))
	}
	if ok {
		return nil
	}
	return fmt.Errorf("takes a func(), a func taking the event's value (a string, number or bool), a form (a struct, or url.Values) or a ui.Event, optionally after a context.Context, returning nothing or an error; this is %T", fn)
}

func payloadOK(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.Struct:
		return true
	case reflect.Pointer:
		return t.Elem().Kind() == reflect.Struct
	case reflect.Map:
		return t.Key().Kind() == reflect.String && t.Elem().Kind() == reflect.Slice && t.Elem().Elem().Kind() == reflect.String
	}
	return false
}
