// Package demo is stateful components compiled by the Vuka compiler, for
// the live package's property tests and benchmarks. The *_html.go files are
// generated: vuka gen -inplace ./live/internal/demo
package demo

import (
	"context"

	"github.com/vuka-lang/ui/v2"
)

// Counters is a page of n counters.
func Counters(n int) func(context.Context) ui.Node {
	return func(context.Context) ui.Node { return CountersPage{N: n} }
}

// Page is a page with two boards and a counter.
func Page(context.Context) ui.Node { return BoardsPage{} }

type item struct {
	ID   int
	Name string
}

// Words are the values RandomEvent sends: Board.Do's commands and more.
var Words = []string{"add", "front", "drop", "pop", "rotate", "reverse", "swap", "rename", "mid", "clear", "big", "note", "x", "<b>&\"", "add", "rotate", "a much longer note, long enough to be kept by reference on the connection......"}
