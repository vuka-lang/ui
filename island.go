package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vuka-lang/vuka"
)

// Islands are client components hydrated in the browser — a framework
// component (Vue/React/Svelte) or a vanilla module — placed in a server-
// rendered page and brought to life on a client:* strategy. A struct type
// annotated with @ui.Island names the component file and declares the props;
// the transpiler lowers a tag of that type to MountIsland. See
// docs/design/islands.md.

// islandInfo is a registered island: the component file, the framework
// inferred from its extension, and the default hydration strategy (empty when
// the decorator set none).
type islandInfo struct {
	Name      string
	Path      string
	Framework string
	Strategy  string
}

var islandReg sync.Map // reflect.Type (the props struct) -> *islandInfo

// Island is the @ui.Island type decorator. @ui.Island("islands/Chart.vue")
// above a struct type registers that type as an island whose props are its
// exported fields; an optional strategy sets the default used when a tag gives
// no client:* directive. The framework is inferred from the file extension
// (.vue, .tsx/.jsx, .svelte, else vanilla .ts/.js).
func Island(componentPath string, strategy ...string) func(*vuka.Type) {
	return func(t *vuka.Type) {
		info := &islandInfo{
			Name:      shortName(t.Name),
			Path:      componentPath,
			Framework: frameworkOf(componentPath),
		}
		if len(strategy) > 0 {
			info.Strategy = strategy[0]
		}
		islandReg.Store(t.Reflect, info)
	}
}

// RegisterIsland registers T as the island whose component is
// componentPath: what the compiler writes for a .vuka file's import of a
// component file (import Counter "./islands/Counter.vue"), T being the
// props type it generates from the component.
func RegisterIsland[T any](componentPath string, strategy ...string) {
	t := reflect.TypeFor[T]()
	info := &islandInfo{Name: t.Name(), Path: componentPath, Framework: frameworkOf(componentPath)}
	if len(strategy) > 0 {
		info.Strategy = strategy[0]
	}
	islandReg.Store(t, info)
}

func shortName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

func frameworkOf(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".vue":
		return "vue"
	case ".tsx", ".jsx":
		return "react"
	case ".svelte":
		return "svelte"
	default:
		return "vanilla"
	}
}

func lookupIsland(t reflect.Type) *islandInfo {
	if v, ok := islandReg.Load(t); ok {
		return v.(*islandInfo)
	}
	return nil
}

// IslandOpt configures a MountIsland call: a hydration strategy, a slot, or a
// fallback. The transpiler emits these from a tag's client:* directive, its
// children, and a <ui.Template fallback>.
type IslandOpt func(*islandConfig)

type islandConfig struct {
	strategy string
	arg      string
	slot     Node
	fallback Node
	ssr      bool
	once     bool
	emits    []islandEmit
	funcs    []islandEmit // function props: a handler or a JS chain per prop name
	states   []islandState

	// A page backed by its island (island_page.go).
	methods          []islandEmit // live: each method's handler
	methodNames      []string
	callURL, callSig string // stateless: where its methods are called
	err              *IslandError
}

type islandState struct {
	prop  string
	state AnyState
}

type islandEmit struct {
	name    string
	handler any
}

// Load hydrates the island immediately on page load (client:load).
func Load() IslandOpt { return func(c *islandConfig) { c.strategy = "load" } }

// Idle hydrates on requestIdleCallback (client:idle).
func Idle() IslandOpt { return func(c *islandConfig) { c.strategy = "idle" } }

// Visible hydrates when the island scrolls into view (client:visible); an
// optional rootMargin widens the trigger. This is the default strategy.
func Visible(rootMargin ...string) IslandOpt {
	return func(c *islandConfig) {
		c.strategy = "visible"
		if len(rootMargin) > 0 {
			c.arg = rootMargin[0]
		}
	}
}

// Media hydrates when the media query matches (client:media).
func Media(query string) IslandOpt {
	return func(c *islandConfig) { c.strategy, c.arg = "media", query }
}

// ClientOnly never server-renders the island (client:only): the fallback shows
// until the client mounts it fresh.
func ClientOnly() IslandOpt { return func(c *islandConfig) { c.strategy = "only" } }

// SSR opts the island into a server-rendered initial HTML via the build's SSR
// sidecar (server:defer); the client then hydrates it. Phase 4.
func SSR() IslandOpt { return func(c *islandConfig) { c.ssr = true } }

// Slot sets the island's slot — the component's own children, handed to the
// framework on mount. Distinct from a Fallback.
func Slot(n Node) IslandOpt { return func(c *islandConfig) { c.slot = n } }

// Fallback sets what shows until the island hydrates (and the SSR target under
// SSR). Distinct from the component's Slot.
func Fallback(n Node) IslandOpt { return func(c *islandConfig) { c.fallback = n } }

// Once keeps the island's props as first rendered: a live re-render whose
// props changed doesn't update the mounted component (the once attribute on
// a tag). Without it an island is live: changed props reach the component's
// update, with no remount.
func Once() IslandOpt { return func(c *islandConfig) { c.once = true } }

// Callback passes a function prop: prop is its wire name (onSelect), fn a
// live view method or other handler (calling the function in the browser
// pushes it, with the function's first argument as its payload) or a JS
// chain (calling it runs the commands). The compiler writes it for an
// attribute of a function-typed prop: <Chart onSelect={p.Zoom}/>.
func Callback(prop string, fn any) IslandOpt {
	return func(c *islandConfig) { c.funcs = append(c.funcs, islandEmit{prop, fn}) }
}

// StateProp passes a client state as a prop: the component gets its value,
// is updated when it changes, and its writes (a Vue model's update:<prop>, a
// React set<Prop>, a Svelte $bindable) set the state — and whatever reads
// it on the page. A ui.Derived is passed one way. The compiler writes it for
// a ui.State attribute of an island tag: <Counter count={n}/>.
func StateProp[T any](prop string, s State[T]) IslandOpt {
	return func(c *islandConfig) { c.states = append(c.states, islandState{prop, s}) }
}

// Ptr is a pointer to v: what the compiler writes for a value given to an
// optional island prop (a pointer field).
func Ptr[T any](v T) *T { return &v }

// Emit binds one of the island component's emits to a live-view handler: the
// transpiler emits it for an on<Emit> attribute on the tag. When the mounted
// component emits name, the handler runs on the server with the emit's payload
// (a struct/*T handler param is bound from the emitted JSON). handler is an
// On-style value (a func, usually a view method).
func Emit(name string, handler any) IslandOpt {
	return func(c *islandConfig) { c.emits = append(c.emits, islandEmit{name, handler}) }
}

// emitAttr normalizes an emit name to the lower-case alphanumerics used as the
// data-vk-emit-<suffix> attribute key; the client adapter normalizes the Vue
// emit name the same way to find it.
func emitAttr(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r - 'A' + 'a')
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Template marks a named region slot inside a <ui.Island> wrapper — e.g.
// <ui.Template fallback>…</ui.Template> gives the island's pre-hydration
// fallback. The transpiler consumes these tags (they render no element of
// their own); this declaration only lets the tag resolve. Calling it directly
// renders nothing.
func Template(...any) Node { return Fragment() }

var (
	nodeType     = reflect.TypeFor[Node]()
	timeType     = reflect.TypeFor[time.Time]()
	anyStateType = reflect.TypeFor[AnyState]()
	jsChainType  = reflect.TypeFor[JSChain]()
	liveType     = reflect.TypeFor[Live]()
)

// An island's props travel as JSON in data-vk-props. Values JSON can't carry
// are tagged objects the runtime turns back into what the component gets:
//
//	{"$vk":"date","v":"2026-10-15T09:00:00Z"}     a time.Time → a Date
//	{"$vk":"fn","e":"onselect"}                   a function prop: the host's
//	                                              data-vk-on-onselect (a live
//	                                              handler) or data-vk-js-onselect
//	                                              (commands) runs when it is called
//	{"$vk":"state","s":key,"i":init,"p":[path]}   a ui.State: its value, kept
//	                                              current, and written back
//	{"$vk":"state","x":expr}                      a ui.Derived: read only

// islandField is one serialized prop of an island's struct.
type islandField struct {
	name  string
	value reflect.Value
}

// islandFields is the props of an island's struct: what isn't a prop is left
// out — unexported fields, json:"-" fields, Node-typed fields (the slot,
// conventionally a Children ui.Node, handed to the component as the slot)
// and omitempty fields that are empty. Function fields (and JS chains) are
// returned apart, as the function props they are.
func islandFields(v any) (fields []islandField, funcs []islandEmit, ok bool) {
	rv := reflect.Indirect(reflect.ValueOf(v))
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return nil, nil, false
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() || f.Anonymous && f.Type == liveType {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Type == nodeType || (f.Type.Kind() == reflect.Interface && f.Type.Implements(nodeType)) {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fv := rv.Field(i)
		if f.Type.Kind() == reflect.Interface && !fv.IsNil() {
			fv = fv.Elem()
		}
		switch {
		case fv.Kind() == reflect.Func:
			if !fv.IsNil() {
				funcs = append(funcs, islandEmit{name, fv.Interface()})
			}
			continue
		case fv.IsValid() && fv.Type().Implements(jsChainType) && !fv.Type().Implements(anyStateType):
			funcs = append(funcs, islandEmit{name, fv.Interface()})
			continue
		}
		if st, ok := fv.Interface().(AnyState); ok && fv.Kind() != reflect.Interface && st.stateOf().c == nil {
			continue // a zero ui.State: not given
		}
		if strings.Contains(","+opts+",", ",omitempty,") && emptyValue(fv) {
			continue
		}
		fields = append(fields, islandField{name, fv})
	}
	return fields, funcs, true
}

func emptyValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	}
	return v.IsZero() && v.Kind() != reflect.Struct
}

// islandPropsJSON is an island's data-vk-props, written when the render's
// context is known: a state's key is the render's.
type islandPropsJSON struct {
	props  any
	fields []islandField
	funcs  []islandEmit
	states []islandState
	plain  bool // props isn't a struct: marshaled as is
}

func (p islandPropsJSON) Render(ctx context.Context, w io.Writer) error {
	j, err := p.json(ctx)
	if err != nil {
		return err
	}
	_, err = w.Write(j)
	return err
}

func (p islandPropsJSON) json(ctx context.Context) ([]byte, error) {
	if p.plain {
		return marshal(p.props)
	}
	out := make(map[string]any, len(p.fields)+len(p.funcs)+len(p.states))
	js := false
	for _, f := range p.fields {
		v := f.value.Interface()
		if av, ok := v.(assignValuer); ok { // a *Assign[T] live prop → its value
			v = av.assignValue()
		}
		if s, ok := v.(AnyState); ok {
			r, err := stateProp(ctx, s)
			if err != nil {
				return nil, fmt.Errorf("ui: island prop %s: %w", f.name, err)
			}
			out[f.name], js = r, true
			continue
		}
		t, err := tagDates(v)
		if err != nil {
			return nil, fmt.Errorf("ui: island prop %s: %w", f.name, err)
		}
		out[f.name] = t
	}
	for _, f := range p.funcs {
		out[f.name] = map[string]string{"$vk": "fn", "e": emitAttr(f.name)}
		if _, ok := f.handler.(JSChain); ok {
			js = true
		}
	}
	for _, s := range p.states {
		r, err := stateProp(ctx, s.state)
		if err != nil {
			return nil, fmt.Errorf("ui: island prop %s: %w", s.prop, err)
		}
		out[s.prop], js = r, true
	}
	if js {
		if pr, ok := liveHost(ctx).(*pageProbe); ok {
			pr.info.JS = true
		}
	}
	return marshal(out)
}

// stateProp is a state prop's tagged reference: the state's key, initial
// value and path, or a derived state's expression (read only).
func stateProp(ctx context.Context, s AnyState) (any, error) {
	sv := s.stateOf()
	r, err := sv.ref(ctx, nil)
	if err != nil {
		return nil, err
	}
	if sv.c.derived != nil {
		return map[string]any{"$vk": "state", "x": r}, nil
	}
	m := r.(map[string]any)
	m["$vk"] = "state"
	return m, nil
}

// tagDates is v ready for JSON with each time.Time in it tagged as a date.
// A value whose type holds no time.Time is returned as is.
func tagDates(v any) (any, error) {
	if v == nil || !holdsTime(reflect.TypeOf(v)) {
		return v, nil
	}
	j, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var x any
	d := json.NewDecoder(bytes.NewReader(j))
	d.UseNumber()
	if err := d.Decode(&x); err != nil {
		return nil, err
	}
	return dateTags(reflect.TypeOf(v), x), nil
}

var holdsTimeOf sync.Map // reflect.Type → bool

// holdsTime reports whether a value of t may hold a time.Time.
func holdsTime(t reflect.Type) bool {
	if b, ok := holdsTimeOf.Load(t); ok {
		return b.(bool)
	}
	holdsTimeOf.Store(t, false) // a recursive type's own reference
	b := false
	switch t.Kind() {
	case reflect.Struct:
		if t == timeType {
			b = true
			break
		}
		for i := 0; i < t.NumField() && !b; i++ {
			b = t.Field(i).IsExported() && holdsTime(t.Field(i).Type)
		}
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		b = holdsTime(t.Elem())
	}
	holdsTimeOf.Store(t, b)
	return b
}

// dateTags walks x, a value of t decoded from its JSON, tagging the dates.
func dateTags(t reflect.Type, x any) any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if x == nil || !holdsTime(t) {
		return x
	}
	switch t.Kind() {
	case reflect.Struct:
		if t == timeType {
			return map[string]any{"$vk": "date", "v": x}
		}
		m, ok := x.(map[string]any)
		if !ok {
			return x
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" {
				name = f.Name
			}
			if v, ok := m[name]; ok {
				m[name] = dateTags(f.Type, v)
			}
		}
	case reflect.Slice, reflect.Array:
		if l, ok := x.([]any); ok {
			for i := range l {
				l[i] = dateTags(t.Elem(), l[i])
			}
		}
	case reflect.Map:
		if m, ok := x.(map[string]any); ok {
			for k := range m {
				m[k] = dateTags(t.Elem(), m[k])
			}
		}
	}
	return x
}

var islandSeq atomic.Uint64

// MountIsland renders the <vk-island> host the client loader hydrates. The
// transpiler emits it for a tag whose type carries @ui.Island; site is a stable
// id for the island's placement (the live-view diff and ctx.ref rely on it not
// changing across renders), and props is the type's value (its exported fields
// are serialized as JSON for the client).
func MountIsland(site string, props any, opts ...IslandOpt) Node {
	var cfg islandConfig
	for _, o := range opts {
		o(&cfg)
	}

	info := lookupIsland(reflect.TypeOf(props))
	if info == nil {
		return El("vk-island", []Attr{{"data-vk-error", "island not registered (missing @ui.Island?)"}})
	}

	strategy := cfg.strategy
	if strategy == "" {
		strategy = info.Strategy
	}
	if strategy == "" {
		strategy = "visible"
	}

	// A stable scope id per placement. The transpiler passes the source site;
	// a direct call without one falls back to a per-process counter (not stable
	// across renders, but direct callers aren't in the live-view diff path).
	scope := site
	if scope == "" {
		scope = "i" + strconv.FormatUint(islandSeq.Add(1), 36)
	}
	attrs := []Attr{
		{"data-vk-s", scope},
		{"data-vk-island", info.Name},
		{"data-vk-client", strategy},
	}
	switch strategy {
	case "media":
		if cfg.arg != "" {
			attrs = append(attrs, Attr{"data-vk-media", cfg.arg})
		}
	case "visible":
		if cfg.arg != "" {
			attrs = append(attrs, Attr{"data-vk-rootmargin", cfg.arg})
		}
	}
	pj := islandPropsJSON{props: props, states: cfg.states}
	fields, funcs, ok := islandFields(props)
	pj.fields, pj.funcs, pj.plain = fields, append(funcs, cfg.funcs...), !ok
	attrs = append(attrs, Attr{"data-vk-props", pj})
	if cfg.ssr {
		attrs = append(attrs, Attr{"data-vk-ssr", true})
	}
	if cfg.once {
		attrs = append(attrs, Attr{"data-vk-once", true})
	}
	if cfg.methodNames != nil {
		j, _ := marshal(cfg.methodNames)
		attrs = append(attrs, Attr{"data-vk-methods", string(j)})
	}
	for _, m := range cfg.methods {
		attrs = append(attrs, Attr{"onm-" + strings.ToLower(m.name), On(m.handler)})
	}
	if cfg.callURL != "" {
		attrs = append(attrs, Attr{"data-vk-call", cfg.callURL}, Attr{"data-vk-sig", cfg.callSig})
	}
	if cfg.err != nil {
		j, _ := marshal(cfg.err)
		attrs = append(attrs, Attr{"data-vk-err", string(j)})
	}
	if u := urlFields(reflect.TypeOf(props)); u != nil {
		j, _ := marshal(u)
		attrs = append(attrs, Attr{"data-vk-url", string(j)})
	}
	if Viewer != nil {
		attrs = append(attrs, Attr{"data-vk-viewer", viewerJSON{}})
	}
	for _, f := range pj.funcs {
		// A function prop's handler (data-vk-on-<prop>) or commands
		// (data-vk-js-<prop>); the prop's tagged value names it.
		attrs = append(attrs, Attr{"on" + emitAttr(f.name), On(f.handler)})
	}
	for _, e := range cfg.emits {
		// An "on<name>" attr with an On(handler) value renders as
		// data-vk-on-<name> with a cN:idx handler id (the live render's event
		// mechanism). The name isn't a DOM event, so nothing fires it from the
		// page; the island's client adapter reads the id and fires it on the
		// component's matching emit (via ctx.emit → the live socket).
		attrs = append(attrs, Attr{"on" + emitAttr(e.name), On(e.handler)})
	}

	var kids []Node
	if cfg.fallback != nil {
		kids = append(kids, cfg.fallback)
	}
	if cfg.slot != nil {
		kids = append(kids, El("template", []Attr{{"data-vk-slot", true}}, cfg.slot))
	}
	return El("vk-island", attrs, kids...)
}
