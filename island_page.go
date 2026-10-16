package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
)

// Pages backed by a component. An @ui.Island struct routed as a page
// (@web.Live, or @web.Get for a stateless one) renders as its island, in
// the layout its decorator's Layout gives: the exported fields are the
// props, the exported methods what the component calls (useLive<T>() in it),
// and the unexported fields the server's own. Embedding ui.Live makes it a
// live page — the compiler writes its Render, IslandPage(x) — whose methods
// run as live events; without it the page is stateless and its methods are
// calls package web serves (CallAt).

// IsIsland reports whether t (a struct, or a pointer to one) is a
// registered island.
func IsIsland(t reflect.Type) bool {
	if t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return lookupIsland(t) != nil
}

// lifecycle are the methods a live component's session calls, and Changed
// (run after the component sets bound fields): never ones the component
// calls.
var lifecycle = map[string]bool{"Render": true, "Mount": true, "Update": true, "Info": true, "Changed": true}

var liveMethods = func() map[string]bool {
	m := map[string]bool{}
	t := reflect.TypeFor[*Live]()
	for i := 0; i < t.NumMethod(); i++ {
		m[t.Method(i).Name] = true
	}
	return m
}()

// IslandMethod is an exported method of a page-backed island that its
// component may call: its wire name (the method's, first letter lower-cased)
// and its arguments' types (a leading context.Context and an error result
// left out).
type IslandMethod struct {
	Name   string
	Method reflect.Method
	Args   []reflect.Type
	ctx    bool
}

var islandMethodsOf sync.Map // reflect.Type (pointer) → []IslandMethod

// IslandMethods are the methods of t (a pointer to an island's struct) its
// component may call: the exported ones that aren't a live component's
// lifecycle (Render, Mount, Update, Info) or ui.Live's, return nothing or
// an error (or a value and an error), and take JSON arguments.
func IslandMethods(t reflect.Type) []IslandMethod {
	if t.Kind() != reflect.Pointer {
		t = reflect.PointerTo(t)
	}
	if v, ok := islandMethodsOf.Load(t); ok {
		return v.([]IslandMethod)
	}
	var out []IslandMethod
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if lifecycle[m.Name] || liveMethods[m.Name] || strings.HasPrefix(m.Name, "Update__") || isLoader(t, m.Name) {
			continue
		}
		mt := m.Type
		if mt.NumOut() > 1 || mt.NumOut() == 1 && mt.Out(0) != errType || mt.IsVariadic() {
			continue
		}
		im := IslandMethod{Name: strings.ToLower(m.Name[:1]) + m.Name[1:], Method: m}
		for j := 1; j < mt.NumIn(); j++ {
			if j == 1 && mt.In(j) == ctxType {
				im.ctx = true
				continue
			}
			im.Args = append(im.Args, mt.In(j))
		}
		out = append(out, im)
	}
	islandMethodsOf.Store(t, out)
	return out
}

// Call calls the method on x (a pointer to the island's struct) with args
// as the JSON array the component sent. Each argument is validated first
// (Validate): a struct's fields are named by their own paths (years,
// level.code), a slice's by index (0.code); any failing, the method doesn't
// run and Call answers Invalid.
func (m IslandMethod) Call(ctx context.Context, x reflect.Value, args json.RawMessage) error {
	var raw []json.RawMessage
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &raw); err != nil {
			raw = []json.RawMessage{args} // one argument, sent bare
		}
	}
	if len(raw) > len(m.Args) {
		return publicError{fmt.Errorf("%s takes %d arguments, not %d", m.Name, len(m.Args), len(raw))}
	}
	in := []reflect.Value{x}
	if m.ctx {
		in = append(in, reflect.ValueOf(ctx))
	}
	bad := Invalid{}
	for i, t := range m.Args {
		v := reflect.New(t)
		if i < len(raw) {
			if err := json.Unmarshal(raw[i], v.Interface()); err != nil {
				return publicError{fmt.Errorf("%s's argument %d: %v", m.Name, i+1, err)}
			}
		}
		if err := walkValidate("", v.Elem(), nil, bad); err != nil {
			return err
		}
		in = append(in, v.Elem())
	}
	if len(bad) > 0 {
		return bad
	}
	out := m.Method.Func.Call(in)
	if len(out) == 1 && !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}

// publicError is an error whose message the browser may read.
type publicError struct{ error }

func (e publicError) PublicMessage() string { return e.Error() }

// IslandError is a failed call as the component's live.error reads it: a
// status, a message and, for an error naming fields (FieldErrors), each
// field's message. An error that isn't public (PublicMessage) shows as
// "error".
type IslandError struct {
	Status  int               `json:"status,omitempty"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// ErrorOf is err as the component reads it.
func ErrorOf(err error) *IslandError {
	e := &IslandError{Message: "error"}
	var pub interface{ PublicMessage() string }
	if errors.As(err, &pub) {
		e.Message = pub.PublicMessage()
	}
	var st interface{ StatusCode() int }
	if errors.As(err, &st) {
		e.Status = st.StatusCode()
	}
	var fe interface{ FieldErrors() map[string]string }
	if errors.As(err, &fe) {
		e.Fields = fe.FieldErrors()
	}
	return e
}

// pageErrors are the live pages' last failed calls, by instance.
var pageErrors sync.Map // pointer → *IslandError

// IsRedirect, set by package web, tells an error that must reach the live
// session (a redirect) from one the component shows.
var IsRedirect = func(error) bool { return false }

// IslandPage is the markup of a page backed by its island: x (a pointer to
// the @ui.Island struct) mounted on load, its methods bound for its
// component — on a live page, each a handler the live session runs, and one
// for the component's sets of bound fields — its fields' handles
// (data-vk-fields) and the last failed call's error (data-vk-err).
func IslandPage(x any, opts ...IslandOpt) Node {
	rv := reflect.ValueOf(x)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return Try(nil, fmt.Errorf("ui: IslandPage takes a pointer to an island's struct, not %T", x))
	}
	o := append([]IslandOpt{Load()}, opts...)
	var probe islandConfig
	for _, op := range opts {
		op(&probe)
	}
	var names []string
	for _, m := range IslandMethods(rv.Type()) {
		names = append(names, m.Name)
		if probe.callURL != "" {
			continue // a stateless page's: called at its URL
		}
		m := m
		handler := func(ctx context.Context, ev Event) error {
			err := m.Call(ctx, rv, ev.Data)
			if err != nil && IsRedirect(err) {
				return err
			}
			if err != nil {
				pageErrors.Store(x, ErrorOf(err))
			} else {
				pageErrors.Delete(x)
			}
			return nil
		}
		o = append(o, func(c *islandConfig) { c.methods = append(c.methods, islandEmit{m.Name, handler}) })
	}
	fields, err := IslandFields(rv.Type())
	if err != nil {
		return Try(nil, fmt.Errorf("ui: IslandPage: %w", err))
	}
	if probe.callURL == "" {
		set := func(ctx context.Context, ev Event) error {
			err := SetBound(ctx, rv, ev.Data)
			if err != nil && IsRedirect(err) {
				return err
			}
			if err != nil {
				pageErrors.Store(x, ErrorOf(err))
			} else {
				pageErrors.Delete(x)
			}
			return nil
		}
		o = append(o, func(c *islandConfig) { c.methods = append(c.methods, islandEmit{"_set", set}) })
	}
	var loads *Loads
	if s, ok := x.(Stateful); ok && probe.callURL == "" {
		loads = liveLoads(s)
	}
	o = append(o, func(c *islandConfig) {
		c.methodNames = names
		c.fields = fieldsJSON(fields)
		c.deferred = true
		if loads != nil {
			c.loads = loads
		}
		if e, ok := pageErrors.Load(x); ok {
			c.err = e.(*IslandError)
		}
	})
	return MountIsland("page:"+rv.Type().Elem().String(), rv.Elem().Interface(), o...)
}

// CallAt makes a page-backed island's methods calls to url, a stateless
// page's: the component posts the method, its arguments and its props with
// sig (the server's signature of them) and is updated with the props the
// call leaves. Package web writes it.
func CallAt(url, sig string) IslandOpt {
	return func(c *islandConfig) { c.callURL, c.callSig = url, sig }
}

// IslandErr shows err as the island's last failed call (data-vk-err).
func IslandErr(err *IslandError) IslandOpt { return func(c *islandConfig) { c.err = err } }

// Viewer is what every island's component knows of the visitor — $vuka in
// Vue, useVuka() in React, the vuka store in Svelte: package web/auth sets
// it to the signed-in identity's id, roles and perms.
var Viewer func(ctx context.Context) any

// viewerJSON is an island's data-vk-viewer, written at render time.
type viewerJSON struct{}

func (viewerJSON) Render(ctx context.Context, w io.Writer) error {
	var v any
	if Viewer != nil {
		v = Viewer(ctx)
	}
	j, err := marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(j)
	return err
}

// urlFields are the fields of an island's struct tagged url:"name": its
// wire name → the query parameter the page's address keeps it in.
func urlFields(t reflect.Type) map[string]string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out map[string]string
	for _, f := range reflect.VisibleFields(t) {
		q, _, _ := strings.Cut(f.Tag.Get("url"), ",")
		if !f.IsExported() || q == "" || q == "-" {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			name = f.Name
		}
		if out == nil {
			out = map[string]string{}
		}
		out[name] = q
	}
	return out
}

// PropsJSON is props (an island's struct) as its data-vk-props holds it;
// package web signs a stateless page's. Its deferred fields not loaded
// (WithLoads; without it, none) are left out, as IslandPage leaves them.
func PropsJSON(ctx context.Context, props any, opts ...IslandOpt) ([]byte, error) {
	var cfg islandConfig
	for _, o := range opts {
		o(&cfg)
	}
	fields, funcs, ok := islandFields(props)
	if ok {
		fields = withoutPending(fields, cfg.loads, reflect.TypeOf(props))
	}
	return islandPropsJSON{props: props, fields: fields, funcs: funcs, plain: !ok}.json(ctx)
}

// UntagProps is props JSON as encoding/json reads it into the island's
// struct: each tagged date as its text; function and state props left out.
func UntagProps(data []byte) ([]byte, error) {
	var m map[string]any
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.UseNumber()
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	var untag func(v any) (any, bool)
	untag = func(v any) (any, bool) {
		switch x := v.(type) {
		case map[string]any:
			switch x["$vk"] {
			case "date":
				return x["v"], true
			case "fn", "state":
				return nil, false
			}
			for k, e := range x {
				if u, keep := untag(e); keep {
					x[k] = u
				} else {
					delete(x, k)
				}
			}
		case []any:
			for i, e := range x {
				x[i], _ = untag(e)
			}
		}
		return v, true
	}
	out, _ := untag(map[string]any(m))
	return json.Marshal(out)
}
