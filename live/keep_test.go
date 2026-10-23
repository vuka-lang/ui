package live_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

// tally is a component counting its mounts and clicks.
type tally struct {
	ui.Live
	Name   string
	n      int
	mounts *int
}

func (c *tally) LiveSchema(l *ui.LiveSchema) { l.Mount(c.mount) }
func (c *tally) mount(context.Context) error { *c.mounts++; return nil }
func (c *tally) Inc()                        { c.n++ }
func (c *tally) Render() ui.Node {
	return ui.El("b", []ui.Attr{a("onClick", ui.On(c.Inc))}, ui.Text(fmt.Sprint(c.Name, "=", c.n)))
}

// TestNavigateKeep: a navigation keeping instances (Route.Keep) keeps the
// layout's component, and mounts afresh the page's, keyed (ui.Keyed) by
// the navigation; a navigation not keeping them mounts both afresh.
func TestNavigateKeep(t *testing.T) {
	mounts := 0
	page := func(key int, name string) func(context.Context) ui.Node {
		return func(context.Context) ui.Node {
			return ui.El("main", nil, ui.Component("side", nil, &tally{Name: "side", mounts: &mounts}),
				ui.Keyed(key, ui.Component("page", nil, &tally{Name: name, mounts: &mounts})))
		}
	}
	s := live.NewSession(context.Background(), page(0, "a"))
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if mounts != 2 {
		t.Fatalf("mounts %d", mounts)
	}
	if _, err := s.Event(ui.Event{Target: "c1:0", Type: "click"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Event(ui.Event{Target: "c2:0", Type: "click"}); err != nil {
		t.Fatal(err)
	}
	html := func() string {
		h, err := s.Render()
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if h := html(); !strings.Contains(h, "side=1") || !strings.Contains(h, "a=1") {
		t.Fatalf("after clicks: %s", h)
	}
	if _, err := s.Navigate(live.Route{Render: page(1, "a"), Keep: true}, false); err != nil {
		t.Fatal(err)
	}
	if h := html(); !strings.Contains(h, "side=1") || !strings.Contains(h, "a=0") || mounts != 3 {
		t.Errorf("kept navigation: mounts %d, %s", mounts, h)
	}
	if _, err := s.Navigate(live.Route{Render: page(1, "a"), Keep: true}, false); err != nil {
		t.Fatal(err)
	}
	if mounts != 3 {
		t.Errorf("the same key mounted again: %d", mounts)
	}
	if _, err := s.Navigate(live.Route{Render: page(2, "b")}, false); err != nil {
		t.Fatal(err)
	}
	if h := html(); !strings.Contains(h, "side=0") || !strings.Contains(h, "b=0") || mounts != 5 {
		t.Errorf("navigation without Keep: mounts %d, %s", mounts, h)
	}
}
