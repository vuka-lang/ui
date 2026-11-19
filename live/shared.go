package live

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/vuka-lang/ui/v2"
)

// A shared fragment is a component that renders once for every page that
// shows it. A component is one by embedding ui.SharedLive in place of
// ui.Live, and a tag of it in a page, which takes no props, is a placeholder: the page's tree holds a
// reference to the fragment's root instance, as it does for a child
// component, so the page's own diff never looks inside. The fragment lives
// in a Session of its own, one per component type in a SharedSet, mounted
// with a context that holds nothing of a viewer. A
// Publish renders it once for a batch of messages, diffs it against its one
// shadow and encodes the change once; the frame is the same bytes for every
// page, so a transport sends one slice to each. A page that missed a frame,
// or joined between two, gets a snapshot instead (SharedFrame, Shared.Do).
//
// A fragment answers no events: its state is every viewer's, so a handler,
// a form or a pushed command in it is a render error, and an event naming
// one of its instances is refused (SharedEventError). Per-viewer interaction
// is browser-side state, or a per-page component that passes the fragment
// props; a change for everyone is a broadcast.

// SharedStaticsBase is where the process-wide statics ids of shared
// fragments start: far above the per-connection ids a page's frames number
// from zero.
const SharedStaticsBase = 1 << 30

type staticTable struct {
	mu  sync.RWMutex
	ids map[uint64]int
}

var sharedStatics = staticTable{ids: map[uint64]int{}}

// intern is the id of t's statics, new when none has them yet.
func (t *staticTable) intern(tr *ui.Tree) int {
	t.mu.RLock()
	id, ok := t.ids[tr.Key]
	t.mu.RUnlock()
	if ok {
		return id
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if id, ok = t.ids[tr.Key]; !ok {
		id = SharedStaticsBase + len(t.ids)
		t.ids[tr.Key] = id
	}
	return id
}

// SharedLinks is where a page attaches to the fragments it shows: the
// transport implements it for its connection. Attach registers the page for
// the fragment's frames and returns the fragment's trees as of the moment, to
// go with the message carrying the page's reference; Detach is called before
// the message that drops the reference.
type SharedLinks interface {
	Attach(sh *Shared) []TreeUpdate
	Detach(sh *Shared)
}

// SharedEventError is the error of an event aimed at a shared fragment.
type SharedEventError struct{ Target string }

func (e *SharedEventError) Error() string {
	return fmt.Sprintf("live: no handler %q: it belongs to a shared fragment, which answers no events", e.Target)
}

func (e *SharedEventError) PublicMessage() string {
	return fmt.Sprintf("live: no handler %q", e.Target)
}

// isSharedID reports whether id is a shared fragment's instance's ("s1.c3").
func isSharedID(id string) bool {
	n, rest, ok := strings.Cut(id, ".")
	if !ok || len(n) < 2 || n[0] != 's' || rest == "" {
		return false
	}
	_, err := strconv.Atoi(n[1:])
	return err == nil
}

// SharedSet is the shared fragments of an app, one per component type.
type SharedSet struct {
	ctx    func(name string) context.Context
	strict bool
	mu     sync.Mutex
	by     map[reflect.Type]*sharedEntry
	n      int
}

type sharedEntry struct {
	once sync.Once
	id   int
	sh   *Shared
	err  error
}

// NewSharedSet is an empty set. ctx is the context a fragment renders with,
// which must hold nothing of a viewer; with strict, a read of one
// (ui.ViewerRead) fails the render.
func NewSharedSet(ctx func(name string) context.Context, strict bool) *SharedSet {
	return &SharedSet{ctx: ctx, strict: strict, by: map[reflect.Type]*sharedEntry{}}
}

// Fragments are the mounted fragments.
func (ss *SharedSet) Fragments() []*Shared {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	var out []*Shared
	for _, e := range ss.by {
		if e.sh != nil {
			out = append(out, e.sh)
		}
	}
	return out
}

// fragment is the fragment of props' type, mounted when it is the first tag.
func (ss *SharedSet) fragment(props ui.Stateful) (*Shared, error) {
	typ := reflect.TypeOf(props)
	if !sameProps(props, reflect.New(typ.Elem()).Interface().(ui.Stateful)) {
		return nil, fmt.Errorf("live: <%s> is shared, rendered once for every page, so its tag takes no props: pass per-viewer values to a per-page component that wraps it", typeName(typ))
	}
	return ss.ForType(typ)
}

// ForType is the fragment of the shared component type typ, mounted now when
// no page or broadcast has yet.
func (ss *SharedSet) ForType(typ reflect.Type) (*Shared, error) {
	name := typeName(typ)
	ui.RegisterShared(reflect.New(typ.Elem()).Interface().(ui.Stateful))
	ss.mu.Lock()
	e := ss.by[typ]
	if e == nil {
		ss.n++
		e = &sharedEntry{id: ss.n}
		ss.by[typ] = e
	}
	ss.mu.Unlock()
	e.once.Do(func() { e.sh, e.err = ss.mount(name, e.id, reflect.New(typ.Elem()).Interface().(ui.Stateful)) })
	if e.err != nil {
		ss.mu.Lock()
		if ss.by[typ] == e {
			delete(ss.by, typ)
		}
		ss.mu.Unlock()
		return nil, e.err
	}
	return e.sh, nil
}

func (ss *SharedSet) mount(name string, id int, props ui.Stateful) (*Shared, error) {
	sh := &Shared{name: name, set: ss, props: props, typ: reflect.TypeOf(props)}
	ctx := ui.WithSharedFragment(ss.ctx(name), name, ss.strict)
	s := newSession(ctx, func(context.Context) ui.Node { return ui.Component("", nil, props) }, "s"+strconv.Itoa(id)+".")
	s.frag, s.c.global = sh, true
	sh.s = s
	if _, err := s.RenderTrees(); err != nil {
		s.Close()
		return nil, err
	}
	if len(s.root.kids) != 1 {
		s.Close()
		return nil, fmt.Errorf("live: the shared <%s> renders no component", name)
	}
	s.root.kids[0].topics = []string{sharedTopic}
	sh.rootID = s.root.kids[0].id
	sh.ver = 1
	return sh, nil
}

// Shared is one fragment: the instance a page's tag stands for, rendered once
// for every page.
type Shared struct {
	name   string
	set    *SharedSet
	props  ui.Stateful
	rootID string

	typ reflect.Type

	mu   sync.Mutex
	s    *Session
	ver  uint64
	snap *SharedSnapshot
}

// Name is the fragment's name.
func (sh *Shared) Name() string { return sh.name }

// RootID is the id of the fragment's root instance, which a page's tree refers to.
func (sh *Shared) RootID() string { return sh.rootID }

// Type is the shared component's type.
func (sh *Shared) Type() reflect.Type { return sh.typ }

// sharedTopic is the one topic a fragment's root component follows, so that
// Deliveries made for it reach that instance and none nested in it.
const sharedTopic = "\x00shared"

// Message is a delivery to the fragment's component.
func Message(msg any) Delivery { return Delivery{Topic: sharedTopic, Msg: msg} }

// HTML is the fragment's markup as last rendered.
func (sh *Shared) HTML() string { return sh.s.HTML() }

// SharedSnapshot is the fragment whole at a version: each instance's tree
// with its statics inline, the entries a page's message carries, and the
// same as a message of its own.
type SharedSnapshot struct {
	Ver   uint64
	Trees []TreeUpdate
	Msg   []byte
	Uses  []int
}

// SharedFrame is what a publish changed, from version Base to Ver: Msg is
// the whole patch message, the same for every page, and Uses the statics ids
// it needs the browser to hold. A page not at Base takes Snapshot.
type SharedFrame struct {
	Base, Ver uint64
	Msg       []byte
	Uses      []int
	snapshot  func() *SharedSnapshot
}

// Snapshot is the fragment as of Ver; call it only from the fan function of Publish.
func (f SharedFrame) Snapshot() *SharedSnapshot { return f.snapshot() }

// Do runs f with the fragment locked: no publish renders or fans out while it
// runs, so a page it registers misses no frame between the snapshot and its
// place among those fanned out. snapshot is the fragment as of now.
func (sh *Shared) Do(f func(snapshot func() *SharedSnapshot)) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	f(sh.snapshotLocked)
}

// Publish delivers ds to the fragment's instances in order, renders it once
// and calls fan, locked, with the frame if anything changed.
func (sh *Shared) Publish(ds []Delivery, fan func(SharedFrame)) (err error) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			if e, ok := ui.Failed(r); ok {
				err = e
			} else {
				err = fmt.Errorf("live: the shared <%s>: %v", sh.name, r)
			}
		}
	}()
	sh.s.c.uses = sh.s.c.uses[:0]
	p, ierr := sh.s.InfoBatch(ds)
	if ierr != nil {
		return ierr
	}
	if p.Error != "" {
		err = errors.New(p.Error)
	}
	trees := slices.DeleteFunc(p.Trees, func(u TreeUpdate) bool { return u.ID == sh.s.root.id })
	if len(trees) == 0 {
		return err
	}
	uses := slices.Clone(sh.s.c.uses)
	slices.Sort(uses)
	uses = slices.Compact(uses)
	base := sh.ver
	sh.ver++
	msg := Marshal(ServerMessage{Type: PatchMsg, Trees: trees})
	fan(SharedFrame{Base: base, Ver: sh.ver, Msg: msg, Uses: uses, snapshot: sh.snapshotLocked})
	return err
}

func (sh *Shared) snapshotLocked() *SharedSnapshot {
	if sh.snap != nil && sh.snap.Ver == sh.ver {
		return sh.snap
	}
	c := conn{global: true}
	e := enc{c: &c, inline: true}
	snap := &SharedSnapshot{Ver: sh.ver}
	sh.s.mu.Lock()
	for _, in := range sh.s.order() {
		if in == sh.s.root || in.tree == nil {
			continue
		}
		e.b = nil
		e.frame(in.tree)
		snap.Trees = append(snap.Trees, TreeUpdate{ID: in.id, Full: true, Tree: e.b})
	}
	sh.s.mu.Unlock()
	slices.Sort(c.uses)
	snap.Uses = slices.Compact(c.uses)
	snap.Msg = Marshal(ServerMessage{Type: PatchMsg, Trees: snap.Trees})
	sh.snap = snap
	return snap
}

// checkFragment fails a fragment's render that did what a fragment may not.
func (s *Session) checkFragment() error {
	if err := s.fragErr; err != nil {
		s.fragErr = nil
		return err
	}
	for _, in := range s.order() {
		if in.comp == nil {
			continue
		}
		if ui.HasStreams(in.comp) || in.temps {
			return fmt.Errorf("live: %s in the shared <%s> has a stream or a temporary region, which are per page", typeName(in.typ), s.frag.name)
		}
	}
	return nil
}

func (s *Session) refuseShared(what string) {
	if s.fragErr == nil {
		s.fragErr = fmt.Errorf("live: the shared <%s> uses %s, which a shared component may not: its state is every viewer's, so it answers no events and holds nothing per viewer; its topics are the topic tag of ui.SharedLive, per-viewer interaction belongs in browser-side state or in a per-page component, and a change for everyone is a broadcast", s.frag.name, what)
	}
}

// SetShared sets the fragments the page may show and, for a page with a
// connection, the links to attach through. Without links the page renders
// fragments as their markup of the moment.
func (s *Session) SetShared(set *SharedSet, links SharedLinks) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shared, s.links = set, links
}

// sharedFragment is the fragment n's tag stands for, noted as shown by the
// instance rendering.
func (sc *scope) sharedFragment(n *ui.ComponentNode) (*Shared, error) {
	sc.in.effects++
	s := sc.in.s
	name := typeName(reflect.TypeOf(n.Props))
	if s.frag != nil {
		return nil, fmt.Errorf("live: the shared <%s> shows the shared <%s>: shared components don't nest", s.frag.name, name)
	}
	if s.shared == nil {
		return nil, fmt.Errorf("live: <%s> is shared, and this session has no shared set (SetShared)", name)
	}
	sh, err := s.shared.fragment(n.Props)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(sc.in.shared, sh) {
		sc.in.shared = append(sc.in.shared, sh)
	}
	return sh, nil
}

// syncShared attaches the page to the fragments it now shows and detaches it
// from those it stopped showing; the snapshots of the new ones go with the
// message carrying the page's references.
func (s *Session) syncShared() ([]TreeUpdate, error) {
	if s.frag != nil {
		return nil, nil
	}
	var use []*Shared
	for _, in := range s.order() {
		for _, sh := range in.shared {
			if !slices.Contains(use, sh) {
				use = append(use, sh)
			}
		}
	}
	for _, sh := range slices.Clone(s.attached) {
		if !slices.Contains(use, sh) && s.links != nil {
			s.links.Detach(sh)
			s.attached = slices.DeleteFunc(s.attached, func(x *Shared) bool { return x == sh })
		}
	}
	var out []TreeUpdate
	for _, sh := range use {
		if slices.Contains(s.attached, sh) {
			continue
		}
		if s.links == nil {
			return nil, fmt.Errorf("live: the page shows the shared <%s>, and its connection cannot attach to it", sh.name)
		}
		out = append(out, s.links.Attach(sh)...)
		s.attached = append(s.attached, sh)
	}
	return out, nil
}

// detachAll detaches the page from every fragment.
func (s *Session) detachAll() {
	if s.links != nil {
		for _, sh := range s.attached {
			s.links.Detach(sh)
		}
	}
	s.attached = nil
}

// sharedTag reports whether n is a tag of a shared component that stands for
// its fragment: every one but the root of the fragment's own session, which
// mounts it.
func (sc *scope) sharedTag(n *ui.ComponentNode) bool {
	return ui.IsShared(n.Props) && (sc.in.s.frag == nil || sc.in != sc.in.s.root)
}
