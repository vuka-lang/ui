package live_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// RowList is a live component rendering each name through its caller's Row
// (a scoped slot), with a handler of its own.
type RowList struct {
	ui.Live
	Names []string
	Row   func(string) ui.Node
	n     int
}

func (s *RowList) Inc() { s.n++ }
func (s *RowList) Render() ui.Node {
	var lis []ui.Node
	for _, name := range s.Names {
		lis = append(lis, ui.El("li", nil, s.Row(name)))
	}
	return ui.El("ul", nil, ui.Child(s.n), ui.El("button", []ui.Attr{a("onClick", ui.On(s.Inc))}, ui.Text("inc")), ui.Fragment(lis...))
}

// Owner fills RowList's Row with a button calling its own method: the slot's
// content is the owner's though the list renders it.
type Owner struct {
	ui.Live
	names []string
}

func (o *Owner) Remove(name string) {
	for i, n := range o.names {
		if n == name {
			o.names = append(o.names[:i:i], o.names[i+1:]...)
		}
	}
}

func (o *Owner) Render() ui.Node {
	return ui.El("section", nil,
		ui.El("h1", nil, ui.Text(strings.Join(o.names, "+"))),
		ui.Component("s:rows", nil, &RowList{Names: o.names, Row: func(name string) ui.Node {
			return ui.El("button", []ui.Attr{a("onClick", ui.On(func() { o.Remove(name) }))}, ui.Text("rm "+name))
		}}))
}

// TestSlotHandler: a handler in slot content a live child renders runs the
// writer's code, and the writer renders again; the child keeps its state.
func TestSlotHandler(t *testing.T) {
	c := join(t, func(context.Context) ui.Node {
		return ui.El("main", nil, ui.Component("s:owner", nil, &Owner{names: []string{"a", "b", "c"}}))
	})
	// handlers in page order: inc, rm a, rm b, rm c
	c.event(ui.Event{Target: c.handler("click", 0), Type: "click"})
	m := c.event(ui.Event{Target: c.handler("click", 2), Type: "click"})
	if len(m.Updates) != 1 || !strings.Contains(m.Updates[0].HTML, "<h1>a+c</h1>") {
		t.Fatalf("the slot's handler patches its writer: %+v", m.Updates)
	}
	if !strings.Contains(c.html, ">1<button") || strings.Contains(c.html, "rm b") || !strings.Contains(c.html, "rm c") {
		t.Fatalf("page after the slot's event: %s", c.html)
	}
}
