package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/vuka-lang/vuka/v2/diag"
)

// Deferred fields. A field of a live page or component that its LiveSchema
// defers (l.Field(&p.Stats, ui.Defer("stats"))) isn't in its first render:
// its group's loader (l.Loads("stats", p.loadStats)) runs after it — on a
// live page once it joins, on a stateless page backed by its island when
// its component asks — and the field arrives as an update: its island's
// component gets it, a template renders again with it (Live.Loading,
// Live.LoadErr). The fields of one group load together, by one loader,
// and groups load concurrently. A loader runs on a copy of the page and
// sets its group's fields; only those are taken from it. A loader is
// declared with @ui.Loads("stats") on its method, never found by its name.

type deferGroup struct {
	name   string
	fields []IslandField
}

type deferResult struct {
	groups []deferGroup
	err    error
}

var deferGroupsOf sync.Map // pointer type → deferResult

// deferGroups are the deferred groups of t (a live page's or component's
// struct, or a pointer to one), in the order of their first fields.
func deferGroups(t reflect.Type) ([]deferGroup, error) {
	if t.Kind() != reflect.Pointer {
		t = reflect.PointerTo(t)
	}
	if v, ok := deferGroupsOf.Load(t); ok {
		r := v.(deferResult)
		return r.groups, r.err
	}
	groups, err := deferGroupsFor(t)
	if err == nil {
		deferGroupsOf.Store(t, deferResult{groups, err})
	}
	return groups, err
}

func deferGroupsFor(t reflect.Type) ([]deferGroup, error) {
	var fields []IslandField
	var err error
	if IsIsland(t) {
		fields, err = IslandFields(t)
	} else {
		fields, err = deferredFields(t.Elem())
	}
	if err != nil {
		return nil, err
	}
	var groups []deferGroup
	at := map[string]int{}
	for _, f := range fields {
		if f.Defer == "" {
			continue
		}
		i, ok := at[f.Defer]
		if !ok {
			i = len(groups)
			at[f.Defer] = i
			groups = append(groups, deferGroup{name: f.Defer})
		}
		groups[i].fields = append(groups[i].fields, f)
	}
	for _, g := range groups {
		if hasLoads(t, g.name) {
			continue
		}
		group := strings.ToLower(g.name[:1]) + g.name[1:]
		help := fmt.Sprintf("mark its loader @ui.Loads(%q)", group)
		if _, ok := t.MethodByName("Load" + g.name); ok {
			help = fmt.Sprintf("mark Load%s @ui.Loads(%q): a loader isn't found by its name", g.name, group)
		}
		return nil, diag.New("E0324", "%s.%s is deferred, and the group %s has no loader", t.Elem().Name(), t.Elem().Field(g.fields[0].Index).Name, g.name).
			WithHelp("%s", help)
	}
	return groups, nil
}

// deferredFields are the fields of t, a component rendered by its own
// markup, that it defers: exported or not, since its markup reads them on
// the server.
func deferredFields(t reflect.Type) ([]IslandField, error) {
	if t.Kind() != reflect.Struct {
		return nil, nil
	}
	if err := schemaErr(t); err != nil {
		return nil, err
	}
	schema := liveSchemaOf(t)
	var out []IslandField
	for i := range t.NumField() {
		f := t.Field(i)
		var so FieldOptions
		if schema != nil {
			so = schema.fields[i]
		}
		d, hasDefer := f.Tag.Lookup("defer")
		if so.Defer != "" {
			d, hasDefer = so.Defer, true
		}
		if !hasDefer || d == "-" {
			continue
		}
		bind, hasBind := f.Tag.Lookup("bind")
		url, _, _ := strings.Cut(f.Tag.Get("url"), ",")
		g, err := deferredGroup(t, f, d, so.Defer == "", hasBind && bind != "-" || so.Bind, so.URLBound || url != "" && url != "-")
		if err != nil {
			return nil, err
		}
		out = append(out, IslandField{Handle: f.Name, Wire: f.Name, Index: i, Type: f.Type, Defer: g})
	}
	return out, nil
}

// IsDeferred reports whether t's (a pointer to a live component's struct)
// field i is deferred: the component's own, never set by its tag.
func IsDeferred(t reflect.Type, i int) bool {
	groups, _ := deferGroups(t)
	for _, g := range groups {
		for _, f := range g.fields {
			if f.Index == i {
				return true
			}
		}
	}
	return false
}

// Loads is the state of a page's deferred groups: loading, loaded, or
// failed with the error its component reads (live.stats.error, or
// LoadErr in its markup).
type Loads struct {
	mu      sync.Mutex
	owner   string          // the page's type, for E0325
	groups  map[string]bool // its deferred groups, once it mounted
	started map[string]bool
	done    map[string]bool
	errs    map[string]*IslandError
	causes  map[string]error // the loaders' errors, as they returned them
}

func newLoads() *Loads {
	return &Loads{started: map[string]bool{}, done: map[string]bool{}, errs: map[string]*IslandError{}, causes: map[string]error{}}
}

// mountLoads gives c, mounting, the state of its deferred groups: none
// loaded. A group without a loader fails the mount (E0324).
func mountLoads(c Stateful) error {
	t := reflect.TypeOf(c)
	groups, err := deferGroups(t)
	if err != nil || len(groups) == 0 {
		return err
	}
	l := liveLoads(c)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.owner, l.groups = t.Elem().Name(), map[string]bool{}
	for _, g := range groups {
		l.groups[g.name] = true
	}
	return nil
}

// LoadsFrom is the state of the deferred groups of t (an island's struct)
// whose page was rendered with props, its props JSON: a group whose fields
// it holds is loaded. A stateless page keeps it in its signed props.
func LoadsFrom(t reflect.Type, props []byte) *Loads {
	l := newLoads()
	groups, _ := deferGroups(t)
	if len(groups) == 0 || len(props) == 0 {
		return l
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(props, &m) != nil {
		return l
	}
	for _, g := range groups {
		all := true
		for _, f := range g.fields {
			if _, ok := m[f.Wire]; !ok {
				all = false
			}
		}
		if all {
			l.done[g.name], l.started[g.name] = true, true
		}
	}
	return l
}

// pending are the wire names of t's deferred fields not loaded yet.
func (l *Loads) pending(t reflect.Type) map[string]bool {
	groups, _ := deferGroups(t)
	if len(groups) == 0 {
		return nil
	}
	out := map[string]bool{}
	if l != nil {
		l.mu.Lock()
		defer l.mu.Unlock()
	}
	for _, g := range groups {
		if l != nil && l.done[g.name] {
			continue
		}
		for _, f := range g.fields {
			out[f.Wire] = true
		}
	}
	return out
}

// JSON is the page's data-vk-defer for t: the wire names still pending
// ("p") and each failed field's error ("e"); nil when t defers nothing.
func (l *Loads) JSON(t reflect.Type) map[string]any {
	groups, _ := deferGroups(t)
	if len(groups) == 0 {
		return nil
	}
	p, e := []string{}, map[string]*IslandError{}
	if l != nil {
		l.mu.Lock()
		defer l.mu.Unlock()
	}
	for _, g := range groups {
		for _, f := range g.fields {
			switch {
			case l != nil && l.done[g.name]:
			case l != nil && l.errs[g.name] != nil:
				e[f.Wire] = l.errs[g.name]
			default:
				p = append(p, f.Wire)
			}
		}
	}
	return map[string]any{"p": p, "e": e}
}

// DeferredLoad is a deferred group of a page, started: Run loads it on a
// copy of the page taken when it started, on any goroutine; Apply, where
// the page's events run, gives the page the group's fields, or records the
// loader's error for them.
type DeferredLoad struct {
	Group string
	l     *Loads
	g     deferGroup
	page  reflect.Value
	copy  reflect.Value
	err   error
}

// Run runs the group's loader with ctx; a ctx done first is its error.
func (d *DeferredLoad) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		d.err = err
		return err
	}
	d.err = func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%s's loader of %s panicked: %v", d.page.Type().Elem().Name(), d.g.name, r)
			}
		}()
		return liveHooks(d.copy).loads[d.g.name](ctx)
	}()
	if d.err == nil {
		d.err = ctx.Err()
	}
	return d.err
}

// Apply gives the page what Run loaded, or records its error: text, when
// not nil, is the error's message for the browser (else ErrorOf's).
func (d *DeferredLoad) Apply(text func(error) string) {
	d.l.mu.Lock()
	defer d.l.mu.Unlock()
	if d.err != nil {
		e := ErrorOf(d.err)
		if text != nil {
			e.Message = text(d.err)
		}
		d.l.errs[d.g.name], d.l.causes[d.g.name] = e, d.err
		return
	}
	for _, f := range d.g.fields {
		settable(d.page.Elem().Field(f.Index)).Set(settable(d.copy.Elem().Field(f.Index)))
	}
	d.l.done[d.g.name] = true
	delete(d.l.errs, d.g.name)
	delete(d.l.causes, d.g.name)
}

// Err is what Run returned.
func (d *DeferredLoad) Err() error { return d.err }

// Start starts the deferred groups of x (a pointer to an island's struct)
// not started yet: each a DeferredLoad on its own copy of x, taken now.
func (l *Loads) Start(x reflect.Value) []*DeferredLoad {
	groups, _ := deferGroups(x.Type())
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []*DeferredLoad
	for _, g := range groups {
		if l.started[g.name] {
			continue
		}
		l.started[g.name] = true
		cp := reflect.New(x.Type().Elem())
		cp.Elem().Set(x.Elem())
		for i := 0; i < cp.Elem().NumField(); i++ {
			if f := cp.Type().Elem().Field(i); f.Anonymous && f.Type == liveType {
				cp.Elem().Field(i).Set(reflect.Zero(liveType))
			}
		}
		out = append(out, &DeferredLoad{Group: g.name, l: l, g: g, page: x, copy: cp})
	}
	return out
}

// Load runs the deferred groups of x not loaded yet, concurrently, and
// applies them (Apply, with text); what a stateless page's component asks
// for after it mounts.
func (l *Loads) Load(ctx context.Context, x reflect.Value, text func(error) string) {
	ds := l.Start(x)
	var wg sync.WaitGroup
	for _, d := range ds {
		wg.Go(func() { d.Run(ctx) })
	}
	wg.Wait()
	for _, d := range ds {
		d.Apply(text)
	}
}

// StartDeferred starts the deferred groups of c, a live page or component,
// not started yet: package live runs each once the page joins, and applies
// it where its events run.
func StartDeferred(c Stateful) []*DeferredLoad {
	x := reflect.ValueOf(c)
	if groups, err := deferGroups(x.Type()); err != nil || len(groups) == 0 {
		return nil
	}
	return liveLoads(c).Start(x)
}

// loadNow loads the deferred groups of c, a component rendered without a
// live session, before its only render: it has no connection to send them
// over later. A page backed by its island leaves them to its component.
func loadNow(ctx context.Context, c Stateful) {
	x := reflect.ValueOf(c)
	if IsIsland(x.Type()) {
		return
	}
	if groups, err := deferGroups(x.Type()); err == nil && len(groups) > 0 {
		liveLoads(c).Load(ctx, x, nil)
	}
}

// state is the group's, as Loading and LoadErr read it: done, or failed
// with err. A group the page doesn't defer fails the render (E0325).
func (l *Loads) state(group string) (done bool, err error) {
	g := group
	if g != "" {
		g = groupName(g)
	}
	if l != nil {
		l.mu.Lock()
		defer l.mu.Unlock()
	}
	if l == nil || !l.groups[g] {
		owner := "the component"
		if l != nil && l.owner != "" {
			owner = l.owner
		}
		panic(mustFailure{diag.New("E0325", "%s defers no group %q", owner, group).
			WithHelp(`defer a field to it, @ui.Defer(%q), or name a group the component defers`, group)})
	}
	if e := l.errs[g]; e != nil {
		return false, &DeferError{Group: group, Message: e.Message, Status: e.Status, Err: l.causes[g]}
	}
	return l.done[g], nil
}

// DeferError is a deferred group's loader's error, as Live.LoadErr gives
// it: Message is what the visitor may read — a web.Error's message, else
// "error"; under vuka run the cause's text — and Err the loader's error.
type DeferError struct {
	Group   string
	Message string
	Status  int // a web.Error's status; 0 for another error
	Err     error
}

func (e *DeferError) Error() string { return e.Message }
func (e *DeferError) Unwrap() error { return e.Err }

// settable is f, a field of an addressable struct, readable and settable
// though it is unexported.
func settable(f reflect.Value) reflect.Value {
	if f.CanSet() {
		return f
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

func liveLoads(c Stateful) *Loads {
	l := c.vukaLive()
	if l.loads == nil {
		l.loads = newLoads()
	}
	return l.loads
}

// WithLoads renders a page backed by its island with its deferred groups
// as l says: those not loaded left out of its props. Package web passes a
// stateless page's.
func WithLoads(l *Loads) IslandOpt { return func(c *islandConfig) { c.loads = l } }
