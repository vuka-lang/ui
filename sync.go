package ui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/vuka-lang/ui/v2/internal/track"
)

// One state (components-seamless.md §12): a ui.State is one logical value
// with two replicas, the server's and the browser's, every write versioned.
// The compiler classifies each state by what the code does:
//
//   - never read by Go code: browser-local — browser writes are never sent;
//   - read (or written) by any live method: synced — the browser carries its
//     dirty value with every event (Event.States), applied before the
//     handler runs, so Get reads the truth as of the event; a server write
//     (the compiler's SrvSet) rides the next render, and the browser adopts
//     it iff its version advanced past the browser's base;
//   - a plain unexported field stays a server-only tracked value (uigen.Assign);
//   - derived values are expressions.
//
// A state the compiler can't follow is classified synced: over-syncing costs
// bytes, under-syncing would be a correctness bug. A synced state is
// browser-writable by definition: nothing authorization-relevant may trust
// one without a server-side check.

// syncedFields is each component type's synced ui.State fields, by name:
// what the compiler registered (uigen.SyncFields). initStates applies it
// when the component mounts.
var syncedFields sync.Map // reflect.Type → map[string]bool

// registerSyncedFields records that t's named ui.State fields are synced.
func registerSyncedFields(t reflect.Type, names []string) {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	syncedFields.Store(t, m)
}

// markSynced flags the cell synced, or browser-local again.
func (c *stateCell) markSynced(on bool) {
	c.mu.Lock()
	c.synced = on
	c.mu.Unlock()
}

// srvSet is a server write: the replica takes v, versioned, and the next
// render carries it for the browser to adopt.
func (c *stateCell) srvSet(v any) {
	c.mu.Lock()
	c.cur, c.hasCur = v, true
	c.ver = track.Next()
	c.srv = true
	c.sdirty = true
	c.synced = true // a server write is Go code touching it
	c.mu.Unlock()
}

// version is the replica's write version, 0 until a write.
func (c *stateCell) version() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ver
}

// StateWrite is one browser write of a synced state, as an event carries it
// (Event.States): the value as JSON and N, the browser's write number for
// the state, which orders its writes and suppresses echoes.
type StateWrite struct {
	V json.RawMessage `json:"v"`
	N uint64          `json:"n"`
}

// ApplyStateWrite applies a browser write carried by an event to the synced
// state of c whose key is key, before the event's handler runs. It reports
// whether c has the state; a value that isn't the state's type, or a write
// to a state that isn't synced, is an error — the value is the visitor's.
// Package live calls it.
func ApplyStateWrite(c Stateful, key string, w StateWrite) (bool, error) {
	v := reflect.ValueOf(c).Elem()
	t := v.Type()
	idx, _ := fieldStates.Load(t)
	if idx == nil {
		return false, nil
	}
	for _, i := range idx.([]int) {
		f := v.Field(i)
		s := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
		cell := s.Interface().(stateOps).stateOf().c
		if cell == nil {
			continue
		}
		cell.mu.Lock()
		at, synced, derived, lastN := cell.key, cell.synced, cell.derived != nil, cell.lastN
		cell.mu.Unlock()
		if at != key {
			continue
		}
		if !synced || derived {
			return true, fmt.Errorf("ui: %s is not a synced state", t.Field(i).Name)
		}
		if w.N <= lastN {
			return true, nil // already applied: a carry repeated until its echo
		}
		nv := reflect.New(cell.typ)
		if err := json.Unmarshal(w.V, nv.Interface()); err != nil {
			return true, fmt.Errorf("ui: %s: %w", t.Field(i).Name, err)
		}
		cell.mu.Lock()
		if w.N > cell.lastN {
			cell.cur, cell.hasCur = nv.Elem().Interface(), true
			cell.ver = track.Next()
			cell.srv = false
			cell.lastN = w.N
			cell.sdirty = true
		}
		cell.mu.Unlock()
		return true, nil
	}
	return false, nil
}

// syncDirty reports whether a synced state of c changed since its last
// render settled, and clear clears the marks. Package ui's TrackedState and
// SettleState use it beside the Assigns.
func syncDirty(c Stateful, clear bool) bool {
	v := reflect.ValueOf(c).Elem()
	idx, _ := fieldStates.Load(v.Type())
	if idx == nil {
		return false
	}
	dirty := false
	for _, i := range idx.([]int) {
		f := v.Field(i)
		s := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
		cell := s.Interface().(stateOps).stateOf().c
		if cell == nil {
			continue
		}
		cell.mu.Lock()
		if cell.sdirty {
			dirty = true
			if clear {
				cell.sdirty = false
			}
		}
		cell.mu.Unlock()
		if dirty && !clear {
			return true
		}
	}
	return dirty
}
