package live

import "github.com/vuka-lang/ui/v2"

// Temporary regions (ui.Temporary): the browser keeps a region's content
// and the session forgets it. Each render builds it, marked with what it
// shows (ui.TreeList.Temp); the diff sends it whole when the shadow at its
// place shows another value, and skips it otherwise; once a reply is
// written, flushTemps empties it in the tree and the shadow, keeping the
// mark, as flushStreams empties a stream's rows.

// TempRegion notes that the instance's render has a Temporary region.
func (sc *scope) TempRegion() { sc.in.temps = true }

// flushTemps drops the content of the Temporary regions the reply just
// written carried, or left as the browser has them, from the trees and
// shadows the session keeps.
func (s *Session) flushTemps() {
	if !s.trees {
		return
	}
	for _, in := range s.byID {
		if in.temps {
			emptyTemps(in.tree, in.shadow)
		}
	}
}

// emptyTemps empties the Temporary regions of t, and of sh, its shadow.
func emptyTemps(t *ui.Tree, sh *sframe) {
	if t == nil {
		return
	}
	if sh != nil && (sh.key != t.Key || len(sh.d) != len(t.Dyn)) {
		sh = nil
	}
	for i, d := range t.Dyn {
		var sd any
		if sh != nil {
			sd = sh.d[i]
		}
		switch d := d.(type) {
		case *ui.Tree:
			f, _ := sd.(*sframe)
			emptyTemps(d, f)
		case *ui.TreeList:
			if d.Temp != "" {
				if len(d.Items) > 0 {
					t.Dyn[i] = &ui.TreeList{Temp: d.Temp}
				}
				if sh != nil {
					if l, ok := sd.(*slist); !ok || l.temp != d.Temp || len(l.items) > 0 {
						sh.d[i] = &slist{temp: d.Temp}
					}
				}
				continue
			}
			l, _ := sd.(*slist)
			for j, it := range d.Items {
				var f *sframe
				if l != nil && j < len(l.items) {
					f = l.items[j]
				}
				emptyTemps(it, f)
			}
		}
	}
}
