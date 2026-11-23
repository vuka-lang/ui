package live

import (
	"errors"
	"reflect"
	"slices"
	"strconv"

	"github.com/vuka-lang/ui/v2"
)

// RenderAhead renders the whole page as Render does, mounting the components
// it meets, for a navigation to take them later (NavigateFrom) instead of
// mounting them then. Nothing of the render is anyone's reply yet, so it
// keeps what a reply would carry with the instances themselves: the
// commands their Mounts pushed, and their streams' rows. A session rendered
// with Render or RenderTrees has handed those to its own client already
// (JS, a written stream batch), and instances taken from it arrive without
// them.
func (s *Session) RenderAhead() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trees = false
	s.refresh()
	err := s.renderAll()
	s.settle()
	return err
}

// NavigateFrom is Navigate, taking what ahead mounted instead of mounting it
// again: ahead is a session that rendered rt's page ahead of time
// (RenderAhead), and at each place the new page renders a component,
//
//  1. the instance s keeps there by Navigate's rules (rt.Keep, a patch's
//     Updates) stays, as it would: a persistent layout keeps its state and
//     subscriptions, whatever ahead holds in its place;
//  2. else ahead's instance at the same place moves into s, unless its type
//     isn't the tag's: it gets an id of s, takes its tag's props as a kept
//     instance does, and renders, but its Mount doesn't run again, nor an
//     Update for rt.Updates — it mounted with the new URL already. Its
//     state, topics, deferred groups (those loading, those not started,
//     those loading when seen) and the commands its Mount pushed go with it;
//  3. else it mounts afresh, as Navigate would mount it.
//
// The same place is the same path from the page down: each instance on it
// rendered by the same tag (the tag's site in the source and its key, or
// its occurrence among its parent's tags of that site without one) inside
// the same keyed elements, each of the same type, the page's own render
// at its top. An instance kept by step 1 stands for its ahead copy on that
// path, so a page inside a kept layout component still finds ahead's page
// inside ahead's copy of the layout. Anything else — another type, another
// key, a tag ahead rendered elsewhere — mounts afresh; NavigateFrom never
// takes an instance from another place. Components a Call opened in ahead
// are never taken.
//
// Ahead is closed when NavigateFrom returns, whether the navigation
// succeeded or not: what it still holds — its copies of the layouts step 1
// kept, instances no place took — is unmounted (Close), and the instances s
// took are s's, so ahead's Topics, Deferred and Loaded no longer see them.
// A deferred load ahead's Deferred started that moved is s's to finish: hand
// it to s.Loaded. Closing ahead again does nothing. A nil or closed ahead
// takes nothing: NavigateFrom is then Navigate.
//
// The reply is Navigate's for the same move: a taken instance is new to the
// client, as a freshly mounted one is, so it is sent the same way, against
// the statics the client already holds.
func (s *Session) NavigateFrom(rt Route, ahead *Session, patch bool) (Patch, error) {
	if ahead == s {
		return Patch{}, errors.New("live: a session can't navigate from itself")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ahead == nil {
		return s.navigateTo(rt, patch)
	}
	ahead.mu.Lock()
	defer ahead.mu.Unlock()
	defer ahead.close()
	if !ahead.closed && !s.closed {
		s.from = &aheadTake{s: ahead, live: s, at: map[*instance]aheadPlace{s.root: {ahead.root, ahead.root.children}}}
		defer func() { s.from = nil }()
	}
	return s.navigateTo(rt, patch)
}

// aheadTake is what NavigateFrom takes instances from: the ahead session,
// and for each instance of the live session's render, the ahead instance
// standing at its place and the places below it there.
type aheadTake struct {
	s, live *Session
	at      map[*instance]aheadPlace
}

// aheadPlace is the ahead side of one of the live session's instances: the
// ahead instance (the instance itself once it moved) and its children by
// place, as ahead rendered them.
type aheadPlace struct {
	in   *instance
	kids map[string]*instance
}

// mirror notes that in, an instance the live session kept at place under
// parent, stands for ahead's instance there, when ahead has one of its type.
func (f *aheadTake) mirror(parent *instance, place string, in *instance) {
	if _, ok := f.at[in]; ok {
		return // noted already, or moved: its places are noted
	}
	ap, ok := f.at[parent]
	if !ok {
		return
	}
	if m := ap.kids[place]; m != nil && m.typ == in.typ && f.held(m) {
		f.at[in] = aheadPlace{m, m.children}
	}
}

// held reports whether in is still ahead's own.
func (f *aheadTake) held(in *instance) bool { return in.s == f.s && f.s.byID[in.id] == in }

// take moves into the live session ahead's instance of type typ at place
// under parent, and returns it; nil when ahead has none there.
func (f *aheadTake) take(parent *instance, place string, typ reflect.Type) *instance {
	if f == nil {
		return nil
	}
	ap, ok := f.at[parent]
	if !ok {
		return nil
	}
	a := ap.kids[place]
	if a == nil || a.typ != typ || a.comp == nil || a.call != nil || !f.held(a) {
		return nil
	}
	f.move(a, ap, place)
	return a
}

// move makes a, ahead's instance at place under ap, the live session's: off
// ahead's render and its index, onto the live session's under a new id,
// with what it renders from its last render dropped — it renders whole
// next, as a new instance does — and what it holds of its own kept.
func (f *aheadTake) move(a *instance, ap aheadPlace, place string) {
	ahead, live := f.s, f.live
	delete(ahead.byID, a.id)
	delete(ap.kids, place)
	if ap.in.s == ahead {
		ap.in.kids = slices.DeleteFunc(ap.in.kids, func(k *instance) bool { return k == a })
	}
	old := a.id
	// commands ahead encoded for its own client name the old id: they are
	// ahead's, and go with it
	ahead.js = slices.DeleteFunc(ahead.js, func(j JSPush) bool { return j.ID == old })
	for d, in := range ahead.loading {
		if in == a {
			delete(ahead.loading, d)
			if live.loading == nil {
				live.loading = map[*ui.DeferredLoad]*instance{}
			}
			live.loading[d] = a
		}
	}
	a.dropTrees()
	a.guards = nil // its kept parts were nodes of the tree it dropped
	live.next++
	a.s, a.id = live, "c"+strconv.Itoa(live.next)
	f.at[a] = aheadPlace{a, a.children}
	a.children, a.kids = map[string]*instance{}, nil
	a.seen, a.fresh, a.changed, a.rendered, a.set = 0, true, false, false, false
	a.own = ""
	a.handlers, a.keyed, a.forms, a.claimed, a.acts = nil, nil, nil, nil, nil
	a.pushes, a.pushIDs, a.pushSeq = nil, nil, 0
	a.dups, a.pageKeys = keyDups{}, 0
	a.unwrapMethods()
}

// takeProps sets a moved instance's props from its tag n, as place sets a
// kept instance's (without the Update a kept one takes), and puts stand-ins
// for its method props bound to its new session.
func (in *instance) takeProps(n *ui.ComponentNode, adopting bool) {
	switch {
	case n.Routed:
		if n.Props != in.src && !adopting {
			setFields(in.comp, n.Props, n.URL)
		}
	case ui.IsIsland(in.typ):
	default:
		copyProps(in.comp, n.Props)
		ui.FillAutoIDs(in.comp, in.id)
	}
	in.trackMethods(n.Props)
}

// unwrapMethods puts back in its fields the method values the instance's
// stand-ins call (methodprops.go): a stand-in notes calls for the session
// it was made in, so a moved instance gets new ones.
func (in *instance) unwrapMethods() {
	if len(in.methods) == 0 {
		return
	}
	v := reflect.ValueOf(in.comp).Elem()
	for _, t := range in.methods {
		fn := t.fn.Load()
		if f := v.FieldByName(t.name); fn != nil && f.CanSet() {
			f.Set(*fn)
		}
	}
	in.methods = nil
}
