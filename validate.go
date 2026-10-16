package ui

import (
	"fmt"
	"net/http"
	"net/mail"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Validate checks v (a struct, a pointer to one, or a slice, array or map
// of them) against its fields' validate:"…" tags, and the structs they
// hold, at any depth. It answers nil, or an error naming each failing
// field by its path of json names (level.code, items.0.name) with its
// message: a 422 whose FieldErrors a component reads as error.fields.
//
//	required        not the zero value (a nil pointer, "", 0, false, an empty slice)
//	omitempty       the rest is skipped for the zero value
//	min=n max=n     a string's characters, a number's value, a slice's or map's items
//	len=n           exactly n of the same
//	oneof=a b c     one of the values (a|b|c also works)
//	email           an email address
func Validate(v any) error {
	return validateAt("", reflect.ValueOf(v), nil)
}

// CheckValidate reports a validate tag of t, or of a struct it holds, that
// isn't one of Validate's rules.
func CheckValidate(t reflect.Type) error {
	_, err := rulesOf(t)
	return err
}

// Invalid is the error Validate answers with: each field's path → its
// message.
type Invalid map[string]string

func (e Invalid) Error() string {
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("invalid")
	for i, k := range keys {
		if i == 0 {
			b.WriteString(": ")
		} else {
			b.WriteString("; ")
		}
		b.WriteString(k + ": " + e[k])
	}
	return b.String()
}

func (e Invalid) PublicMessage() string          { return e.Error() }
func (Invalid) StatusCode() int                  { return http.StatusUnprocessableEntity }
func (e Invalid) FieldErrors() map[string]string { return e }

// validateAt validates v, found at path; rules are the tag of the field
// holding it.
func validateAt(path string, v reflect.Value, rules []rule) error {
	bad := Invalid{}
	if err := walkValidate(path, v, rules, bad); err != nil {
		return err
	}
	if len(bad) == 0 {
		return nil
	}
	return bad
}

func walkValidate(path string, v reflect.Value, rules []rule, bad Invalid) error {
	if !v.IsValid() {
		return nil
	}
	for _, r := range rules {
		if r.name == "omitempty" {
			if v.IsZero() {
				return nil
			}
			continue
		}
		if msg := r.check(v); msg != "" {
			if _, ok := bad[pathKey(path)]; !ok {
				bad[pathKey(path)] = msg
			}
			return nil
		}
	}
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		tr, err := rulesOf(v.Type())
		if err != nil {
			return err
		}
		for _, f := range tr.fields {
			fv, err := v.FieldByIndexErr(f.index)
			if err != nil {
				continue
			}
			if err := walkValidate(joinPath(path, f.name), fv, f.rules, bad); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if !holdsStructs(v.Type().Elem()) {
			return nil
		}
		for i := 0; i < v.Len(); i++ {
			if err := walkValidate(joinPath(path, strconv.Itoa(i)), v.Index(i), nil, bad); err != nil {
				return err
			}
		}
	case reflect.Map:
		if !holdsStructs(v.Type().Elem()) {
			return nil
		}
		iter := v.MapRange()
		for iter.Next() {
			if err := walkValidate(joinPath(path, fmt.Sprint(iter.Key().Interface())), iter.Value(), nil, bad); err != nil {
				return err
			}
		}
	}
	return nil
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func pathKey(path string) string {
	if path == "" {
		return "value"
	}
	return path
}

func holdsStructs(t reflect.Type) bool { return baseType(t).Kind() == reflect.Struct }

type rule struct {
	name, param string
	n           float64
	set         []string
}

type fieldRules struct {
	name  string
	index []int
	rules []rule
}

type typeRules struct {
	fields []fieldRules
	err    error
}

var (
	typeRulesOf sync.Map // reflect.Type → *typeRules, complete
	rulesMu     sync.Mutex
)

// rulesOf are t's fields with rules or structs inside, t a struct (or a
// pointer, slice, array or map of them); a type holding itself is fine.
func rulesOf(t reflect.Type) (*typeRules, error) {
	t = baseType(t)
	if t.Kind() != reflect.Struct {
		return &typeRules{}, nil
	}
	if v, ok := typeRulesOf.Load(t); ok {
		tr := v.(*typeRules)
		return tr, tr.err
	}
	rulesMu.Lock()
	defer rulesMu.Unlock()
	building := map[reflect.Type]*typeRules{}
	tr := buildRules(t, building)
	for bt, r := range building {
		typeRulesOf.Store(bt, r)
	}
	return tr, tr.err
}

func baseType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	return t
}

func buildRules(t reflect.Type, building map[reflect.Type]*typeRules) *typeRules {
	if v, ok := typeRulesOf.Load(t); ok {
		return v.(*typeRules)
	}
	if tr, ok := building[t]; ok {
		return tr
	}
	tr := &typeRules{}
	building[t] = tr
	for _, f := range reflect.VisibleFields(t) {
		if !f.IsExported() || f.Anonymous {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		rules, err := parseRules(f.Tag.Get("validate"))
		if err != nil {
			tr.err = fmt.Errorf("%s.%s: %w", t.Name(), f.Name, err)
			return tr
		}
		if ft := baseType(f.Type); ft.Kind() == reflect.Struct {
			if inner := buildRules(ft, building); inner.err != nil {
				tr.err = inner.err
				return tr
			}
		}
		if len(rules) > 0 || holdsStructs(f.Type) {
			tr.fields = append(tr.fields, fieldRules{name: name, index: f.Index, rules: rules})
		}
	}
	return tr
}

func parseRules(tag string) ([]rule, error) {
	if tag == "" || tag == "-" {
		return nil, nil
	}
	var out []rule
	for _, part := range strings.Split(tag, ",") {
		name, param, _ := strings.Cut(strings.TrimSpace(part), "=")
		r := rule{name: name, param: param}
		switch name {
		case "":
			continue
		case "required", "omitempty", "email":
			if param != "" {
				return nil, fmt.Errorf("validate:%q: %s takes no value", tag, name)
			}
		case "min", "max", "len":
			n, err := strconv.ParseFloat(param, 64)
			if err != nil {
				return nil, fmt.Errorf("validate:%q: %s takes a number (%s=3)", tag, name, name)
			}
			r.n = n
		case "oneof":
			r.set = strings.FieldsFunc(param, func(c rune) bool { return c == ' ' || c == '|' })
			if len(r.set) == 0 {
				return nil, fmt.Errorf("validate:%q: oneof takes values (oneof=a b c)", tag)
			}
		default:
			return nil, fmt.Errorf("validate:%q: unknown rule %q (required, omitempty, min, max, len, oneof, email)", tag, name)
		}
		out = append(out, r)
	}
	return out, nil
}

// check is v's message when it breaks r, else "".
func (r rule) check(v reflect.Value) string {
	if r.name == "required" {
		if v.IsZero() {
			return "required"
		}
		return ""
	}
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch r.name {
	case "min", "max", "len":
		size, unit, ok := measure(v)
		if !ok {
			return ""
		}
		n := strconv.FormatFloat(r.n, 'f', -1, 64)
		if unit != "" && n != "1" {
			unit += "s"
		}
		if unit != "" {
			unit = " " + unit
		}
		switch {
		case r.name == "min" && size < r.n:
			return "at least " + n + unit
		case r.name == "max" && size > r.n:
			return "at most " + n + unit
		case r.name == "len" && size != r.n:
			return "exactly " + n + unit
		}
	case "oneof":
		var s string
		switch v.Kind() {
		case reflect.String:
			s = v.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64, reflect.Bool:
			s = fmt.Sprint(v.Interface())
		default:
			return ""
		}
		for _, x := range r.set {
			if s == x {
				return ""
			}
		}
		return "one of " + strings.Join(r.set, ", ")
	case "email":
		if v.Kind() != reflect.String || v.String() == "" {
			return ""
		}
		a, err := mail.ParseAddress(v.String())
		if err != nil || a.Address != v.String() || !strings.Contains(a.Address[strings.LastIndex(a.Address, "@")+1:], ".") {
			return "an email address"
		}
	}
	return ""
}

func measure(v reflect.Value) (float64, string, bool) {
	switch v.Kind() {
	case reflect.String:
		return float64(utf8.RuneCountInString(v.String())), "character", true
	case reflect.Slice, reflect.Array, reflect.Map:
		return float64(v.Len()), "item", true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), "", true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), "", true
	case reflect.Float32, reflect.Float64:
		return v.Float(), "", true
	}
	return 0, "", false
}
