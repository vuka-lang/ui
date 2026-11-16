package live_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/ui/v2/uigen"
)

// regionOrders is the orders the page and its table read as they render.
type regionOrders struct{ ids []int }

// RegionPage holds a filter dialog the visitor opens, a summary of its
// own markup and the orders table, a component.
type RegionPage struct {
	ui.Live
	Page   int
	db     *regionOrders `vuka:"-"`
	mounts *int          `vuka:"-"`
	open   uigen.Assign[bool]
}

func (p *RegionPage) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }
func (p *RegionPage) mount(context.Context) error { *p.mounts++; return nil }
func (p *RegionPage) OpenFilter()                 { p.open.Set(true) }

func (p *RegionPage) Render() ui.Node {
	b := func(id string, ch any) ui.Node {
		return ui.El("button", []ui.Attr{a("id", id), a("onClick", ui.On(ch))}, ui.Text(id))
	}
	dialog := ui.Node(ui.Text(""))
	if p.open.Get() {
		dialog = ui.El("dialog", []ui.Attr{a("id", "filter"), a("open", "")}, ui.Text("filter"))
	}
	return ui.El("main", nil,
		b("open-filter", p.OpenFilter),
		b("summary-reload", ui.JS.Reload("#summary")),
		b("table-reload", ui.JS.Reload("#orders")),
		b("page-reload", ui.JS.Reload()),
		b("nowhere", ui.JS.Reload("#nope")),
		ui.El("p", []ui.Attr{a("id", "mounts")}, ui.Child(*p.mounts)),
		ui.El("section", []ui.Attr{a("id", "summary")}, ui.Textf("%d orders", len(p.db.ids))),
		dialog,
		ui.Component("orders.table", nil, &RegionTable{DB: p.db}))
}

// RegionTable is the orders table: its root carries the region's id.
type RegionTable struct {
	ui.Live
	DB *regionOrders
}

func (t *RegionTable) Delete(id int) {
	t.DB.ids = slices.DeleteFunc(t.DB.ids, func(x int) bool { return x == id })
}
func (t *RegionTable) Archive(id int) ui.Commands {
	t.Delete(id)
	return ui.JS.Reload("#orders")
}

func (t *RegionTable) Render() ui.Node {
	rows := []ui.Node{}
	for _, id := range t.DB.ids {
		rows = append(rows, ui.El("tr", []ui.Attr{a("key", id)}, ui.El("td", nil, ui.Textf("order %d", id)),
			ui.El("td", nil,
				ui.El("button", []ui.Attr{a("class", "delete"), a("onClick", ui.On(ui.JS.Push(t.Delete, id).Success(ui.JS.Reload("#orders"))))}),
				ui.El("button", []ui.Attr{a("class", "archive"), a("onClick", ui.On(ui.JS.Push(t.Archive, id)))}))))
	}
	return ui.El("table", []ui.Attr{a("id", "orders")}, ui.El("tbody", nil, rows...))
}

func regionRouter(db *regionOrders, mounts *int) live.Router {
	return func(u string) live.Route {
		p := &RegionPage{Page: 1, db: db, mounts: mounts}
		return live.Route{Render: func(context.Context) ui.Node {
			return &ui.ComponentNode{Site: "orders", Props: p, Routed: true}
		}}
	}
}

// ordersChanged is a broadcast's message: sel is the region whose render
// the watcher's Update asks to carry in the same reply.
type ordersChanged struct{ sel string }

// ReplyWatcher hears of new orders beside the table and has a region
// render again in the reply itself (ui.Live.Reload), with no round trip of
// the browser's.
type ReplyWatcher struct{ ui.Live }

func (w *ReplyWatcher) LiveSchema(l *ui.LiveSchema) { l.Mount(w.mount) }
func (w *ReplyWatcher) mount(context.Context) error { w.Subscribe("orders"); return nil }
func (w *ReplyWatcher) Update(m ordersChanged) {
	if m.sel != "" {
		w.Reload(m.sel)
	}
}
func (w *ReplyWatcher) Render() ui.Node { return ui.El("span", nil) }

// replyOrders is what WatchedTable lists: read from a package variable, so
// the table has no props, and with no state of its own, is skipped by the
// page's renders until something renders its region.
var replyOrders = &regionOrders{}

// WatchedTable is the region: no props, no state.
type WatchedTable struct{ ui.Live }

func (t *WatchedTable) Render() ui.Node {
	rows := []ui.Node{}
	for _, id := range replyOrders.ids {
		rows = append(rows, ui.El("tr", []ui.Attr{a("key", id)}, ui.El("td", nil, ui.Textf("order %d", id))))
	}
	return ui.El("table", []ui.Attr{a("id", "orders")}, ui.El("tbody", nil, rows...))
}

// WatchedPage holds its summary in its own markup, the watcher and the
// table, a component of its own.
type WatchedPage struct {
	ui.Live
	mounts *int `vuka:"-"`
}

func (p *WatchedPage) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }
func (p *WatchedPage) mount(context.Context) error { *p.mounts++; return nil }
func (p *WatchedPage) Added(ctx context.Context) error {
	replyOrders.ids = append(replyOrders.ids, len(replyOrders.ids)+1)
	p.Reload("#orders")
	return nil
}

func (p *WatchedPage) Render() ui.Node {
	return ui.El("main", nil,
		ui.El("button", []ui.Attr{a("id", "add"), a("onClick", ui.On(ui.JS.Push(p.Added)))}, ui.Text("add")),
		ui.El("p", []ui.Attr{a("id", "mounts")}, ui.Child(*p.mounts)),
		ui.El("section", []ui.Attr{a("id", "summary")}, ui.Textf("%d orders", len(replyOrders.ids))),
		ui.Component("orders.watched", nil, &ReplyWatcher{}),
		ui.Component("orders.table", nil, &WatchedTable{}))
}

// TestReloadInReply: a handler — an event's, or a broadcast's Update with
// no event from the browser — has a region's render carried in its own
// reply with ui.Live.Reload: the patch renders the instance whose render
// has the element, mounted as it is, and pushes no command, so no round
// trip follows. A region in the page's own markup renders the page
// instance; an id the render hasn't falls back to the pushed command,
// which the browser reports.
func TestReloadInReply(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = false // the page and the table read the orders, not state
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			replyOrders.ids = []int{1}
			mounts := 0
			s := live.NewSession(context.Background(), func(context.Context) ui.Node {
				return &ui.ComponentNode{Site: "watched", Props: &WatchedPage{mounts: &mounts}, Routed: true}
			})
			c, err := livetest.Join(s, v)
			if err != nil {
				t.Fatal(err)
			}
			rows := func() int { return strings.Count(c.HTML(), "<tr ") }

			replyOrders.ids = append(replyOrders.ids, 2) // added elsewhere
			if err := c.Info("orders", ordersChanged{}); err != nil {
				t.Fatal(err)
			}
			if rows() != 1 {
				t.Fatalf("a broadcast without the reload rendered the region: %d rows\n%s", rows(), c.HTML())
			}
			if err := c.Info("orders", ordersChanged{sel: "#orders"}); err != nil {
				t.Fatal(err)
			}
			if rows() != 2 || c.Text("#mounts") != "1" {
				t.Fatalf("the table in the broadcast's reply: %d rows\n%s", rows(), c.HTML())
			}
			replyOrders.ids = append(replyOrders.ids, 3)
			if err := c.Info("orders", ordersChanged{sel: "#summary"}); err != nil {
				t.Fatal(err)
			}
			if c.Text("#summary") != "3 orders" || rows() != 3 || c.Text("#mounts") != "1" {
				t.Fatalf("the page's own region (its children with it): %q, %d rows, mounts %q", c.Text("#summary"), rows(), c.Text("#mounts"))
			}

			if err := c.Click("#add"); err != nil {
				t.Fatal(err)
			}
			if rows() != 4 || c.Text("#mounts") != "1" {
				t.Fatalf("an event's reply: %d rows\n%s", rows(), c.HTML())
			}
			if c.HTML() != s.HTML() {
				t.Fatal("the client and server differ")
			}

			// The reply itself carries the render and pushes nothing.
			replyOrders.ids = append(replyOrders.ids, 9)
			p, err := s.Info("orders", ordersChanged{sel: "#orders"})
			if err != nil {
				t.Fatal(err)
			}
			if len(p.JS) != 0 || len(p.Updates)+len(p.Trees) == 0 {
				t.Fatalf("the patch: %d updates, %d trees, %d pushes", len(p.Updates), len(p.Trees), len(p.JS))
			}
			if err := c.Apply(live.Marshal(live.PatchMessage(0, p))); err != nil {
				t.Fatal(err)
			}
			if rows() != 5 {
				t.Fatalf("applied: %d rows", rows())
			}

			if err := c.Info("orders", ordersChanged{sel: "#nope"}); err == nil || !strings.Contains(err.Error(), `Reload("#nope")`) {
				t.Fatalf("an id the render hasn't: %v", err)
			}
		})
	}
}

// TestReloadRegion: a region reload renders the instance the element is in
// again, mounted as it is: the table shows rows added since, and the
// dialog the visitor opened stays open — a whole reload mounts the page
// again and closes it. A region in the page's own markup renders the page
// instance again, its state kept. No element with the id is an error.
func TestReloadRegion(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = false // the page and the table read the orders, not state
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			db, mounts := &regionOrders{ids: []int{1, 2}}, 0
			router := regionRouter(db, &mounts)
			s := live.NewSession(context.Background(), router("/orders").Render)
			s.SetRouter(router)
			c, err := livetest.Join(s, v)
			if err != nil {
				t.Fatal(err)
			}
			c.SetURL("/orders")
			rows := func() int { return strings.Count(c.HTML(), "<tr ") }
			check := func(what string, wantRows int, summary string, open bool) {
				t.Helper()
				if rows() != wantRows || c.Text("#summary") != summary || c.Exists("#filter") != open || c.Text("#mounts") != "1" {
					t.Fatalf("%s: %d rows, %q, dialog %v, mounts %s\n%s", what, rows(), c.Text("#summary"), c.Exists("#filter"), c.Text("#mounts"), c.HTML())
				}
				if c.HTML() != s.HTML() {
					t.Fatalf("%s: the client and server differ", what)
				}
			}
			if err := c.Click("#open-filter"); err != nil {
				t.Fatal(err)
			}
			check("opened", 2, "2 orders", true)

			db.ids = append(db.ids, 3) // added elsewhere
			if err := c.Click("#table-reload"); err != nil {
				t.Fatal(err)
			}
			check("the table reloaded", 3, "2 orders", true)
			if err := c.Click("#summary-reload"); err != nil {
				t.Fatal(err)
			}
			check("the summary reloaded", 3, "3 orders", true)

			if err := c.Click("tr .delete"); err != nil {
				t.Fatal(err)
			}
			check("deleted, the table reloaded in Success", 2, "3 orders", true)
			if err := c.Click("tr .archive"); err != nil {
				t.Fatal(err)
			}
			check("archived, the reload returned", 1, "3 orders", true)

			if err := c.Click("#nowhere"); err == nil || !strings.Contains(err.Error(), `Reload("#nope")`) {
				t.Fatalf("no element with the id: %v", err)
			}

			if err := c.Click("#page-reload"); err != nil {
				t.Fatal(err)
			}
			if c.Exists("#filter") || c.Text("#mounts") != "2" || c.Text("#summary") != "1 orders" {
				t.Fatalf("a whole reload mounts the page again: %s", c.HTML())
			}
		})
	}
}
