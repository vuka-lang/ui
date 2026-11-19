package live

import (
	"testing"

	"github.com/vuka-lang/ui/v2"
)

func leaf(s string, hash uint64) *ui.Tree {
	t := &ui.Tree{Key: 1, Statics: []string{"", ""}, Dyn: []any{s}}
	t.Share.Hash = hash
	return t
}

// Two trees with the same hash and different content are two entries; neither
// is taken for the other, and letting go of one leaves the other found.
func TestStoreHashCollision(t *testing.T) {
	st := new(treeStore)
	a, b, a2 := leaf("a", 7), leaf("b", 7), leaf("a", 7)
	ca, cb, ca2 := st.tree(a), st.tree(b), st.tree(a2)
	if ca != a || cb != b || ca2 != a {
		t.Fatalf("stored %p %p %p, want %p %p %p", ca, cb, ca2, a, b, a)
	}
	if got := st.stats(); got.Trees != 2 || got.Refs != 3 {
		t.Fatalf("stats %+v", got)
	}
	st.release(ca)
	st.release(ca2)
	if got := st.stats(); got.Trees != 1 || got.Refs != 1 {
		t.Fatalf("after letting go of a: %+v", got)
	}
	if got := st.lookupTree(leaf("b", 7)); got != b {
		t.Fatalf("b is no longer found: %p", got)
	}
	if got := st.lookupTree(leaf("a", 7)); got != nil {
		t.Fatal("a is still found")
	}
	st.release(cb)
	if got := st.stats(); got.Trees != 0 {
		t.Fatalf("stats %+v", got)
	}
}

// A node's content is its children's: a parent over equal children is found,
// and the children are held once for the parent however many copies there are.
func TestStoreChildren(t *testing.T) {
	st := new(treeStore)
	mk := func() *ui.Tree {
		return &ui.Tree{Key: 2, Statics: []string{"", "", ""}, Dyn: []any{leaf("x", 0), &ui.TreeList{Items: []*ui.Tree{leaf("y", 0), leaf("y", 0)}, Keys: []string{"1", "2"}}}}
	}
	p1, p2 := st.tree(mk()), st.tree(mk())
	if p1 != p2 {
		t.Fatal("equal trees are two nodes")
	}
	if got := st.stats(); got.Trees != 3 || got.Lists != 1 {
		t.Fatalf("stats %+v", got)
	}
	l := p1.Dyn[1].(*ui.TreeList)
	if l.Items[0] != l.Items[1] {
		t.Error("equal rows are two nodes")
	}
	st.release(p1)
	st.release(p2)
	if got := st.stats(); got.Trees != 0 || got.Lists != 0 {
		t.Fatalf("stats %+v", got)
	}
}

func TestMustOwn(t *testing.T) {
	st := new(treeStore)
	n := st.tree(leaf("a", 0))
	defer func() {
		if recover() == nil {
			t.Error("a stored tree may be changed")
		}
	}()
	mustOwn(&n.Share)
}
