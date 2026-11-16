package live

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/vuka-lang/ui/v2"
)

// An instance keeps what each of its guarded parts (ui.Guard) built at its
// last render, with the handlers it named, and gives it back for the next
// render when the part's deps are the same, naming the same handlers again
// without running the part. Only a part that did nothing else can be kept:
// one rendering a stateful component, a form, a lazy region, a state, a
// stream, a Temporary region, a Call, a library's random id, or reading the
// context or a template variable renders every time. So does one where its
// handlers would be named otherwise: inside a key, after another number of
// handlers, or under a key another part of the render names too.

// kept is what a guarded part built.
type kept struct {
	render   int   // the instance's render it was built or kept in
	deps     []any // its deps then
	v        any   // its value in the tree
	start    int   // the handlers named before it
	fns      []any // the handlers it named outside a key
	keyed    []keyedRec
	pageKeys int
}

// keyedRec is a handler named under a key: its name, after the instance's
// id, and the key path it was named under.
type keyedRec struct {
	name, path string
	fn         any
	dup        bool // named under a duplicate key (keyDups)
}

// Guarded is a guarded part's value in the instance's tree: the one it built
// at the instance's last render when its deps are the same and it can stand,
// else render's.
func (sc *scope) Guarded(_ context.Context, site uint64, deps []any, render func() (any, error)) (any, error) {
	in := sc.in
	if in.guardN == nil {
		in.guardN = map[uint64]int{}
	}
	in.guardN[site]++
	k := in.guards[site]
	if in.guardN[site] > 1 {
		// met twice in one render: kept by neither
		delete(in.guards, site)
		return render()
	}
	reuse := k != nil && sc.reusable(k, deps)
	if reuse && !in.s.verify {
		in.handlers = append(in.handlers, k.fns...)
		if len(k.keyed) > 0 && in.keyed == nil {
			in.keyed = map[string]any{}
		}
		for _, r := range k.keyed {
			in.keyed[r.name] = r.fn
			in.keyLog = append(in.keyLog, r)
		}
		k.render = in.renders
		in.kept = append(in.kept, k.v)
		return k.v, nil
	}
	start, keyed, effects := len(in.handlers), len(in.keyLog), in.effects
	v, err := render()
	if err != nil {
		return v, err
	}
	if reuse && !sameDyn(k.v, v) {
		return nil, fmt.Errorf("live: %s (%s) renders a guarded part differently though the fields it reads didn't change: does it read something else, a field written without Assign.Set? (VUKA_LIVE_VERIFY)", typeName(in.typ), in.id)
	}
	if in.effects != effects || sc.push || sc.keys != "" || !keepable(v) {
		delete(in.guards, site)
		return v, nil
	}
	nk := &kept{render: in.renders, deps: slices.Clone(deps), v: v, start: start, pageKeys: in.pageKeys}
	if n := len(in.handlers) - start; n > 0 {
		nk.fns = slices.Clone(in.handlers[start:])
	}
	if n := len(in.keyLog) - keyed; n > 0 {
		nk.keyed = slices.Clone(in.keyLog[keyed:])
		for _, r := range nk.keyed {
			if r.dup {
				delete(in.guards, site)
				return v, nil
			}
		}
	}
	if in.guards == nil {
		in.guards = map[uint64]*kept{}
	}
	in.guards[site] = nk
	return v, nil
}

// reusable reports whether k, kept at the instance's last render, stands for
// a render of its part now: the same deps, and its handlers named as they
// were.
func (sc *scope) reusable(k *kept, deps []any) bool {
	in := sc.in
	if k.render != in.renders-1 || in.s.redo || sc.push || sc.keys != "" || len(in.handlers) != k.start || in.pageKeys != k.pageKeys || !slices.Equal(k.deps, deps) {
		return false
	}
	for _, r := range k.keyed {
		if _, taken := in.keyed[r.name]; taken || in.dups.has(r.path) {
			return false
		}
	}
	return true
}

// reused reports whether d is the value of a part the render under way kept.
func (in *instance) reused(d any) bool {
	if len(in.kept) == 0 {
		return false
	}
	switch d.(type) {
	case *ui.Tree, *ui.TreeList:
		for _, v := range in.kept {
			if v == d {
				return true
			}
		}
	}
	return false
}

// keepable reports whether a tree value can be given back as it is: nothing
// in it is emptied once a reply is written (a stream's rows, a Temporary
// region), names another instance, or has a random id the session renames.
func keepable(v any) bool {
	switch v := v.(type) {
	case string:
		return nextRandomID(v, 0) < 0 && !strings.Contains(v, "\x00vk:")
	case *ui.Tree:
		for _, d := range v.Dyn {
			if !keepable(d) {
				return false
			}
		}
		return true
	case *ui.TreeList:
		if v.Stream || v.Temp != "" {
			return false
		}
		for _, it := range v.Items {
			if !keepable(it) {
				return false
			}
		}
		return true
	}
	return false
}

// sameDyn reports whether two tree values render alike.
func sameDyn(a, b any) bool { return sameTree(ui.OpaqueTree(a), ui.OpaqueTree(b)) }
