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
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/vuka-lang/ui"
)

// Session is one connected page: its render function and the instances of the
// stateful components it renders. Its methods are safe to call from several
// goroutines; each runs alone.
type Session struct {
	mu     sync.Mutex
	ctx    context.Context
	render func(ctx context.Context) ui.Node
	root   *instance
	byID   map[string]*instance
	next   int
	pass   int
	closed bool
	trees  bool // a v2 connection: instances render to trees
	c      conn
	verify bool
	whole  bool     // each instance one dynamic of its HTML: wholeTrees's
	js     []JSPush // pushed by Render or RenderTrees since JS last drained them, for the join
	router Router

	updates map[ui.Stateful]any // a navigation's messages for kept instances, by their tags' props
	uerr    error               // what their Update methods returned

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
	if render != nil {
		s.render = render
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
		return Patch{}, publicErr{fmt.Errorf("live: no handler %q", ev.Target)}
	}
	herr := callHandler(s.ctx, fn, ev)
	p, err := s.patch()
	if herr != nil {
		p.Error = s.errorText(herr)
	}
	return p, err
}

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
// ("c1:0"), or a method of its component by name ("c1:Inc", what the
// component's scripts push) when AllowPush allows it.
func (s *Session) handler(target string) any {
	id, n, ok := strings.Cut(target, ":")
	in := s.byID[id]
	if !ok || in == nil {
		return nil
	}
	h, err := strconv.Atoi(n)
	if err != nil {
		return pushable(in.comp, n)
	}
	if h < 0 || h >= len(in.handlers) {
		return nil
	}
	return in.handlers[h]
}

// AllowPush lets a component's scripts push its methods by name: typ is
// the component's struct type as reflect spells it ("main.Counter",
// "example.com/app/views.Chart"), methods the names its scripts push,
// replacing any allowed before (none: no pushes by name). The vuka command
// lists them (each ctx.push("Name") of the scripts) and web registers them
// from its manifest. A component nothing allows accepts no pushes by name;
// the handlers its page renders work regardless. Render, Mount, an Update
// and methods promoted from what it embeds are never pushed.
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
	if c == nil || name == "Render" || name == "Mount" || ui.IsHook(c, name) {
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
	for _, in := range s.order() {
		if in.comp == nil || topic != "" && !slices.Contains(in.topics, topic) {
			continue
		}
		ok, err := callUpdate(s.ctx, in.comp, msg)
		called = called || ok
		herr = errors.Join(herr, err)
	}
	if !called {
		return Patch{}, herr
	}
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
	if !patch || len(rt.Updates) == 0 {
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
	topics   []string
	js       []ui.Commands // pushed since the last reply
	seen     int
	own      string // its HTML, each child as a marker
	changed  bool
	fresh    bool
	key      any // its tag's key, written on its root as data-vk-key

	tree     *ui.Tree // its last render, on a v2 connection
	shadow   *sframe  // what the client holds of it; nil: nothing yet
	rendered bool     // rendered this pass (not skipped)
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
			ops, err := ui.EncodeJS(ui.WithLiveHost(s.ctx, &scope{in: in}), c)
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
}

func (sc *scope) Handler(_ context.Context, fn any) string {
	sc.in.handlers = append(sc.in.handlers, fn)
	return sc.in.id + ":" + strconv.Itoa(len(sc.in.handlers)-1)
}

// StateKey names a state the instance renders: its id, the state's site,
// and how many of the site's states it rendered before in this render.
func (sc *scope) StateKey(_ context.Context, site string) string {
	occ := "\x00state " + site
	sc.in.occ[occ]++
	return sc.in.id + ":" + site + ":" + strconv.Itoa(sc.in.occ[occ])
}

func (sc *scope) Keyed(ctx context.Context, key any) context.Context {
	return ui.WithLiveHost(ctx, &scope{sc.in, sc.keys + keyText(key) + "/"})
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
		if err := mount(s.ctx, in.comp); err != nil {
			return nil, err
		}
		parent.children[place] = in
		s.byID[in.id] = in
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
	in.key = n.Key
	parent.kids = append(parent.kids, in)
	return in, s.visit(in, same)
}

func marker(id string) string { return "\x00vk:" + id + "\x00" }

// copyProps sets the exported fields of to from from: a parent's render sets
// every prop again.
func copyProps(to, from ui.Stateful) {
	dst, src := reflect.ValueOf(to).Elem(), reflect.ValueOf(from).Elem()
	live := reflect.TypeFor[ui.Live]()
	for i := 0; i < dst.NumField(); i++ {
		if f := dst.Type().Field(i); f.IsExported() && f.Type != live {
			dst.Field(i).Set(src.Field(i))
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
	if same && !in.fresh {
		if tracked, changed := ui.TrackedState(in.comp); tracked && !changed {
			if s.verify {
				return s.verifySkip(in)
			}
			return s.keep(in)
		}
	}
	return s.renderInst(in)
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

func (s *Session) renderInst(in *instance) error {
	in.seen, in.rendered = s.pass, true
	in.handlers, in.kids, in.occ = in.handlers[:0], in.kids[:0], map[string]int{}
	ctx := ui.WithLiveHost(s.ctx, &scope{in: in})
	var node ui.Node
	if in.comp == nil {
		node = s.render(ctx)
	} else {
		node = rooted(in.comp.Render(), in.id, in.key)
	}
	if in.comp != nil {
		defer ui.SettleState(in.comp)
	}
	if s.trees && !s.whole {
		t, err := ui.BuildTree(ctx, node, s.verify)
		if err != nil {
			return err
		}
		in.tree = splitMarkers(t)
		return nil
	}
	var b strings.Builder
	if node != nil {
		if err := node.Render(ctx, &b); err != nil {
			return err
		}
	}
	own := b.String()
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
