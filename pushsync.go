package ui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// One state over HTTP (components-seamless.md §12): on a page that isn't
// live, the browser's dirty synced states ride the same request that
// carries a push of a page method, and the reply carries the states the
// method wrote — versioned, adopted by the browser exactly as a live
// patch's are. A push call's receiver is fresh, so a state is matched by
// its site, the name of its declaration, which both page kinds share.

// stateSiteOf is the site in a state key: "site:n" (a static render's),
// "cN:site:occ" (a live instance's). A page state's key ("g:site") has
// none: page states are never synced.
func stateSiteOf(key string) string {
	parts := strings.Split(key, ":")
	switch {
	case len(parts) == 2 && parts[0] != "g":
		return parts[0]
	case len(parts) == 3:
		return parts[1]
	}
	return ""
}

// pushCells is each synced-capable state field of x (a pointer to a push
// call's receiver), with its cell: the zero fields get their cells first,
// as mounting gives them.
func pushCells(x any) []*stateCell {
	v := reflect.ValueOf(x)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	initStates(v)
	idx, _ := fieldStates.Load(v.Elem().Type())
	if idx == nil {
		return nil
	}
	var out []*stateCell
	for _, i := range idx.([]int) {
		f := v.Elem().Field(i)
		s := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
		if c := s.Interface().(stateOps).stateOf().c; c != nil && c.derived == nil {
			out = append(out, c)
		}
	}
	return out
}

// ApplyPushStates applies the browser's dirty synced states carried by a
// push call to x, the call's fresh receiver, before the method runs: each
// write lands on the state declared at its key's site. A write naming no
// state of x is dropped (another component's); one naming a state that
// isn't synced, or carrying a value that isn't its type, is an error — the
// value is the visitor's.
func ApplyPushStates(x any, writes map[string]StateWrite) error {
	cells := pushCells(x)
	if len(writes) == 0 {
		return nil
	}
	for key, w := range writes {
		site := stateSiteOf(key)
		if site == "" {
			continue
		}
		for _, c := range cells {
			if c.site != site {
				continue
			}
			c.mu.Lock()
			synced := c.synced
			if synced && w.N > c.lastN {
				c.mu.Unlock()
				nv := reflect.New(c.typ)
				if err := json.Unmarshal(w.V, nv.Interface()); err != nil {
					return fmt.Errorf("ui: a push's state %s: %w", key, err)
				}
				c.mu.Lock()
				c.cur, c.hasCur = nv.Elem().Interface(), true
				c.ver = nextVersion()
				c.srv = false
				c.lastN = w.N
				c.sdirty = true
			}
			c.mu.Unlock()
			if !synced {
				return fmt.Errorf("ui: %s is not a synced state", key)
			}
			break
		}
	}
	return nil
}

// CollectPushStates is the synced states the push call's method wrote on x
// — the echoes of the applied writes and the method's own writes — as the
// reply carries them: by the key the request carried when one matches the
// state's site, else by the site, which the browser resolves to the one
// state it holds there. Nil when the method wrote none.
func CollectPushStates(x any, carried map[string]StateWrite) map[string]any {
	var out map[string]any
	for _, c := range pushCells(x) {
		c.mu.Lock()
		if !c.hasCur || !c.synced {
			c.mu.Unlock()
			continue
		}
		e := map[string]any{"v": c.cur, "ver": c.ver}
		if c.srv {
			e["sw"] = 1
		} else {
			e["bn"] = c.lastN
		}
		site := c.site
		c.mu.Unlock()
		key := site
		for k := range carried {
			if stateSiteOf(k) == site {
				key = k
				break
			}
		}
		if out == nil {
			out = map[string]any{}
		}
		out[key] = e
	}
	return out
}
