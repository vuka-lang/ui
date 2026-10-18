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
				return nil, diag.New("E0322", "%s.%s: bind: is for a prop the component sets (an exported field of a JSON value)", t.Name(), f.Name).
					WithHelp("drop bind:, or make %s an exported field of a plain value", f.Name)
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
		if err := CheckValidate(f.Type); err != nil {
			return nil, err
		}
		h.rules = rules
		if d, ok := f.Tag.Lookup("defer"); ok && d != "-" {
			if d == "" {
				d = f.Name
			}
			if !isIdent(d) {
				return nil, diag.New("E0323", "%s.%s: defer:%q: a group is a name", t.Name(), f.Name, d).
					WithHelp("name the group: defer:\"stats\", loaded by LoadStats")
			}
			if hasBind {
				return nil, diag.New("E0323", "%s.%s: a deferred field is the server's to load", t.Name(), f.Name).WithHelp("drop bind: or defer:")
			}
			if q, _, _ := strings.Cut(f.Tag.Get("url"), ","); q != "" && q != "-" {
				return nil, diag.New("E0323", "%s.%s: a deferred field is the server's to load", t.Name(), f.Name).WithHelp("drop url: or defer:")
			}
			h.Defer = strings.ToUpper(d[:1]) + d[1:]
		}
		if hasBind {
			n, opts, _ := strings.Cut(bind, ",")
			if n != "" && n != h.Handle && n != h.Wire {
				return nil, diag.New("E0322", "%s.%s: bind:%q names the field live.%s", t.Name(), f.Name, n, h.Handle).
					WithSuggest(strconv.Quote(h.Handle)).WithHelp("write bind:%q, or leave the name out: bind:\"\"", h.Handle)
			}
			if reservedHandles[h.Handle] {
				return nil, diag.New("E0322", "%s.%s: live.%s is live's own", t.Name(), f.Name, h.Handle).
					WithHelp("give the field another json name to bind it")
			}
			h.Bind, h.Debounce = true, DefaultDebounce
			for _, o := range strings.Split(opts, ",") {
				k, v, _ := strings.Cut(strings.TrimSpace(o), "=")
				switch k {
				case "":
				case "debounce":
					d, err := parseDebounce(v)
					if err != nil {
						return nil, diag.New("E0322", "%s.%s: bind:%q: bad debounce %q", t.Name(), f.Name, bind, v).
							WithHelp("debounce is milliseconds (debounce=500) or a duration (debounce=1s)")
					}
					h.Debounce = d
				default:
					return nil, diag.New("E0322", "%s.%s: bind:%q: unknown option %q", t.Name(), f.Name, bind, k).WithSuggest("debounce=500")
				}
			}
		}
		out = append(out, h)
	}
	return out, nil
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
	if m, ok := t.MethodByName("Changed"); ok {
		if _, ok := changedArgs(m.Type); !ok {
			return diag.New("E0327", "%s.Changed is %s", t.Elem().Name(), m.Type).
				WithHelp("declare Changed(), Changed(ctx context.Context) or Changed(ctx context.Context, changed ui.Changed), returning nothing or an error")
		}
	}
	if _, err := deferGroups(t); err != nil {
		return err
	}
	return nil
}

// Changed is what a set of the component changed, as Changed(ctx, changed
// ui.Changed) gets it: the bound fields the set holds.
type Changed struct{ names, also []string }

// Has reports whether the set holds the field name: its wire name (its json
// name, else its Go name, as Names has it), its handle (live.q: q) or its Go
// name (Q).
func (c Changed) Has(name string) bool {
	return slices.Contains(c.names, name) || slices.Contains(c.also, name)
}

// Names are the wire names of the fields the set holds, sorted.
func (c Changed) Names() []string { return slices.Clone(c.names) }

var changedType = reflect.TypeFor[Changed]()

// changedArgs are the arguments Changed takes after its receiver — a
// context.Context, then a Changed, each optional — as "c" and "n".
func changedArgs(mt reflect.Type) (string, bool) {
	var args string
	for i := 1; i < mt.NumIn(); i++ {
		switch {
		case mt.In(i) == ctxType && args == "":
			args += "c"
		case mt.In(i) == changedType && !strings.Contains(args, "n"):
			args += "n"
		default:
			return "", false
		}
	}
	return args, mt.NumOut() == 0 || mt.NumOut() == 1 && mt.Out(0) == errType
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
	m := x.MethodByName("Changed")
	if !m.IsValid() {
		return nil
	}
	mm, _ := x.Type().MethodByName("Changed")
	args, _ := changedArgs(mm.Type)
	var in []reflect.Value
	for _, a := range args {
		if a == 'c' {
			in = append(in, reflect.ValueOf(ctx))
		} else {
			in = append(in, reflect.ValueOf(changed))
		}
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
		if f.Defer != "" {
			e["p"] = 1
		}
		m[f.Handle] = e
	}
	j, _ := json.Marshal(m)
	return string(j)
}
