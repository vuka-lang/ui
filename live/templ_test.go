package live_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/live/internal/templdemo"
	"github.com/vuka-lang/ui/live/livetest"
)

// TestTemplEvent: a click changing one value inside a templ component — the
// badge of a Panel — sends that value alone, where the Panel as one piece of
// markup sent all of it.
func TestTemplEvent(t *testing.T) {
	sizes := map[bool]int{}
	for _, opaque := range []bool{true, false} {
		page := func(context.Context) ui.Node {
			return ui.Component("inbox", nil, &templdemo.Inbox{Unread: 3, Opaque: opaque})
		}
		s := live.NewSession(context.Background(), page)
		c, err := livetest.Join(s, 2)
		if err != nil {
			t.Fatal(err)
		}
		b := c.Bytes
		m, err := c.Event(click(c, "button"))
		if err != nil || c.HTML() != s.HTML() || !strings.Contains(c.Text("span"), "2") {
			t.Fatalf("opaque %v: %+v %v\n%s\nserver's %s", opaque, m, err, c.HTML(), s.HTML())
		}
		sizes[opaque] = c.Bytes - b
		tree := string(m.Trees[0].Tree)
		t.Logf("opaque %v: %d bytes: %s", opaque, c.Bytes-b, live.Marshal(m))
		if !opaque && (len(m.Trees) != 1 || strings.ContainsAny(tree, "<>") || !strings.Contains(tree, `"2"`)) {
			t.Fatalf("the change carries more than the badge: %s", live.Marshal(m))
		}
		// Down to no unread: the badge's branch renders nothing.
		for range 2 {
			if _, err := c.Event(click(c, "button")); err != nil {
				t.Fatal(err)
			}
		}
		if c.HTML() != s.HTML() || c.Exists("span") {
			t.Fatalf("opaque %v: %s\nserver's %s", opaque, c.HTML(), s.HTML())
		}
	}
	if sizes[false]*4 > sizes[true] {
		t.Fatalf("recorded %d bytes, opaque %d", sizes[false], sizes[true])
	}
}

// click is the event of a click on the element sel names.
func click(c *livetest.Client, sel string) ui.Event {
	id, _ := c.Attr(sel, "data-vk-on-click")
	return ui.Event{Target: id, Type: "click"}
}

// templRouter serves two pages sharing the templ Layout, each wrapped by
// wrap.
func templRouter(wrap func(ui.Node) ui.Node) live.Router {
	return func(u string) live.Route {
		switch u {
		case "/inbox":
			return live.Route{Render: func(context.Context) ui.Node { return wrap(templdemo.InboxPage()) }}
		case "/books":
			return live.Route{Render: func(context.Context) ui.Node {
				return wrap(templdemo.BooksPage([]string{"Dune", "Solaris", "Kindred"}))
			}}
		}
		return live.Route{}
	}
}

// TestTemplNavigate: pages sharing a templ layout navigate by what differs
// inside it — the title, the nav's current link, the main content — not
// the document again.
func TestTemplNavigate(t *testing.T) {
	steps := []struct{ url, title string }{
		{"/books", "Books · Templ demo"},
		{"/inbox", "Inbox · Templ demo"},
		{"/books", "Books · Templ demo"},
	}
	got := map[string][]int{}
	for _, mode := range []string{"opaque", "recorded"} {
		wrap := func(n ui.Node) ui.Node { return n }
		if mode == "opaque" {
			wrap = templdemo.Opaque
		}
		router := templRouter(wrap)
		s := live.NewSession(context.Background(), router("/inbox").Render)
		s.SetRouter(router)
		c, err := livetest.Join(s, 2)
		if err != nil {
			t.Fatal(err)
		}
		for i, step := range steps {
			b := c.Bytes
			m, err := c.Navigate(step.url)
			if err != nil || c.HTML() != s.HTML() || c.Text("title") != step.title {
				t.Fatalf("%s %s: %+v %v\n%s\nserver's %s", mode, step.url, m, err, c.HTML(), s.HTML())
			}
			got[mode] = append(got[mode], c.Bytes-b)
			t.Logf("%s → %s: %d bytes: %s", mode, step.url, c.Bytes-b, live.Marshal(m))
			if mode != "recorded" {
				continue
			}
			c0 := string(m.Trees[0].Tree)
			if m.Trees[0].ID != live.RootID || m.Trees[0].Full || strings.Contains(c0, "<head>") || strings.Contains(c0, "<footer") {
				t.Fatalf("the layout travelled again: %s", live.Marshal(m))
			}
			if i == 2 && strings.Contains(string(live.Marshal(m)), `"s":`) {
				t.Fatalf("back on a page it has seen, the client holds every static: %s", live.Marshal(m))
			}
		}
	}
	t.Logf("navigate bytes (/books, /inbox, /books again): opaque %v, recorded %v", got["opaque"], got["recorded"])
	sum := func(xs []int) int { return xs[0] + xs[1] + xs[2] }
	if o, r := got["opaque"], got["recorded"]; sum(r)*2 > sum(o) || r[0]*2 > o[0] || r[2]*4 > o[2] {
		t.Fatalf("recorded trees save little: opaque %v, recorded %v", o, r)
	}
}

// TestTemplProperty: random clicks on a page of templ components over v1
// and v2 leave both clients holding what the server rendered.
func TestTemplProperty(t *testing.T) {
	page := func(context.Context) ui.Node {
		return ui.Fragment(templdemo.InboxPage(), ui.Component("b", nil, &templdemo.Inbox{Unread: 9}))
	}
	s1, s2 := live.NewSession(context.Background(), page), live.NewSession(context.Background(), page)
	c1, err := livetest.Join(s1, 1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := livetest.Join(s2, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		if c1.HTML() != c2.HTML() || c2.HTML() != s2.HTML() {
			t.Fatalf("step %d:\nv1 %s\nv2 %s", i, c1.HTML(), c2.HTML())
		}
		hs := c2.Handlers()
		ev := ui.Event{Target: hs[i%len(hs)][1], Type: "click"}
		if _, err := c1.Event(ev); err != nil {
			t.Fatal(err)
		}
		if _, err := c2.Event(ev); err != nil {
			t.Fatal(err)
		}
	}
}
