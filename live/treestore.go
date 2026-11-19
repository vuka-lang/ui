package live

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/vuka-lang/ui/v2"
)

// A page's render tree is stored once per process, by content, however many
// sessions render it. Sessions hold pointers into a table of trees and lists;
// a tree names its children by pointer, so identical subtrees are one, and a
// session's view of a page is mostly pointers to what other sessions hold.
// Rendering and diffing stay per page: a render builds its own tree, which is
// handed to the table whole, and what the table already has is used instead.
//
// A stored node is immutable. A render that changes a subtree builds new
// nodes along the way to it (emptyStreams, emptyTemps), and the session
// replaces its tree; nothing a session does reaches the nodes another session
// holds. What the client holds (the shadow) is the stored tree it was last
// sent, so it costs a pointer.
//
// Every node counts its holders in ui.Share.Refs: sessions (an instance's tree
// and shadow) and the stored nodes containing it. The count falls to zero when
// the last holder lets go, and the node leaves the table and lets go of its
// children. A node with no count is not stored (a render being built, a part
// a guard kept before it was stored, any node of a session that doesn't
// share), and the calls on it do nothing. The table is hashed by 64-bit
// content hash, sharded; a hash match is verified against the content, so a
// collision costs a second entry, never another page's markup.

// ShareTrees is whether new sessions store their render trees once per
// process. VUKA_LIVE_SHARE=off turns it off.
var ShareTrees = os.Getenv("VUKA_LIVE_SHARE") != "off"

const storeShards = 64

type storeShard struct {
	mu    sync.RWMutex
	trees map[uint64]*ui.Tree
	lists map[uint64]*ui.TreeList
	moreT map[uint64][]*ui.Tree // nodes whose hash another, different one has
	moreL map[uint64][]*ui.TreeList
}

type treeStore struct{ shards [storeShards]storeShard }

var treeNodes treeStore

// StoreStats is what the process-wide table holds.
type StoreStats struct {
	Trees, Lists int
	Refs         int64 // holders of all of them
}

// TreeStoreStats counts the nodes in the table and their holders.
func TreeStoreStats() StoreStats { return treeNodes.stats() }

func (st *treeStore) stats() StoreStats {
	var r StoreStats
	for i := range st.shards {
		sh := &st.shards[i]
		sh.mu.RLock()
		r.Trees += len(sh.trees)
		r.Lists += len(sh.lists)
		for _, t := range sh.trees {
			r.Refs += int64(atomic.LoadInt32(&t.Share.Refs))
		}
		for _, l := range sh.lists {
			r.Refs += int64(atomic.LoadInt32(&l.Share.Refs))
		}
		for _, ts := range sh.moreT {
			r.Trees += len(ts)
			for _, t := range ts {
				r.Refs += int64(atomic.LoadInt32(&t.Share.Refs))
			}
		}
		for _, ls := range sh.moreL {
			r.Lists += len(ls)
			for _, l := range ls {
				r.Refs += int64(atomic.LoadInt32(&l.Share.Refs))
			}
		}
		sh.mu.RUnlock()
	}
	return r
}

func (st *treeStore) shard(h uint64) *storeShard { return &st.shards[h%storeShards] }

func hmix(h, x uint64) uint64 {
	h ^= x + 0x9e3779b97f4a7c15 + h<<6 + h>>2
	h *= 0xff51afd7ed558ccd
	return h ^ h>>33
}

// stored reports whether a node is in the table (and so held, and immutable).
func stored(s *ui.Share) bool { return atomic.LoadInt32(&s.Refs) > 0 }

// corrupt is an accounting error: a node held after it was let go, or let go
// twice. It stops a verifying run; otherwise the node is treated as unstored.
func corrupt(what string) {
	if Verify {
		panic("live: stored tree " + what)
	}
}

// treeHash is the content hash of t, kept in t for the nodes it holds or was
// built from: its key (the statics), and each dynamic's.
func treeHash(t *ui.Tree) uint64 {
	if t.Share.Hash != 0 {
		return t.Share.Hash
	}
	h := hmix(hmix(0x7472, t.Key), uint64(len(t.Dyn)))
	for _, d := range t.Dyn {
		h = hmix(h, dynHash(d))
	}
	if h == 0 {
		h = 1
	}
	t.Share.Hash = h
	return h
}

func listHash(l *ui.TreeList) uint64 {
	if l.Share.Hash != 0 {
		return l.Share.Hash
	}
	h := hmix(0x6c69, uint64(len(l.Items)))
	if l.Stream {
		h = hmix(h, 1)
	}
	h = hmix(h, sig(l.Temp))
	if l.Keys == nil {
		h = hmix(h, 5)
	} else {
		h = hmix(h, 6+uint64(len(l.Keys)))
		for _, k := range l.Keys {
			h = hmix(h, sig(k))
		}
	}
	for _, it := range l.Items {
		h = hmix(h, treeHash(it))
	}
	if h == 0 {
		h = 1
	}
	l.Share.Hash = h
	return h
}

func dynHash(d any) uint64 {
	switch d := d.(type) {
	case string:
		return sig(d)
	case ui.TreeRef:
		return sig(string(d)) ^ 0x5265660000000000
	case *ui.Tree:
		if d != nil {
			return treeHash(d)
		}
	case *ui.TreeList:
		if d != nil {
			return listHash(d)
		}
	}
	return 3
}

func equalTree(c, t *ui.Tree) bool {
	if c == t {
		return true
	}
	if c.Key != t.Key || len(c.Dyn) != len(t.Dyn) || c.Share.Hash != t.Share.Hash {
		return false
	}
	for i, d := range c.Dyn {
		if !equalDyn(d, t.Dyn[i]) {
			return false
		}
	}
	return true
}

func equalList(c, l *ui.TreeList) bool {
	if c == l {
		return true
	}
	if c.Stream != l.Stream || c.Temp != l.Temp || len(c.Items) != len(l.Items) || (c.Keys == nil) != (l.Keys == nil) || len(c.Keys) != len(l.Keys) || c.Share.Hash != l.Share.Hash {
		return false
	}
	for i, k := range c.Keys {
		if k != l.Keys[i] {
			return false
		}
	}
	for i, it := range c.Items {
		o := l.Items[i]
		if it != o && stored(&it.Share) && stored(&o.Share) {
			return false
		}
		if !equalTree(it, o) {
			return false
		}
	}
	return true
}

// equalDyn compares two dynamics. Two different stored nodes differ: the table
// holds each content once.
func equalDyn(a, b any) bool {
	switch a := a.(type) {
	case string:
		b, ok := b.(string)
		return ok && a == b
	case ui.TreeRef:
		b, ok := b.(ui.TreeRef)
		return ok && a == b
	case *ui.Tree:
		b, ok := b.(*ui.Tree)
		if !ok || a == nil || b == nil {
			return ok && a == b
		}
		if a != b && stored(&a.Share) && stored(&b.Share) {
			return false
		}
		return equalTree(a, b)
	case *ui.TreeList:
		b, ok := b.(*ui.TreeList)
		if !ok || a == nil || b == nil {
			return ok && a == b
		}
		if a != b && stored(&a.Share) && stored(&b.Share) {
			return false
		}
		return equalList(a, b)
	case nil:
		return b == nil
	}
	return false
}

func (sh *storeShard) findTree(h uint64, t *ui.Tree) *ui.Tree {
	if c := sh.trees[h]; c != nil {
		if equalTree(c, t) {
			return c
		}
		for _, c := range sh.moreT[h] {
			if equalTree(c, t) {
				return c
			}
		}
	}
	return nil
}

func (sh *storeShard) findList(h uint64, l *ui.TreeList) *ui.TreeList {
	if c := sh.lists[h]; c != nil {
		if equalList(c, l) {
			return c
		}
		for _, c := range sh.moreL[h] {
			if equalList(c, l) {
				return c
			}
		}
	}
	return nil
}

func (sh *storeShard) addTree(h uint64, t *ui.Tree) {
	if sh.trees == nil {
		sh.trees = map[uint64]*ui.Tree{}
	}
	if sh.trees[h] == nil {
		sh.trees[h] = t
		return
	}
	if sh.moreT == nil {
		sh.moreT = map[uint64][]*ui.Tree{}
	}
	sh.moreT[h] = append(sh.moreT[h], t)
}

func (sh *storeShard) addList(h uint64, l *ui.TreeList) {
	if sh.lists == nil {
		sh.lists = map[uint64]*ui.TreeList{}
	}
	if sh.lists[h] == nil {
		sh.lists[h] = l
		return
	}
	if sh.moreL == nil {
		sh.moreL = map[uint64][]*ui.TreeList{}
	}
	sh.moreL[h] = append(sh.moreL[h], l)
}

func (sh *storeShard) removeTree(h uint64, t *ui.Tree) {
	if sh.trees[h] == t {
		delete(sh.trees, h)
		if more := sh.moreT[h]; len(more) > 0 {
			sh.trees[h] = more[0]
			if len(more) == 1 {
				delete(sh.moreT, h)
			} else {
				sh.moreT[h] = more[1:]
			}
		}
		return
	}
	more := sh.moreT[h]
	for i, c := range more {
		if c == t {
			more = append(more[:i:i], more[i+1:]...)
			if len(more) == 0 {
				delete(sh.moreT, h)
			} else {
				sh.moreT[h] = more
			}
			return
		}
	}
}

func (sh *storeShard) removeList(h uint64, l *ui.TreeList) {
	if sh.lists[h] == l {
		delete(sh.lists, h)
		if more := sh.moreL[h]; len(more) > 0 {
			sh.lists[h] = more[0]
			if len(more) == 1 {
				delete(sh.moreL, h)
			} else {
				sh.moreL[h] = more[1:]
			}
		}
		return
	}
	more := sh.moreL[h]
	for i, c := range more {
		if c == l {
			more = append(more[:i:i], more[i+1:]...)
			if len(more) == 0 {
				delete(sh.moreL, h)
			} else {
				sh.moreL[h] = more
			}
			return
		}
	}
}

// tree is the stored node with t's content, held once more for the caller. t
// is the caller's to give: its children are replaced by stored ones, or it
// becomes the stored node itself. A tree already stored is just held again.
func (st *treeStore) tree(t *ui.Tree) *ui.Tree {
	if t == nil {
		return nil
	}
	switch n := atomic.LoadInt32(&t.Share.Refs); {
	case n > 0:
		atomic.AddInt32(&t.Share.Refs, 1)
		return t
	case n < 0:
		corrupt("held after it was released")
		t.Share = ui.Share{}
	}
	h := treeHash(t)
	sh := st.shard(h)
	sh.mu.RLock()
	c := sh.findTree(h, t)
	if c != nil {
		atomic.AddInt32(&c.Share.Refs, 1)
	}
	sh.mu.RUnlock()
	if c != nil {
		return c
	}
	for i, d := range t.Dyn {
		switch d := d.(type) {
		case *ui.Tree:
			t.Dyn[i] = st.tree(d)
		case *ui.TreeList:
			t.Dyn[i] = st.list(d)
		}
	}
	sh.mu.Lock()
	if c = sh.findTree(h, t); c != nil {
		atomic.AddInt32(&c.Share.Refs, 1)
		sh.mu.Unlock()
		st.letGoKids(t)
		return c
	}
	atomic.StoreInt32(&t.Share.Refs, 1)
	sh.addTree(h, t)
	sh.mu.Unlock()
	return t
}

// list is tree for a list.
func (st *treeStore) list(l *ui.TreeList) *ui.TreeList {
	if l == nil {
		return nil
	}
	switch n := atomic.LoadInt32(&l.Share.Refs); {
	case n > 0:
		atomic.AddInt32(&l.Share.Refs, 1)
		return l
	case n < 0:
		corrupt("held after it was released")
		l.Share = ui.Share{}
	}
	h := listHash(l)
	sh := st.shard(h)
	sh.mu.RLock()
	c := sh.findList(h, l)
	if c != nil {
		atomic.AddInt32(&c.Share.Refs, 1)
	}
	sh.mu.RUnlock()
	if c != nil {
		return c
	}
	for i, it := range l.Items {
		l.Items[i] = st.tree(it)
	}
	sh.mu.Lock()
	if c = sh.findList(h, l); c != nil {
		atomic.AddInt32(&c.Share.Refs, 1)
		sh.mu.Unlock()
		st.letGoKids(l)
		return c
	}
	atomic.StoreInt32(&l.Share.Refs, 1)
	sh.addList(h, l)
	sh.mu.Unlock()
	return l
}

// lookupTree is the stored node with t's content, not held for the caller; nil
// when there is none or t has not been hashed. It is for a node something else
// already holds.
func (st *treeStore) lookupTree(t *ui.Tree) *ui.Tree {
	if t.Share.Hash == 0 {
		return nil
	}
	sh := st.shard(t.Share.Hash)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.findTree(t.Share.Hash, t)
}

func (st *treeStore) lookupList(l *ui.TreeList) *ui.TreeList {
	if l.Share.Hash == 0 {
		return nil
	}
	sh := st.shard(l.Share.Hash)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.findList(l.Share.Hash, l)
}

// retain holds a stored node once more; a node not stored is left alone.
func retainTree(t *ui.Tree) {
	if t == nil {
		return
	}
	switch n := atomic.LoadInt32(&t.Share.Refs); {
	case n > 0:
		atomic.AddInt32(&t.Share.Refs, 1)
	case n < 0:
		corrupt("held after it was released")
	}
}

// release lets go of a hold. The last one removes the node from the table and
// lets go of its children.
func (st *treeStore) release(t *ui.Tree) {
	if t == nil {
		return
	}
	for {
		n := atomic.LoadInt32(&t.Share.Refs)
		if n == 0 {
			return
		}
		if n < 0 {
			corrupt("released twice")
			return
		}
		if n > 1 {
			if atomic.CompareAndSwapInt32(&t.Share.Refs, n, n-1) {
				return
			}
			continue
		}
		break
	}
	h := t.Share.Hash
	sh := st.shard(h)
	sh.mu.Lock()
	if atomic.AddInt32(&t.Share.Refs, -1) > 0 {
		sh.mu.Unlock()
		return
	}
	sh.removeTree(h, t)
	atomic.StoreInt32(&t.Share.Refs, -1)
	sh.mu.Unlock()
	st.letGoKids(t)
}

func (st *treeStore) releaseList(l *ui.TreeList) {
	if l == nil {
		return
	}
	for {
		n := atomic.LoadInt32(&l.Share.Refs)
		if n == 0 {
			return
		}
		if n < 0 {
			corrupt("released twice")
			return
		}
		if n > 1 {
			if atomic.CompareAndSwapInt32(&l.Share.Refs, n, n-1) {
				return
			}
			continue
		}
		break
	}
	h := l.Share.Hash
	sh := st.shard(h)
	sh.mu.Lock()
	if atomic.AddInt32(&l.Share.Refs, -1) > 0 {
		sh.mu.Unlock()
		return
	}
	sh.removeList(h, l)
	atomic.StoreInt32(&l.Share.Refs, -1)
	sh.mu.Unlock()
	st.letGoKids(l)
}

// letGoKids lets go of the holds a node has on its children.
func (st *treeStore) letGoKids(n any) {
	switch n := n.(type) {
	case *ui.Tree:
		for _, d := range n.Dyn {
			switch d := d.(type) {
			case *ui.Tree:
				st.release(d)
			case *ui.TreeList:
				st.releaseList(d)
			}
		}
	case *ui.TreeList:
		for _, it := range n.Items {
			st.release(it)
		}
	}
}

// mustOwn stops a render from changing a node the table holds.
func mustOwn(s *ui.Share) {
	if stored(s) {
		panic("live: a stored tree is being changed")
	}
}

// setTree makes t the instance's tree, stored once, and lets go of the last.
func (in *instance) setTree(t *ui.Tree) {
	old, st := in.tree, in.s.nodes
	if st == nil {
		in.tree = t
		return
	}
	in.tree = st.tree(t)
	st.release(old)
	in.guardsStored(st)
}

// guardsStored points the guarded parts that were built but not kept by the
// table, because it had the content already, at the table's node: the next
// render gives them back, and they should not be hashed again.
func (in *instance) guardsStored(st *treeStore) {
	for _, k := range in.guards {
		switch v := k.v.(type) {
		case *ui.Tree:
			if !stored(&v.Share) {
				if c := st.lookupTree(v); c != nil {
					k.v = c
				}
			}
		case *ui.TreeList:
			if !stored(&v.Share) {
				if c := st.lookupList(v); c != nil {
					k.v = c
				}
			}
		}
	}
}

// setShadow makes t, which is the instance's tree or nil, what the client
// holds, and lets go of what it held.
func (in *instance) setShadow(t *ui.Tree) {
	old, st := in.shadow, in.s.nodes
	if st != nil {
		retainTree(t)
	}
	in.shadow = t
	if st != nil {
		st.release(old)
	}
}

// dropTrees lets go of the instance's tree and shadow.
func (in *instance) dropTrees() {
	if st := in.s.nodes; st != nil {
		st.release(in.tree)
		st.release(in.shadow)
	}
	in.tree, in.shadow, in.trimmed = nil, nil, false
}
