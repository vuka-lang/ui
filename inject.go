package ui

import (
	"context"
	"encoding"
	"fmt"
	"net/url"
	"reflect"
	"runtime"
	"slices"
	"strings"
)

// An Injector fills the parameters of a page's hooks and event handlers
// that the call itself doesn't: the request, the signed-in user, the app's
// services. A hook or handler declares what it needs by type, and the
// runtime fills each parameter from the one source its type names:
//
//	context.Context              the session's or request's context
//	ui.Event                     the event (a handler)
//	url.Values                   the form's fields, as sent (a handler)
//	a struct, or pointer to one  the form's fields bound to its fields (a handler)
//	a string, number or bool     the event's value, parsed (a handler)
//	ui.Changed                   the fields set (an OnChange hook)
//	anything the Injector fills  the Injector's value
//
// The Injector is asked first, after the context: a type it fills is never
// bound from the event. The app serving a page puts its Injector in the
// context its renders and sessions run with (WithInjector); package web
// does, giving the request, its identity types and every type registered
// with app.Service or web.Provide. CheckParams reports, before anything is
// served, the parameters of a page's hooks and handlers nothing fills.
type Injector interface {
	// Inject is how to fill a parameter of type t, called with the
	// hook's or handler's context; nil when the injector doesn't fill t.
	Inject(t reflect.Type) func(ctx context.Context) (reflect.Value, error)
}

type injectorKey struct{}

// WithInjector is ctx carrying inj, which fills the hooks' and handlers'
// parameters of the pages rendered and run with it.
func WithInjector(ctx context.Context, inj Injector) context.Context {
	return context.WithValue(ctx, injectorKey{}, inj)
}

// InjectorOf is the Injector ctx carries; nil when none.
func InjectorOf(ctx context.Context) Injector {
	inj, _ := ctx.Value(injectorKey{}).(Injector)
	return inj
}

// callKind is what a call gives beyond the context and the injector.
type callKind int

const (
	hookCall   callKind = iota // Mount, a Loads loader: nothing
	changeCall                 // OnChange: the Changed set
	eventCall                  // an event handler: the event's one payload
)

// slot is how one parameter is filled.
type slot struct {
	t       reflect.Type
	ctx     bool
	changed bool
	payload bool
	inject  func(context.Context) (reflect.Value, error)
}

// issue is a parameter nothing fills: its position (the receiver
// excluded), its type, and why.
type issue struct {
	index int
	t     reflect.Type
	dep   bool // a pointer or interface nothing provides
	why   string
}

var (
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	eventType           = reflect.TypeFor[Event]()
	changedType         = reflect.TypeFor[Changed]()
)

// methodName is a function's or method's short name: "mount", "Add".
func methodName(fn reflect.Value) string {
	f := runtime.FuncForPC(fn.Pointer())
	if f == nil {
		return fn.Type().String()
	}
	name := strings.TrimSuffix(f.Name(), "-fm")
	return name[strings.LastIndexByte(name, '.')+1:]
}

// payloadType reports whether an event can fill a parameter of type t: the
// event itself, its form, a struct its form binds, or its value.
func payloadType(t reflect.Type) bool {
	switch {
	case t == eventType:
		return true
	case t.Kind() == reflect.Map:
		return reflect.TypeFor[url.Values]().ConvertibleTo(t)
	case t.Kind() == reflect.Struct:
		return true
	case t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct:
		return exportedFields(t.Elem()) // a *sql.DB is a missing dependency, not a form
	}
	return scalarType(t)
}

func exportedFields(t reflect.Type) bool {
	for _, f := range reflect.VisibleFields(t) {
		if f.IsExported() && !f.Anonymous {
			return true
		}
	}
	return false
}

// scalarType reports whether t is set from an event's value: a string, a
// number, a bool, or a TextUnmarshaler.
func scalarType(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// plan says how each of ft's parameters after the first skip is filled for
// a call of kind, inj filling what the call doesn't; the parameters nothing
// fills are its issues.
func plan(ft reflect.Type, skip int, kind callKind, inj Injector) ([]slot, []issue) {
	var slots []slot
	var issues []issue
	payload := -1
	for i := skip; i < ft.NumIn(); i++ {
		t := ft.In(i)
		s := slot{t: t}
		switch {
		case t == ctxType:
			s.ctx = true
		case inj != nil && inj.Inject(t) != nil:
			s.inject = inj.Inject(t)
		case kind == changeCall && t == changedType:
			s.changed = true
		case kind == eventCall && payloadType(t) && payload < 0:
			s.payload, payload = true, i-skip
		case kind == eventCall && payloadType(t):
			issues = append(issues, issue{i - skip, t, false, fmt.Sprintf("is a second value from the event, which carries one (the %s parameter takes it)", ordinal(payload+1))})
		case t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface:
			issues = append(issues, issue{i - skip, t, true, "is not provided"})
		case kind == eventCall:
			issues = append(issues, issue{i - skip, t, false, "can't come from an event"})
		default:
			issues = append(issues, issue{i - skip, t, false, "can't come from a hook's call, which gives the context" + map[bool]string{true: " and the fields set", false: ""}[kind == changeCall]})
		}
		slots = append(slots, s)
	}
	return slots, issues
}

// call calls fn with recv first, its other parameters filled for a call of
// kind: c is an OnChange hook's set, bind an event handler's payload.
func call(ctx context.Context, fn reflect.Value, recv []reflect.Value, kind callKind, c Changed, bind func(reflect.Type) (reflect.Value, error)) error {
	ft := fn.Type()
	if ft.IsVariadic() || ft.NumOut() > 1 || ft.NumOut() == 1 && ft.Out(0) != errType {
		return fmt.Errorf("%s is %s: a hook or handler returns nothing or an error", methodName(fn), ft)
	}
	slots, issues := plan(ft, len(recv), kind, InjectorOf(ctx))
	if len(issues) > 0 {
		is := issues[0]
		return fmt.Errorf("%s's %s parameter, %s, %s", methodName(fn), ordinal(is.index+1), is.t, is.why)
	}
	args := append(make([]reflect.Value, 0, ft.NumIn()), recv...)
	for _, s := range slots {
		var v reflect.Value
		var err error
		switch {
		case s.ctx:
			v = reflect.ValueOf(&ctx).Elem()
		case s.inject != nil:
			v, err = s.inject(ctx)
		case s.changed:
			v = reflect.ValueOf(c)
		case s.payload:
			v, err = bind(s.t)
		}
		if err != nil {
			return err
		}
		if !v.IsValid() {
			v = reflect.Zero(s.t)
		}
		args = append(args, v)
	}
	out := fn.Call(args)
	if len(out) == 1 && !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}

// CallHandler calls an event handler, fn, its parameters filled by type
// (see Injector): bind gives the event's payload, as a parameter of the
// type it is asked for. Package live calls it for each event.
func CallHandler(ctx context.Context, fn any, bind func(t reflect.Type) (reflect.Value, error)) error {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() {
		return fmt.Errorf("a handler is %T, not a function", fn)
	}
	return call(ctx, v, nil, eventCall, Changed{}, bind)
}

// A ParamError is a parameter of a page's hook or event handler nothing
// fills, as CheckParams reports it.
type ParamError struct {
	Page       reflect.Type // the page's struct
	Method     string       // the hook's or handler's method: "Add", "mount"
	Hook       bool         // a hook (Mount, OnChange, Loads), not an event handler
	Index      int          // the parameter's position, from 0, the receiver excluded
	Type       reflect.Type // the parameter's
	Dependency bool         // a pointer or interface nothing provides
	Reason     string       // "is not provided", "can't come from an event", …
	File       string       // where the method is declared, as the compiler places it
	Line       int
}

func (e *ParamError) Error() string {
	at := ""
	if e.File != "" {
		at = fmt.Sprintf("%s:%d: ", e.File, e.Line)
	}
	return fmt.Sprintf("%s%s.%s's %s parameter, %s, %s", at, e.Page.Name(), e.Method, ordinal(e.Index+1), e.Type, e.Reason)
}

// CheckParams reports each parameter of the page t's hooks and event
// handlers that nothing fills, inj filling what the call doesn't. A hook
// (what its LiveSchema sets) is reported for any parameter; an event
// handler — an exported method returning nothing or an error, but its
// lifecycle's (Render, Mount, Update, Info) and those promoted from what it
// embeds — for a pointer or interface nothing provides, when the rest of
// its parameters fit an event (a method taking two values is one the
// page's own Go calls, and an on… attribute naming it fails as it
// renders). An app checks its pages with it before serving them. A page
// backed by its island, whose methods take JSON arguments, isn't checked.
func CheckParams(t reflect.Type, inj Injector) []*ParamError {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || IsIsland(t) {
		return nil
	}
	var out []*ParamError
	add := func(name string, hook bool, ft reflect.Type, skip int, kind callKind, file string, line int) {
		_, issues := plan(ft, skip, kind, inj)
		if !hook && slices.ContainsFunc(issues, func(is issue) bool { return !is.dep }) {
			// No event fits it — two values, a channel: a method the page's
			// Go calls (a closure's p.Toggle(id, done)), which an on…
			// attribute naming it reports as it renders.
			return
		}
		for _, is := range issues {
			out = append(out, &ParamError{Page: t, Method: name, Hook: hook, Index: is.index, Type: is.t,
				Dependency: is.dep, Reason: is.why, File: file, Line: line})
		}
	}
	if s := liveSchemaOf(t); s != nil {
		for _, h := range s.hookFns {
			kind := hookCall
			if h.kind == "OnChange" {
				kind = changeCall
			}
			add(h.name, true, h.fn.Type(), 0, kind, h.file, h.line)
		}
	}
	pt := reflect.PointerTo(t)
	for i := 0; i < pt.NumMethod(); i++ {
		m := pt.Method(i)
		if lifecycle[m.Name] || m.Name == "LiveSchema" || IsUpdate(m) || isHook(pt, m.Name) || promoted(t, m.Name) {
			continue
		}
		mt := m.Type
		if mt.IsVariadic() || mt.NumOut() > 1 || mt.NumOut() == 1 && mt.Out(0) != errType {
			continue
		}
		file, line := declaredAt(m.Func)
		if file == "<autogenerated>" {
			if vm, ok := t.MethodByName(m.Name); ok {
				file, line = declaredAt(vm.Func)
			}
		}
		add(m.Name, false, mt, 1, eventCall, file, line)
	}
	// The Clean and Choices methods of the page's forms, called as hooks are.
	for _, f := range reflect.VisibleFields(t) {
		ft := derefType(f.Type)
		if !f.IsExported() || f.Anonymous || !isForm(ft) {
			continue
		}
		form := formTypeOf(ft)
		methods := []*reflect.Method{form.clean}
		for _, fl := range form.fields {
			methods = append(methods, form.cleans[fl.path], fl.choices)
		}
		for _, m := range methods {
			if m != nil {
				file, line := declaredAt(m.Func)
				add(f.Name+"."+m.Name, true, m.Type, 1, hookCall, file, line)
			}
		}
	}
	return out
}

// promoted reports whether t's method name is one of a field it embeds.
func promoted(t reflect.Type, name string) bool {
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); f.Anonymous {
			if _, ok := reflect.PointerTo(f.Type).MethodByName(name); ok {
				return true
			}
			if _, ok := f.Type.MethodByName(name); ok {
				return true
			}
		}
	}
	return false
}

// declaredAt is where fn's code is: the source file's base name and line.
func declaredAt(fn reflect.Value) (string, int) {
	f := runtime.FuncForPC(fn.Pointer())
	if f == nil {
		return "", 0
	}
	file, line := f.FileLine(f.Entry())
	if i := strings.LastIndexByte(file, '/'); i >= 0 {
		file = file[i+1:]
	}
	return file, line
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
