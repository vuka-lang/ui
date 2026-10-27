package ui

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/vuka-lang/ui/v2/forms"
)

// Browser checks: a control carries the constraint attributes whose
// meaning in the browser is exactly the server's, and the rules the
// runtime can check before a submit sent without a connection, with their
// messages — the server's words, the server still deciding.
//
// Left out, and why (vuka vet notes it on the field):
//   - pattern, always: the browser's regular expressions aren't Go's;
//   - minlength and maxlength on text, which the browser counts in UTF-16
//     units and Go in characters (an emoji is 2 to one, 1 to the other):
//     written only on a numeric field (input:"numeric"), whose characters
//     count the same both ways;
//   - required, unless Config.RequiredAttr: the browser takes spaces for a
//     value, where the binder trims them.

// constraintAttrs are a control's constraint attributes: minlength and
// maxlength on a numeric text field, min and max on a number input.
func constraintAttrs(f *formField, typ string) []Attr {
	var out []Attr
	base := derefType(f.typ)
	for _, r := range f.rules {
		switch {
		case f.input == "numeric" && base.Kind() == reflect.String:
			switch r.name {
			case "len":
				out = append(out, Attr{"minlength", r.param}, Attr{"maxlength", r.param})
			case "min":
				out = append(out, Attr{"minlength", r.param})
			case "max":
				out = append(out, Attr{"maxlength", r.param})
			}
		case typ == "number" && (isInt(base) || isFloat(base)):
			switch r.name {
			case "min", "max":
				out = append(out, Attr{r.name, r.param})
			}
		}
	}
	return out
}

// checkKind is how the runtime reads a field's value for its checks: "s"
// text, "i" a whole number, "f" a number, "b" a checkbox, "l" the ticked
// boxes of a group; "" for a value it can't read as the binder does (a
// date, a TextUnmarshaler).
func checkKind(f *formField) string {
	base := derefType(f.typ)
	switch {
	case base == timeType || reflect.PointerTo(base).Implements(textUnmarshaler) && base.Kind() != reflect.String:
		return ""
	case base.Kind() == reflect.String:
		return "s"
	case isInt(base):
		return "i"
	case isFloat(base):
		return "f"
	case base.Kind() == reflect.Bool:
		return "b"
	case base.Kind() == reflect.Slice && base.Elem().Kind() != reflect.Uint8:
		if e := derefType(base.Elem()); e.Kind() == reflect.String || isInt(e) {
			return "l"
		}
	}
	return ""
}

// checkSpec is a control's data-vk-check: the field's rules the runtime
// checks as the server does, in the server's order, each with its message
// and key, stopping at the first it can't (nil when it can check none):
//
//	{"k":"s","t":1,"r":[["required",null,"Enter your email address","validate.required.email"],["email",null,…]]}
//
// k is the kind (checkKind), t whether the value is trimmed first, d that
// only digits are allowed (input:"numeric": any other character, and the
// runtime leaves the field to the server).
func (b boundField) checkSpec(ctx context.Context, raw bool) any {
	f, st := b.f, b.st
	kind := checkKind(f)
	if kind == "" || len(f.rules) == 0 {
		return nil
	}
	var rules [][]any
	rule := func(r rule, param any) {
		e := f.errorFor(r, f.kind(r))
		key := e.Key
		if key == "" {
			key = forms.DefaultKey(e)
		}
		rules = append(rules, []any{r.name, param, st.message(ctx, e), key})
	}
loop:
	for _, r := range f.rules {
		switch r.name {
		case "omitempty":
			rules = append(rules, []any{"omitempty"})
		case "required":
			rule(r, nil)
		case "email":
			if kind != "s" {
				break loop
			}
			rule(r, nil)
		case "min", "max", "len":
			rule(r, r.n)
		case "oneof":
			if kind != "s" {
				break loop
			}
			rule(r, r.set)
		default:
			break loop
		}
	}
	if len(rules) == 0 {
		return nil
	}
	spec := map[string]any{"k": kind, "r": rules}
	if !raw {
		spec["t"] = 1
	}
	if f.input == "numeric" {
		spec["d"] = 1
	}
	out, err := json.Marshal(spec)
	if err != nil {
		return nil
	}
	return string(out)
}
