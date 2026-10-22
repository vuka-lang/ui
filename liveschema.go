package ui

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/vuka-lang/vuka/diag"
)

// LiveSchema is what a live page declares beyond its Go types: attributes
// of its fields and the hooks the runtime calls, as values. A person writes
// them as attributes in the page's source — @ui.Bind, @ui.URL("q") and
// @ui.Defer("lists") on a field, @ui.Mount, @ui.OnChange and
// @ui.Loads("lists") on a method — and the compiler writes them into the
// page's LiveSchema method:
//
//	type Dashboard struct {
//		ui.Live
//		Q    string @ui.Bind{Debounce: 300 * time.Millisecond} @ui.URL("q")
//		Rows []Task @ui.Defer("lists")
//	}
//
//	@ui.Loads("lists")
//	func (d *Dashboard) loadLists(ctx context.Context) error { … }
//
//	@ui.Mount
//	func (d *Dashboard) mount(ctx context.Context) error { … }
//
// is, in the generated Go,
//
//	func (d *Dashboard) LiveSchema(l *ui.LiveSchema) {
//		l.Field(&d.Q, ui.Bind{Debounce: 300 * time.Millisecond}, ui.URL("q"))
//		l.Field(&d.Rows, ui.Defer("lists"))
//		l.Loads("lists", d.loadLists)
//		l.Mount(d.mount)
//	}
//
// LiveSchema is exported so that generated code, ejected code included,
// builds with go build; it is the compiler's output, not an API to write by
// hand, and vukavet reports a LiveSchema method a person wrote (W0306). A
// field is named by its address and a hook by its method value, so the Go
// compiler checks both. The runtime calls LiveSchema on each page it runs:
// the hooks are that page's methods, never ones its component calls.
type LiveSchema struct {
	root     uintptr
	t        reflect.Type
	fields   map[int]FieldOptions
	mount    func(context.Context) error
	onChange func(context.Context, Changed) error
	loads    map[string]func(context.Context) error
	hooks    map[string]bool // the hooks' method names
	hookFns  []hookFn        // the hooks, as declared: read once per type, for CheckParams
	errs     []error
}

// hookFn is a hook as LiveSchema was given it, and where.
type hookFn struct {
	kind, name string
	fn         reflect.Value
	file       string
	line       int
}

// A FieldAttr configures a live page's field: Bind, URL, Defer, and the
// rules of package validate. Attributes apply in order, a later one's set
// members over an earlier one's.
type FieldAttr interface{ Apply(*FieldOptions) }

// FieldOptions is what a page's field attributes say of it.
type FieldOptions struct {
	Bind     bool          // the component may set it
	Debounce time.Duration // how long a set waits for more before it is sent
	URL      string        // the query parameter the page's address keeps it in
	Defer    string        // the group it loads in after the first render
	Validate []string      // rules, as a validate tag writes them: "required", "max=50"
}

// Bind lets the component set the field (live.q.set(v)); a set waits
// Debounce for more before it is sent: DefaultDebounce when zero, no time
// when negative.
type Bind struct{ Debounce time.Duration }

func (b Bind) Apply(o *FieldOptions) {
	o.Bind, o.Debounce = true, DefaultDebounce
	if b.Debounce != 0 {
		o.Debounce = max(b.Debounce, 0)
	}
}

// URL keeps the field in the page's address, as the query parameter it
// names: the page reads it on load, and a set changes the address.
type URL string

func (u URL) Apply(o *FieldOptions) { o.URL = string(u) }

// Defer leaves the field out of the page's first render: the group it
// names loads it after, by the loader LiveSchema's Loads gives the group.
type Defer string

func (d Defer) Apply(o *FieldOptions) { o.Defer = string(d) }

// Field adds attributes to the field ptr points to, one of the page's own
// fields.
func (l *LiveSchema) Field(ptr any, attrs ...FieldAttr) {
	if l.fields == nil {
		return // a page's hooks, read again: its fields are known
	}
	v := reflect.ValueOf(ptr)
	i := -1
	if v.Kind() == reflect.Pointer && !v.IsNil() && v.Pointer() >= l.root {
		off := v.Pointer() - l.root
		for j := range l.t.NumField() {
			if f := l.t.Field(j); f.Offset == off && f.Type == v.Type().Elem() {
				i = j
			}
		}
	}
	if i < 0 {
		l.fail("E0335", "l.Field is given %T, not the address of one of %s's fields", "pass the address of the receiver's field: l.Field(&d.Q, …)", ptr, l.t.Name())
		return
	}
	o := l.fields[i]
	for _, a := range attrs {
		if a == nil {
			l.fail("E0335", "l.Field(&%s, …) is given a nil attribute", "drop it", l.t.Field(i).Name)
			continue
		}
		a.Apply(&o)
	}
	l.fields[i] = o
}

// Mount sets the hook that runs when the page mounts, after its props are
// set and before its first render: a method of the page, d.mount,
// returning nothing or an error, its parameters filled by type — a
// context.Context, and what the app's Injector fills (the request, the
// signed-in user, a service).
func (l *LiveSchema) Mount(fn any) {
	if rv, ok := l.hook("Mount", "Mount", fn); ok {
		l.mount = func(ctx context.Context) error { return call(ctx, rv, nil, hookCall, Changed{}, nil) }
	}
}

// OnChange sets the hook that runs after the component sets bound fields:
// a method of the page as Mount takes, which may take a Changed naming
// them too.
func (l *LiveSchema) OnChange(fn any) {
	if rv, ok := l.hook("OnChange", "OnChange", fn); ok {
		l.onChange = func(ctx context.Context, c Changed) error { return call(ctx, rv, nil, changeCall, c, nil) }
	}
}

// Loads sets the loader of the deferred group, a method of the page as
// Mount takes: it runs after the page's first render, on a copy of the
// page, and sets the group's fields.
func (l *LiveSchema) Loads(group string, fn any) {
	if !isIdent(group) {
		l.fail("E0336", "l.Loads(%q, …): a group is a name", `name the group as ui.Defer does: l.Loads("stats", …)`, group)
		return
	}
	rv, ok := l.hook("Loads", "Loads("+group+")", fn)
	if !ok {
		return
	}
	if l.loads == nil {
		l.loads = map[string]func(context.Context) error{}
	}
	g := groupName(group)
	if _, ok := l.loads[g]; ok {
		l.fail("E0336", "l.Loads(%q, …) is called twice", "give the group one loader", group)
	}
	l.loads[g] = func(ctx context.Context) error { return call(ctx, rv, nil, hookCall, Changed{}, nil) }
}

// hook checks fn, a hook of kind; read once per type (l.hooks set), it
// records the hook and the place that set it — LiveSchema's line, which
// the compiler places at the hook's method.
func (l *LiveSchema) hook(kind, what string, fn any) (reflect.Value, bool) {
	rv := reflect.ValueOf(fn)
	if !rv.IsValid() || rv.Kind() != reflect.Func || rv.IsNil() {
		l.fail("E0336", "l.%s is given %T, not a method of the page", "pass a method of the page: l.Mount(d.mount)", what, fn)
		return rv, false
	}
	ft := rv.Type()
	if ft.IsVariadic() || ft.NumOut() > 1 || ft.NumOut() == 1 && ft.Out(0) != errType {
		l.fail("E0336", "l.%s is given %s", "a hook returns nothing or an error; its parameters are filled by type", what, ft)
		return rv, false
	}
	if l.hooks != nil {
		name := methodName(rv)
		l.hooks[name] = true
		h := hookFn{kind: kind, name: name, fn: rv}
		if _, file, line, ok := runtime.Caller(2); ok {
			h.file, h.line = file[strings.LastIndexByte(file, '/')+1:], line
		}
		l.hookFns = append(l.hookFns, h)
	}
	return rv, true
}

func (l *LiveSchema) fail(code, format, help string, args ...any) {
	l.errs = append(l.errs, diag.New(diag.Code(code), "%s.LiveSchema: "+format, append([]any{l.t.Name()}, args...)...).WithHelp("%s", help))
}

func groupName(g string) string { return strings.ToUpper(g[:1]) + g[1:] }

type liveSchemer interface{ LiveSchema(*LiveSchema) }

var liveSchemerType = reflect.TypeFor[liveSchemer]()

// vukaSchema marks the types embedding Live as declaring a LiveSchema: Vuka
// writes their field and hook attributes into one.
func (Live) vukaSchema(liveSchemer) {}

var liveSchemas sync.Map // reflect.Type (struct) → *LiveSchema

// liveSchemaOf is what t (a struct, or a pointer to one) declares in its
// LiveSchema method, read once on a zero value; nil when it has none.
func liveSchemaOf(t reflect.Type) *LiveSchema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || !reflect.PointerTo(t).Implements(liveSchemerType) {
		return nil
	}
	if v, ok := liveSchemas.Load(t); ok {
		return v.(*LiveSchema)
	}
	x := reflect.New(t)
	l := &LiveSchema{root: x.Pointer(), t: t, fields: map[int]FieldOptions{}, hooks: map[string]bool{}}
	func() {
		defer func() {
			if r := recover(); r != nil {
				l.fail("E0335", "panicked: %v", "name the page's own fields and methods only", r)
			}
		}()
		x.Interface().(liveSchemer).LiveSchema(l)
	}()
	v, _ := liveSchemas.LoadOrStore(t, l)
	return v.(*LiveSchema)
}

// liveHooks are the hooks x's LiveSchema sets, bound to x; nil when x
// declares no LiveSchema.
func liveHooks(x reflect.Value) *LiveSchema {
	if x.Kind() != reflect.Pointer || x.IsNil() {
		return nil
	}
	s, ok := x.Interface().(liveSchemer)
	if !ok {
		return nil
	}
	l := &LiveSchema{t: x.Type().Elem()}
	s.LiveSchema(l)
	return l
}

// hasMount reports whether t's LiveSchema sets a mount hook.
func hasMount(t reflect.Type) bool {
	s := liveSchemaOf(t)
	return s != nil && s.mount != nil
}

// hasLoads reports whether t's LiveSchema gives the group (as groupName
// names it) a loader.
func hasLoads(t reflect.Type, group string) bool {
	s := liveSchemaOf(t)
	return s != nil && s.loads[group] != nil
}

// schemaErr is what is wrong with t's LiveSchema.
func schemaErr(t reflect.Type) error {
	if s := liveSchemaOf(t); s != nil && len(s.errs) > 0 {
		return s.errs[0]
	}
	return nil
}

// isHook reports whether t's (a pointer type's) method name is LiveSchema
// or one of the hooks its LiveSchema sets, which the component doesn't
// call.
func isHook(t reflect.Type, name string) bool {
	s := liveSchemaOf(t)
	return s != nil && (name == "LiveSchema" || s.hooks[name])
}

// IsHook reports whether method is LiveSchema or one of the hooks c's
// LiveSchema sets: never called from the browser. Package live asks.
func IsHook(c any, method string) bool { return isHook(reflect.TypeOf(c), method) }

// FieldOptionsOf is what the page t's LiveSchema says of its field f:
// zero when it says nothing. Package web reads a field's URL from it.
func FieldOptionsOf(t reflect.Type, f reflect.StructField) FieldOptions {
	if s := liveSchemaOf(t); s != nil && len(f.Index) == 1 {
		return s.fields[f.Index[0]]
	}
	return FieldOptions{}
}
