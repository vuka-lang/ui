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
	"net/url"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Pushing over HTTP (JS.Push on a page that isn't live, or with Read): a
// method of the page or of a component on it, called on the page's own
// URL — no socket — on a fresh copy of the page, its result put Into a
// target.
//
// Vuka's compiler registers each method a Push or Options step names
// (RegisterPush); no other method can be called this way. The step
// carries the method's id signed for the route of the page that rendered
// it (with the app's form key, the same on every replica), so a page can
// only call the methods its own markup — or its components' — names.

// resultKind is what a handler returns besides an error, and so what its
// result does to the push's target.
type resultKind int

const (
	resNone     resultKind = iota // nothing: an event
	resOptions                    // []Option: a select's options
	resGroups                     // []OptionGroup: the same, in <optgroup>s
	resNode                       // Node: the target's content
	resScalar                     // string, number, bool, time.Time: a value or text
	resStruct                     // a struct: the form controls of its fields' names
	resCommands                   // Commands: run
)

// pushMethod is a handler's shape: how a call binds its parameters and
// what its result is. A registered one (a method expression) has an id and
// a receiver; one read from a live handler (a method value, a closure) has
// neither.
type pushMethod struct {
	id     string
	recv   reflect.Type // the receiver as declared: *T or T; nil for a func value
	fn     reflect.Value
	name   string // "Checkout.CitiesOf"
	ctx    bool   // takes a context.Context first
	args   []reflect.Type
	argAt  []int // each argument's parameter, from 0 the receiver excluded
	form   int   // the argument a struct form fills when the step passes none for it; -1
	result resultKind
	errs   bool // returns an error too
}

var pushMethods = struct {
	sync.RWMutex
	byName map[string]*pushMethod // the method's Go name: "pkg.(*T).M"
	byID   map[string]*pushMethod
}{byName: map[string]*pushMethod{}, byID: map[string]*pushMethod{}}

var (
	optionListType   = reflect.TypeFor[[]Option]()
	optionGroupsType = reflect.TypeFor[[]OptionGroup]()
	commandsType     = reflect.TypeFor[Commands]()
	commandsOnType   = reflect.TypeFor[CommandsOn]()
)

// RegisterPush makes method m, a method expression ((*Checkout).CitiesOf),
// callable over HTTP by the Push and Options steps naming it. Vuka's
// compiler calls it for each method such a step in a page's or
// component's markup names; an app doesn't.
//
// The method's parameters after the receiver are a context.Context
// (optional, first), then the step's arguments in order — the parameters
// that aren't pointers or interfaces, one of them a struct at most — and
// the dependencies the app injects (pointers and interfaces). It returns
// nothing, an error, or a result and maybe an error: []Option,
// []OptionGroup, a Node, a string, number, bool or time.Time, a struct,
// or Commands. RegisterPush panics on another shape.
func RegisterPush(m any) {
	pm, err := newPushMethod(reflect.ValueOf(m))
	if err != nil {
		panic(err)
	}
	pushMethods.Lock()
	defer pushMethods.Unlock()
	pushMethods.byName[funcName(pm.fn)] = pm
	pushMethods.byID[pm.id] = pm
}

// RegisterOptions is RegisterPush, as vuka 2.14's compiler named it for
// the methods Options steps name.
//
// Deprecated: the compiler calls RegisterPush.
func RegisterOptions(m any) { RegisterPush(m) }

// funcName is a function's Go name, without a method value's -fm suffix.
func funcName(fn reflect.Value) string {
	f := runtime.FuncForPC(fn.Pointer())
	if f == nil {
		return ""
	}
	return strings.TrimSuffix(f.Name(), "-fm")
}

func newPushMethod(fn reflect.Value) (*pushMethod, error) {
	if fn.Kind() != reflect.Func || fn.IsNil() || fn.Type().NumIn() == 0 {
		return nil, fmt.Errorf("ui: RegisterPush takes a method expression, (*Page).Method, not %v", fn.Type())
	}
	ft := fn.Type()
	recv := ft.In(0)
	base := recv
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	pm := &pushMethod{recv: recv, fn: fn}
	full := funcName(fn)
	pm.name = base.Name() + "." + full[strings.LastIndexByte(full, '.')+1:]
	sum := sha256.Sum256([]byte(full))
	pm.id = base64.RawURLEncoding.EncodeToString(sum[:9])
	if err := pm.shape(ft, 1); err != nil {
		return nil, fmt.Errorf("ui: %s %w", pm.name, err)
	}
	return pm, nil
}

// handlerShape is the shape of fn, a live handler (a method value, a
// closure, a func prop).
func handlerShape(fn any) (*pushMethod, error) {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() {
		return nil, fmt.Errorf("a handler is %T, not a function", fn)
	}
	pm := &pushMethod{fn: v, name: methodName(v)}
	if err := pm.shape(v.Type(), 0); err != nil {
		return nil, fmt.Errorf("%s %w", pm.name, err)
	}
	return pm, nil
}

// resultOf is what a handler's result of type t does.
func resultOf(t reflect.Type) (resultKind, bool) {
	switch {
	case t == optionListType:
		return resOptions, true
	case t == optionGroupsType:
		return resGroups, true
	case t == commandsType || t == commandsOnType:
		return resCommands, true
	case t.Implements(nodeType):
		return resNode, true
	case t == timeType || scalarType(t) || reflect.PointerTo(t).Implements(textUnmarshaler) && t.Kind() != reflect.Struct:
		return resScalar, true
	case t.Kind() == reflect.Struct, t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct && t.Elem() != timeType:
		return resStruct, true
	}
	return resNone, false
}

// formParam reports whether a parameter of type t is a struct a form's
// fields fill: a struct that is a group of fields, not one value.
func formParam(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && !isLeaf(t)
}

// shape reads the method's parameters (those after skip, the receiver)
// and results (see RegisterPush).
func (pm *pushMethod) shape(ft reflect.Type, skip int) error {
	if ft.IsVariadic() {
		return errors.New("is variadic: a pushed method takes its arguments one by one")
	}
	const want = "returns nothing, an error, or a result — []ui.Option, []ui.OptionGroup, a ui.Node, a string, number, bool or time.Time, a struct, ui.Commands — and maybe an error"
	switch n := ft.NumOut(); {
	case n == 0:
	case n == 1 && ft.Out(0) == errType:
		pm.errs = true
	case n <= 2 && (n == 1 || ft.Out(1) == errType):
		k, ok := resultOf(ft.Out(0))
		if !ok {
			return fmt.Errorf("returns %s: a pushed method %s", ft.Out(0), want)
		}
		pm.result, pm.errs = k, n == 2
	default:
		return fmt.Errorf("is %s: a pushed method %s", ft, want)
	}
	pm.form = -1
	for i := skip; i < ft.NumIn(); i++ {
		switch t := ft.In(i); {
		case t == ctxType && i == skip:
			pm.ctx = true
		case t == ctxType:
			return errors.New("takes a context.Context after other parameters: put it first")
		case t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface:
			// a dependency, injected
		case t.Kind() == reflect.Chan || t.Kind() == reflect.Func || t.Kind() == reflect.UnsafePointer:
			return fmt.Errorf("takes a %s, which no argument can be", t)
		default:
			if formParam(t) {
				if pm.form >= 0 {
					return fmt.Errorf("takes two structs, %s and %s: a pushed method takes one struct at most, its form", pm.args[pm.form], t)
				}
				pm.form = len(pm.args)
			}
			pm.args = append(pm.args, t)
			pm.argAt = append(pm.argAt, i-skip)
		}
	}
	return nil
}

// fromForm reports whether a call passing n arguments has its struct
// filled from the enclosing form: it has one, and n is one short.
func (pm *pushMethod) fromForm(n int) bool {
	return pm.form >= 0 && n == len(pm.args)-1
}

// checkCount is the error of a step passing n arguments to pm.
func (pm *pushMethod) checkCount(n int) error {
	if n == len(pm.args) || pm.fromForm(n) {
		return nil
	}
	if pm.form >= 0 {
		return fmt.Errorf("%s takes %d arguments, not %d (or %d, its %s filled from the form)", pm.name, len(pm.args), n, len(pm.args)-1, pm.args[pm.form])
	}
	return fmt.Errorf("%s takes %d arguments, not %d", pm.name, len(pm.args), n)
}

// pushOf is the registered method fn, a step's handler, is: a method value
// of a method vuka registered; nil when it isn't one.
func pushOf(fn any) *pushMethod {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() {
		return nil
	}
	pushMethods.RLock()
	defer pushMethods.RUnlock()
	return pushMethods.byName[funcName(v)]
}

type pushScopeKey struct{}

// WithPushScope is ctx rendering a page whose Push and Options steps are
// signed for scope: the route the page is served under ("/orgs/{org}"),
// so a reference stays good while a live page moves to other parameters
// of it. Without one, the page's path (PageURL) is the scope. Package web
// sets it for every route.
func WithPushScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, pushScopeKey{}, scope)
}

// WithOptionsScope is WithPushScope.
//
// Deprecated: use WithPushScope.
func WithOptionsScope(ctx context.Context, scope string) context.Context {
	return WithPushScope(ctx, scope)
}

// pushScope is what the steps ctx renders are signed for.
func pushScope(ctx context.Context) string {
	if s, _ := ctx.Value(pushScopeKey{}).(string); s != "" {
		return s
	}
	if u := PageURL(ctx); u != nil && u.Path != "" {
		return u.Path
	}
	return "/"
}

// pushRef is the method's reference as the page carries it: its id;
// for a read "r", with how long its reply may be cached in seconds when
// it may ("r60"); and their signature for the page's scope.
func pushRef(ctx context.Context, pm *pushMethod, read bool, cache time.Duration) string {
	id := pm.id
	if read {
		id += ".r"
		if s := int64(cache / time.Second); s > 0 {
			id += strconv.FormatInt(s, 10)
		}
	}
	return id + "." + pushMAC(formKeyOf(ctx), pushScope(ctx), id)
}

func pushMAC(key []byte, scope, id string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("vuka options\x00" + scope + "\x00" + id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18])
}

// ErrPushRef is the error of a call naming a method the page at its path
// doesn't render a step for: a reference from another page, forged, or
// from before a deploy that removed the method.
var ErrPushRef = errors.New("ui: the pushed method isn't one this page calls")

// ErrOptionsRef is ErrPushRef.
//
// Deprecated: use ErrPushRef.
var ErrOptionsRef = ErrPushRef

// A PushCall is a call a Push or Options step sends to its page: the
// method it names and the arguments it read. Package web answers it.
type PushCall struct {
	m     *pushMethod
	args  []json.RawMessage
	form  url.Values
	read  bool
	cache time.Duration
}

// OptionsCall is PushCall.
//
// Deprecated: use PushCall.
type OptionsCall = PushCall

// ParsePushCall reads the call of a step sent to a page: ref, the method's
// reference the page rendered; args, the JSON array of its arguments; form,
// the JSON object of the enclosing form's fields ({"name": ["value"]}), or
// "". The reference must be signed with ctx's form key for the page's
// scope: ctx's (WithPushScope), else its path (PageURL) (ErrPushRef).
func ParsePushCall(ctx context.Context, ref, args, form string) (*PushCall, error) {
	i := strings.LastIndexByte(ref, '.')
	if i < 0 {
		return nil, ErrPushRef
	}
	id, sig := ref[:i], ref[i+1:]
	if !hmac.Equal([]byte(sig), []byte(pushMAC(formKeyOf(ctx), pushScope(ctx), id))) {
		return nil, ErrPushRef
	}
	c := &PushCall{}
	if base, mode, ok := strings.Cut(id, "."); ok {
		secs, isRead := strings.CutPrefix(mode, "r")
		if !isRead {
			return nil, ErrPushRef
		}
		if secs != "" {
			n, err := strconv.ParseInt(secs, 10, 64)
			if err != nil || n <= 0 {
				return nil, ErrPushRef
			}
			c.cache = time.Duration(n) * time.Second
		}
		id, c.read = base, true
	}
	pushMethods.RLock()
	c.m = pushMethods.byID[id]
	pushMethods.RUnlock()
	if c.m == nil {
		return nil, ErrPushRef
	}
	if args != "" {
		if err := json.Unmarshal([]byte(args), &c.args); err != nil {
			return nil, badPushCall("the call's arguments aren't a JSON array")
		}
	}
	if form != "" {
		if err := json.Unmarshal([]byte(form), &c.form); err != nil {
			return nil, badPushCall("the call's form isn't a JSON object of lists")
		}
	}
	if err := c.m.checkCount(len(c.args)); err != nil {
		return nil, badPushCall(err.Error())
	}
	return c, nil
}

// ParseOptionsCall is ParsePushCall with no form.
//
// Deprecated: use ParsePushCall.
func ParseOptionsCall(ctx context.Context, ref, args string) (*PushCall, error) {
	return ParsePushCall(ctx, ref, args, "")
}

// Receiver is the type the call's method is declared on: the struct, not a
// pointer to it.
func (c *PushCall) Receiver() reflect.Type {
	if c.m.recv.Kind() == reflect.Pointer {
		return c.m.recv.Elem()
	}
	return c.m.recv
}

// Method is the method's name, "Checkout.CitiesOf".
func (c *PushCall) Method() string { return c.m.name }

// Read reports whether the step rendered the call as a read (ui.Read): a
// GET may make it. Any other is a write, which only a POST may make, so
// the app's CSRF protection checks it.
func (c *PushCall) Read() bool { return c.read }

// Cache is how long the browser may keep the call's reply (ui.Cache);
// 0 when it may not.
func (c *PushCall) Cache() time.Duration { return c.cache }

// Call calls the method on recv, a pointer to a Receiver(): its arguments
// bound from the call's (a string read into a number or bool parsed as a
// URL parameter is; a struct from the form when the call passes none for
// it), its dependencies from ctx's Injector. The result is the reply's
// JSON, as the runtime applies it to the step's target: {"options"},
// {"html"}, {"value"}, {"fields"}, {"js"}, or {} for none.
func (c *PushCall) Call(ctx context.Context, recv reflect.Value) (any, error) {
	if c.m.recv.Kind() != reflect.Pointer {
		recv = recv.Elem()
	}
	return c.m.call(ctx, recv, c.args, c.form)
}

// CallPush calls fn, a live handler, as a Push step with arguments does:
// its parameters bound from args and form as PushCall.Call binds them, its
// result encoded as the reply carries it.
func CallPush(ctx context.Context, fn any, args []json.RawMessage, form url.Values) (any, error) {
	pm, err := handlerShape(fn)
	if err != nil {
		return nil, err
	}
	if err := pm.checkCount(len(args)); err != nil {
		return nil, badPushCall(err.Error())
	}
	return pm.call(ctx, reflect.Value{}, args, form)
}

func (pm *pushMethod) call(ctx context.Context, recv reflect.Value, args []json.RawMessage, form url.Values) (any, error) {
	ft := pm.fn.Type()
	skip := 0
	in := make([]reflect.Value, ft.NumIn())
	if recv.IsValid() {
		in[0], skip = recv, 1
	}
	byForm := pm.fromForm(len(args))
	next, arg := 0, 0
	inj := InjectorOf(ctx)
	for i := skip; i < ft.NumIn(); i++ {
		t := ft.In(i)
		switch {
		case pm.ctx && i == skip:
			in[i] = reflect.ValueOf(&ctx).Elem()
		case next < len(pm.argAt) && pm.argAt[next] == i-skip:
			v := reflect.New(t).Elem()
			if byForm && next == pm.form {
				if err := bindStruct(v, form); err != nil {
					return nil, err
				}
			} else {
				if err := bindPushArg(v, args[arg]); err != nil {
					return nil, badPushCall(fmt.Sprintf("%s's %s argument: %v", pm.name, ordinal(arg+1), err))
				}
				arg++
			}
			in[i] = v
			next++
		default:
			var fill func(context.Context) (reflect.Value, error)
			if inj != nil {
				fill = inj.Inject(t)
			}
			if fill == nil {
				return nil, fmt.Errorf("%s's %s parameter, %s, is not provided", pm.name, ordinal(i-skip+1), t)
			}
			v, err := fill(ctx)
			if err != nil {
				return nil, err
			}
			if !v.IsValid() {
				v = reflect.Zero(t)
			}
			in[i] = v
		}
	}
	out := pm.fn.Call(in)
	return pm.reply(ctx, out)
}

// reply is a call's results as its reply carries them (see PushCall.Call).
func (pm *pushMethod) reply(ctx context.Context, out []reflect.Value) (any, error) {
	if pm.errs {
		if e := out[len(out)-1]; !e.IsNil() {
			return nil, e.Interface().(error)
		}
	}
	if pm.result == resNone {
		return map[string]any{}, nil
	}
	return encodeResult(ctx, pm.result, out[0])
}

// HandlerResult is the reply to a live handler's call, given its results
// out: fn's result encoded as a push's reply carries it ({"html"}, …), or
// nil when fn returns none. The error is fn's.
func HandlerResult(ctx context.Context, fn any, out []reflect.Value) (any, error) {
	pm, err := handlerShape(fn)
	if err != nil {
		return nil, err
	}
	r, err := pm.reply(ctx, out)
	if pm.result == resNone {
		return nil, err
	}
	return r, err
}

// encodeResult is v, a result of kind k, as the runtime takes it.
func encodeResult(ctx context.Context, k resultKind, v reflect.Value) (any, error) {
	switch k {
	case resOptions:
		os, _ := v.Interface().([]Option)
		return map[string]any{"options": optionsJSON(os, nil)}, nil
	case resGroups:
		gs, _ := v.Interface().([]OptionGroup)
		return map[string]any{"options": optionsJSON(nil, gs)}, nil
	case resNode:
		var b bytes.Buffer
		if n, _ := v.Interface().(Node); n != nil {
			if err := n.Render(ctx, &b); err != nil {
				return nil, err
			}
		}
		return map[string]any{"html": b.String()}, nil
	case resScalar:
		return map[string]any{"value": scalarText(v)}, nil
	case resStruct:
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return map[string]any{}, nil
			}
			v = v.Elem()
		}
		return map[string]any{"fields": structFields(v)}, nil
	case resCommands:
		var ch JSChain = Commands{}
		if c, ok := v.Interface().(JSChain); ok {
			ch = c
		}
		j, err := EncodeJS(ctx, ch)
		if err != nil {
			return nil, err
		}
		return map[string]any{"js": json.RawMessage(j)}, nil
	}
	return map[string]any{}, nil
}

// scalarText is a scalar result as the text a field or element takes: a
// zero number "0", unlike a form field's empty one.
func scalarText(v reflect.Value) string {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, v.Type().Bits())
	}
	return valueText(v)
}

// structFields is a struct result's fields by their form names (a form:
// tag, else the field's name in lowerCamel; a nested struct's as
// "address.city"): a value's text, a bool, or a list for a slice. An
// Option left None is left out, so its control keeps what it holds.
func structFields(v reflect.Value) map[string]any {
	out := map[string]any{}
	for _, f := range formTypeOf(v.Type()).fields {
		if f.group {
			continue
		}
		fv, err := v.FieldByIndexErr(f.index)
		if err != nil {
			continue
		}
		if n, ok := fv.Interface().(interface{ IsNone() bool }); ok && n.IsNone() {
			continue
		}
		switch {
		case fv.Kind() == reflect.Bool:
			out[f.path] = fv.Bool()
		case fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() != reflect.Uint8:
			list := make([]string, fv.Len())
			for i := range list {
				list[i] = valueText(fv.Index(i))
			}
			out[f.path] = list
		default:
			out[f.path] = valueText(fv)
		}
	}
	return out
}

// bindStruct sets v, a struct, from a form's fields by their form names,
// as a submit binds a form, then checks its validate: tags. A value that
// doesn't parse fails the call.
func bindStruct(v reflect.Value, form url.Values) error {
	for _, f := range formTypeOf(v.Type()).fields {
		if f.group {
			continue
		}
		fv, err := v.FieldByIndexErr(f.index)
		if err != nil || !fv.CanSet() {
			continue
		}
		vs, sent := lookupValues(form, f.path)
		if !sent {
			continue
		}
		if rule, text := setField(fv, vs, f.raw, false); rule != "" {
			what := map[string]string{"number": "a number", "whole": "a whole number"}[rule]
			if what == "" {
				what = "a valid value"
			}
			return badPushCall(fmt.Sprintf("%s: %q isn't %s", f.path, text, what))
		}
	}
	return ValidateTags(v.Addr().Interface(), "form", "json")
}

// bindPushArg sets v from an argument's JSON: decoded as v's type, else
// a string parsed as a URL parameter's value (an element's value read
// into a number).
func bindPushArg(v reflect.Value, raw json.RawMessage) error {
	if err := json.Unmarshal(raw, v.Addr().Interface()); err == nil {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return fmt.Errorf("can't take %s as a %s", raw, v.Type())
		}
		s = fmt.Sprint(b)
	}
	if s == "" && v.Kind() != reflect.String {
		v.SetZero()
		return nil
	}
	if err := parseText(v, []string{s}); err != nil {
		return fmt.Errorf("can't take %q as a %s", s, v.Type())
	}
	return nil
}

// badPushCall is a call the browser sent wrong: 400, its message shown.
type badPushCall string

func (e badPushCall) Error() string         { return string(e) }
func (e badPushCall) PublicMessage() string { return string(e) }
func (e badPushCall) StatusCode() int       { return 400 }
