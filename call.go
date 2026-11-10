package ui

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// Opening a component from markup: JS.Call(component) in an on… attribute,
// ui.Call(ctx, component) in a handler. The component opens in the page's
// overlay host, outside the page's own markup, in the presentation its
// root element gives it (a <ui.Dialog>) or a dialog frame, until it closes.
//
// Vuka's compiler registers each component a Call names (RegisterCall); no
// other can be opened. The step carries a token: the component's name, its
// props as JSON, what it is bound to — a live page's session, else the
// visitor's CSRF token (the session cookie and who is signed in) — and an
// expiry, sealed with the app's form key. A click sends it over the page's
// connection, or over HTTP to the page's own URL on a page that isn't
// live; the server opens what the token says and nothing else. The token
// proves only what the page rendered: the component's Mount still checks
// that the visitor may see what it shows.

// callType is a component a Call can open.
type callType struct {
	id   string
	typ  reflect.Type // the struct
	name string       // "components.EditProduct"
	live bool         // a stateful component: *T is Stateful
}

var callTypes = struct {
	sync.RWMutex
	byType map[reflect.Type]*callType
	byID   map[string]*callType
}{byType: map[reflect.Type]*callType{}, byID: map[string]*callType{}}

var (
	statefulType = reflect.TypeFor[Stateful]()
	actionType   = reflect.TypeFor[Action]()
)

// RegisterCall makes component's type one a Call can open: a struct whose
// value or pointer is a Node, or a stateful component. Vuka's compiler
// calls it for each component a Call in a page's or component's markup
// names, ui.RegisterCall(components.EditProduct{}); an app doesn't.
func RegisterCall(component any) {
	t := reflect.TypeOf(component)
	if t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct || t.Name() == "" || strings.ContainsRune(t.Name(), '[') {
		panic(fmt.Sprintf("ui: RegisterCall takes a component's value, components.EditProduct{}, not %T", component))
	}
	pt := reflect.PointerTo(t)
	live := pt.Implements(statefulType)
	if !live && !t.Implements(nodeType) && !pt.Implements(nodeType) {
		panic(fmt.Sprintf("ui: RegisterCall: %s isn't a component: it renders no markup", t))
	}
	full := t.PkgPath() + "." + t.Name()
	sum := sha256.Sum256([]byte("vuka call\x00" + full))
	ct := &callType{id: base64.RawURLEncoding.EncodeToString(sum[:9]), typ: t, name: t.String(), live: live}
	callTypes.Lock()
	defer callTypes.Unlock()
	callTypes.byType[t] = ct
	callTypes.byID[ct.id] = ct
}

// callTypeOf is v's registered type and v as a struct value.
func callTypeOf(v any) (*callType, reflect.Value, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, rv, fmt.Errorf("ui: JS Call: a nil %T", v)
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return nil, rv, fmt.Errorf("ui: JS Call takes a function's name or a component's value, not %T", v)
	}
	callTypes.RLock()
	ct := callTypes.byType[rv.Type()]
	callTypes.RUnlock()
	if ct == nil {
		return nil, rv, fmt.Errorf("ui: JS Call: %s isn't a component vuka's compiler registered: name it in a Call in a page's or component's markup", rv.Type())
	}
	return ct, rv, nil
}

// A CallOption is how a component opened with Call shows: given with the
// component, JS.Call(component, opts...): As, Into, CallKey; a Size
// among them is one too.
type CallOption func(*callOpts)

// callStep is a Call step's component and options.
type callStep struct {
	comp any
	opts []CallOption
}

func newCallStep(comp any, args []any) *push {
	st := &callStep{comp: comp}
	for _, a := range args {
		if sz, ok := a.(Size); ok {
			a = sizeOption(sz)
		}
		o, ok := a.(CallOption)
		if !ok {
			panic(fmt.Sprintf("ui: JS Call of %T takes CallOptions after the component, not %T: its props go in the component's value", comp, a))
		}
		st.opts = append(st.opts, o)
	}
	return &push{call: st}
}

// A CallHost is a live host that opens components a Call names over the
// page's connection: package live's sessions.
type CallHost interface {
	// CallBinding is what the tokens its pages render are bound to: the
	// session, so another can't use them.
	CallBinding() string
	// CallTarget is the handler id an open names: the event carries the
	// token, and what the browser read.
	CallTarget() string
	// CallAction keeps fn, an Action prop of a Call the render under way
	// shows, and returns its id, which the opened component's prop takes.
	CallAction(ctx context.Context, fn any) string
	// CallOwner is the instance whose render shows the Call: what a
	// component opened Into a region of the page belongs to.
	CallOwner() string
}

const callPurpose = "vk-call"

// CallTTL is how long a Call's token stays good: a page left open longer
// is reloaded before its Calls open. A token is also bound to its page's
// session (see Commands.Call), so it ends with it.
var CallTTL = 12 * time.Hour

// callExpiry is a token's expiry: CallTTL from now, rounded up to the hour,
// so a Call renders the same token through the hour and a live page's
// patches don't resend it.
func callExpiry() string {
	t := time.Now().Add(CallTTL).Truncate(time.Hour).Add(time.Hour)
	return strconv.FormatInt(t.Unix(), 10)
}

// bindingTag is what a token carries of its binding: a digest, so the
// page shows neither the session's nonce nor the visitor's CSRF token in it.
func bindingTag(binding string) string {
	sum := sha256.Sum256([]byte("vuka call binding\x00" + binding))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}

// encodeCall is a Call step's component as the step carries it: "t", its
// token; "reads", the props the browser reads when the step runs; "h",
// the handler a live page's open names, or "csrf" for one over HTTP.
func encodeCall(ctx context.Context, host LiveHost, st *callStep) ([]jsArg, error) {
	ct, v, err := callTypeOf(st.comp)
	if err != nil {
		return nil, err
	}
	probe, _ := host.(*pageProbe)
	ch, _ := host.(CallHost)
	if probe != nil && ct.live {
		probe.Handler(ctx, nil) // a stateful component lives on the page's session
	}
	props, reads, err := encodeCallProps(ctx, host, ch, probe, v)
	if err != nil {
		return nil, fmt.Errorf("ui: JS Call of %s: %w", ct.name, err)
	}
	binding := ""
	switch {
	case ch != nil:
		binding = ch.CallBinding()
	case formHostOf(ctx) != nil:
		binding = formHostOf(ctx).FormToken(ctx)
	}
	names := make([]string, 0, len(reads))
	for n := range reads {
		names = append(names, n)
	}
	slices.Sort(names)
	var o callOpts
	o.apply(st.opts)
	if o.Into != "" && ch != nil {
		o.Owner = ch.CallOwner()
	}
	oj, err := marshal(o)
	if err != nil {
		return nil, err
	}
	kvs := []jsArg{{"t", seal(formKeyOf(ctx), callPurpose, bindingTag(binding), ct.id, string(props), strings.Join(names, ","), string(oj), callExpiry())}}
	if len(reads) > 0 {
		kvs = append(kvs, jsArg{"reads", reads})
	}
	if o.Key != "" {
		kvs = append(kvs, jsArg{"key", o.Key})
	}
	if o.Into != "" {
		kvs = append(kvs, jsArg{"into", o.Into})
	}
	if ch != nil {
		kvs = append(kvs, jsArg{"h", ch.CallTarget()})
	} else if h := formHostOf(ctx); h != nil {
		if tok := h.FormToken(ctx); tok != "" {
			kvs = append(kvs, jsArg{"csrf", tok})
		}
	}
	return kvs, nil
}

// encodeCallProps is v's props as a token carries them, a JSON object of
// the fields set by their Go names, and the reads among them.
func encodeCallProps(ctx context.Context, host LiveHost, ch CallHost, probe *pageProbe, v reflect.Value) ([]byte, map[string]any, error) {
	props := map[string]json.RawMessage{}
	reads := map[string]any{}
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)
		if !f.IsExported() || f.Type == liveType || fv.IsZero() {
			continue
		}
		switch {
		case f.Type == actionType || f.Type == jsChainType:
			raw, err := actionProp(ctx, host, ch, probe, f, fv.Interface())
			if err != nil {
				return nil, nil, err
			}
			props[f.Name] = raw
			continue
		case holdsNode(f.Type, 0) || f.Type.Kind() == reflect.Interface && fv.Elem().Type().Implements(nodeType):
			return nil, nil, fmt.Errorf("%s is markup (a ui.Node, a slot or Children), which a Call can't send: pass what it is made from", f.Name)
		case f.Type.Kind() == reflect.Interface:
			if c, ok := fv.Interface().(Conditional); ok {
				reads[f.Name] = map[string]Condition{"$read": c.cond()}
				continue
			}
			if k := fv.Elem().Kind(); k == reflect.Func || k == reflect.Chan {
				return nil, nil, fmt.Errorf("%s holds a %s, which a Call can't send: declare an Action prop for what it does", f.Name, fv.Elem().Type())
			}
		case f.Type.Kind() == reflect.Func || f.Type.Kind() == reflect.Chan || f.Type.Kind() == reflect.UnsafePointer:
			return nil, nil, fmt.Errorf("%s is a %s, which a Call can't send: declare it ui.Action", f.Name, f.Type)
		}
		j, err := marshal(fv.Interface())
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		props[f.Name] = j
	}
	j, err := marshal(props)
	return j, reads, err
}

// holdsNode reports whether a value of type t can hold markup: a ui.Node,
// a func returning one (a slot taking values), or a slice, array, map or
// pointer of them, or a struct with such a field (slots given per element).
func holdsNode(t reflect.Type, depth int) bool {
	if depth > 4 {
		return false
	}
	switch {
	case t == nodeType, t.Kind() == reflect.Interface && t != actionType && t.NumMethod() > 0 && t.Implements(nodeType):
		return true
	case t.Kind() != reflect.Interface && t.Implements(nodeType) && t.Kind() != reflect.Struct:
		return true
	}
	switch t.Kind() {
	case reflect.Func:
		for i := range t.NumOut() {
			if holdsNode(t.Out(i), depth+1) {
				return true
			}
		}
	case reflect.Slice, reflect.Array, reflect.Pointer, reflect.Map:
		return holdsNode(t.Elem(), depth+1)
	case reflect.Struct:
		if t.Implements(nodeType) || reflect.PointerTo(t).Implements(nodeType) {
			return true // a component's value: markup
		}
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() && holdsNode(f.Type, depth+1) {
				return true
			}
		}
	}
	return false
}

// actionProp is an Action prop as a token carries it: {"$js": commands} for
// a chain, or a method a page that isn't live pushes over HTTP; {"$a": id}
// for a func a live page's session keeps (CallHost.CallAction).
func actionProp(ctx context.Context, host LiveHost, ch CallHost, probe *pageProbe, f reflect.StructField, a any) (json.RawMessage, error) {
	if eh, ok := a.(EventHandler); ok {
		a = eh.Fn
	}
	encode := func(c JSChain) (json.RawMessage, error) {
		j, err := EncodeJS(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		return marshal(map[string]json.RawMessage{"$js": j})
	}
	if c, ok := a.(JSChain); ok {
		return encode(c)
	}
	if v := reflect.ValueOf(a); v.Kind() != reflect.Func || v.IsNil() {
		return nil, fmt.Errorf("%s is %T, not a ui.Action: a chain of commands, a method or a closure", f.Name, a)
	}
	switch {
	case ch != nil:
		return marshal(map[string]string{"$a": ch.CallAction(ctx, a)})
	case probe != nil && pushOf(a) == nil:
		probe.Handler(ctx, a) // a closure: the page is live
		return marshal(map[string]string{"$a": ""})
	case pushOf(a) != nil:
		return encode(JS.Push(a))
	}
	return nil, fmt.Errorf("%s is a closure (or a func vuka didn't register), which only a live page keeps: on a page that isn't live pass a method of the page", f.Name)
}

// ErrCallToken is the error of an open whose token the page didn't render
// for this visitor: from another session, changed, expired, or naming a
// component no Call names.
var ErrCallToken = errors.New("ui: the call isn't one this page rendered for this visitor")

// A CallOpen is an open a Call step sent, its token checked: the component
// it names and its props. Package live and package web answer it.
type CallOpen struct {
	ct    *callType
	props map[string]json.RawMessage
	reads map[string]json.RawMessage
	opts  callOpts
}

// Options are the Call's options, As, Into, Key and Size, as the token
// carries them: what CallView takes.
func (c *CallOpen) Options() []CallOption { return c.opts.options() }

// Key is the Call's Key, "" without one.
func (c *CallOpen) Key() string { return c.opts.Key }

// Into is the element the Call opens its component in ("#side"), and the
// instance that component belongs to (a live page's): "" for the overlay
// host.
func (c *CallOpen) Into() (sel, owner string) { return c.opts.Into, c.opts.Owner }

// CallOptions reads opts: the Key, Into and owner they give.
func CallOptions(opts []CallOption) (key, into string) {
	var o callOpts
	o.apply(opts)
	return o.Key, o.Into
}

// ParseCall reads an open: token, the step's; reads, the JSON object of
// the props the browser read ({"Query": "shoes"}), or nil. The token must
// be sealed with ctx's form key, bound to binding — the session's
// CallBinding, or the visitor's CSRF token over HTTP — and unexpired, and
// name a registered component (ErrCallToken); reads may fill only the
// props the token names as reads.
func ParseCall(ctx context.Context, binding, token string, reads json.RawMessage) (*CallOpen, error) {
	parts, ok := unseal(formKeyOf(ctx), callPurpose, token, 6)
	if !ok || !hmac.Equal([]byte(parts[0]), []byte(bindingTag(binding))) {
		return nil, ErrCallToken
	}
	callTypes.RLock()
	ct := callTypes.byID[parts[1]]
	callTypes.RUnlock()
	if ct == nil {
		return nil, ErrCallToken
	}
	c := &CallOpen{ct: ct}
	if json.Unmarshal([]byte(parts[2]), &c.props) != nil || json.Unmarshal([]byte(parts[4]), &c.opts) != nil {
		return nil, ErrCallToken
	}
	if len(reads) > 0 && string(reads) != "null" {
		if err := json.Unmarshal(reads, &c.reads); err != nil {
			return nil, badPushCall("the call's reads aren't a JSON object")
		}
	}
	allowed := strings.Split(parts[3], ",")
	for name := range c.reads {
		if parts[3] == "" || !slices.Contains(allowed, name) {
			return nil, badPushCall(fmt.Sprintf("the call reads no prop %s", name))
		}
	}
	return c, nil
}

// Component is the component's type as an error names it,
// "components.EditProduct".
func (c *CallOpen) Component() string { return c.ct.name }

// Stateful reports whether the component is a stateful one, which lives
// on a live page's session.
func (c *CallOpen) Stateful() bool { return c.ct.live }

// Value is a new value of the component with its props: a pointer to it.
// An Action prop given a func on a live page is resolve's, by the id the
// token carries; nil resolve takes none.
func (c *CallOpen) Value(resolve func(id string) (any, bool)) (any, error) {
	p := reflect.New(c.ct.typ)
	v := p.Elem()
	for name, raw := range c.props {
		f, ok := c.ct.typ.FieldByName(name)
		if !ok || !f.IsExported() || len(f.Index) != 1 {
			return nil, ErrCallToken
		}
		fv := v.Field(f.Index[0])
		if f.Type == actionType || f.Type == jsChainType {
			var a struct {
				JS json.RawMessage `json:"$js"`
				ID *string         `json:"$a"`
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return nil, ErrCallToken
			}
			switch {
			case a.JS != nil:
				fv.Set(reflect.ValueOf(rawCommands(a.JS)))
			case a.ID != nil && resolve != nil && f.Type == actionType:
				fn, ok := resolve(*a.ID)
				if !ok {
					return nil, fmt.Errorf("ui: %s's %s is gone from the page that opened it: reload the page", c.ct.name, name)
				}
				fv.Set(reflect.ValueOf(fn))
			default:
				return nil, ErrCallToken
			}
			continue
		}
		if err := json.Unmarshal(raw, fv.Addr().Interface()); err != nil {
			return nil, fmt.Errorf("ui: %s's %s: %w", c.ct.name, name, err)
		}
	}
	for name, raw := range c.reads {
		f, ok := c.ct.typ.FieldByName(name)
		if !ok || len(f.Index) != 1 || f.Type.Kind() != reflect.Interface {
			return nil, ErrCallToken
		}
		var x any
		if err := json.Unmarshal(raw, &x); err != nil {
			return nil, badPushCall("the call's reads aren't JSON")
		}
		switch x.(type) {
		case nil, string, bool:
		default:
			return nil, badPushCall(fmt.Sprintf("the call's read of %s isn't a string or a bool", name))
		}
		if x != nil {
			v.Field(f.Index[0]).Set(reflect.ValueOf(x))
		}
	}
	return p.Interface(), nil
}

// OnErrorOf is a component's OnError prop when it is an Action that is set:
// what runs when its Mount fails. Nil otherwise.
func OnErrorOf(comp any) any {
	v := reflect.ValueOf(comp)
	for v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName("OnError")
	if !f.IsValid() || f.Type() != actionType || f.IsNil() {
		return nil
	}
	return f.Interface()
}

// viewOf is what a stateless component renders: its View, else itself.
func viewOf(comp any) Node {
	if v, ok := comp.(interface{ View() Node }); ok {
		return v.View()
	}
	if n, ok := comp.(Node); ok {
		return n
	}
	if rv := reflect.ValueOf(comp); rv.Kind() == reflect.Pointer && !rv.IsNil() {
		return viewOf(rv.Elem().Interface())
	}
	return nil
}

// InjectFields gives comp, a pointer to a component a Call opens, its
// dependencies: each unexported field of a pointer or interface type, not
// set, that ctx's Injector fills, as a page's dependency fields are.
func InjectFields(ctx context.Context, comp any) error {
	inj := InjectorOf(ctx)
	v := reflect.ValueOf(comp)
	if inj == nil || v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	v = v.Elem()
	for i := range v.NumField() {
		f, fv := v.Type().Field(i), v.Field(i)
		if f.IsExported() || f.Anonymous || fv.Kind() != reflect.Pointer && fv.Kind() != reflect.Interface || !fv.IsNil() {
			continue
		}
		fill := inj.Inject(f.Type)
		if fill == nil {
			continue
		}
		x, err := fill(ctx)
		if err != nil {
			return err
		}
		if x.IsValid() {
			reflect.NewAt(f.Type, unsafe.Pointer(fv.UnsafeAddr())).Elem().Set(x)
		}
	}
	return nil
}

// RenderCall renders comp, a component's value a CallOpen made, opened on
// a page that isn't live, in its presentation (CallView) as key's: its
// dependencies injected (InjectFields), a stateful one mounted first. failed is its Mount's error, or its
// render's, and the HTML then the frame's error state.
func RenderCall(ctx context.Context, key string, comp any, opts ...CallOption) (html string, failed error) {
	var view Node
	failed = InjectFields(ctx, comp)
	s, live := comp.(Stateful)
	switch {
	case failed != nil:
	case live:
		if failed = CheckStateful(s); failed == nil {
			fillStatic(ctx, s, s, "")
			if failed = MountLive(ctx, s); failed == nil {
				loadNow(ctx, s)
				view = s.Render()
			}
		}
	default:
		view = viewOf(comp)
	}
	ctx = WithEager(ctx)
	var b bytes.Buffer
	if failed == nil {
		if failed = renderHTML(ctx, &b, CallView(key, view, nil, nil, opts...)); failed == nil {
			return b.String(), nil
		}
		b.Reset()
	}
	if err := renderHTML(ctx, &b, CallView(key, nil, nil, failed, opts...)); err != nil {
		return "", errors.Join(failed, err)
	}
	return b.String(), failed
}

type callRootKey struct{}

// CallView is an opened component's view in its presentation: opts' As,
// else the component's root's when it is a Presentation (a Dialog, a
// Drawer…), else a dialog frame around it; in place, without one, Into a
// region; the frame's error state when failed (its Mount's error). Its
// root carries data-vk-call naming it key, data-vk-present the kind of
// presentation the runtime shows (modal, popover, toast, inline), and
// close, when not nil, is the handler its closing calls (a live page's).
func CallView(key string, view Node, close any, failed error, opts ...CallOption) Node {
	var o callOpts
	o.apply(opts)
	return ctxNode(func(ctx context.Context) Node {
		if v, ok := view.(interface{ View() Node }); ok {
			if _, own := view.(Presentation); !own {
				view = v.View()
			}
		}
		attrs := []Attr{{"data-vk-call", key}}
		if close != nil {
			attrs = append(attrs, Attr{"onVkclose", On(close)})
		}
		if o.Size != "" {
			attrs = append(attrs, Attr{"data-vk-size", o.Size})
		}
		root, own := rootPresentation(view)
		parts := presentParts{Children: view}
		if own {
			parts = root.parts()
		}
		as, err := o.as()
		if err != nil {
			return Try(nil, err)
		}
		switch {
		case failed != nil:
			parts = presentParts{Children: El("p", []Attr{{"class", "vk-call__error"}, {"role", "alert"}}, Text(tr(ctx, "call.failed")))}
			if o.Into != "" {
				return presentation("section", "vk-call-inline vk-call--failed", append(attrs, Attr{"data-vk-present", "inline"}), ctx, parts, true)
			}
			return El("dialog", append(attrs, Attr{"class", "vk-call vk-call--failed"}, Attr{"data-vk-present", "modal"}),
				callClose(ctx, "vk-call__close"), parts.Children)
		case o.Into != "":
			return presentation("section", "vk-call-inline", append(attrs, Attr{"data-vk-present", "inline"}), ctx, parts, true)
		case as != nil:
			view = as.withParts(parts)
		case !own:
			return El("dialog", append(attrs, Attr{"class", "vk-call"}, Attr{"data-vk-present", "modal"}),
				callClose(ctx, "vk-call__close"),
				El("div", []Attr{{"class", "vk-call__body"}}, view))
		}
		return withContext(func(ctx context.Context) context.Context {
			return context.WithValue(ctx, callRootKey{}, attrs)
		}, view)
	})
}

// callClose is a close button.
func callClose(ctx context.Context, class string) Node {
	return El("button", []Attr{{"type", "button"}, {"class", class}, {"aria-label", tr(ctx, "call.close")}, {"onClick", JS.Close()}}, Text("×"))
}

// callRootAttrs takes the attributes a Call's presentation puts on the
// root CallView found it is: the opened component's, read once.
func callRootAttrs(ctx context.Context) ([]Attr, context.Context) {
	attrs, _ := ctx.Value(callRootKey{}).([]Attr)
	if attrs == nil {
		return nil, ctx
	}
	return attrs, context.WithValue(ctx, callRootKey{}, []Attr(nil))
}

// A Caller opens and closes components from the server (ui.Call,
// ui.Close): a live page's session for its handlers, the push of a page
// that isn't live for its method. ctx carries it (WithCaller).
type Caller interface {
	OpenCall(ctx context.Context, component any, opts []CallOption) error
	CloseCall(ctx context.Context)
}

type callerKey struct{}

// WithCaller is ctx whose ui.Call and ui.Close c answers.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// ErrNoCaller is ui.Call's error outside an event's handler or a push.
var ErrNoCaller = errors.New("ui: Call opens a component from an event's handler or a pushed method")

// Call opens component as JS.Call(component) in the page does, from the
// handler of an event or a pushed method: once the reply is applied, the
// component shows in the page's overlay host. On a live page it opens on
// the session, its Mount run now (its error is Call's); on a page that
// isn't live the reply carries the Call, and the browser opens it over
// HTTP, as a click would.
func Call(ctx context.Context, component any, opts ...CallOption) error {
	c, _ := ctx.Value(callerKey{}).(Caller)
	if c == nil {
		return ErrNoCaller
	}
	return c.OpenCall(ctx, component, opts)
}

// Close closes the component opened with Call whose handler (or pushed
// method) is running, the nearest around the element the event came from,
// once the reply is applied: the instance is destroyed. Outside one it
// does nothing.
func Close(ctx context.Context) {
	if c, _ := ctx.Value(callerKey{}).(Caller); c != nil {
		c.CloseCall(ctx)
	}
}

// pushCaller answers ui.Call and ui.Close in a method a page that isn't
// live pushes: its reply opens the components (in "run") and closes the
// component the push came from ("close").
type pushCaller struct {
	mu    sync.Mutex
	run   Commands
	close bool
}

func (p *pushCaller) OpenCall(ctx context.Context, component any, opts []CallOption) error {
	if _, _, err := callTypeOf(component); err != nil {
		return err
	}
	args := make([]any, len(opts))
	for i, o := range opts {
		args[i] = o
	}
	p.mu.Lock()
	p.run = p.run.Call(component, args...)
	p.mu.Unlock()
	return nil
}

func (p *pushCaller) CloseCall(context.Context) {
	p.mu.Lock()
	p.close = true
	p.mu.Unlock()
}

// reply adds what the method asked to res, the call's reply.
func (p *pushCaller) reply(ctx context.Context, res any) (any, error) {
	m, ok := res.(map[string]any)
	if !ok || len(p.run.ops) == 0 && !p.close {
		return res, nil
	}
	if p.close {
		m["close"] = true
	}
	if len(p.run.ops) > 0 {
		j, err := EncodeJS(ctx, p.run)
		if err != nil {
			return nil, err
		}
		m["run"] = json.RawMessage(j)
	}
	return m, nil
}

// CallValue is a new copy of component, a pointer to it, for ui.Call to
// open: an error unless its type is one vuka's compiler registered.
func CallValue(component any) (any, error) {
	_, v, err := callTypeOf(component)
	if err != nil {
		return nil, err
	}
	p := reflect.New(v.Type())
	p.Elem().Set(v)
	return p.Interface(), nil
}
