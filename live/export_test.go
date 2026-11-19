package live

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/vuka-lang/ui/v2"
)

// SetWholeTrees makes new sessions render every instance without its
// frames, as components without render tracking render.
func SetWholeTrees(on bool) { wholeTrees = on }

// SameProps is sameProps, for its tests.
var SameProps = sameProps

// FuncName is the name of a func's code.
func FuncName(fn any) string { return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name() }

// Retained is what the session keeps of its last render for its v2
// client: the frames of its instances' trees and shadows (references, which
// sessions share: not each its own memory), and the bytes of
// the markup they hold.
func Retained(s *Session) (frames, bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var tree func(t *ui.Tree)
	tree = func(t *ui.Tree) {
		if t == nil {
			return
		}
		frames++
		for _, d := range t.Dyn {
			switch d := d.(type) {
			case string:
				bytes += len(d)
			case *ui.Tree:
				tree(d)
			case *ui.TreeList:
				for _, it := range d.Items {
					tree(it)
				}
			}
		}
	}
	for _, in := range s.byID {
		tree(in.tree)
		tree(in.shadow)
	}
	return frames, bytes + len(s.c.strs)*8
}

// Holds is how many holds the session has on stored nodes: one for each
// instance's tree and each shadow.
func Holds(s *Session) (n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, in := range s.byID {
		if in.tree != nil {
			n++
		}
		if in.shadow != nil {
			n++
		}
	}
	return n
}

// Store is a table of stored trees; a test gives its sessions one of their own.
type Store = treeStore

func NewStore() *Store { return new(Store) }

// UseStore makes s store its trees in st; before its first render.
func UseStore(s *Session, st *Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = st
}

func StoreCounts(st *Store) (trees, lists int) {
	r := st.stats()
	return r.Trees, r.Lists
}

// RootTree is the tree of the page's own instance.
func RootTree(s *Session) *ui.Tree {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root.tree
}

// TreeDump writes the instances' trees in id order, in full.
func TreeDump(s *Session) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id := range s.byID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var b strings.Builder
	var dump func(d any)
	dump = func(d any) {
		switch d := d.(type) {
		case string:
			fmt.Fprintf(&b, "%q", d)
		case ui.TreeRef:
			fmt.Fprintf(&b, "ref(%s)", string(d))
		case *ui.Tree:
			fmt.Fprintf(&b, "T%x[", d.Key)
			for _, x := range d.Dyn {
				dump(x)
				b.WriteByte(',')
			}
			b.WriteByte(']')
		case *ui.TreeList:
			fmt.Fprintf(&b, "L(s=%v,t=%q,k=%q)[", d.Stream, d.Temp, d.Keys)
			for _, it := range d.Items {
				dump(it)
				b.WriteByte(',')
			}
			b.WriteByte(']')
		}
	}
	for _, id := range ids {
		in := s.byID[id]
		fmt.Fprintf(&b, "%s tree=", id)
		if in.tree != nil {
			dump(in.tree)
		}
		fmt.Fprintf(&b, " shadow=")
		if in.shadow != nil {
			dump(in.shadow)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// CheckStore compares every node the sessions reach with what the table
// counts: holders as each session's trees and shadows plus the parents that
// contain it, once per place; and that the table holds just those nodes.
func CheckStore(st *Store, ss ...*Session) error {
	want := map[any]int32{}
	var walk func(n any)
	walk = func(n any) {
		switch n := n.(type) {
		case *ui.Tree:
			if want[n]++; want[n] > 1 {
				return
			}
			for _, d := range n.Dyn {
				switch d.(type) {
				case *ui.Tree, *ui.TreeList:
					walk(d)
				}
			}
		case *ui.TreeList:
			if want[n]++; want[n] > 1 {
				return
			}
			for _, it := range n.Items {
				walk(it)
			}
		}
	}
	for _, s := range ss {
		s.mu.Lock()
		for _, in := range s.byID {
			if in.tree != nil {
				walk(in.tree)
			}
			if in.shadow != nil {
				walk(in.shadow)
			}
		}
		s.mu.Unlock()
	}
	var trees, lists int
	for n, w := range want {
		switch n := n.(type) {
		case *ui.Tree:
			trees++
			if got := n.Share.Refs; got != w {
				return fmt.Errorf("tree %x: counted %d holders, found %d", n.Share.Hash, got, w)
			}
			if st.lookupTree(n) != n {
				return fmt.Errorf("tree %x is not in the table", n.Share.Hash)
			}
		case *ui.TreeList:
			lists++
			if got := n.Share.Refs; got != w {
				return fmt.Errorf("list %x: counted %d holders, found %d", n.Share.Hash, got, w)
			}
			if st.lookupList(n) != n {
				return fmt.Errorf("list %x is not in the table", n.Share.Hash)
			}
		}
	}
	if r := st.stats(); r.Trees != trees || r.Lists != lists {
		return fmt.Errorf("the table holds %d trees, %d lists; the sessions reach %d, %d", r.Trees, r.Lists, trees, lists)
	}
	return nil
}
