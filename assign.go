package ui

import (
	"reflect"
	"sync"
	"unsafe"
)

// A stateful component's server state may be tracked: a field of type
// uigen.Assign[T], which a .vuka.html file's unexported fields are (the
// compiler writes their reads as Get and their writes as Set). Set and
// Update mark it changed. A component whose state fields (its unexported
// ones, except those tagged `vuka:"-"`) are all Assigns renders again only
// when one of them changed or its props did; otherwise the session keeps
// its last render without calling Render. Its ui.State fields don't count:
// they are the browser's.
//
// Render must then read nothing but props and Assigns (and fields tagged
// `vuka:"-"`, which must not change what it renders); in tests (or with
// VUKA_LIVE_VERIFY=1) the session renders skipped components anyway and
// fails when the result differs.

// assigner is an Assign: its changed mark.
type assigner interface{ AssignDirty() *bool }

// assignValuer is an Assign's value, for island props.
type assignValuer interface{ AssignValue() any }

// stateFields is a component type's Assign fields, or tracked false when a
// state field isn't one.
type stateFields struct {
	tracked bool
	fields  []int
}

var stateCache sync.Map // reflect.Type (the struct) → stateFields

var assignerType = reflect.TypeFor[assigner]()

func stateOf(t reflect.Type) stateFields {
	if v, ok := stateCache.Load(t); ok {
		return v.(stateFields)
	}
	sf := stateFields{tracked: true}
	live := reflect.TypeFor[Live]()
	states := reflect.TypeFor[stateOps]()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		switch {
		case f.IsExported() || f.Type == live || f.Tag.Get("vuka") == "-":
		case f.Type.Implements(states):
			// A ui.State field is the browser's: the server holds its
			// handle, which a render reads the same way every time.
		case reflect.PointerTo(f.Type).Implements(assignerType):
			sf.fields = append(sf.fields, i)
		default:
			sf.tracked = false
		}
	}
	stateCache.Store(t, sf)
	return sf
}

func assignsOf(c Stateful, each func(dirty *bool)) bool {
	v := reflect.ValueOf(c).Elem()
	sf := stateOf(v.Type())
	if !sf.tracked {
		return false
	}
	for _, i := range sf.fields {
		f := v.Field(i)
		each(reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Interface().(assigner).AssignDirty())
	}
	return true
}

// TrackedState reports whether c's state is all Assigns (tracked), and
// whether one of them changed since SettleState. Package live calls it.
func TrackedState(c Stateful) (tracked, changed bool) {
	tracked = assignsOf(c, func(d *bool) { changed = changed || *d })
	return tracked, changed
}

// SettleState marks c's Assigns unchanged, after a render.
func SettleState(c Stateful) { assignsOf(c, func(d *bool) { *d = false }) }
