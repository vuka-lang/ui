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
	"sync"
	"unsafe"
)

// URL state: a component's field marked @ui.URL("tab") is kept in the
// page's address. It is filled from the URL's query parameter when the
// component renders — a prop the parent passes (a value that isn't the
// zero value) winning, then the parameter, then the attribute's default
// (@ui.URL("page").Default(1)), else the zero value — and again whenever
// the URL changes (a patch, back and forward). An event assigning it
// writes the URL: a live patch that keeps the other parameters, in place
// of the current history entry. A bare @ui.URL binds the route's path
// parameter of the field's name instead, read only.
//
// An instance with a urlKey (<Table urlKey="requests" />) keeps its keys
// under that name: "requests.page".

type pageURLKey struct{}

type pageURL struct {
	u     *url.URL
	param func(string) string
}

// WithPageURL is ctx carrying the URL of the page being rendered and its
// route's path parameters (param, nil when it has none): what the @ui.URL
// fields of the components it renders read. Package web sets it for every
// request it serves.
func WithPageURL(ctx context.Context, u *url.URL, param func(name string) string) context.Context {
	return context.WithValue(ctx, pageURLKey{}, pageURL{u, param})
}

// PageURL is the URL of the page ctx renders (WithPageURL); nil when it
// carries none.
func PageURL(ctx context.Context) *url.URL {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(pageURLKey{}).(pageURL)
	return p.u
}

// PageParam is the path parameter name of the route of the page ctx
// renders, "" when it has none of that name.
func PageParam(ctx context.Context, name string) string {
	if ctx == nil {
		return ""
	}
	if p, _ := ctx.Value(pageURLKey{}).(pageURL); p.param != nil {
		return p.param(name)
	}
	return ""
}

// URLEditor changes the URL of a live page from one of its events: its
// session, which a handler's context carries (WithURLEditor). Package live
// implements it.
type URLEditor interface {
	// EditURL applies edit to the page's query; push adds a history entry
	// instead of taking the current one's place.
	EditURL(edit func(url.Values), push bool)
	// Query is the page's query as its event sees it: with the edits made
	// so far.
	Query() url.Values
}

type urlEditorKey struct{}

// WithURLEditor is ctx carrying e, the session whose event ctx's code runs
// for.
func WithURLEditor(ctx context.Context, e URLEditor) context.Context {
	return context.WithValue(ctx, urlEditorKey{}, e)
}

// EditURL applies edit to the query of the live page whose event ctx
// belongs to, as a patch of the page once the event's reply is applied,
// the other parameters kept; it reports false, doing nothing, outside a
// live page's event. Package web's Query is the API.
func EditURL(ctx context.Context, edit func(url.Values), push bool) bool {
	if ctx == nil {
		return false
	}
	e, _ := ctx.Value(urlEditorKey{}).(URLEditor)
	if e == nil {
		return false
	}
	e.EditURL(edit, push)
	return true
}

// EventQuery is the query of the live page whose event ctx belongs to, with
// the edits its code made so far; else the query of the page ctx renders,
// nil when there is none.
func EventQuery(ctx context.Context) url.Values {
	if ctx == nil {
		return nil
	}
	if e, _ := ctx.Value(urlEditorKey{}).(URLEditor); e != nil {
		return e.Query()
	}
	if u := PageURL(ctx); u != nil {
		return u.Query()
	}
	return nil
}

// URLDefault is @ui.URL("page").Default(1): the URL attribute with the
// value the field takes when the URL has no parameter for it. A field
// holding its default leaves the parameter out of the URL.
type URLDefault struct {
	Name  string
	Value any
}

func (d URLDefault) Apply(o *FieldOptions) {
	o.URL, o.URLBound, o.URLDefault, o.HasURLDefault = d.Name, true, d.Value, true
}

// Default is the URL attribute with the value the field takes when the
// URL has no parameter for it: @ui.URL("page").Default(1).
func (u URL) Default(v any) URLDefault { return URLDefault{Name: string(u), Value: v} }

// urlField is a field a component's LiveSchema keeps in the URL.
type urlField struct {
	index    int
	name     string // the query parameter, or the path parameter (path)
	path     bool   // a bare @ui.URL: the route's path parameter of the field's name
	exported bool   // a prop: the parent's when it passes it
	def      string // the default's text; "" with no default
	defVals  []string
	typ      reflect.Type // the value's type: an Assign's element
	assign   bool         // an Assign: read with Get, written with Set
}

var urlFieldCache sync.Map // reflect.Type (struct) → []urlField

// urlFieldsOf is the URL-bound fields of the struct t, or of the struct
// a pointer type t points to.
func urlFieldsOf(t reflect.Type) []urlField {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	if v, ok := urlFieldCache.Load(t); ok {
		return v.([]urlField)
	}
	var out []urlField
	if s := liveSchemaOf(t); s != nil {
		for i, o := range s.fields {
			if !o.URLBound {
				continue
			}
			f := t.Field(i)
			uf := urlField{index: i, name: o.URL, exported: f.IsExported(), typ: f.Type}
			if et, ok := assignElem(f.Type); ok {
				uf.typ, uf.assign = et, true
			}
			if uf.name == "" {
				uf.name, uf.path = f.Name, true
			}
			if o.HasURLDefault {
				v, _ := defaultValue(o.URLDefault, uf.typ)
				uf.defVals = urlText(v)
				uf.def = strings.Join(uf.defVals, "\x00")
			}
			out = append(out, uf)
		}
	}
	slices.SortFunc(out, func(a, b urlField) int { return a.index - b.index })
	v, _ := urlFieldCache.LoadOrStore(t, out)
	return v.([]urlField)
}

// checkURLField reports a field @ui.URL can't keep: one whose type has no
// text form, or whose default isn't of its type.
func checkURLField(f reflect.StructField, o FieldOptions) error {
	t := f.Type
	if et, ok := assignElem(t); ok {
		t = et
	}
	if !textual(t) {
		return fmt.Errorf("%s is a %s, which a URL can't hold: give it a string, a number, a bool, a type with UnmarshalText, or a slice or pointer of one", f.Name, t)
	}
	if o.HasURLDefault {
		if _, ok := defaultValue(o.URLDefault, t); !ok {
			return fmt.Errorf("%s's default %#v isn't a %s", f.Name, o.URLDefault, t)
		}
	}
	return nil
}

// defaultValue is d as a t: d itself when it is one, a number converted to
// another number type, a string or bool to a type of its kind; false (the
// zero t) for anything else.
func defaultValue(d any, t reflect.Type) (reflect.Value, bool) {
	v := reflect.New(t).Elem()
	dv := reflect.ValueOf(d)
	switch {
	case !dv.IsValid():
		return v, false
	case dv.Type().AssignableTo(t):
		v.Set(dv)
	case numberKind(dv.Kind()) && numberKind(t.Kind()) || dv.Kind() == t.Kind() && (t.Kind() == reflect.String || t.Kind() == reflect.Bool):
		v.Set(dv.Convert(t))
	default:
		return v, false
	}
	return v, true
}

func numberKind(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Float64 && k != reflect.Uintptr
}

// textual reports whether a URL parameter's text can set a t.
func textual(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return true
	}
	switch t.Kind() {
	case reflect.Pointer:
		return t.Elem().Kind() != reflect.Pointer && t.Elem().Kind() != reflect.Slice && textual(t.Elem())
	case reflect.Slice:
		return t.Elem().Kind() != reflect.Slice && t.Elem().Kind() != reflect.Pointer && textual(t.Elem())
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// assignElem is T of a uigen.Assign[T] type.
func assignElem(t reflect.Type) (reflect.Type, bool) {
	if t.Kind() != reflect.Struct || !reflect.PointerTo(t).Implements(assignerType) {
		return nil, false
	}
	m, ok := reflect.PointerTo(t).MethodByName("Get")
	if !ok || m.Type.NumOut() != 1 {
		return nil, false
	}
	return m.Type.Out(0), true
}

// get is the field's value (an Assign's, its Get).
func (uf urlField) get(c reflect.Value) reflect.Value {
	f := field(c, uf.index)
	if uf.assign {
		return f.Addr().MethodByName("Get").Call(nil)[0]
	}
	return f
}

// set sets the field to v (an Assign through Set, marking it changed).
func (uf urlField) set(c reflect.Value, v reflect.Value) {
	f := field(c, uf.index)
	if uf.assign {
		f.Addr().MethodByName("Set").Call([]reflect.Value{v})
		return
	}
	f.Set(v)
}

// fromText is the field's value for the URL's text: its default when
// there is none, or when it doesn't parse.
func (uf urlField) fromText(vals []string) reflect.Value {
	v := reflect.New(uf.typ).Elem()
	if len(vals) == 0 || parseText(v, vals) != nil {
		v = reflect.New(uf.typ).Elem()
		if uf.defVals != nil {
			parseText(v, uf.defVals)
		}
	}
	return v
}

// key is the parameter the field is kept in, under the instance's urlKey.
func (uf urlField) key(prefix string) string {
	if prefix == "" {
		return uf.name
	}
	return prefix + "." + uf.name
}

// explicit reports whether the parent passes the prop: tag's value of the
// field isn't the zero value.
func (uf urlField) explicit(tag reflect.Value) bool {
	return uf.exported && tag.IsValid() && !field(tag, uf.index).IsZero()
}

// field is the struct c points to's field i, settable even when
// unexported.
func field(c reflect.Value, i int) reflect.Value {
	f := c.Elem().Field(i)
	if !f.CanSet() {
		f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	}
	return f
}

// parseText sets v from a URL parameter's values, as package web binds a
// page's: UnmarshalText, else by kind; a slice takes each value, anything
// else the first.
func parseText(v reflect.Value, vals []string) error {
	t := v.Type()
	if u, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
		return u.UnmarshalText([]byte(vals[0]))
	}
	switch t.Kind() {
	case reflect.Pointer:
		p := reflect.New(t.Elem())
		if err := parseText(p.Elem(), vals); err != nil {
			return err
		}
		v.Set(p)
		return nil
	case reflect.Slice:
		s := reflect.MakeSlice(t, len(vals), len(vals))
		for i, x := range vals {
			if err := parseText(s.Index(i), []string{x}); err != nil {
				return err
			}
		}
		v.Set(s)
		return nil
	}
	s := vals[0]
	switch t.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, t.Bits())
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, t.Bits())
		if err != nil {
			return err
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, t.Bits())
		if err != nil {
			return err
		}
		v.SetFloat(f)
	default:
		return fmt.Errorf("can't take a %s", t)
	}
	return nil
}

// urlText is v as URL parameter values: nil for a nil pointer, each element
// of a slice, MarshalText's, else the value formatted as parseText reads
// it.
func urlText(v reflect.Value) []string {
	if m, ok := v.Interface().(encoding.TextMarshaler); ok && (v.Kind() != reflect.Pointer || !v.IsNil()) {
		b, err := m.MarshalText()
		if err != nil {
			return nil
		}
		return []string{string(b)}
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return urlText(v.Elem())
	case reflect.Slice:
		var out []string
		for i := range v.Len() {
			out = append(out, urlText(v.Index(i))...)
		}
		return out
	case reflect.String:
		return []string{v.String()}
	case reflect.Bool:
		return []string{strconv.FormatBool(v.Bool())}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return []string{strconv.FormatInt(v.Int(), 10)}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []string{strconv.FormatUint(v.Uint(), 10)}
	case reflect.Float32, reflect.Float64:
		return []string{strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits())}
	}
	return []string{fmt.Sprint(v.Interface())}
}

// QueryText is v as URL parameter values, as a field @ui.URL keeps
// formats it: a string as it is, a number or bool as strconv writes it,
// MarshalText's, each element of a slice; none for a nil pointer.
func QueryText(v any) []string {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil
	}
	return urlText(rv)
}

// stored is the field's text as the URL keeps it: none when it holds its
// default, or the zero value without one.
func (uf urlField) stored(v reflect.Value) []string {
	vals := urlText(v)
	if uf.defVals != nil {
		if slices.Equal(vals, uf.defVals) {
			return nil
		}
		return vals
	}
	if v.IsZero() {
		return nil
	}
	return vals
}

// fillStatic sets c's URL-bound fields from the page ctx renders, for a
// render without a session: tag is the props its tag passed.
func fillStatic(ctx context.Context, c, tag Stateful, prefix string) {
	fs := urlFieldsOf(reflect.TypeOf(c))
	if len(fs) == 0 {
		return
	}
	var q url.Values
	if u := PageURL(ctx); u != nil {
		q = u.Query()
	}
	cv, tv := reflect.ValueOf(c), reflect.ValueOf(tag)
	for _, uf := range fs {
		if uf.explicit(tv) {
			continue
		}
		var vals []string
		if uf.path {
			if p := PageParam(ctx, uf.name); p != "" {
				vals = []string{p}
			}
		} else {
			vals = q[uf.key(prefix)]
		}
		uf.set(cv, uf.fromText(vals))
	}
}

// URLFields keeps a live component instance's @ui.URL fields in step with
// its page's URL: package live holds one per instance and calls Fill as
// its tag renders, Snapshot before an event and Changes after it.
type URLFields struct {
	seen map[int]string // per field: the URL's text it was last filled from
	snap map[int]string // per field: its text before the event
}

// HasURLFields reports whether c's type keeps fields in the URL.
func HasURLFields(c Stateful) bool { return len(urlFieldsOf(reflect.TypeOf(c))) > 0 }

// Fill sets c's URL-bound fields from the URL (its query q, and the path
// parameters of ctx's page) where the URL changed since the last Fill, or
// every one the first time; a prop the parent passes in tag (not its zero
// value) is the parent's, and on a routed page (routed) the page's exported
// fields are the route's to bind. A prop the parent leaves out is set
// every time: the tag's render reset it. It reports whether it changed a
// field.
func (u *URLFields) Fill(ctx context.Context, c, tag Stateful, prefix string, routed bool, q url.Values) bool {
	fs := urlFieldsOf(reflect.TypeOf(c))
	if len(fs) == 0 {
		return false
	}
	changed := false
	if u.seen == nil {
		u.seen = map[int]string{}
	}
	cv, tv := reflect.ValueOf(c), reflect.ValueOf(tag)
	for _, uf := range fs {
		if routed && uf.exported || uf.explicit(tv) {
			continue
		}
		var vals []string
		if uf.path {
			if p := PageParam(ctx, uf.name); p != "" {
				vals = []string{p}
			}
		} else {
			vals = q[uf.key(prefix)]
		}
		text := strings.Join(vals, "\x00")
		if len(vals) == 0 {
			text = "\x01" + uf.def // no parameter: the default, told apart from a parameter set to it
		}
		last, ok := u.seen[uf.index]
		u.seen[uf.index] = text
		if ok && last == text && !uf.exported {
			continue // the URL didn't change: the field is the component's
		}
		v := uf.fromText(vals)
		if cur := uf.get(cv); !reflect.DeepEqual(cur.Interface(), v.Interface()) {
			uf.set(cv, v)
			changed = true
		}
	}
	return changed
}

// Snapshot records the texts of c's URL-bound fields, before an event.
func (u *URLFields) Snapshot(c Stateful) {
	fs := urlFieldsOf(reflect.TypeOf(c))
	if len(fs) == 0 {
		return
	}
	u.snap = map[int]string{}
	cv := reflect.ValueOf(c)
	for _, uf := range fs {
		if !uf.path {
			u.snap[uf.index] = strings.Join(urlText(uf.get(cv)), "\x00")
		}
	}
}

// Changes writes into q the URL-bound fields of c the event changed since
// Snapshot, under prefix, and reports whether there was one: a field
// holding its default (or the zero value, without one) leaves its
// parameter out. A prop the parent passes (tag) and a path parameter are
// never written.
func (u *URLFields) Changes(c, tag Stateful, prefix string, q url.Values) bool {
	if u.snap == nil {
		return false
	}
	defer func() { u.snap = nil }()
	changed := false
	cv, tv := reflect.ValueOf(c), reflect.ValueOf(tag)
	for _, uf := range urlFieldsOf(reflect.TypeOf(c)) {
		before, ok := u.snap[uf.index]
		if !ok || uf.explicit(tv) {
			continue
		}
		v := uf.get(cv)
		if strings.Join(urlText(v), "\x00") == before {
			continue
		}
		key := uf.key(prefix)
		if vals := uf.stored(v); vals == nil {
			q.Del(key)
		} else {
			q[key] = vals
		}
		changed = true
	}
	return changed
}

// URLKey is the stateful component c (a component, or a ui.Key of one)
// keeping its @ui.URL fields under key: a tag's urlKey attribute,
// <Table urlKey="requests" />, whose page parameter becomes
// "requests.page". Two instances on one page binding the same parameter
// need keys of their own.
func URLKey(key string, c any) Node {
	switch x := c.(type) {
	case *ComponentNode:
		cp := *x
		cp.URLKey = key
		return &cp
	case Stateful:
		return &ComponentNode{Props: x, URLKey: key}
	}
	panic(fmt.Sprintf("ui: URLKey is given %T, not a stateful component", c))
}
