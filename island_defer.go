package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/vuka-lang/vuka/diag"
)

// Deferred fields. A field of a page backed by its island that its
// LiveSchema defers (l.Field(&p.Stats, ui.Defer("stats"))) isn't in the
// page's first render: its group's loader (l.Loads("stats", p.loadStats))
// runs after it — on a live page once it joins, on a stateless one when its
// component asks — and the field reaches the component as an update. The
// fields of one group load together, by one loader, and groups load
// concurrently. A loader runs on a copy of the page and sets its group's
// fields; only those are taken from it. Earlier releases' form, the tag
// defer:"stats" loaded by the method LoadStats(ctx) error, still works.

type deferGroup struct {
	name   string
	hook   bool // loaded by the LiveSchema's Loads; else by method, Load<Group>
	method reflect.Method
	ctx    bool
	fields []IslandField
}

type deferResult struct {
	groups []deferGroup
	err    error
}

var deferGroupsOf sync.Map // pointer type → deferResult

// deferGroups are the deferred groups of t (an island's struct, or a
// pointer to one), in the order of their first fields. A failed check
// isn't kept: the page's constructor may declare a loader after it.
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
	fields, err := IslandFields(t)
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
	for i := range groups {
		g := &groups[i]
		if hasLoads(t, g.name) {
			g.hook = true
			continue
		}
		name := "Load" + g.name
		m, ok := t.MethodByName(name)
		if !ok {
			return nil, diag.New("E0324", "%s.%s is deferred, and the group %s has no loader", t.Elem().Name(), t.Elem().Field(g.fields[0].Index).Name, g.name).
				WithHelp("give the group a loader in the page's LiveSchema:\nl.Loads(%q, %s.load%s)", strings.ToLower(g.name[:1])+g.name[1:], strings.ToLower(t.Elem().Name()[:1]), g.name)
		}
		mt := m.Type
		in := mt.NumIn() == 1 || mt.NumIn() == 2 && mt.In(1) == ctxType
		out := mt.NumOut() == 0 || mt.NumOut() == 1 && mt.Out(0) == errType
		if !in || !out {
			return nil, diag.New("E0325", "%s.%s is the loader of the deferred group %s, and it is %s", t.Elem().Name(), name, g.name, mt).
				WithHelp("declare it %s() or %s(ctx context.Context), returning nothing or an error", name, name)
		}
		g.method, g.ctx = m, mt.NumIn() == 2
	}
	return groups, nil
}

// isLoader reports whether the method name of t (a pointer) is a deferred
// group's loader, which the component doesn't call.
func isLoader(t reflect.Type, name string) bool {
	groups, _ := deferGroups(t)
	for _, g := range groups {
		if !g.hook && g.method.Name == name {
			return true
		}
	}
	return false
}

// Loads is the state of a page's deferred groups: loading, loaded, or
// failed with the error its component reads (live.stats.error).
type Loads struct {
	mu      sync.Mutex
	started map[string]bool
	done    map[string]bool
	errs    map[string]*IslandError
}

func newLoads() *Loads {
	return &Loads{started: map[string]bool{}, done: map[string]bool{}, errs: map[string]*IslandError{}}
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
	var in []reflect.Value
	if d.g.ctx {
		in = append(in, reflect.ValueOf(ctx))
	}
	d.err = func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%s's loader of %s panicked: %v", d.page.Type().Elem().Name(), d.g.name, r)
			}
		}()
		if d.g.hook {
			return liveHooks(d.copy).loads[d.g.name](ctx)
		}
		out := d.copy.MethodByName(d.g.method.Name).Call(in)
		if len(out) == 1 && !out[0].IsNil() {
			return out[0].Interface().(error)
		}
		return nil
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
		d.l.errs[d.g.name] = e
		return
	}
	for _, f := range d.g.fields {
		d.page.Elem().Field(f.Index).Set(d.copy.Elem().Field(f.Index))
	}
	d.l.done[d.g.name] = true
	delete(d.l.errs, d.g.name)
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

// StartDeferred starts the deferred groups of c, a live page backed by its
// island, not started yet (none when c is no such page): package live runs
// each once the page joins, and applies it where its events run.
func StartDeferred(c Stateful) []*DeferredLoad {
	x := reflect.ValueOf(c)
	if !IsIsland(x.Type()) {
		return nil
	}
	if groups, err := deferGroups(x.Type()); err != nil || len(groups) == 0 {
		return nil
	}
	return liveLoads(c).Start(x)
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
