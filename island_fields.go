package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IslandField is a prop of a page-backed island as its component reads it
// through useLive(): live.<Handle>.get(), .value, and .set(v) when Bind.
type IslandField struct {
	Handle   string // the wire name, first letter lower-cased
	Wire     string // the prop's key: its json name, else the Go name
	Index    int
	Type     reflect.Type
	Bind     bool          // bind: the component may set it
	Debounce time.Duration // how long a set waits for more before it is sent
	rules    []rule
}

// DefaultDebounce is how long a bound field's set waits for more sets
// before it is sent; bind:"q,debounce=500" changes it.
const DefaultDebounce = 300 * time.Millisecond

// reservedHandles are live's own members: a field of one of these names
// is a prop, but has no handle.
var reservedHandles = map[string]bool{"pending": true, "error": true, "subscribe": true}

type islandFieldsResult struct {
	fields []IslandField
	err    error
}

var islandFieldsOf sync.Map // reflect.Type (struct) → islandFieldsResult

// IslandFields are the props of t (an island's struct, or a pointer to one)
// that get a handle: the exported fields that are plain values — not the
// slot, a function, a JS chain or a state. A field tagged bind:"name" is
// bound; name is its handle (or its wire name), debounce=500 (milliseconds,
// or a duration: 1s) sets its wait.
func IslandFields(t reflect.Type) ([]IslandField, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v, ok := islandFieldsOf.Load(t); ok {
		r := v.(islandFieldsResult)
		return r.fields, r.err
	}
	fields, err := islandFieldsFor(t)
	islandFieldsOf.Store(t, islandFieldsResult{fields, err})
	return fields, err
}

func islandFieldsFor(t reflect.Type) ([]IslandField, error) {
	if t.Kind() != reflect.Struct {
		return nil, nil
	}
	var out []IslandField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		bind, hasBind := f.Tag.Lookup("bind")
		if hasBind && bind == "-" {
			hasBind = false
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		prop := f.IsExported() && !(f.Anonymous && f.Type == liveType) && name != "-" &&
			f.Type != nodeType && !(f.Type.Kind() == reflect.Interface && f.Type.Implements(nodeType)) &&
			f.Type.Kind() != reflect.Func && !f.Type.Implements(jsChainType) && !f.Type.Implements(anyStateType)
		if !prop {
			if hasBind {
				return nil, fmt.Errorf("%s.%s: bind: is for a prop the component sets (an exported field of a JSON value)", t.Name(), f.Name)
			}
			continue
		}
		if name == "" {
			name = f.Name
		}
		h := IslandField{Handle: strings.ToLower(name[:1]) + name[1:], Wire: name, Index: i, Type: f.Type}
		rules, err := parseRules(f.Tag.Get("validate"))
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", t.Name(), f.Name, err)
		}
		if err := CheckValidate(f.Type); err != nil {
			return nil, err
		}
		h.rules = rules
		if hasBind {
			n, opts, _ := strings.Cut(bind, ",")
			if n != "" && n != h.Handle && n != h.Wire {
				return nil, fmt.Errorf("%s.%s: bind:%q names the field live.%s: bind:%q", t.Name(), f.Name, n, h.Handle, h.Handle)
			}
			if reservedHandles[h.Handle] {
				return nil, fmt.Errorf("%s.%s: live.%s is live's own: give the field another json name to bind it", t.Name(), f.Name, h.Handle)
			}
			h.Bind, h.Debounce = true, DefaultDebounce
			for _, o := range strings.Split(opts, ",") {
				k, v, _ := strings.Cut(strings.TrimSpace(o), "=")
				switch k {
				case "":
				case "debounce":
					d, err := parseDebounce(v)
					if err != nil {
						return nil, fmt.Errorf("%s.%s: bind:%q: debounce is milliseconds (500) or a duration (1s)", t.Name(), f.Name, bind)
					}
					h.Debounce = d
				default:
					return nil, fmt.Errorf("%s.%s: bind:%q: unknown option %q (debounce=500)", t.Name(), f.Name, bind, k)
				}
			}
		}
		out = append(out, h)
	}
	return out, nil
}

func parseDebounce(v string) (time.Duration, error) {
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Millisecond, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("bad debounce %q", v)
	}
	return d, nil
}

// CheckIsland reports what a page backed by t (a pointer to its island's
// struct) can't be: a bind: tag that doesn't fit, a method and a field (or
// two fields) of one wire name, a Changed that isn't Changed([ctx]) [error].
func CheckIsland(t reflect.Type) error {
	if t.Kind() != reflect.Pointer {
		t = reflect.PointerTo(t)
	}
	if info := lookupIsland(t.Elem()); info != nil && info.err != nil {
		return info.err
	}
	fields, err := IslandFields(t)
	if err != nil {
		return err
	}
	seen := map[string]string{}
	for _, f := range fields {
		if other, ok := seen[f.Handle]; ok {
			return fmt.Errorf("%s: fields %s and %s are both live.%s", t.Elem().Name(), other, t.Elem().Field(f.Index).Name, f.Handle)
		}
		seen[f.Handle] = t.Elem().Field(f.Index).Name
	}
	for _, m := range IslandMethods(t) {
		for _, a := range m.Args {
			if err := CheckValidate(a); err != nil {
				return fmt.Errorf("%s.%s: %w", t.Elem().Name(), m.Method.Name, err)
			}
		}
		if f, ok := seen[m.Name]; ok {
			return fmt.Errorf("%s: the method %s and the field %s are both live.%s: rename one (or give the field another json name)", t.Elem().Name(), m.Method.Name, f, m.Name)
		}
	}
	if m, ok := t.MethodByName("Changed"); ok {
		mt := m.Type
		in := mt.NumIn() == 1 || mt.NumIn() == 2 && mt.In(1) == ctxType
		out := mt.NumOut() == 0 || mt.NumOut() == 1 && mt.Out(0) == errType
		if !in || !out {
			return fmt.Errorf("%s.Changed is Changed() or Changed(ctx context.Context), returning nothing or an error", t.Elem().Name())
		}
	}
	return nil
}

// setError is a refused or invalid set, as the component's live.error
// reads it.
type setError struct {
	status       int
	field, cause string
}

func (e setError) Error() string         { return e.field + ": " + e.cause }
func (e setError) PublicMessage() string { return e.Error() }
func (e setError) StatusCode() int       { return e.status }
func (e setError) FieldErrors() map[string]string {
	if e.field == "" {
		return nil
	}
	return map[string]string{e.field: e.cause}
}

// SetBound sets x's (a pointer to the island's struct) bound fields from
// data, the JSON object of wire names and values the component set, then
// runs its Changed. A field that isn't bound is refused, a value breaking
// its field's validate: tag (or the tags of a struct it holds) is Invalid
// under its wire name (q, address.city), and either way nothing is set.
func SetBound(ctx context.Context, x reflect.Value, data json.RawMessage) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return setError{status: http.StatusBadRequest, field: "set", cause: "an object of fields"}
	}
	fields, err := IslandFields(x.Type())
	if err != nil {
		return err
	}
	byWire := make(map[string]IslandField, len(fields))
	for _, f := range fields {
		byWire[f.Wire] = f
	}
	vals := make(map[int]reflect.Value, len(m))
	bad := Invalid{}
	for k, raw := range m {
		f, ok := byWire[k]
		if !ok || !f.Bind {
			return setError{status: http.StatusForbidden, field: k, cause: "can't be set"}
		}
		v := reflect.New(f.Type)
		if err := json.Unmarshal(raw, v.Interface()); err != nil {
			return setError{status: http.StatusUnprocessableEntity, field: k, cause: "not a " + f.Type.String()}
		}
		if err := walkValidate(k, v.Elem(), f.rules, bad); err != nil {
			return err
		}
		vals[f.Index] = v.Elem()
	}
	if len(bad) > 0 {
		return bad
	}
	for i, v := range vals {
		x.Elem().Field(i).Set(v)
	}
	return runChanged(ctx, x)
}

func runChanged(ctx context.Context, x reflect.Value) error {
	m := x.MethodByName("Changed")
	if !m.IsValid() {
		return nil
	}
	var in []reflect.Value
	if m.Type().NumIn() == 1 {
		in = append(in, reflect.ValueOf(ctx))
	}
	out := m.Call(in)
	if len(out) == 1 && !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}

// fieldsJSON is a page's data-vk-fields: each handle → its wire name and,
// for a bound field, its debounce in milliseconds.
func fieldsJSON(fields []IslandField) string {
	m := make(map[string]map[string]any, len(fields))
	for _, f := range fields {
		e := map[string]any{"w": f.Wire}
		if f.Bind {
			e["d"] = f.Debounce.Milliseconds()
		}
		m[f.Handle] = e
	}
	j, _ := json.Marshal(m)
	return string(j)
}
