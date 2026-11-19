package live

import (
	"slices"

	"github.com/vuka-lang/ui/v2"
)

// Temporary regions (ui.Temporary): the browser keeps a region's content
// and the session forgets it. Each render builds it, marked with what it
// shows (ui.TreeList.Temp); the diff sends it whole when the shadow at its
// place shows another value, and skips it otherwise; once a reply is
// written, flushTemps empties it in the tree and the shadow, keeping the
// mark, as flushStreams empties a stream's rows.

// TempRegion notes that the instance's render has a Temporary region.
func (sc *scope) TempRegion() { sc.in.temps = true; sc.in.effects++ }

// flushTemps drops the content of the Temporary regions the reply just
// written carried, or left as the browser has them, from the trees and
// shadows the session keeps.
func (s *Session) flushTemps() {
	if !s.trees {
		return
	}
	for _, in := range s.byID {
		if in.temps {
			in.emptied(emptyTemps)
		}
	}
}

// emptyTemps is t with the Temporary regions emptied: t itself when it holds
// none with content, else a copy of the frames down to them.
func emptyTemps(t *ui.Tree) *ui.Tree {
	if t == nil {
		return nil
	}
	var dyn []any
	set := func(i int, v any) {
		if dyn == nil {
			dyn = slices.Clone(t.Dyn)
		}
		dyn[i] = v
	}
	for i, d := range t.Dyn {
		switch d := d.(type) {
		case *ui.Tree:
			if n := emptyTemps(d); n != d {
				set(i, n)
			}
		case *ui.TreeList:
			if d.Temp != "" {
				if len(d.Items) > 0 {
					set(i, &ui.TreeList{Temp: d.Temp})
				}
				continue
			}
			var items []*ui.Tree
			for j, it := range d.Items {
				if n := emptyTemps(it); n != it {
					if items == nil {
						items = slices.Clone(d.Items)
					}
					items[j] = n
				}
			}
			if items != nil {
				set(i, &ui.TreeList{Items: items, Keys: d.Keys, Stream: d.Stream})
			}
		}
	}
	if dyn == nil {
		return t
	}
	return &ui.Tree{Key: t.Key, Statics: t.Statics, Dyn: dyn}
}
