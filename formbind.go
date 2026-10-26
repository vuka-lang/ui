package ui

import (
	"context"
	"encoding"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/vuka-lang/ui/v2/forms"
)

// The hidden fields a <ui.Form> carries, which no form field may be named:
const (
	FormIDField     = "_vk_form"   // the form's identity: its type's name and its key
	FormFieldsField = "_vk_fields" // the fields it rendered, signed
	FormUsedField   = "_vk_used"   // the fields the visitor used, as the runtime marks them
	FormIdemField   = "_vk_idem"   // the submit's idempotency key
	FormCSRFField   = "_vk_csrf"   // the CSRF token
	FormSeqField    = "_vk_seq"    // the runtime's sequence number of a change
)

// bind sets the form's fields from what a submit or a change sent. The form
// is the allow-list: a value naming no field of it is ignored. A field the
// values leave out keeps its value, unless the form rendered it — then it is
// empty: an unticked checkbox (false), a checkbox group with none ticked (an
// empty slice). rendered names each rendered field with its model
// modifiers ("raw", "number", "lazy", comma-separated). A value that doesn't
// parse leaves the field empty and is kept as typed, its error the field's
// until it changes.
func (st *formState) bind(form reflect.Value, vals url.Values, rendered map[string]string) {
	st.raw, st.parsed = nil, nil
	for _, f := range st.ft.fields {
		if f.group {
			continue
		}
		fv, err := form.FieldByIndexErr(f.index)
		if err != nil || !fv.CanSet() {
			continue
		}
		mods, shown := rendered[f.path]
		vs, sent := lookupValues(vals, f.path)
		if !sent {
			if shown {
				fv.SetZero()
			}
			continue
		}
		raw := f.raw || hasMod(mods, "raw")
		if rule, text := setField(fv, vs, raw, hasMod(mods, "number")); rule != "" {
			if st.raw == nil {
				st.raw = map[string]string{}
			}
			st.raw[f.path] = text
			st.parseErr(f, rule)
		}
	}
}

// parseErr records a value that didn't parse as the field's error.
func (st *formState) parseErr(f *formField, rule string) {
	if st.parsed == nil {
		st.parsed = map[string]forms.Error{}
	}
	e := forms.Error{Path: f.path, Rule: rule}
	if k := f.msgs[rule]; k != "" {
		e.Key = k
	}
	st.parsed[f.path] = e
}

func hasMod(mods, m string) bool {
	return mods != "" && slices.Contains(strings.Split(mods, ","), m)
}

// lookupValues are the values sent for path, its name matched exactly,
// else ignoring case.
func lookupValues(vals url.Values, path string) ([]string, bool) {
	if vs, ok := vals[path]; ok {
		return vs, true
	}
	for k, vs := range vals {
		if strings.EqualFold(k, path) {
			return vs, true
		}
	}
	return nil, false
}

// setField sets v from the values sent for it: a slice takes each, anything
// else the last (a checkbox's hidden fallback comes first). Text is trimmed
// unless raw. A value that doesn't parse leaves v empty; the rule it broke
// ("number", "whole", "format") and its text are returned.
func setField(v reflect.Value, vs []string, raw, number bool) (string, string) {
	clean := func(s string) string {
		if raw {
			return s
		}
		return strings.TrimSpace(s)
	}
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
		var kept []string
		for _, s := range vs {
			if s = clean(s); s != "" {
				kept = append(kept, s)
			}
		}
		if len(kept) == 0 {
			v.SetZero()
			return "", ""
		}
		out := reflect.MakeSlice(v.Type(), len(kept), len(kept))
		for i, s := range kept {
			if rule := setOne(out.Index(i), s, number); rule != "" {
				v.SetZero()
				return rule, strings.Join(kept, ", ")
			}
		}
		v.Set(out)
		return "", ""
	}
	s := ""
	if len(vs) > 0 {
		s = clean(vs[len(vs)-1])
	}
	if rule := setOne(v, s, number); rule != "" {
		return rule, s
	}
	return "", ""
}

// setOne sets v from s; the rule s broke when it doesn't parse, leaving v
// empty.
func setOne(v reflect.Value, s string, number bool) string {
	if v.Kind() == reflect.Pointer {
		if s == "" {
			v.SetZero()
			return ""
		}
		p := reflect.New(v.Type().Elem())
		if rule := setOne(p.Elem(), s, number); rule != "" {
			v.SetZero()
			return rule
		}
		v.Set(p)
		return ""
	}
	if v.Type() == timeType {
		if s == "" {
			v.SetZero()
			return ""
		}
		for _, layout := range []string{time.DateOnly, "2006-01-02T15:04", "2006-01-02T15:04:05", time.RFC3339, "15:04"} {
			if t, err := time.Parse(layout, s); err == nil {
				v.Set(reflect.ValueOf(t))
				return ""
			}
		}
		v.SetZero()
		return "format"
	}
	if u, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok && v.Kind() != reflect.String {
		if err := u.UnmarshalText([]byte(s)); err != nil {
			v.SetZero()
			return "format"
		}
		return ""
	}
	switch v.Kind() {
	case reflect.String:
		if number && s != "" {
			if _, err := strconv.ParseFloat(s, 64); err != nil {
				v.SetZero()
				return "number"
			}
		}
		v.SetString(s)
	case reflect.Bool:
		switch strings.ToLower(s) {
		case "", "false", "off", "0", "no":
			v.SetBool(false)
		default:
			v.SetBool(true)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if s == "" {
			v.SetInt(0)
			return ""
		}
		n, err := strconv.ParseInt(strings.ReplaceAll(s, ",", ""), 10, v.Type().Bits())
		if err != nil {
			v.SetZero()
			if _, ferr := strconv.ParseFloat(s, 64); ferr == nil {
				return "whole"
			}
			return "number"
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if s == "" {
			v.SetUint(0)
			return ""
		}
		n, err := strconv.ParseUint(strings.ReplaceAll(s, ",", ""), 10, v.Type().Bits())
		if err != nil {
			v.SetZero()
			if _, ferr := strconv.ParseFloat(s, 64); ferr == nil {
				return "whole"
			}
			return "number"
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		if s == "" {
			v.SetFloat(0)
			return ""
		}
		n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), v.Type().Bits())
		if err != nil {
			v.SetZero()
			return "number"
		}
		v.SetFloat(n)
	default:
		return "format"
	}
	return ""
}

// fieldText is a field's value as its control shows it: what was typed
// when it didn't parse, else the value formatted.
func (st *formState) fieldText(form reflect.Value, f *formField) string {
	if s, ok := st.raw[f.path]; ok {
		return s
	}
	v, err := form.FieldByIndexErr(f.index)
	if err != nil {
		return ""
	}
	return valueText(v)
}

// valueText is v as a control's value: a date as 2006-01-02, a number
// without exponent, a TextMarshaler's text.
func valueText(v reflect.Value) string {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Type() == timeType {
		t := v.Interface().(time.Time)
		if t.IsZero() {
			return ""
		}
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
			return t.Format(time.DateOnly)
		}
		return t.Format("2006-01-02T15:04")
	}
	if m, ok := v.Interface().(encoding.TextMarshaler); ok && v.Kind() != reflect.String {
		b, err := m.MarshalText()
		if err != nil {
			return ""
		}
		return string(b)
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Int() == 0 {
			return ""
		}
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Uint() == 0 {
			return ""
		}
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		if v.Float() == 0 {
			return ""
		}
		return strconv.FormatFloat(v.Float(), 'f', -1, v.Type().Bits())
	}
	return fmt.Sprint(v.Interface())
}

// optionText is one option's or item's value as text, to match against
// what a control sends.
func optionText(v any) string {
	if v == nil {
		return ""
	}
	return valueText(reflect.ValueOf(v))
}

// selected reports whether the field's value is, or (a slice) holds, the
// option's.
func selected(form reflect.Value, f *formField, option string) bool {
	v, err := form.FieldByIndexErr(f.index)
	if err != nil {
		return false
	}
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
		for i := 0; i < v.Len(); i++ {
			if valueText(v.Index(i)) == option {
				return true
			}
		}
		return false
	}
	if v.Kind() == reflect.Bool {
		return v.Bool() && (option == "" || option == "true" || option == "on")
	}
	s := valueText(v)
	return s == option
}

// digitsOnly reports whether s holds only digits (and spaces, which a
// numeric code may be typed with).
func digitsOnly(s string) bool {
	for _, c := range s {
		if !unicode.IsDigit(c) && c != ' ' {
			return false
		}
	}
	return true
}

// formCtx is the context a form's checks run with: live while the visitor
// types.
func formCtx(ctx context.Context, live bool) context.Context {
	if live {
		return forms.WithLive(ctx)
	}
	return ctx
}
