package live_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/live/livetest"
)

var mounts int

// Shelf pages through books; a patch moves it to another page.
type Shelf struct {
	ui.Live
	Page    int
	mount   int
	updates []int
	gone    *[]string
}

type shelfParams struct{ Page int }

func (s *Shelf) Mount() { mounts++; s.mount = mounts }
func (s *Shelf) Update(p shelfParams) error {
	if p.Page < 0 {
		return errors.New("no such page")
	}
	s.updates = append(s.updates, p.Page)
	return nil
}
func (s *Shelf) Unmount() { *s.gone = append(*s.gone, "shelf") }
func (s *Shelf) Render() ui.Node {
	return ui.El("section", nil,
		ui.El("p", []ui.Attr{a("id", "at")}, ui.Text(fmt.Sprintf("page %d, mount %d, updates %v", s.Page, s.mount, s.updates))),
		ui.Link(ui.LinkProps{Href: fmt.Sprint("/shelf?page=", s.Page+1), Patch: true, ID: "next", Children: ui.Text("next")}),
		ui.Link(ui.LinkProps{Href: fmt.Sprint("/book/", s.Page*10), ID: "book", Children: ui.El("b", nil, ui.Text("a book"))}),
		ui.Link(ui.LinkProps{Href: "/book/1", ID: "tab", Target: "_blank", Children: ui.Text("in a tab")}),
		ui.El("button", []ui.Attr{a("id", "js"), a("onClick", ui.On(ui.JS.Patch("/shelf?page=7")))}))
}

// Book is another page, which sends the browser back to the shelf.
type Book struct {
	ui.Live
	ID int
}

func (b *Book) Back() { b.PushNavigate("/shelf?page=3") }
func (b *Book) Render() ui.Node {
	return ui.El("article", nil, ui.El("h1", nil, ui.Text(fmt.Sprint("book ", b.ID))),
		ui.El("button", []ui.Attr{a("id", "back"), a("onClick", ui.On(b.Back))}))
}

// shelfRouter is the app the pages are in: one layout, the shelf and books.
func shelfRouter(gone *[]string) live.Router {
	layout := func(page ui.Node) func(context.Context) ui.Node {
		return func(context.Context) ui.Node {
			return ui.El("main", nil, ui.El("header", nil, ui.Text("The library, a header long enough to be worth keeping")), page)
		}
	}
	return func(u string) live.Route {
		switch path, q, _ := strings.Cut(u, "?"); {
		case path == "/shelf":
			n, _ := strconv.Atoi(strings.TrimPrefix(q, "page="))
			s := &Shelf{Page: n, gone: gone}
			return live.Route{Render: layout(ui.Component("shelf", nil, s)), Updates: map[ui.Stateful]any{s: shelfParams{n}}}
		case strings.HasPrefix(path, "/book/"):
			id, _ := strconv.Atoi(strings.TrimPrefix(path, "/book/"))
			return live.Route{Render: layout(ui.Component("book", nil, &Book{ID: id}))}
		}
		return live.Route{}
	}
}

func TestNavigation(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			var gone []string
			router := shelfRouter(&gone)
			s := live.NewSession(context.Background(), router("/shelf?page=1").Render)
			s.SetRouter(router)
			c, err := livetest.Join(s, v)
			if err != nil {
				t.Fatal(err)
			}
			first := mounts
			check := func(url, at string) {
				t.Helper()
				if c.URL() != url || c.Text("#at") != at || c.HTML() != s.HTML() {
					t.Fatalf("at %q %q, want %q %q; page %s\nserver's %s", c.URL(), c.Text("#at"), url, at, c.HTML(), s.HTML())
				}
			}

			// A patch keeps the shelf mounted; its props and Update follow the URL.
			if err := c.Click("#next"); err != nil {
				t.Fatal(err)
			}
			check("/shelf?page=2", fmt.Sprintf("page 2, mount %d, updates [2]", first))
			if err := c.Click("#js"); err != nil {
				t.Fatal(err)
			}
			check("/shelf?page=7", fmt.Sprintf("page 7, mount %d, updates [2 7]", first))
			if m, _ := c.Patch("/shelf?page=-1"); m.Error != "no such page" {
				t.Fatalf("Update's error: %+v", m)
			}

			// A navigate mounts the other page; the layout isn't sent again.
			b := c.Bytes
			m, err := c.Navigate("/book/42")
			if err != nil || len(gone) != 1 || !strings.Contains(c.Text("h1"), "book 42") || c.Exists("#at") || c.HTML() != s.HTML() {
				t.Fatalf("navigate: %+v %v; gone %v; %s", m, err, gone, c.HTML())
			}
			t.Logf("navigate: %d bytes: %s", c.Bytes-b, live.Marshal(m))
			if v == 2 && (strings.Contains(string(live.Marshal(m)), "The library") || c.Bytes-b > 400) {
				t.Fatalf("a navigate under v2 sends the layout again (%d bytes): %s", c.Bytes-b, live.Marshal(m))
			}

			// A handler's PushNavigate, then a link's click; a link with a
			// target is the browser's.
			if err := c.Click("#back"); err != nil {
				t.Fatal(err)
			}
			check("/shelf?page=3", fmt.Sprintf("page 3, mount %d, updates []", first+1))
			if err := c.Click("#tab"); err != nil || c.URL() != "/shelf?page=3" {
				t.Fatalf("a target=_blank link: %v %s", err, c.URL())
			}
			if err := c.Click("#book b"); err != nil || !strings.Contains(c.Text("h1"), "book 30") {
				t.Fatalf("a click inside a link: %v %s", err, c.HTML())
			}

			// What isn't a page is a full load: a redirect, the page as it was.
			page := c.HTML()
			if m, err := c.Navigate("/report.pdf"); err != nil || m.Type != live.Redirect || m.URL != "/report.pdf" || c.URL() != "/report.pdf" || c.HTML() != page {
				t.Fatalf("not a page: %+v %v %s", m, err, c.URL())
			}
		})
	}
}

// A navigate drops the optimistic pushes still waiting, without their hooks.
func TestNavigationDropsPushes(t *testing.T) {
	var gone []string
	router := shelfRouter(&gone)
	page := func(ctx context.Context) ui.Node {
		return ui.El("main", nil, ui.El("button", []ui.Attr{a("id", "like"),
			a("onClick", ui.On(ui.JS.AddClass(ui.This, "liked").Push(func() error { return errors.New("no") }).
				OnError(ui.JS.AddClass("main", "failed"))))}))
	}
	s := live.NewSession(context.Background(), page)
	s.SetRouter(router)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.Hold()
	if err := c.Click("#like"); err != nil || c.Pending() != 1 {
		t.Fatalf("%v %d", err, c.Pending())
	}
	if _, err := c.Navigate("/book/1"); err != nil || c.Pending() != 0 {
		t.Fatalf("%v %d", err, c.Pending())
	}
	if err := c.Release(); err == nil || c.HasClass("main", "failed") {
		t.Fatalf("the push's handler is gone with its page (%v), and its hook doesn't run: %s", err, c.HTML())
	}
}

func TestNavigateWithoutRouter(t *testing.T) {
	c := join(t, func(context.Context) ui.Node { return ui.El("p", nil) })
	if m := c.send(live.ClientMessage{Type: live.Navigate, URL: "/x"}); m.Type != live.Redirect || m.URL != "/x" {
		t.Fatalf("%+v", m)
	}
}

func TestLink(t *testing.T) {
	for _, tc := range []struct {
		n    ui.Node
		want string
	}{
		{ui.Link(ui.LinkProps{Href: "/a", Children: ui.Text("A")}), `<a href="/a" data-vk-link>A</a>`},
		{ui.Link(ui.LinkProps{Href: "/a?p=2", Patch: true, Class: "next", Attrs: []ui.Attr{a("aria-current", "page")}}), `<a href="/a?p=2" data-vk-link="patch" class="next" aria-current="page"></a>`},
		{ui.Link(ui.LinkProps{Href: "javascript:alert(1)", ID: "x", Target: "_blank", Rel: "noopener"}), `<a href="about:invalid#TemplFailedSanitizationURL" data-vk-link id="x" target="_blank" rel="noopener"></a>`},
	} {
		if got, err := ui.String(context.Background(), tc.n); err != nil || got != tc.want {
			t.Errorf("got  %s %v\nwant %s", got, err, tc.want)
		}
	}
}
