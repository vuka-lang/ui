package ui

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/vuka-lang/vuka/diag"
)

// OnMount sets the page's mount hook from its constructor: a method
// expression, (*Login).mount, taking the page and optionally a
// context.Context, returning nothing or an error. It runs as LiveSchema's
// Mount does.
//
//	func NewLogin() *Login {
//		l := &Login{}
//		l.OnMount((*Login).mount)
//		return l
//	}
//
// The hooks are the page type's: the first instance to declare one sets it,
// and an instance declaring another for the same type is an error (E0336).
// A page built without its constructor has the hooks its type already set.
func (l *Live) OnMount(fn any) { l.declare("OnMount", "", fn) }

// Loads sets the loader of the deferred group from the page's constructor,
// a method expression as OnMount takes: (*Dashboard).loadLists.
func (l *Live) Loads(group string, fn any) {
	if !isIdent(group) {
		l.fail(fn, diag.New("E0336", "Loads(%q, …): a group is a name", group).
			WithHelp(`name the group as ui.Defer does: l.Loads("stats", …)`))
		return
	}
	l.declare("Loads", group, fn)
}

// OnChange sets the hook that runs after the component sets bound fields,
// from the page's constructor: a method expression taking the page and
// optionally a context.Context and a Changed, returning nothing or an error.
func (l *Live) OnChange(fn any) { l.declare("OnChange", "", fn) }

// ctorHooks are the hooks a page type's constructor declares.
type ctorHooks struct {
	mu       sync.Mutex
	mount    reflect.Value
	onChange reflect.Value
	loads    map[string]reflect.Value
	names    map[string]bool // the hooks' method names
	errs     []error
}

var ctorTables sync.Map // reflect.Type (struct) → *ctorHooks

func ctorHooksOf(t reflect.Type) *ctorHooks {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return nil
	}
	if v, ok := ctorTables.Load(t); ok {
		return v.(*ctorHooks)
	}
	return nil
}

func (l *Live) declare(kind, group string, fn any) {
	rv := reflect.ValueOf(fn)
	what := kind
	if group != "" {
		what += "(" + group + ")"
	}
	if !rv.IsValid() || rv.Kind() != reflect.Func || rv.IsNil() {
		l.fail(fn, diag.New("E0336", "%s is given %T, not a method of the page", what, fn).
			WithHelp("pass a method expression: l.%s((*Login).mount)", kind))
		return
	}
	ft := rv.Type()
	page := pageOf(ft)
	if page == nil {
		l.fail(fn, diag.New("E0336", "%s is given %s, which doesn't take the page first", what, ft).
			WithHelp("pass a method expression of the type embedding ui.Live: (*Login).mount, not l.mount"))
		return
	}
	if !hookSignature(kind, ft) {
		help := "declare it func(*%[1]s) or func(*%[1]s, context.Context), returning nothing or an error"
		if kind == "OnChange" {
			help = "declare it func(*%[1]s), adding a context.Context and a ui.Changed as it needs them, returning nothing or an error"
		}
		l.fail(fn, diag.New("E0336", "%s.%s is given %s", page.Name(), what, ft).WithHelp(help, page.Name()))
		return
	}
	v, _ := ctorTables.LoadOrStore(page, &ctorHooks{})
	h := v.(*ctorHooks)
	h.mu.Lock()
	defer h.mu.Unlock()
	slot := h.slot(kind, group)
	if slot.IsValid() {
		if slot.Pointer() != rv.Pointer() {
			h.errs = append(h.errs, diag.New("E0336", "%s.%s is declared twice, as %s and as %s", page.Name(), what, funcName(slot), funcName(rv)).
				WithHelp("give the hook one method: every instance's constructor declares the same"))
		}
		return
	}
	switch kind {
	case "OnMount":
		h.mount = rv
	case "OnChange":
		h.onChange = rv
	default:
		if h.loads == nil {
			h.loads = map[string]reflect.Value{}
		}
		h.loads[groupName(group)] = rv
	}
	if h.names == nil {
		h.names = map[string]bool{}
	}
	h.names[funcName(rv)] = true
}

func (h *ctorHooks) slot(kind, group string) reflect.Value {
	switch kind {
	case "OnMount":
		return h.mount
	case "OnChange":
		return h.onChange
	}
	return h.loads[groupName(group)]
}

// fail keeps err with the instance, which reports it as it mounts, and
// with the page type fn names, when it names one.
func (l *Live) fail(fn any, err error) {
	if l.hookErr == nil {
		l.hookErr = err
	}
	ft := reflect.TypeOf(fn)
	if ft == nil || ft.Kind() != reflect.Func || pageOf(ft) == nil {
		return
	}
	v, _ := ctorTables.LoadOrStore(pageOf(ft), &ctorHooks{})
	h := v.(*ctorHooks)
	h.mu.Lock()
	h.errs = append(h.errs, err)
	h.mu.Unlock()
}

// pageOf is the struct a hook's first parameter points to, when that
// struct embeds Live.
func pageOf(ft reflect.Type) reflect.Type {
	if ft.NumIn() == 0 || ft.IsVariadic() || ft.In(0).Kind() != reflect.Pointer {
		return nil
	}
	t := ft.In(0).Elem()
	if t.Kind() != reflect.Struct {
		return nil
	}
	if f, ok := t.FieldByName("Live"); !ok || !f.Anonymous || f.Type != liveType {
		return nil
	}
	return t
}

func hookSignature(kind string, ft reflect.Type) bool {
	out := ft.NumOut() == 0 || ft.NumOut() == 1 && ft.Out(0) == errType
	if kind == "OnChange" {
		_, ok := changedArgs(ft)
		return ok && out
	}
	return out && (ft.NumIn() == 1 || ft.NumIn() == 2 && ft.In(1) == ctxType)
}

func funcName(rv reflect.Value) string {
	f := runtime.FuncForPC(rv.Pointer())
	if f == nil {
		return "?"
	}
	name := strings.TrimSuffix(f.Name(), "-fm")
	return name[strings.LastIndexByte(name, '.')+1:]
}

// snapshot is the table's hooks and errors, read under its lock.
func (h *ctorHooks) snapshot() (mount, onChange reflect.Value, loads map[string]reflect.Value, errs []error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mount, h.onChange, h.loads, h.errs
}

// ctorErr is what is wrong with the hooks t's constructor declares: a
// declaration's mistake, or a hook LiveSchema sets too.
func ctorErr(t reflect.Type) error {
	h := ctorHooksOf(t)
	if h == nil {
		return nil
	}
	mount, onChange, loads, errs := h.snapshot()
	if len(errs) > 0 {
		return errs[0]
	}
	s := liveSchemaOf(t)
	if s == nil {
		return nil
	}
	name := s.t.Name()
	twice := func(what string) error {
		return diag.New("E0336", "%s's %s is declared by its constructor and by its LiveSchema", name, what).
			WithHelp("declare the hook in one place")
	}
	switch {
	case mount.IsValid() && s.mount != nil:
		return twice("mount hook")
	case onChange.IsValid() && s.onChange != nil:
		return twice("OnChange hook")
	}
	for g := range loads {
		if s.loads[g] != nil {
			return twice("loader of " + g)
		}
	}
	return nil
}

// bind adds the constructor's hooks to l, bound to the page x.
func (h *ctorHooks) bind(l *LiveSchema, x reflect.Value) {
	mount, onChange, loads, _ := h.snapshot()
	if mount.IsValid() {
		l.mount = func(ctx context.Context) error { return callHook(mount, x, ctx, Changed{}) }
	}
	if onChange.IsValid() {
		l.onChange = func(ctx context.Context, c Changed) error { return callHook(onChange, x, ctx, c) }
	}
	for g, fn := range loads {
		if l.loads == nil {
			l.loads = map[string]func(context.Context) error{}
		}
		l.loads[g] = func(ctx context.Context) error { return callHook(fn, x, ctx, Changed{}) }
	}
}

func callHook(fn, x reflect.Value, ctx context.Context, c Changed) error {
	ft := fn.Type()
	in := []reflect.Value{x}
	for i := 1; i < ft.NumIn(); i++ {
		if ft.In(i) == ctxType {
			in = append(in, reflect.ValueOf(&ctx).Elem())
		} else {
			in = append(in, reflect.ValueOf(c))
		}
	}
	out := fn.Call(in)
	if len(out) == 1 && !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}
