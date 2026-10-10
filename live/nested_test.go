package live_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
)

type Item struct {
	ui.Live
	Name   string
	OnPick func(string)
	n      int
}

func (c *Item) Inc() { c.n++ }
func (c *Item) Pick() {
	if c.OnPick != nil {
		c.OnPick(c.Name)
	}
}
func (c *Item) Render() ui.Node {
	return ui.El("li", nil, ui.Text(c.Name), ui.Child(c.n),
		ui.El("button", []ui.Attr{a("onClick", ui.On(c.Inc))}, ui.Text("+")),
		ui.El("button", []ui.Attr{a("onClick", ui.On(c.Pick))}, ui.Text("pick")))
}

type Board struct {
	ui.Live
	picked string
	title  int
}

func (b *Board) Retitle() { b.title++ }
func (b *Board) Render() ui.Node {
	return ui.El("section", nil,
		ui.El("h1", nil, ui.Text("board "), ui.Child(b.title), ui.Text(" picked:"), ui.Text(b.picked)),
		ui.El("button", []ui.Attr{a("onClick", ui.On(b.Retitle))}, ui.Text("retitle")),
		ui.El("ul", nil,
			ui.Component("n:item", "x", &Item{Name: "x", OnPick: func(s string) { b.picked = s }}),
			ui.Component("n:item", "y", &Item{Name: "y", OnPick: func(s string) { b.picked = s }})))
}

func TestNested(t *testing.T) {
	c := join(t, func(context.Context) ui.Node { return ui.El("main", nil, ui.Component("n:board", nil, &Board{})) })
	// handlers in page order: retitle, x+, xpick, y+, ypick
	m := c.event(ui.Event{Target: c.handler("click", 3), Type: "click"})
	if len(m.Updates) != 1 || strings.Contains(m.Updates[0].HTML, "board") || !strings.Contains(m.Updates[0].HTML, "y1") {
		t.Fatalf("a child's own event patches only the child: %+v", m.Updates)
	}
	m = c.event(ui.Event{Target: c.handler("click", 0), Type: "click"})
	if len(m.Updates) != 1 || !strings.Contains(m.Updates[0].HTML, "board 1") {
		t.Fatalf("the parent's event patches the parent: %+v", m.Updates)
	}
	if !strings.Contains(c.html, "y1") {
		t.Fatalf("the child keeps its state when the parent re-renders: %s", c.html)
	}
	m = c.event(ui.Event{Target: c.handler("click", 2), Type: "click"})
	if len(m.Updates) != 1 || !strings.Contains(m.Updates[0].HTML, "picked:x") {
		t.Fatalf("a child changes the parent through a func prop: %+v", m.Updates)
	}
	if !strings.Contains(c.html, "picked:x") || !strings.Contains(c.html, "y1") || !strings.Contains(c.html, "board 1") {
		t.Fatalf("page after nested events: %s", c.html)
	}
}
