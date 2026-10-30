// Package live runs stateful components for a connected browser: a Session
// keeps the component instances of one page, calls their event handlers and
// Update methods, and answers each with the HTML of the components whose
// output changed. It knows no transport; a WebSocket server drives it with the
// messages of protocol.go.
package live

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/vuka-lang/ui/v2"
)

// Session is one connected page: its render function and the instances of the
// stateful components it renders. Its methods are safe to call from several
// goroutines; each runs alone.
type Session struct {
	mu     sync.Mutex
	ctx    context.Context
	render func(ctx context.Context) ui.Node
	// adopting is set by Adopt until the next render of the page: its tags
	// set no routed page's fields.
	adopting bool
	root     *instance
	byID     map[string]*instance
	next     int
	pass     int
	ids      ids // the pass's names for library components' random ids (ids.go)
	closed   bool
	trees    bool // a v2 connection: instances render to trees
	c        conn
	verify   bool
	whole    bool     // each instance one dynamic of its HTML: wholeTrees's
	js       []JSPush // pushed by Render or RenderTrees since JS last drained them, for the join
	router   Router

	updates map[ui.Stateful]any // a navigation's messages for kept instances, by their tags' props
	uerr    error               // what their Update methods returned

	url *url.URL   // the page's URL as an event's edit left it; nil: its context's (ui.PageURL)
	q   url.Values // its query, parsed (query)

	errText func(error) string // SetErrorText's
}

// Patch is what an event or message changed: the HTML of each component whose
// output changed, outermost first (a changed component's update includes the
// components inside it), and the error a handler returned.
type Patch struct {
	Updates []Update     `json:"updates,omitempty"`
	Trees   []TreeUpdate `json:"trees,omitempty"`
	JS      []JSPush     `json:"js,omitempty"`
	Error   string       `json:"error,omitempty"`
	// Err is the handler's error as it returned it, for a transport to
	// read (a plain form post answering 422, or a redirect); never sent.
	Err error `json:"-"`
}

// JSPush is JS commands a component pushed (ui.Live.PushJS), for the
// browser to run once the message carrying it is applied: Ops, the
// commands' JSON, run with the element whose data-vk-id is ID as This.
type JSPush struct {
	ID  string          `json:"id"`
	Ops json.RawMessage `json:"ops"`
}

// TreeUpdate is an instance's change on a v2 connection (see RenderTrees):
// its whole tree (Full), the change to the tree the client holds, or its
// whole HTML when that is smaller. Tree is JSON, encoded as components.md
// ("Protocol v2") describes.
type TreeUpdate struct {
	ID   string          `json:"id"`
	Full bool            `json:"full,omitempty"`
	Tree json.RawMessage `json:"tree,omitempty"`
	HTML string          `json:"html,omitempty"`
}

// Update is a component's new HTML. ID is the data-vk-id of the element it
// replaces; RootID is the page itself, whose HTML replaces the container's
// content.
type Update struct {
	ID   string `json:"id"`
	HTML string `json:"html"`
}

// RootID is the id of the page's own render, which owns the handlers of
// elements outside any stateful component.
const RootID = "c0"

// NewSession is a session rendering the page render returns. ctx is the
// context of Mount, handlers and Update methods, and of every render.
func NewSession(ctx context.Context, render func(ctx context.Context) ui.Node) *Session {
	s := &Session{ctx: ctx, render: render, byID: map[string]*instance{}, verify: Verify, whole: wholeTrees}
	s.root = &instance{s: s, id: RootID, children: map[string]*instance{}}
	s.byID[RootID] = s.root
	return s
}

// Render renders the whole page, mounting the components it meets for the
// first time, and returns its HTML: what the page's container holds.
func (s *Session) Render() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trees = false
	s.refresh()
	if err := s.renderAll(); err != nil {
		return "", err
	}
	js, err := s.pushed()
	if err != nil {
		return "", err
	}
	s.js = append(s.js, js...)
	s.settle()
	return s.root.html(), nil
}

// RenderTrees makes the session a v2 one and renders the whole page as every
// instance's render tree, the page's first: what a v2 client joins with.
// From then on patches carry Trees instead of Updates. It starts the
// connection afresh: every statics id and kept string is forgotten.
func (s *Session) RenderTrees() ([]TreeUpdate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trees = true
	s.c.reset()
	s.refresh()
	if err := s.renderAll(); err != nil {
		return nil, err
	}
	js, err := s.pushed()
	if err != nil {
		return nil, err
	}
	s.js = append(s.js, js...)
	ups := s.treeUpdates()
	s.settle()
	return ups, nil
}

// JS is the commands components pushed while the last Render or
// RenderTrees mounted them (from a connected Mount), for the join's reply;
// it returns them once.
func (s *Session) JS() []JSPush {
	s.mu.Lock()
	defer s.mu.Unlock()
	js := s.js
	s.js = nil
	return js
}

// Adopt hands the session to a new connection: ctx is the context of its
// handlers, Update methods and renders from now on, and render (when not
// nil) the page's render function. The instances stay mounted with their
// state, so a session kept from a page's HTTP render can serve its live
// connection without mounting again: the next Render or RenderTrees renders
// the kept instances, and commands they pushed while mounting are still in
// JS. The old context must not be needed again — it is usually a finished
// request's.
func (s *Session) Adopt(ctx context.Context, render func(ctx context.Context) ui.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("live: the session is closed")
	}
	s.ctx = ctx
	s.setURL(nil)
	if render != nil {
		s.render, s.adopting = render, true
	}
	return nil
}

// refresh makes every instance render again, its output sent whole.
func (s *Session) refresh() {
	for _, in := range s.byID {
		in.fresh, in.shadow = true, nil
	}
}

// HTML is the page's HTML as last rendered, without rendering it again.
func (s *Session) HTML() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trees {
		var b strings.Builder
		s.root.treeHTML(&b)
		return b.String()
	}
	return s.root.html()
}

// Event calls the handler ev.Target names with the event's payload, renders
// the page again, and returns the components whose output changed. A
// handler's error is the patch's Error; the returned error is the session's:
// an unknown handler, or a render that failed.
func (s *Session) Event(ev ui.Event) (Patch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn := s.handler(ev.Target)
	if fn == nil {
		return Patch{}, s.noHandler(ev.Target)
	}
	return s.run(fn, ev)
}

// run calls a handler for ev, renders the page again and returns what
// changed. A handler's error redirecting (ui.RedirectOf: web.Redirect) moves
// the page there instead of being shown, and one that shows only in the page
// (ui.QuietError: a form that didn't pass its checks) isn't the patch's
// Error; both are its Err.
func (s *Session) run(fn any, ev ui.Event) (Patch, error) {
	ctx, edit := s.beginURL(s.ctx)
	herr := callHandler(ctx, fn, ev)
	s.root.js = append(s.root.js, s.endURL(edit)...)
	if to, ok := ui.RedirectOf(herr); ok && ui.LoadableURL(to) {
		s.root.js = append(s.root.js, ui.JS.Navigate(to))
	}
	p, err := s.patch()
	p.Err = herr
	if _, redirect := ui.RedirectOf(herr); herr != nil && !redirect && !ui.QuietError(herr) {
		p.Error = s.errorText(herr)
	}
	return p, err
}

// Submit submits the form of the page whose identity is id (its _vk_form
// field) with values, as a plain HTML POST of it does: the submit handler
// its last render registered runs as the form's submit event would, and
// the page renders again. The patch's Err is the handler's: ui.ErrFormInvalid
// when the form didn't pass its checks, a redirect, or the handler's own.
func (s *Session) Submit(id string, values url.Values) (Patch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var fn any
	for _, in := range s.order() {
		if f, ok := in.forms[id]; ok {
			fn = f
			break
		}
	}
	if fn == nil {
		return Patch{}, &NoFormError{ID: id}
	}
	return s.run(fn, ui.Event{Target: "form:" + id, Type: "submit", Form: values})
}

// NoFormError is the error of a submit naming a form the page doesn't
// render (Session.Submit).
type NoFormError struct{ ID string }

func (e *NoFormError) Error() string { return fmt.Sprintf("live: the page has no form %q", e.ID) }

// Handler is the function an event's target names ("c1:0"): a method
// value such as c.Inc, for a transport to say which handler an event ran
// (a development toolbar's "Counter.Inc"); nil when it names none.
func (s *Session) Handler(target string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handler(target)
}

// Component is the component an event's target belongs to ("c1:0",
// "c1:Inc"): the instance's, for a transport to read what its type
// declares (a gate on the type, refusing every event of it) before the
// handler runs; nil when the target names no instance, or one that is no
// component (the page's root).
func (s *Session) Component(target string) ui.Stateful {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, _, _ := strings.Cut(target, ":")
	if in := s.byID[id]; in != nil {
		return in.comp
	}
	return nil
}

// handler is the function target names: an instance's handler by number
// ("c1:0"), one a command it pushed names ("c1:0p", see pushHandler), or
// a method of its component by name ("c1:Inc", what the component's
// scripts push) when AllowPush allows it.
func (s *Session) handler(target string) any {
	id, n, ok := strings.Cut(target, ":")
	in := s.byID[id]
	if !ok || in == nil {
		return nil
	}
	var fn any
	if p, ok := strings.CutSuffix(n, "p"); ok && isNumber(p) {
		fn = in.pushes[n]
	} else if h, err := strconv.Atoi(n); err != nil {
		fn = pushable(in.comp, n)
	} else if h >= 0 && h < len(in.handlers) {
		fn = in.handlers[h]
	}
	if set, ok := fn.(ui.Setter); ok && in.src != nil {
		// what it sets are props: the tag's too, or its next render would
		// set them back — unless the tag is a routed page's, which sets
		// none.
		return func(ctx context.Context, ev ui.Event) error {
			if err := set(ctx, ev); err != nil {
				return err
			}
			in.set = true
			if !in.routed {
				copyProps(in.src, in.comp)
			}
			return nil
		}
	}
	return fn
}

// isNumber reports whether s is a decimal number, no sign.
func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// noHandler is the error of an event whose target names no handler, saying
// whose it would have been and why it's gone.
func (s *Session) noHandler(target string) error {
	id, n, _ := strings.Cut(target, ":")
	in := s.byID[id]
	e := &NoHandlerError{Target: target}
	switch {
	case in == nil:
		e.reason = fmt.Sprintf("no component %s is mounted (it was unmounted, or the page navigated)", id)
		return e
	case in.comp == nil && n != "":
		e.Component = "page"
		e.reason = fmt.Sprintf("the page's last render has %d", len(in.handlers))
		return e
	}
	e.Component = typeName(in.typ)
	if p, ok := strings.CutSuffix(n, "p"); ok && isNumber(p) {
		e.reason = fmt.Sprintf("%s (%s) keeps the handlers of its last %d pushed commands, and this one is older", e.Component, id, maxPushes)
	} else if isNumber(n) {
		e.reason = fmt.Sprintf("%s (%s): its last render has %d", e.Component, id, len(in.handlers))
	} else {
		e.reason = fmt.Sprintf("%s (%s) takes no push named %s", e.Component, id, n)
	}
	return e
}

// typeName is a component's type as an error names it, its package's name
// and the type's ("views.Table"), a generic one's type arguments elided
// ("views.Table[…]").
func typeName(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name, _, generic := strings.Cut(t.String(), "[")
	if generic {
		name += "[…]"
	}
	return name
}

// AllowPush lets a component's scripts push its methods by name: typ is
// the component's struct type as reflect spells it ("main.Counter",
// "example.com/app/views.Chart"), methods the names its scripts push,
// replacing any allowed before (none: no pushes by name). The vuka command
// lists them (each ctx.push("Name") of the scripts) and web registers them
// from its manifest. A component nothing allows accepts no pushes by name;
// the handlers its page renders work regardless. Render, its hooks, an
// Update and methods promoted from what it embeds are never pushed. "_set" allows
// setting its bind: fields (ui.SetBound), as a page script's
// live.<field>.set(v) does.
func AllowPush(typ string, methods ...string) {
	pushMu.Lock()
	defer pushMu.Unlock()
	if len(methods) == 0 {
		delete(pushAllowed, typ)
		return
	}
	pushAllowed[typ] = slices.Clone(methods)
}

var (
	pushMu      sync.RWMutex
	pushAllowed = map[string][]string{}
)

func pushable(c ui.Stateful, name string) any {
	if c == nil || name == "Render" || ui.IsHook(c, name) {
		return nil
	}
	if m, ok := reflect.TypeOf(c).MethodByName(name); ok && ui.IsUpdate(m) {
		return nil
	}
	v := reflect.ValueOf(c)
	if v.Kind() != reflect.Pointer || v.Type().Elem().Kind() != reflect.Struct {
		return nil
	}
	t := v.Type().Elem()
	pushMu.RLock()
	ok := slices.Contains(pushAllowed[t.PkgPath()+"."+t.Name()], name)
	pushMu.RUnlock()
	if !ok {
		return nil
	}
	if name == "_set" {
		// a page script's live.<field>.set(v): its bind: fields, as an
		// island page sets them.
		return ui.Setter(func(ctx context.Context, ev ui.Event) error { return ui.SetBound(ctx, v, ev.Data) })
	}
	m := v.MethodByName(name)
	if !m.IsValid() {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); f.Anonymous {
			if _, ok := reflect.PointerTo(f.Type).MethodByName(name); ok {
				return nil
			}
			if _, ok := f.Type.MethodByName(name); ok {
				return nil
			}
		}
	}
	return m.Interface()
}

// Info delivers msg to the Update method fitting its type of every instance
// subscribed to topic (of every instance, for ""), renders the page again if
// any took it, and returns what changed. An Update's error is the patch's Error.
func (s *Session) Info(topic string, msg any) (Patch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var herr error
	called := false
	ctx, edit := s.beginURL(s.ctx)
	for _, in := range s.order() {
		if in.comp == nil || topic != "" && !slices.Contains(in.topics, topic) {
			continue
		}
		ok, err := callUpdate(ctx, in.comp, msg)
		called = called || ok
		herr = errors.Join(herr, err)
	}
	if !called {
		return Patch{}, herr
	}
	s.root.js = append(s.root.js, s.endURL(edit)...)
	p, err := s.patch()
	if herr != nil {
		p.Error = s.errorText(herr)
	}
	return p, err
}

// Deferred starts the deferred loads of the pages backed by their islands
// the session holds (ui.StartDeferred), each once: the transport runs each
// (Run, with a context ending when the page does) once the page has joined,
// and hands it to Loaded.
func (s *Session) Deferred() []*ui.DeferredLoad {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*ui.DeferredLoad
	for _, in := range s.order() {
		if in.comp != nil {
			out = append(out, ui.StartDeferred(in.comp)...)
		}
	}
	return out
}

// Loaded gives its page what the deferred load d loaded (or its error,
// as the session's error text says, for the component to read), renders
// the page again, and returns what changed.
func (s *Session) Loaded(d *ui.DeferredLoad) (Patch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Patch{}, errors.New("live: the session is closed")
	}
	d.Apply(s.errorTextFn())
	return s.patch()
}

func (s *Session) errorTextFn() func(error) string {
	text := s.errText
	return func(err error) string {
		if m, ok := PublicMessage(err); ok {
			return m
		}
		if text == nil {
			return "error"
		}
		return text(err)
	}
}

// Topics is every topic an instance subscribes to, sorted: what the transport
// routes to Info. It changes as instances come and go.
func (s *Session) Topics() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ts []string
	for _, in := range s.byID {
		for _, t := range in.topics {
			if !slices.Contains(ts, t) {
				ts = append(ts, t)
			}
		}
	}
	slices.Sort(ts)
	return ts
}

// Route is the page a URL renders, what a Router gives Navigate: Render,
// the page's render function, with Ctx (when set) the context of what runs
// from then on, or, with Render nil, Redirect, a URL for the browser to load
// (the URL itself when empty). Updates holds messages for a patch: an
// instance kept for a tag whose props are a key takes the value in its
// Update method before it renders.
type Route struct {
	Ctx      context.Context
	Render   func(ctx context.Context) ui.Node
	Redirect string
	Updates  map[ui.Stateful]any
	// Keep keeps, on a navigation, the instances the new page renders
	// where the old one rendered them, as a patch does, instead of
	// unmounting every one first; those it doesn't render are unmounted
	// after it renders. A router keeps a layout's components this way,
	// giving the page below the layout a ui.Keyed key of its own per
	// page, so that the page's components mount afresh.
	Keep bool
	// URL is the page's URL, what its components' @ui.URL fields read,
	// when Ctx doesn't carry it (ui.WithPageURL); the navigate message
	// sets it.
	URL string
}

// Router resolves the URLs a connected page navigates to.
type Router func(url string) Route

// SetRouter sets what the navigate message renders. Without a router every
// navigation is a full page load: the reply is a redirect.
func (s *Session) SetRouter(r Router) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.router = r
}

// Navigate makes rt's page the session's, renders it, and returns what
// changed, as an event's patch. As a patch (patch true), the instances the
// new page renders where the old one rendered them stay mounted, their props
// set from its tags, and those rt.Updates names take their message first.
// Otherwise every instance is unmounted
// and the new page's mount afresh. The page itself (c0) is the same
// instance either way, so a client gets only what changed of its markup — on
// a v2 connection, against the statics it already holds.
//
// The router decides what may be kept, not the client asking for a patch:
// a patch with no rt.Updates is a navigation, every instance mounting
// afresh, and an instance whose tag rt.Updates names is kept only when an
// Update method of its takes the message — else it mounts afresh, so its
// Mount sees the new URL. A component that checks in Mount whether the
// visitor may see what its props name must check again in that Update.
//
// An error — a Mount's or an Update's, or the render's — fails the
// navigation: Navigate returns it, every instance is unmounted, and the
// page has no handlers until it navigates or joins again (a browser
// loads the URL itself, getting the route's own answer).
func (s *Session) Navigate(rt Route, patch bool) (Patch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Patch{}, errors.New("live: the session is closed")
	}
	if rt.Render == nil {
		return Patch{}, errors.New("live: Navigate to a route without a page")
	}
	s.render = rt.Render
	if rt.Ctx != nil {
		s.ctx = rt.Ctx
	}
	s.setURL(nil)
	if ui.PageURL(s.ctx) == nil && rt.URL != "" {
		if u, err := url.Parse(rt.URL); err == nil {
			s.setURL(u)
		}
	}
	if !rt.Keep && (!patch || len(rt.Updates) == 0) {
		for _, in := range s.order()[1:] {
			unmount(in)
		}
		s.byID = map[string]*instance{RootID: s.root}
		s.root.children, s.root.kids = map[string]*instance{}, nil
	}
	s.updates, s.uerr = maps.Clone(rt.Updates), nil
	defer func() { s.updates, s.uerr = nil, nil }()
	p, err := s.patch()
	if err = cmp.Or(err, s.uerr); err != nil {
		// nothing of a page that failed to render, or whose Update refused
		// its URL, stays mounted to take events
		for _, in := range s.order()[1:] {
			unmount(in)
		}
		s.byID = map[string]*instance{RootID: s.root}
		s.root.children, s.root.kids, s.root.handlers = map[string]*instance{}, nil, nil
		return Patch{}, err
	}
	return p, nil
}

// Close unmounts every instance. The session renders nothing after it.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, in := range s.order() {
		unmount(in)
	}
	s.byID, s.closed = map[string]*instance{}, true
}

// instance is a stateful component in the session, or the page itself (comp nil).
type instance struct {
	s        *Session
	id       string
	comp     ui.Stateful
	typ      reflect.Type
	children map[string]*instance // by place: key path, site, key or occurrence
	kids     []*instance          // in render order, this pass
	occ      map[string]int
	handlers []any
	pushes   map[string]any // the handlers of the commands it pushed, by id (pushHandler)
	forms    map[string]any // the submit handlers of the forms its last render has, by identity
	pushIDs  []string       // their ids, oldest first, at most maxPushes
	pushSeq  int            // the number of the next one
	topics   []string
	js       []ui.Commands // pushed since the last reply
	seen     int
	src      ui.Stateful // the props its tag last rendered it with
	routed   bool        // a routed page: its exported fields are its own
	shown    ui.Stateful // a routed page's fields as it last rendered
	set      bool        // a page script set its props: it renders again
	own      string      // its HTML, each child as a marker
	changed  bool
	fresh    bool
	key      any          // its tag's key, written on its root as data-vk-key
	url      ui.URLFields // its @ui.URL fields' bookkeeping
	urlKey   string       // its tag's urlKey

	tree       *ui.Tree          // its last render, on a v2 connection
	shadow     *sframe           // what the client holds of it; nil: nothing yet
	rendered   bool              // rendered this pass (not skipped)
	contextual bool              // context-dependent output must not use state-only render reuse
	vars       map[string]uint64 // the template variables its last render read, at their versions
	idn        int               // the random ids its render named so far (ids.go)
}

func (in *instance) ID() string { return in.id }

func (in *instance) Subscribe(topics ...string) {
	for _, t := range topics {
		if !slices.Contains(in.topics, t) {
			in.topics = append(in.topics, t)
		}
	}
}

func (in *instance) PushJS(c ui.Commands) { in.js = append(in.js, c) }

// pushed encodes the commands instances pushed, in render order, each Push
// step's handler registered with its instance after its render's own.
func (s *Session) pushed() ([]JSPush, error) {
	var out []JSPush
	for _, in := range s.order() {
		for _, c := range in.js {
			ops, err := ui.EncodeJS(ui.WithLiveHost(s.ctx, &scope{in: in, push: true}), c)
			if err != nil {
				in.js = nil
				return nil, err
			}
			out = append(out, JSPush{in.id, ops})
		}
		in.js = nil
	}
	return out, nil
}

// scope is where a render is: the instance whose output it is, and the keys
// of the elements around it.
type scope struct {
	in   *instance
	keys string
	push bool // encoding commands the instance pushed, not its render
}

func (sc *scope) ContextRead() { sc.in.contextual = true }

// VarRead records a template variable the instance's render read
// (ui.ReadVar), at its version.
func (sc *scope) VarRead(name string, ver uint64) {
	if sc.in.vars == nil {
		sc.in.vars = map[string]uint64{}
	}
	sc.in.vars[name] = ver
}

// Form keeps the submit handler of a form the instance renders, for a
// plain POST of it (Submit).
func (sc *scope) Form(id string, submit any) {
	if sc.push {
		return
	}
	if sc.in.forms == nil {
		sc.in.forms = map[string]any{}
	}
	sc.in.forms[id] = submit
}

func (sc *scope) Handler(_ context.Context, fn any) string {
	if sc.push {
		return sc.in.pushHandler(fn)
	}
	sc.in.handlers = append(sc.in.handlers, fn)
	return sc.in.id + ":" + strconv.Itoa(len(sc.in.handlers)-1)
}

// maxPushes is how many pushed commands' handlers an instance keeps.
const maxPushes = 64

// pushHandler registers fn, the handler of a Push step in a command the
// instance pushed (ui.Live.PushJS), and returns its id: "c1:0p", its
// number among the instance's pushes. Unlike its render's handlers, which
// each render numbers afresh, it stays until maxPushes later pushes have
// replaced it, so that a render between the push and the event it sends —
// the join's after an HTTP render, another component's event, a message —
// leaves it working, and its id never names another handler.
func (in *instance) pushHandler(fn any) string {
	if in.pushes == nil {
		in.pushes = map[string]any{}
	}
	n := strconv.Itoa(in.pushSeq) + "p"
	in.pushSeq++
	in.pushes[n] = fn
	in.pushIDs = append(in.pushIDs, n)
	if len(in.pushIDs) > maxPushes {
		delete(in.pushes, in.pushIDs[0])
		in.pushIDs = slices.Delete(in.pushIDs, 0, 1)
	}
	return in.id + ":" + n
}

// StateKey names a state the instance renders: its id, the state's site,
// and how many of the site's states it rendered before in this render.
func (sc *scope) StateKey(_ context.Context, site string) string {
	occ := "\x00state " + site
	sc.in.occ[occ]++
	return sc.in.id + ":" + site + ":" + strconv.Itoa(sc.in.occ[occ])
}

func (sc *scope) Keyed(ctx context.Context, key any) context.Context {
	return ui.WithLiveHost(ctx, &scope{sc.in, sc.keys + keyText(key) + "/", sc.push})
}

func keyText(k any) string { return ui.KeyText(k) }

// Component renders a stateful component's tag: the instance at its place, or
// a new one, mounted, with the tag's props; its HTML is a marker the
// session replaces.
func (sc *scope) Component(ctx context.Context, n *ui.ComponentNode, w io.Writer) error {
	in, err := sc.place(n)
	if in == nil || err != nil {
		return err
	}
	_, err = io.WriteString(w, marker(in.id))
	return err
}

// ComponentRef renders a stateful component's tag in a render tree: its
// instance's id.
func (sc *scope) ComponentRef(ctx context.Context, n *ui.ComponentNode) (string, error) {
	in, err := sc.place(n)
	if in == nil || err != nil {
		return "", err
	}
	return in.id, nil
}

// place finds (or mounts) the instance at a tag's place, sets its props and
// renders it, unless Assign tracking says its output can't have changed.
func (sc *scope) place(n *ui.ComponentNode) (*instance, error) {
	if n.Props == nil {
		return nil, nil
	}
	s, parent := sc.in.s, sc.in
	place := sc.keys + n.Site + "#"
	if n.Key != nil {
		place += "k" + keyText(n.Key)
	} else {
		parent.occ[place]++
		place += strconv.Itoa(parent.occ[place])
	}
	typ := reflect.TypeOf(n.Props)
	in := parent.children[place]
	msg, upd := s.updates[n.Props]
	if upd {
		delete(s.updates, n.Props)
		if in != nil && in.typ == typ && in.seen != s.pass && !takesUpdate(in.comp, msg) {
			// a navigation keeps an instance only for an Update to take
			// the new URL; this one mounts again, for Mount to see it
			unmount(in)
			delete(s.byID, in.id)
			in = nil
		}
	}
	same := false
	switch {
	case in != nil && in.seen == s.pass && in.typ == typ:
		return nil, fmt.Errorf("live: two <%s> in one place share the key %v", typ.Elem().Name(), n.Key)
	case in == nil || in.typ != typ:
		s.next++
		if err := ui.CheckStateful(n.Props); err != nil {
			return nil, err
		}
		in = &instance{s: s, id: "c" + strconv.Itoa(s.next), typ: typ, comp: ui.CopyLive(n.Props), children: map[string]*instance{}, fresh: true}
		ui.AttachLive(in.comp, in)
		s.fillURL(in, n)
		if err := mount(s.ctx, in.comp); err != nil {
			return nil, err
		}
		parent.children[place] = in
		s.byID[in.id] = in
	case n.Routed:
		// A routed page: its exported fields are its own, set from the URL
		// at mount. Only a new tag, a patch's, sets the fields its URL
		// binds; the patch's Update takes the new URL too. A page backed
		// by its island always renders: what it sends its component
		// includes more than its fields.
		same = n.Props == in.src && sameProps(in.comp, in.shown) && !ui.IsIsland(typ)
		if n.Props != in.src && !s.adopting {
			setFields(in.comp, n.Props, n.URL)
		}
		if upd {
			_, err := callUpdate(s.ctx, in.comp, msg)
			s.uerr = errors.Join(s.uerr, err)
			same = false
		}
	case ui.IsIsland(typ):
		// A page backed by its island: its exported fields are its own —
		// the props it sends its component — set from the URL at mount, not
		// by the page's render. A patch's Update takes the new URL.
		if upd {
			_, err := callUpdate(s.ctx, in.comp, msg)
			s.uerr = errors.Join(s.uerr, err)
		}
	default:
		same = sameProps(in.comp, n.Props)
		copyProps(in.comp, n.Props)
		if upd {
			_, err := callUpdate(s.ctx, in.comp, msg)
			s.uerr = errors.Join(s.uerr, err)
			same = false
		}
	}
	if !in.fresh && s.fillURL(in, n) {
		same = false
	}
	in.key, in.src, in.routed = n.Key, n.Props, n.Routed
	if in.routed {
		in.shown = ui.CopyLive(in.comp)
	}
	parent.kids = append(parent.kids, in)
	return in, s.visit(in, same)
}

func marker(id string) string { return "\x00vk:" + id + "\x00" }

// copyProps sets the exported fields of to from from: a parent's render sets
// every prop again, but a form (ui.IsFormField), which is the instance's
// own: what the visitor typed in it, its errors.
func copyProps(to, from ui.Stateful) {
	dst, src := reflect.ValueOf(to).Elem(), reflect.ValueOf(from).Elem()
	live := reflect.TypeFor[ui.Live]()
	for i := 0; i < dst.NumField(); i++ {
		if f := dst.Type().Field(i); f.IsExported() && f.Type != live && !ui.IsFormField(f.Type) {
			dst.Field(i).Set(src.Field(i))
		}
	}
}

// setFields sets the fields of to named names from from.
func setFields(to, from ui.Stateful, names []string) {
	dst, src := reflect.ValueOf(to).Elem(), reflect.ValueOf(from).Elem()
	for _, n := range names {
		if f := dst.FieldByName(n); f.CanSet() {
			f.Set(src.FieldByName(n))
		}
	}
}

func (s *Session) renderAll() error {
	if s.closed {
		return errors.New("live: the session is closed")
	}
	s.pass++
	if err := s.renderInst(s.root); err != nil {
		return err
	}
	for id, in := range s.byID {
		if in.seen != s.pass {
			unmount(in)
			delete(s.byID, id)
		}
	}
	for _, in := range s.byID {
		for place, c := range in.children {
			if c.seen != s.pass {
				delete(in.children, place)
			}
		}
	}
	return nil
}

// visit renders an instance, or keeps its last render when its state is all
// Assigns, none changed, and its props are the same (same).
func (s *Session) visit(in *instance, same bool) error {
	set := in.set
	in.set = false
	if same && !in.fresh && !set && !in.contextual && !s.varsMoved(in) {
		if tracked, changed := ui.TrackedState(in.comp); tracked && !changed {
			if s.verify {
				return s.verifySkip(in)
			}
			return s.keep(in)
		}
	}
	return s.renderInst(in)
}

// varsMoved reports whether a template variable the instance's last render
// read has another version now: what the variable's processor read changed
// (the URL, the signed-in identity, its permissions), and so may what the
// render showed.
func (s *Session) varsMoved(in *instance) bool {
	if len(in.vars) == 0 {
		return false
	}
	src := ui.VarSourceOf(s.ctx)
	if src == nil {
		return true
	}
	for name, ver := range in.vars {
		if src.VarVersion(s.ctx, name) != ver {
			return true
		}
	}
	return false
}

// keep keeps an instance's last render and visits the instances it rendered,
// whose props it would have set the same.
func (s *Session) keep(in *instance) error {
	in.seen, in.rendered, in.changed = s.pass, false, false
	for _, k := range in.kids {
		if err := s.visit(k, true); err != nil {
			return err
		}
	}
	return nil
}

// verifySkip renders an instance keep would have kept, and fails if its
// output differs from the last.
func (s *Session) verifySkip(in *instance) error {
	own, tree := in.own, in.tree
	if err := s.renderInst(in); err != nil {
		return err
	}
	if s.trees && !sameTree(tree, in.tree) || !s.trees && own != in.own {
		return fmt.Errorf("live: %s (%s) renders differently though its Assigns and props didn't change: does Render read a field that isn't an Assign or a prop? (VUKA_LIVE_VERIFY)", in.typ.Elem().Name(), in.id)
	}
	return nil
}

func (s *Session) renderInst(in *instance) (err error) {
	defer ui.Catch(&err) // a (T, error) call in its markup failing
	in.seen, in.rendered, in.idn = s.pass, true, 0
	in.handlers, in.kids, in.occ = in.handlers[:0], in.kids[:0], map[string]int{}
	clear(in.forms)
	ctx := ui.WithLiveHost(s.ctx, &scope{in: in})
	in.contextual = false
	clear(in.vars)
	var node ui.Node
	if in.comp == nil {
		// An adopted page's new tags are its URL's again: what its hooks set
		// from the URL stays.
		defer func() { s.adopting = false }()
		node = s.render(ctx)
	} else {
		node = rooted(ui.ResolveContext(ctx, in.comp.Render()), in.id, in.key)
	}
	if in.comp != nil {
		defer ui.SettleState(in.comp)
	}
	if s.trees && !s.whole {
		t, err := ui.BuildTree(ctx, node, s.verify)
		if err != nil {
			return err
		}
		s.stableTreeIDs(in, t)
		in.tree = splitMarkers(t)
		return nil
	}
	var b strings.Builder
	if node != nil {
		if err := node.Render(ctx, &b); err != nil {
			return err
		}
	}
	own := s.stableIDs(in, b.String())
	if s.trees {
		in.tree = splitMarkers(ui.OpaqueTree(own))
		return nil
	}
	in.changed = in.fresh || own != in.own
	in.own = own
	return nil
}

// rooted is a component's output with its id, and its tag's key, on the root
// element, or on a <vk-c> around it when it isn't one element.
func rooted(n ui.Node, id string, key any) ui.Node {
	attrs := []ui.Attr{{Name: "data-vk-id", Value: id}}
	if key != nil {
		attrs = append(attrs, ui.Attr{Name: "data-vk-key", Value: fmt.Sprint(key)})
	}
	if r, ok := ui.RootAttrs(n, attrs...); ok {
		return r
	}
	fp, shape := vkcFP, "Eds(h)"
	if key != nil {
		fp, shape = vkcKeyedFP, "Edds(h)"
	}
	return ui.F(fp, shape, ui.El("vk-c", append(attrs, ui.Attr{Name: "style", Value: "display:contents"}), n))
}

// The fingerprints of the <vk-c> frames rooted puts around a component's
// output: a frame like JSX's, so what it holds (a templ component, a
// fragment) is a frame of the tree too.
const (
	vkcFP      uint64 = 0x766b2d6300000001
	vkcKeyedFP uint64 = 0x766b2d6300000002
)

// html is an instance's HTML with its children's in place of their markers.
func (in *instance) html() string {
	if !strings.Contains(in.own, "\x00vk:") {
		return in.own
	}
	var b strings.Builder
	rest := in.own
	for {
		i := strings.Index(rest, "\x00vk:")
		if i < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:i])
		rest = rest[i+4:]
		j := strings.IndexByte(rest, 0)
		if j < 0 {
			b.WriteString(rest)
			return b.String()
		}
		if c := in.s.byID[rest[:j]]; c != nil && c != in {
			b.WriteString(c.html())
		}
		rest = rest[j+1:]
	}
}

// patch renders the page again and collects the outermost changed instances.
func (s *Session) patch() (Patch, error) {
	if err := s.renderAll(); err != nil {
		return Patch{}, err
	}
	js, err := s.pushed()
	if err != nil {
		return Patch{}, err
	}
	p := Patch{JS: js}
	if s.trees {
		p.Trees = s.treeUpdates()
		s.settle()
		return p, nil
	}
	var walk func(in *instance)
	walk = func(in *instance) {
		if in.changed {
			p.Updates = append(p.Updates, Update{in.id, in.html()})
			return
		}
		for _, c := range in.kids {
			walk(c)
		}
	}
	walk(s.root)
	s.settle()
	return p, nil
}

func (s *Session) settle() {
	for _, in := range s.byID {
		in.changed, in.fresh, in.rendered = false, false, false
	}
}

// order is the instances in render order, the page first.
func (s *Session) order() []*instance {
	var out []*instance
	var walk func(in *instance)
	walk = func(in *instance) {
		out = append(out, in)
		for _, c := range in.kids {
			walk(c)
		}
	}
	walk(s.root)
	return out
}

func unmount(in *instance) {
	if u, ok := in.comp.(interface{ Unmount() }); ok {
		u.Unmount()
	}
}
