package ui

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

// Options loading (JS.Options): a select's options come from a method of
// the page or of a component on it, called over HTTP on the page's own
// URL — no socket, so pages that aren't live load them too.
//
// Vuka's compiler registers each method an Options command names
// (RegisterOptions); no other method can be called this way. The command
// carries the method's id, signed for the path of the page that rendered
// it (with the app's form key, the same on every replica), so a page can
// only call the methods its own markup — or its components' — names.

// OptionGroup is a labelled group of options, an <optgroup>: a method
// loading a select's options returns []OptionGroup for grouped choices.
type OptionGroup struct {
	Label   string
	Options []Option
}

// optionsMethod is a method an Options command may call.
type optionsMethod struct {
	id      string
	recv    reflect.Type // the receiver as declared: *T or T
	fn      reflect.Value
	name    string // "Checkout.CitiesOf"
	ctx     bool   // takes a context.Context first
	args    []reflect.Type
	argAt   []int // each argument's parameter, from 0 the receiver excluded
	grouped bool  // returns []OptionGroup
	errs    bool  // returns an error too
}

var optionsMethods = struct {
	sync.RWMutex
	byName map[string]*optionsMethod // the method's Go name: "pkg.(*T).M"
	byID   map[string]*optionsMethod
}{byName: map[string]*optionsMethod{}, byID: map[string]*optionsMethod{}}

var (
	optionListType   = reflect.TypeFor[[]Option]()
	optionGroupsType = reflect.TypeFor[[]OptionGroup]()
)

// RegisterOptions makes method m, a method expression ((*Checkout).CitiesOf),
// callable by the Options commands naming it. Vuka's compiler calls it for
// each method an Options command in a page's or component's markup names;
// an app doesn't.
//
// The method returns []Option or []OptionGroup, and maybe an error. Its
// parameters after the receiver are a context.Context (optional, first),
// then the command's arguments in order — the parameters that aren't
// pointers or interfaces — and the dependencies the app injects (pointers
// and interfaces). RegisterOptions panics on another shape.
func RegisterOptions(m any) {
	om, err := newOptionsMethod(reflect.ValueOf(m))
	if err != nil {
		panic(err)
	}
	optionsMethods.Lock()
	defer optionsMethods.Unlock()
	optionsMethods.byName[funcName(om.fn)] = om
	optionsMethods.byID[om.id] = om
}

// funcName is a function's Go name, without a method value's -fm suffix.
func funcName(fn reflect.Value) string {
	f := runtime.FuncForPC(fn.Pointer())
	if f == nil {
		return ""
	}
	return strings.TrimSuffix(f.Name(), "-fm")
}

func newOptionsMethod(fn reflect.Value) (*optionsMethod, error) {
	if fn.Kind() != reflect.Func || fn.IsNil() || fn.Type().NumIn() == 0 {
		return nil, fmt.Errorf("ui: RegisterOptions takes a method expression, (*Page).Method, not %v", fn.Type())
	}
	ft := fn.Type()
	recv := ft.In(0)
	base := recv
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	om := &optionsMethod{recv: recv, fn: fn}
	full := funcName(fn)
	om.name = base.Name() + "." + full[strings.LastIndexByte(full, '.')+1:]
	sum := sha256.Sum256([]byte(full))
	om.id = base64.RawURLEncoding.EncodeToString(sum[:9])
	if err := om.shape(ft); err != nil {
		return nil, fmt.Errorf("ui: %s %w", om.name, err)
	}
	return om, nil
}

// shape reads the method's parameters and results (see RegisterOptions).
func (om *optionsMethod) shape(ft reflect.Type) error {
	switch {
	case ft.IsVariadic():
		return errors.New("is variadic: a method loading options takes its arguments one by one")
	case ft.NumOut() == 0 || ft.NumOut() > 2 || ft.NumOut() == 2 && ft.Out(1) != errType:
		return fmt.Errorf("is %s: a method loading options returns []ui.Option or []ui.OptionGroup, and maybe an error", ft)
	}
	switch ft.Out(0) {
	case optionListType:
	case optionGroupsType:
		om.grouped = true
	default:
		return fmt.Errorf("returns %s: a method loading options returns []ui.Option or []ui.OptionGroup", ft.Out(0))
	}
	om.errs = ft.NumOut() == 2
	for i := 1; i < ft.NumIn(); i++ {
		switch t := ft.In(i); {
		case t == ctxType && i == 1:
			om.ctx = true
		case t == ctxType:
			return errors.New("takes a context.Context after other parameters: put it first")
		case t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface:
			// a dependency, injected
		default:
			om.args = append(om.args, t)
			om.argAt = append(om.argAt, i-1)
		}
	}
	return nil
}

// optionsOf is the method fn, an Options command's handler, is: a method
// value of a method vuka registered.
func optionsOf(fn any) (*optionsMethod, error) {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() {
		return nil, fmt.Errorf("ui: JS Options takes a method of the page or of a component (p.CitiesOf), not %T", fn)
	}
	optionsMethods.RLock()
	om := optionsMethods.byName[funcName(v)]
	optionsMethods.RUnlock()
	if om == nil {
		return nil, fmt.Errorf("ui: JS Options: %s isn't a method vuka's compiler registered: pass a method of the page or of a component, p.CitiesOf, in its markup", funcName(v))
	}
	return om, nil
}

type optionsScopeKey struct{}

// WithOptionsScope is ctx rendering a page whose Options commands are
// signed for scope: the route the page is served under ("/orgs/{org}"), so
// a reference stays good while a live page moves to other parameters of
// it. Without one, the page's path (PageURL) is the scope. Package web
// sets it for every route.
func WithOptionsScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, optionsScopeKey{}, scope)
}

// optionsScope is what the Options commands ctx renders are signed for.
func optionsScope(ctx context.Context) string {
	if s, _ := ctx.Value(optionsScopeKey{}).(string); s != "" {
		return s
	}
	if u := PageURL(ctx); u != nil && u.Path != "" {
		return u.Path
	}
	return "/"
}

// optionsRef is the method's reference as the page carries it: its id,
// signed for the page's scope.
func optionsRef(ctx context.Context, om *optionsMethod) string {
	return om.id + "." + optionsMAC(formKeyOf(ctx), optionsScope(ctx), om.id)
}

func optionsMAC(key []byte, scope, id string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("vuka options\x00" + scope + "\x00" + id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18])
}

// ErrOptionsRef is the error of a call naming a method the page at its
// path doesn't render an Options command for: a reference from another
// page, forged, or from before a deploy that removed the method.
var ErrOptionsRef = errors.New("ui: the options method isn't one this page loads options from")

// An OptionsCall is a call an Options command sends to its page: the method
// it names and the arguments it read. Package web answers it.
type OptionsCall struct {
	m    *optionsMethod
	args []json.RawMessage
}

// ParseOptionsCall reads the call of an Options command sent to a page:
// ref, the method's reference the page rendered, and args, the JSON array
// of its arguments. The reference must be signed with ctx's form key for
// the page's scope: ctx's (WithOptionsScope), else its path (PageURL)
// (ErrOptionsRef).
func ParseOptionsCall(ctx context.Context, ref, args string) (*OptionsCall, error) {
	id, sig, ok := strings.Cut(ref, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(optionsMAC(formKeyOf(ctx), optionsScope(ctx), id))) {
		return nil, ErrOptionsRef
	}
	optionsMethods.RLock()
	om := optionsMethods.byID[id]
	optionsMethods.RUnlock()
	if om == nil {
		return nil, ErrOptionsRef
	}
	c := &OptionsCall{m: om}
	if args != "" {
		if err := json.Unmarshal([]byte(args), &c.args); err != nil {
			return nil, badOptionsCall("the options call's arguments aren't a JSON array")
		}
	}
	if len(c.args) != len(om.args) {
		return nil, badOptionsCall(fmt.Sprintf("%s takes %d arguments, not %d", om.name, len(om.args), len(c.args)))
	}
	return c, nil
}

// Receiver is the type the call's method is declared on: the struct, not a
// pointer to it.
func (c *OptionsCall) Receiver() reflect.Type {
	if c.m.recv.Kind() == reflect.Pointer {
		return c.m.recv.Elem()
	}
	return c.m.recv
}

// Method is the method's name, "Checkout.CitiesOf".
func (c *OptionsCall) Method() string { return c.m.name }

// Call calls the method on recv, a pointer to a Receiver(): its arguments
// bound from the call's (a string read into a number or bool parsed as a
// URL parameter is), its dependencies from ctx's Injector. The result is
// the options, []Option or []OptionGroup, as the runtime takes them.
func (c *OptionsCall) Call(ctx context.Context, recv reflect.Value) (any, error) {
	om, ft := c.m, c.m.fn.Type()
	if om.recv.Kind() != reflect.Pointer {
		recv = recv.Elem()
	}
	in := make([]reflect.Value, ft.NumIn())
	in[0] = recv
	next := 0
	inj := InjectorOf(ctx)
	for i := 1; i < ft.NumIn(); i++ {
		t := ft.In(i)
		switch {
		case om.ctx && i == 1:
			in[i] = reflect.ValueOf(&ctx).Elem()
		case next < len(om.argAt) && om.argAt[next] == i-1:
			v := reflect.New(t).Elem()
			if err := bindOptionsArg(v, c.args[next]); err != nil {
				return nil, badOptionsCall(fmt.Sprintf("%s's %s argument: %v", om.name, ordinal(next+1), err))
			}
			in[i] = v
			next++
		default:
			var fill func(context.Context) (reflect.Value, error)
			if inj != nil {
				fill = inj.Inject(t)
			}
			if fill == nil {
				return nil, fmt.Errorf("%s's %s parameter, %s, is not provided", om.name, ordinal(i), t)
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
	out := om.fn.Call(in)
	if om.errs && !out[1].IsNil() {
		return nil, out[1].Interface().(error)
	}
	if om.grouped {
		gs, _ := out[0].Interface().([]OptionGroup)
		return optionsJSON(nil, gs), nil
	}
	os, _ := out[0].Interface().([]Option)
	return optionsJSON(os, nil), nil
}

// bindOptionsArg sets v from an argument's JSON: decoded as v's type, else
// a string parsed as a URL parameter's value (an element's value read
// into a number).
func bindOptionsArg(v reflect.Value, raw json.RawMessage) error {
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

// optionJSON is an option as the runtime takes it: {"value", "label"}, or a
// group, {"group", "options"}.
type optionJSON struct {
	Value   string       `json:"value,omitempty"`
	Label   string       `json:"label,omitempty"`
	Group   string       `json:"group,omitempty"`
	Options []optionJSON `json:"options,omitempty"`
}

func optionsJSON(os []Option, gs []OptionGroup) []optionJSON {
	out := []optionJSON{}
	for _, o := range os {
		out = append(out, optionJSON{Value: o.Value, Label: o.Label})
	}
	for _, g := range gs {
		og := optionJSON{Group: g.Label, Options: []optionJSON{}}
		for _, o := range g.Options {
			og.Options = append(og.Options, optionJSON{Value: o.Value, Label: o.Label})
		}
		out = append(out, og)
	}
	return out
}

// badOptionsCall is a call the browser sent wrong: 400, its message shown.
type badOptionsCall string

func (e badOptionsCall) Error() string         { return string(e) }
func (e badOptionsCall) PublicMessage() string { return string(e) }
func (e badOptionsCall) StatusCode() int       { return 400 }
