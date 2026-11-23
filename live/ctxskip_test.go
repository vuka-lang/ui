package live_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/internal/benchpages"
	"github.com/vuka-lang/ui/v2/uigen"
)

// rowRemove is the target of the Remove item of the row of user id.
func rowRemove(id int) string { return "c" + strconv.Itoa(id+1) + ":1" }

// TestActionMenuRowsKept: a row whose menu items are ui.ActionMenus is kept
// across another row's event as a row of plain buttons is — the page and
// the changed row render, nothing else — and a row kept across several
// events still shows its whole menu, and its items still act.
func TestActionMenuRowsKept(t *testing.T) {
	for _, verify := range []bool{true, false} {
		t.Run(map[bool]string{true: "verify", false: "count"}[verify], func(t *testing.T) {
			defer func(v bool) { live.Verify = v }(live.Verify)
			live.Verify = verify
			const n = 20
			events := []int{3, 5, 3, 11}
			renders := func(s *live.Session) (out []int) {
				for _, id := range events {
					r0 := live.Renders(s)
					replyTo(t, s, clickMsg(rowPromote(id)))
					out = append(out, live.Renders(s)-r0)
				}
				return out
			}
			plain, _ := joinV2(t, usersPage(n, true))
			want := renders(plain)
			plain.Close()
			s, _ := joinV2(t, usersPage(n, false))
			defer s.Close()
			if got := renders(s); !verify && !slices.Equal(got, want) {
				t.Fatalf("renders per event %v, with plain buttons %v", got, want)
			}
			// row 7 was kept by every event so far
			h := s.HTML()
			for _, want := range []string{`popovertarget="menu-7"`, `id="menu-7"`, `href="/users/7"`, ">Promote<", ">Remove<"} {
				if !strings.Contains(h, want) {
					t.Fatalf("kept row lost %s:\n%s", want, h)
				}
			}
			out := string(replyTo(t, s, clickMsg(rowPromote(7))))
			if role := benchpages.NextRole(benchpages.Users(n)[6].Role); !strings.Contains(out, `"id":"c8"`) || !strings.Contains(out, role) {
				t.Fatalf("promote 7 from a kept row: %s", out)
			}
			replyTo(t, s, clickMsg(rowRemove(9)))
			if h := s.HTML(); strings.Contains(h, `id="menu-9"`) || strings.Count(h, `<tr class="row"`) != n-1 {
				t.Fatalf("remove 9 from a kept row:\n%s", h)
			}
		})
	}
}

type menuBook struct {
	ID    int
	Title string
}

// menuShelf is a client DataTable of books whose rows' actions menus are
// ActionMenu items closing the menu they are in, inside a component of its
// own beside a counter of the page's.
type menuShelf struct {
	ui.Live
	NoMenu  bool
	removed uigen.Assign[int]
}

func (s *menuShelf) Remove(id int) { s.removed.Set(id) }

func (s *menuShelf) Render() ui.Node {
	books := []menuBook{{1, "Dune"}, {2, "Emma"}, {3, "Ulysses"}}
	actions := func(b menuBook) ui.Node {
		return ui.ActionMenu{Icon: "trash", Danger: true, On: ui.JS.Push(s.Remove, ui.Value(b.ID)), Children: ui.Text("Remove")}
	}
	if s.NoMenu {
		actions = nil
	}
	return ui.El("section", nil,
		ui.El("p", []ui.Attr{a("class", "removed")}, ui.Text(strconv.Itoa(s.removed.Get()))),
		ui.DataTable[menuBook]{
			ID: "books", Client: true, Rows: books,
			RowKey:  func(b menuBook) any { return b.ID },
			Columns: []ui.Column[menuBook]{{Header: ui.Text("Title"), Cell: func(b menuBook) ui.Node { return ui.Text(b.Title) }}},
			Actions: actions,
		})
}

type shelfPage struct {
	ui.Live
	NoMenu bool
	n      uigen.Assign[int]
}

func (p *shelfPage) Inc() { p.n.Set(p.n.Get() + 1) }

func (p *shelfPage) Render() ui.Node {
	return ui.El("main", nil,
		ui.El("button", []ui.Attr{a("onClick", ui.On(p.Inc))}, ui.Text("n="+strconv.Itoa(p.n.Get()))),
		ui.Component("shelf", nil, &menuShelf{NoMenu: p.NoMenu}))
}

// TestActionMenuInTableKept: a component showing a DataTable whose rows'
// menus close on choosing an item (the items read the menu's id from the
// render) is kept across the page's events, its menus as they were; an
// item of a kept menu still runs its handler.
func TestActionMenuInTableKept(t *testing.T) {
	for _, verify := range []bool{true, false} {
		t.Run(map[bool]string{true: "verify", false: "count"}[verify], func(t *testing.T) {
			defer func(v bool) { live.Verify = v }(live.Verify)
			live.Verify = verify
			page := func(noMenu bool) func(context.Context) ui.Node {
				return func(context.Context) ui.Node { return ui.Component("page", nil, &shelfPage{NoMenu: noMenu}) }
			}
			bare, _ := joinV2(t, page(true))
			r0 := live.Renders(bare)
			replyTo(t, bare, clickMsg("c1:0"))
			want := live.Renders(bare) - r0
			bare.Close()
			s, _ := joinV2(t, page(false))
			defer s.Close()
			before := s.HTML()
			if !strings.Contains(before, `popovertargetaction="hide"`) {
				t.Fatalf("items don't close their menu:\n%s", before)
			}
			for i := 1; i <= 3; i++ {
				r0 := live.Renders(s)
				out := string(replyTo(t, s, clickMsg("c1:0")))
				if got := live.Renders(s) - r0; !verify && got != want {
					t.Fatalf("event %d: %d renders, without menus %d: %s", i, got, want, out)
				}
				if strings.Contains(out, `"id":"c2"`) {
					t.Fatalf("event %d patched the kept shelf: %s", i, out)
				}
			}
			after := s.HTML()
			if strings.Replace(after, "n=3", "n=0", 1) != before {
				t.Fatalf("the kept shelf changed:\n%s\n%s", before, after)
			}
			out := string(replyTo(t, s, pushMsg("c2:2:0", "2")))
			if !strings.Contains(s.HTML(), `<p class="removed">2</p>`) {
				t.Fatalf("a kept menu's item: %s\n%s", out, s.HTML())
			}
		})
	}
}
