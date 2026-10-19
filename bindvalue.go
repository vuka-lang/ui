package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Setter is the handler of a bind:value input: it sets a page's bind:
// field as SetBound does. Package live renders the page again after it,
// and keeps what it set as the page's props.
type Setter func(ctx context.Context, ev Event) error

// BindValue is the attributes of <input bind:value={p.Q}>, field a pointer
// to a field of the struct owner points to: the field's value as the
// input's, and an onInput setting the field to what is typed, after the
// field's debounce, validated as a set of a bound field is. The field is
// bound by its owner's LiveSchema (l.Field(&p.Q, ui.Bind{})) or a bind:
// tag; any other is an error at render.
func BindValue(owner, field any) []Attr {
	ov, fv := reflect.ValueOf(owner), reflect.ValueOf(field)
	if ov.Kind() != reflect.Pointer || ov.Elem().Kind() != reflect.Struct || fv.Kind() != reflect.Pointer {
		return []Attr{{Name: "value", Value: Try(nil, fmt.Errorf("ui: bind:value takes a field of a page's struct, not %T", field))}}
	}
	fields, err := IslandFields(ov.Type())
	if err != nil {
		return []Attr{{Name: "value", Value: Try(nil, err)}}
	}
	st := ov.Elem()
	for _, f := range fields {
		if st.Field(f.Index).Addr().Pointer() != fv.Pointer() {
			continue
		}
		if !f.Bind {
			return []Attr{{Name: "value", Value: Try(nil, fmt.Errorf("ui: bind:value: %s.%s isn't bound: bind it in LiveSchema, l.Field(&p.%[2]s, ui.Bind{})", ov.Type().Elem().Name(), st.Type().Field(f.Index).Name))}}
		}
		wire, str := f.Wire, f.Type.Kind() == reflect.String
		set := Setter(func(ctx context.Context, ev Event) error {
			raw := json.RawMessage(strings.TrimSpace(ev.Value))
			if str || !json.Valid(raw) {
				raw, _ = json.Marshal(ev.Value)
			}
			data, _ := json.Marshal(map[string]json.RawMessage{wire: raw})
			return SetBound(ctx, ov, data)
		})
		return []Attr{{Name: "value", Value: st.Field(f.Index).Interface()}, {Name: "onInput", Value: JS.Debounce(f.Debounce).Push(set)}}
	}
	return []Attr{{Name: "value", Value: Try(nil, fmt.Errorf("ui: bind:value: the field isn't one of %s's", ov.Type().Elem().Name()))}}
}

// CSSVars is a style attribute with CSS variables: style:--accent={c}
// beside style="…" or style={…}. style is the attribute's own value (a
// string of declarations, a Style or nil), vars names and values in turn;
// the values are sanitized as a Style's are.
func CSSVars(style any, vars ...any) Style {
	out := Style{}
	switch s := style.(type) {
	case Style:
		for k, v := range s {
			out[k] = v
		}
	case map[string]string:
		for k, v := range s {
			out[k] = v
		}
	case string:
		for _, decl := range strings.Split(s, ";") {
			if k, v, ok := strings.Cut(decl, ":"); ok {
				out[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	for i := 0; i+1 < len(vars); i += 2 {
		out[fmt.Sprint(vars[i])] = toString(vars[i+1])
	}
	return out
}
