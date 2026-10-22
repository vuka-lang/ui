package live_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
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

type userKey struct{}

// TestCtx: a node ui.Ctx builds from the render's context is walked as the
// node it builds, on the first render and over a session: a component in it
// is live, with both protocols.
func TestCtx(t *testing.T) {
	ctx := context.WithValue(context.Background(), userKey{}, "ada")
	page := func(context.Context) ui.Node {
		return ui.Ctx(func(ctx context.Context) ui.Node {
			name, _ := ctx.Value(userKey{}).(string)
			return ui.El("main", nil, ui.Text(name), ui.Component("ctx:item", nil, &Item{Name: "x"}))
		})
	}
	if html, err := ui.String(ctx, page(ctx)); err != nil || !strings.Contains(html, "<main>ada<li>x0") {
		t.Fatalf("ui.Ctx renders the node it builds: %s %v", html, err)
	}
	for _, v := range []int{1, 2} {
		c, err := livetest.Join(live.NewSession(ctx, page), v)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Event(ui.Event{Target: c.Handlers()[0][1], Type: "click"}); err != nil {
			t.Fatal(err)
		}
		if h := c.HTML(); !strings.Contains(h, "<main>ada<li") || !strings.Contains(h, "x1") {
			t.Fatalf("v%d: a component inside ui.Ctx's node takes its event: %s", v, h)
		}
	}
}
