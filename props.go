package ui

import (
	"context"
	"encoding"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"sync"
	"unsafe"
)

// Props is a component's data as its script reads it: the exported fields
// of the struct v (or *v) as a JSON object, named as encoding/json names
// them — but Nodes, funcs, channels and an embedded Live, which never
// leave the server, and secrets: a form (a struct embedding FormState),
// whose values are what the visitor typed, and a field tagged as a
// password or a one-time code (input:"password", "new-password",
// "current-password", "one-time-code"), at any depth of the fields' types
// (a value held in an interface is written as encoding/json writes it). A script reads a
// form's values from its fields. A .vuka.html template with a component
// script writes it on its root as data-vk-p, in the page's HTML and in
// each live patch.
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
		if secretField(f) {
			continue
		}
		out[name] = propValue(rv.Field(i), false)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// secretField reports whether a struct field never leaves the server in
// props: a form, or a password or one-time code by its input tag.
func secretField(f reflect.StructField) bool {
	if IsFormField(f.Type) {
		return true
	}
	switch f.Tag.Get("input") {
	case "password", "new-password", "current-password", "one-time-code":
		return true
	}
	return false
}

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
	secretTypes   sync.Map // reflect.Type → bool
)

// holdsSecret reports whether a value of type t can hold a secret field
// (secretField) that encoding/json would write. A type marshalling itself
// (json.Marshaler, encoding.TextMarshaler) decides its own JSON.
func holdsSecret(t reflect.Type) bool {
	if v, ok := secretTypes.Load(t); ok {
		return v.(bool)
	}
	secretTypes.Store(t, false) // a recursive type's cycle reads false
	has := holdsSecretOf(t)
	secretTypes.Store(t, has)
	return has
}

func holdsSecretOf(t reflect.Type) bool {
	if t.Kind() != reflect.Interface {
		for _, m := range []reflect.Type{jsonMarshaler, textMarshaler} {
			if t.Implements(m) || reflect.PointerTo(t).Implements(m) {
				return false
			}
		}
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return holdsSecret(t.Elem())
	case reflect.Map:
		return holdsSecret(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() && !f.Anonymous || f.Tag.Get("json") == "-" {
				continue
			}
			if secretField(f) || holdsSecret(f.Type) {
				return true
			}
		}
	}
	return false
}

// propValue is v as a prop: v itself when its type holds no secret, else
// a copy encoding/json writes as it would v, its secret fields left out.
// For an island (dates), its dates are tagged as tagDates tags them.
func propValue(v reflect.Value, dates bool) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if IsFormField(v.Type()) {
		return nil
	}
	if !holdsSecret(v.Type()) {
		if dates {
			if t, err := tagDates(v.Interface()); err == nil {
				return t
			}
		}
		return v.Interface()
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return propValue(v.Elem(), dates)
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = propValue(v.Index(i), dates)
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		out := make(map[string]any, v.Len())
		for it := v.MapRange(); it.Next(); {
			b, err := json.Marshal(it.Key().Interface())
			if err != nil {
				continue
			}
			k := strings.Trim(string(b), `"`)
			out[k] = propValue(it.Value(), dates)
		}
		return out
	case reflect.Struct:
		out := map[string]any{}
		propFields(v, out, dates)
		return out
	}
	return v.Interface()
}

// propFields adds the fields of the struct v encoding/json would write to
// out, its secret fields left out; an embedded struct without a json name
// adds its fields, as encoding/json flattens it.
func propFields(v reflect.Value, out map[string]any, dates bool) {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() && !f.Anonymous || secretField(f) {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		fv := v.Field(i)
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				if fv.IsNil() {
					continue
				}
				ft, fv = ft.Elem(), fv.Elem()
			}
			if ft.Kind() == reflect.Struct {
				propFields(fv, out, dates)
				continue
			}
			if !f.IsExported() {
				continue
			}
		}
		if name == "" {
			name = f.Name
		}
		if strings.Contains(opts, "omitempty") && fv.IsZero() {
			continue
		}
		out[name] = propValue(fv, dates)
	}
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
