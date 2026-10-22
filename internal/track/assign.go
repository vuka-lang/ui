// Package track holds the server state a live session tracks. Generated
// code names it as uigen.Assign.
package track

// Assign is a stateful component's state value whose changes a live session
// tracks: Set and Update mark it changed. A component whose state fields
// (its unexported ones, except those tagged `vuka:"-"`) are all Assigns
// renders again only when one of them changed or its props did; otherwise
// the session keeps its last render without calling Render.
type Assign[T any] struct {
	v     T
	dirty bool
}

// Get is the value.
func (a *Assign[T]) Get() T { return a.v }

// Set sets the value and marks it changed.
func (a *Assign[T]) Set(v T) { a.v, a.dirty = v, true }

// Update sets the value to f of it and marks it changed.
func (a *Assign[T]) Update(f func(T) T) { a.v, a.dirty = f(a.v), true }

// AssignDirty is the changed mark, which the session reads and clears.
func (a *Assign[T]) AssignDirty() *bool { return &a.dirty }

// AssignValue is the current value as any, for island props serialization:
// an Assign island prop is sent to the client as its value, and a live
// re-render with a changed value drives the island's update().
func (a *Assign[T]) AssignValue() any { return a.v }
