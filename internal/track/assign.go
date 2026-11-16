// Package track holds the server state a live session tracks. Generated
// code names it as uigen.Assign.
package track

import "sync/atomic"

// Assign is a stateful component's state value whose changes a live session
// tracks: Set and Update mark it changed. A component whose state fields
// (its unexported ones, except those tagged `vuka:"-"`) are all Assigns
// renders again only when one of them changed or its props did; otherwise
// the session keeps its last render without calling Render.
type Assign[T any] struct {
	v     T
	dirty bool
	ver   uint64
}

// versions numbers the writes to every Assign, so that no two writes share a
// version, even of two Assigns or of one copied over another.
var versions atomic.Uint64

// Get is the value.
func (a *Assign[T]) Get() T { return a.v }

// Set sets the value and marks it changed.
func (a *Assign[T]) Set(v T) { a.v, a.dirty, a.ver = v, true, versions.Add(1) }

// Update sets the value to f of it and marks it changed.
func (a *Assign[T]) Update(f func(T) T) { a.v, a.dirty, a.ver = f(a.v), true, versions.Add(1) }

// Version is the value's version: another one after every Set and Update. A
// guarded part of a render (ui.Guard) compares the versions of the fields
// it reads.
func (a *Assign[T]) Version() uint64 { return a.ver }

// AssignDirty is the changed mark, which the session reads and clears.
func (a *Assign[T]) AssignDirty() *bool { return &a.dirty }

// AssignValue is the current value as any, for island props serialization:
// an Assign island prop is sent to the client as its value, and a live
// re-render with a changed value drives the island's update().
func (a *Assign[T]) AssignValue() any { return a.v }
