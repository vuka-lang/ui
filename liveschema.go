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
// of its fields and the hooks the runtime calls, as values. A page declares
// them in a LiveSchema method:
//
//	type Dashboard struct {
//		ui.Live
//		Q     string
//		Rows  []Task
//		Total int
//	}
//
//	func (d *Dashboard) LiveSchema(l *ui.LiveSchema) {
//		l.Field(&d.Q, ui.Bind{Debounce: 300 * time.Millisecond}, ui.URL("q"), validate.Max(100))
//		l.Field(&d.Rows, ui.Defer("lists"))
//		l.Field(&d.Total, ui.Defer("lists"))
//		l.Loads("lists", d.loadLists)
//		l.OnChange(d.changed)
//		l.Mount(d.mount)
//	}
//
// A field is named by its address, so a rename is checked by the compiler,
// and a hook by its method value, so its signature is. The runtime calls
// LiveSchema on each page it runs: the hooks are that page's methods. They
// may be unexported; a page's exported methods are what its component
// calls, the hooks excepted.
//
// In Vuka the same values are attributes: Q string @ui.Bind{…} @ui.URL("q")
// on the field, @ui.Mount, @ui.OnChange and @ui.Loads("lists") on the
// method; the compiler writes the LiveSchema method.
type LiveSchema struct {
	root     uintptr
	t        reflect.Type
	fields   map[int]FieldOptions
	mount    func(context.Context) error
	onChange func(context.Context, Changed) error
	loads    map[string]func(context.Context) error
	hooks    map[string]bool // the hooks' method names
	errs     []error
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
// set and before its first render.
func (l *LiveSchema) Mount(fn func(ctx context.Context) error) {
	if l.hook("Mount", fn) {
		l.mount = fn
	}
}

// OnChange sets the hook that runs after the component sets bound fields,
// c naming them.
func (l *LiveSchema) OnChange(fn func(ctx context.Context, c Changed) error) {
	if l.hook("OnChange", fn) {
		l.onChange = fn
	}
}

// Loads sets the loader of the deferred group: it runs after the page's
// first render, on a copy of the page, and sets the group's fields.
func (l *LiveSchema) Loads(group string, fn func(ctx context.Context) error) {
	if !isIdent(group) {
		l.fail("E0336", "l.Loads(%q, …): a group is a name", `name the group as ui.Defer does: l.Loads("stats", …)`, group)
		return
	}
	if !l.hook("Loads("+group+")", fn) {
		return
	}
	if l.loads == nil {
		l.loads = map[string]func(context.Context) error{}
	}
	g := groupName(group)
	if _, ok := l.loads[g]; ok {
		l.fail("E0336", "l.Loads(%q, …) is called twice", "give the group one loader", group)
	}
	l.loads[g] = fn
}

func (l *LiveSchema) hook(what string, fn any) bool {
	rv := reflect.ValueOf(fn)
	if rv.IsNil() {
		l.fail("E0336", "l.%s is given nil", "pass a method of the page: l.Mount(d.mount)", what)
		return false
	}
	if l.hooks != nil {
		if f := runtime.FuncForPC(rv.Pointer()); f != nil {
			name := strings.TrimSuffix(f.Name(), "-fm")
			l.hooks[name[strings.LastIndexByte(name, '.')+1:]] = true
		}
	}
	return true
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
	s, ok := x.Interface().(liveSchemer)
	if !ok || x.Kind() != reflect.Pointer || x.IsNil() {
		return nil
	}
	l := &LiveSchema{t: x.Type().Elem()}
	s.LiveSchema(l)
	return l
}

// schemaErr is what is wrong with t's LiveSchema.
func schemaErr(t reflect.Type) error {
	if s := liveSchemaOf(t); s != nil && len(s.errs) > 0 {
		return s.errs[0]
	}
	return nil
}

// isHook reports whether t's (a pointer type's) method name is one of its
// LiveSchema's hooks, which the component doesn't call.
func isHook(t reflect.Type, name string) bool {
	s := liveSchemaOf(t)
	return name == "LiveSchema" && s != nil || s != nil && s.hooks[name]
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
