package live

import (
	"reflect"
	"runtime"

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
// client: the frames of its instances' trees and shadows, and the bytes of
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
	var shadow func(f *sframe)
	shadow = func(f *sframe) {
		if f == nil {
			return
		}
		frames++
		for _, d := range f.d {
			switch d := d.(type) {
			case sstr:
				bytes += len(d.long)
			case *sframe:
				shadow(d)
			case *slist:
				for _, it := range d.items {
					shadow(it)
				}
			}
		}
	}
	for _, in := range s.byID {
		tree(in.tree)
		shadow(in.shadow)
	}
	return frames, bytes + len(s.c.strs)*8
}
