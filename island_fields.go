package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vuka-lang/vuka/diag"
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
	Defer    string        // defer: the group it loads in after the first render; "" when it doesn't
	rules    []rule
}

// DefaultDebounce is how long a bound field's set waits for more sets
// before it is sent; bind:"q,debounce=500" changes it.
const DefaultDebounce = 300 * time.Millisecond

// NoDebounce, as a Bind's Debounce, sends each set at once:
// ui.Bind{Debounce: ui.NoDebounce}. A zero Debounce is DefaultDebounce.
const NoDebounce time.Duration = -1

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
	if err := schemaErr(t); err != nil {
		return nil, err
	}
	schema := liveSchemaOf(t)
	var out []IslandField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		var so FieldOptions
		if schema != nil {
			so = schema.fields[i]
		}
		bind, hasBind := f.Tag.Lookup("bind")
		if hasBind && bind == "-" {
			hasBind = false
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		prop := f.IsExported() && !(f.Anonymous && f.Type == liveType) && name != "-" &&
			f.Type != nodeType && !(f.Type.Kind() == reflect.Interface && f.Type.Implements(nodeType)) &&
			f.Type.Kind() != reflect.Func && !f.Type.Implements(jsChainType) && !f.Type.Implements(anyStateType)
		if !prop {
			if hasBind || so.Bind {
				what := "ui.Bind"
				if hasBind {
					what = "bind:"
				}
				return nil, diag.New("E0322", "%s.%s: %s is for a prop the component sets (an exported field of a JSON value)", t.Name(), f.Name, what).
					WithHelp("drop %s, or make %s an exported field of a plain value", what, f.Name)
			}
			continue
		}
		if name == "" {
			name = f.Name
		}
		h := IslandField{Handle: strings.ToLower(name[:1]) + name[1:], Wire: name, Index: i, Type: f.Type}
		rules, err := parseRules(f.Tag.Get("validate"))
		if err != nil {
			return nil, within(t.Name()+"."+f.Name, err)
		}
		for _, spec := range so.Validate {
			r, err := parseRules(spec)
			if err != nil {
				return nil, within(t.Name()+"."+f.Name, err)
			}
			rules = append(rules, r...)
		}
		if err := CheckValidate(f.Type); err != nil {
			return nil, err
		}
		h.rules = rules
		d, hasDefer := f.Tag.Lookup("defer")
		if so.Defer != "" {
			d, hasDefer = so.Defer, true
		}
		if hasDefer && d != "-" {
			if d == "" {
				d = f.Name
			}
			if !isIdent(d) {
				what := fmt.Sprintf("ui.Defer(%q)", d)
				if so.Defer == "" {
					what = fmt.Sprintf("defer:%q", d)
				}
				return nil, diag.New("E0323", "%s.%s: %s: a group is a name", t.Name(), f.Name, what).
					WithHelp("name the group: ui.Defer(\"stats\"), loaded by l.Loads(\"stats\", …)")
			}
			url, _, _ := strings.Cut(f.Tag.Get("url"), ",")
			switch {
			case hasBind || so.Bind:
				return nil, diag.New("E0323", "%s.%s: a deferred field is the server's to load", t.Name(), f.Name).WithHelp("drop Bind or Defer")
			case so.URL != "" || url != "" && url != "-":
				return nil, diag.New("E0323", "%s.%s: a deferred field is the server's to load", t.Name(), f.Name).WithHelp("drop URL or Defer")
			}
			h.Defer = groupName(d)
		}
		if (hasBind || so.Bind) && reservedHandles[h.Handle] {
			return nil, diag.New("E0322", "%s.%s: live.%s is live's own", t.Name(), f.Name, h.Handle).
				WithHelp("give the field another json name to bind it")
		}
		if so.Bind {
			h.Bind, h.Debounce = true, so.Debounce
		} else if hasBind {
			if err := bindTag(t, f, bind, &h); err != nil {
				return nil, err
			}
		}
		out = append(out, h)
	}
	return out, nil
}

// bindTag reads a bind:"name,debounce=500" tag, the form of earlier
// releases.
func bindTag(t reflect.Type, f reflect.StructField, bind string, h *IslandField) error {
	n, opts, _ := strings.Cut(bind, ",")
	if n != "" && n != h.Handle && n != h.Wire {
		return diag.New("E0322", "%s.%s: bind:%q names the field live.%s", t.Name(), f.Name, n, h.Handle).
			WithSuggest(strconv.Quote(h.Handle)).WithHelp("write bind:%q, or leave the name out: bind:\"\"", h.Handle)
	}
	h.Bind, h.Debounce = true, DefaultDebounce
	for _, o := range strings.Split(opts, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(o), "=")
		switch k {
		case "":
		case "debounce":
			d, err := parseDebounce(v)
			if err != nil {
				return diag.New("E0322", "%s.%s: bind:%q: bad debounce %q", t.Name(), f.Name, bind, v).
					WithHelp("debounce is milliseconds (debounce=500) or a duration (debounce=1s)")
			}
			h.Debounce = d
		default:
			return diag.New("E0322", "%s.%s: bind:%q: unknown option %q", t.Name(), f.Name, bind, k).WithSuggest("debounce=500")
		}
	}
	return nil
}

func isIdent(s string) bool {
	for i, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return s != ""
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
// struct) can't be: a mistake in its LiveSchema, a Bind that doesn't fit, a
// method and a field (or two fields) of one wire name, a method named as
// earlier releases found a hook by that isn't declared one.
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
			return diag.New("E0326", "%s: fields %s and %s are both live.%s", t.Elem().Name(), other, t.Elem().Field(f.Index).Name, f.Handle).
				WithHelp("give one of them another json name")
		}
		seen[f.Handle] = t.Elem().Field(f.Index).Name
	}
	for _, m := range IslandMethods(t) {
		for _, a := range m.Args {
			if err := CheckValidate(a); err != nil {
				return within(t.Elem().Name()+"."+m.Method.Name, err)
			}
		}
		if f, ok := seen[m.Name]; ok {
			return diag.New("E0326", "%s: the method %s and the field %s are both live.%s", t.Elem().Name(), m.Method.Name, f, m.Name).
				WithHelp("rename one, or give the field another json name")
		}
	}
	if err := namedHookErr(t); err != nil {
		return err
	}
	if _, err := deferGroups(t); err != nil {
		return err
	}
	return nil
}

// Changed is what a set of the component changed, as the page's OnChange
// hook gets it: the bound fields the set holds.
type Changed struct{ names, also []string }

// Has reports whether the set holds the field name: its wire name (its json
// name, else its Go name, as Names has it), its handle (live.q: q) or its Go
// name (Q).
func (c Changed) Has(name string) bool {
	return slices.Contains(c.names, name) || slices.Contains(c.also, name)
}

// Names are the wire names of the fields the set holds, sorted.
func (c Changed) Names() []string { return slices.Clone(c.names) }

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
// runs its OnChange hook.
// A field that isn't bound is refused, a value breaking its field's rules
// (or the validate: tags of a struct it holds) is Invalid under its wire
// name (q, address.city), and either way nothing is set.
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
	var changed Changed
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
		changed.names = append(changed.names, f.Wire)
		changed.also = append(changed.also, f.Handle, x.Type().Elem().Field(f.Index).Name)
	}
	slices.Sort(changed.names)
	if len(bad) > 0 {
		return bad
	}
	for i, v := range vals {
		x.Elem().Field(i).Set(v)
	}
	return runChanged(ctx, x, changed)
}

func runChanged(ctx context.Context, x reflect.Value, changed Changed) error {
	if !hasOnChange(x.Type()) {
		return nil
	}
	return liveHooks(x).onChange(ctx, changed)
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
		if f.Defer != "" {
			e["p"] = 1
		}
		m[f.Handle] = e
	}
	j, _ := json.Marshal(m)
	return string(j)
}

// hasOnChange reports whether t's LiveSchema sets an OnChange hook.
func hasOnChange(t reflect.Type) bool {
	s := liveSchemaOf(t)
	return s != nil && s.onChange != nil
}
