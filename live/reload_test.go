package live_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/ui/v2/uigen"
)

// orderRows is the orders' table, kept outside the pages: what a
// database is to them.
type orderRows struct{ rows []string }

// OrderList is a page whose state is all Assigns: a handler that only
// writes to orderRows leaves it unrendered.
type OrderList struct {
	ui.Live
	Page   int
	rows   *orderRows `vuka:"-"`
	mounts *int       `vuka:"-"`
	open   ui.State[bool]
}

type orderListParams struct{ Page int }

func (p *OrderList) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }
func (p *OrderList) mount(context.Context) error {
	*p.mounts++
	p.open = uigen.State("orders.open", true)
	return nil
}
func (p *OrderList) Update(orderListParams) {}

func (p *OrderList) Touch() { p.rows.rows = append(p.rows.rows, "touched") }
func (p *OrderList) Add() (ui.Commands, error) {
	p.rows.rows = append(p.rows.rows, "added")
	return ui.JS.Reload(), nil
}
func (p *OrderList) Close() ui.Commands {
	p.rows.rows = append(p.rows.rows, "closed")
	return p.open.Set(false).Reload()
}

func (p *OrderList) Render() ui.Node {
	b := func(id string, ch ui.Commands) ui.Node {
		return ui.El("button", []ui.Attr{a("id", id), a("onClick", ui.On(ch))}, ui.Text(id))
	}
	return ui.El("main", nil,
		ui.El("p", []ui.Attr{a("id", "rows")}, ui.Text(strings.Join(p.rows.rows, ","))),
		ui.El("p", []ui.Attr{a("id", "at")}, ui.Textf("page %d, mounts %d", p.Page, *p.mounts)),
		ui.El("div", []ui.Attr{a("id", "panel"), a("hidden", ui.BindExpr(!p.open.Get(), `["!",{"$":0}]`, p.open))}),
		b("stale", ui.JS.Push(p.Touch)),
		b("touch", ui.JS.Push(p.Touch).Success(ui.JS.Reload())),
		b("add", ui.JS.Push(p.Add)),
		b("close", ui.JS.Push(p.Close)),
		ui.Component("orders.form", nil, &OrderForm{Rows: p.rows}))
}

// OrderForm is a child of the page: it saves a row the page shows.
type OrderForm struct {
	ui.Live
	Rows *orderRows
}

func (f *OrderForm) Save() (ui.Commands, error) {
	f.Rows.rows = append(f.Rows.rows, "saved")
	return ui.JS.Reload(), nil
}

func (f *OrderForm) Render() ui.Node {
	return ui.El("form", nil, ui.El("button", []ui.Attr{a("id", "save"), a("onClick", ui.On(ui.JS.Push(f.Save)))}, ui.Text("save")))
}

// ordersRouter routes /orders?page=N to an OrderList, which stays mounted
// on a patch when update is set (its Update takes the URL).
func ordersRouter(rows *orderRows, mounts *int, update bool) live.Router {
	return func(u string) live.Route {
		path, q, _ := strings.Cut(u, "?")
		if path != "/orders" {
			return live.Route{}
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(q, "page="))
		p := &OrderList{Page: n, rows: rows, mounts: mounts}
		rt := live.Route{Render: func(context.Context) ui.Node {
			return ui.El("div", nil, &ui.ComponentNode{Site: "orders", Props: p, Routed: true, URL: []string{"Page"}})
		}}
		if update {
			rt.Updates = map[ui.Stateful]any{p: orderListParams{n}}
		}
		return rt
	}
}

// TestReload: a reload renders the page again at its URL, though none of
// its tracked state changed, from a push's Success, from a handler's
// returned commands, with a ui.State write before it, and from a child
// component's handler; the history entry is replaced.
func TestReload(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = false // the page reads orderRows, which isn't an Assign
	for _, v := range []int{1, 2} {
		for _, update := range []bool{true, false} {
			t.Run(fmt.Sprintf("v%d update=%v", v, update), func(t *testing.T) {
				rows, mounts := &orderRows{}, 0
				router := ordersRouter(rows, &mounts, update)
				s := live.NewSession(context.Background(), router("/orders?page=2").Render)
				s.SetRouter(router)
				c, err := livetest.Join(s, v)
				if err != nil {
					t.Fatal(err)
				}
				c.SetURL("/orders?page=2")
				want := func(rowsText string, wantMounts int) {
					t.Helper()
					if got := c.Text("#rows"); got != rowsText {
						t.Fatalf("rows %q, want %q; page %s", got, rowsText, c.HTML())
					}
					if got, at := c.Text("#at"), fmt.Sprintf("page 2, mounts %d", wantMounts); got != at {
						t.Fatalf("%q, want %q", got, at)
					}
					if h, i := c.History(); c.URL() != "/orders?page=2" || len(h) != 1 || i != 0 {
						t.Fatalf("at %s, history %v %d: a reload replaces the entry", c.URL(), h, i)
					}
					if c.HTML() != s.HTML() {
						t.Fatalf("client and server differ:\n%s\n%s", c.HTML(), s.HTML())
					}
				}
				remount := 0
				if !update {
					remount = 1 // a page whose Update doesn't take the URL mounts again
				}
				if err := c.Click("#stale"); err != nil {
					t.Fatal(err)
				}
				want("", 1) // no reload: the page isn't rendered again
				if err := c.Click("#touch"); err != nil {
					t.Fatal(err)
				}
				want("touched,touched", 1+remount)
				if err := c.Click("#add"); err != nil {
					t.Fatal(err)
				}
				want("touched,touched,added", 1+2*remount)
				if err := c.Click("#close"); err != nil {
					t.Fatal(err)
				}
				want("touched,touched,added,closed", 1+3*remount)
				if err := c.Click("#save"); err != nil {
					t.Fatal(err)
				}
				want("touched,touched,added,closed,saved", 1+4*remount)
			})
		}
	}
}

// TestReloadNeedsURL: livetest reloads the URL the client holds.
func TestReloadNeedsURL(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = false
	rows, mounts := &orderRows{}, 0
	router := ordersRouter(rows, &mounts, true)
	s := live.NewSession(context.Background(), router("/orders?page=2").Render)
	s.SetRouter(router)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Click("#add"); err == nil || !strings.Contains(err.Error(), "SetURL") {
		t.Fatalf("a reload without a URL: %v", err)
	}
}
