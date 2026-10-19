package ui

import (
	"encoding/json"
	"reflect"
	"strings"
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
