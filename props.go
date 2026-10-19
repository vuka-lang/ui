package ui

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"unsafe"
)

// Props is a component's data as its script reads it: the exported fields
// of the struct v (or *v) as a JSON object, named as encoding/json names
// them — but Nodes, funcs, channels and an embedded Live, which never
// leave the server. A .vuka.html template with a component script writes
// it on its root as data-vk-p.
func Props(v any) string {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return "{}"
	}
	out := map[string]any{}
	node := reflect.TypeFor[Node]()
	t := rv.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		ft := f.Type
		if !f.IsExported() || f.Anonymous && ft == reflect.TypeFor[Live]() || ft.Kind() == reflect.Func || ft.Kind() == reflect.Chan || ft.Implements(node) || ft.Kind() == reflect.Interface && node.Implements(ft) {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		if strings.Contains(opts, "omitempty") && rv.Field(i).IsZero() {
			continue
		}
		out[name] = rv.Field(i).Interface()
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// JSON is v as JSON for a script: {json(v)} in an inline script. Marshal's
// escaping of <, > and & keeps it from ending the script.
func JSON(v any) Node {
	b, err := json.Marshal(v)
	if err != nil {
		return Try(nil, err)
	}
	return RawHTML(b)
}

// States are a component's states as its script reaches them —
// ctx.state("open") — each ui.State field of the struct v (or *v),
// exported or not, by its json name or its name with its first letter
// lower-cased: a reference to the state the browser keeps. Nil when v has
// none. A template whose script calls ctx.state writes it on its root as
// data-vk-s.
func States(v any) Node {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	cp := reflect.New(rv.Type()).Elem()
	cp.Set(rv)
	anyState := reflect.TypeFor[AnyState]()
	var states stateRefs
	t := rv.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous || !f.Type.Implements(anyState) {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name[:1]) + f.Name[1:]
		}
		fv := cp.Field(i)
		s := reflect.NewAt(f.Type, unsafe.Pointer(fv.UnsafeAddr())).Elem().Interface().(AnyState)
		states = append(states, namedState{name, s})
	}
	if len(states) == 0 {
		return nil
	}
	return states
}

// stateRefs are States' value: a Node, written once the render's context
// gives the states their keys.
type stateRefs []namedState

type namedState struct {
	name string
	s    AnyState
}

func (r stateRefs) Render(ctx context.Context, w io.Writer) error {
	out := make(map[string]any, len(r))
	for _, s := range r {
		ref, err := stateProp(ctx, s.s)
		if err != nil {
			return err
		}
		out[s.name] = ref
	}
	b, err := marshal(out)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
