// Package bridge is how package uigen reaches what package ui keeps
// unexported: ui sets these when it is initialised. Values are ui's types,
// passed as any so this package needn't import ui.
package bridge

var (
	// Declare is the state of zero's type declared at site with init,
	// the page's when page.
	Declare func(zero any, site string, page bool, init any) any
	// Derived is a state of zero's type that never changes, of value v.
	Derived func(zero any, v any) any
	// At is the field at path of struct state s, as a state of out's type.
	At func(s, out any, path string) any
	// Op is the derived state op (Not, And, …, Len) of s, with o its
	// operand when it takes one.
	Op func(op string, s, o any) any
	// Write is chain c, then the write op (Toggle, Inc, Dec) of s.
	Write func(op string, c, s any) any
	// States gives each zero ui.State field of the struct p points to a
	// state of its own.
	States func(p any)
)
