package live_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// orderTable is what OrderRows shows: the orders' ids, and whether a
// delete fails or does nothing.
type orderTable struct {
	ids        []int
	fail, noop bool
}

// OrderRows is a table of keyed rows, each with a button removing it at
// once and deleting the order.
type OrderRows struct {
	ui.Live
	DB    *orderTable
	infos int
}

func (p *OrderRows) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }
func (p *OrderRows) mount(context.Context) error { p.Subscribe("orders"); return nil }
func (p *OrderRows) UpdateAny(msg any)           { p.infos++ }

func (p *OrderRows) Delete(id int) error {
	if p.DB.fail {
		return errors.New("still shipping")
	}
	if !p.DB.noop {
		p.DB.ids = slices.DeleteFunc(p.DB.ids, func(x int) bool { return x == id })
	}
	return nil
}
func (p *OrderRows) Dismiss() ui.Commands { return ui.JS.Remove("#notice") }
func (p *OrderRows) Touch()               {}

func (p *OrderRows) Render() ui.Node {
	rows := make([]ui.Node, 0, len(p.DB.ids))
	for _, id := range p.DB.ids {
		row := func(name string, ch ui.JSChain) ui.Node {
			return ui.El("button", []ui.Attr{a("class", name), a("onClick", ui.On(ch))}, ui.Text(name))
		}
		rows = append(rows, ui.El("tr", []ui.Attr{a("id", "row-"+strconv.Itoa(id)), a("key", id)},
			ui.El("td", nil, ui.Textf("order %d", id)),
			ui.El("td", nil,
				row("delete", ui.JS.Push(p.Delete, id).Before(ui.JS.Remove(ui.Closest("tr"))).Error(ui.JS.SetText("#err", ui.LoadError))),
				row("keep", ui.JS.Remove(ui.Closest("tr")).Push(p.Delete, id, ui.KeepOnError())),
				row("slow", ui.JS.On(ui.Closest("tr")).Remove().Push(p.Delete, id, ui.Timeout(time.Second))))))
	}
	return ui.El("main", nil,
		ui.El("p", []ui.Attr{a("id", "notice")}, ui.Text("welcome")),
		ui.El("p", []ui.Attr{a("id", "err")}),
		ui.El("p", []ui.Attr{a("id", "infos")}, ui.Child(p.infos)),
		ui.El("button", []ui.Attr{a("id", "dismiss"), a("onClick", ui.On(ui.JS.Remove("#notice")))}),
		ui.El("button", []ui.Attr{a("id", "dismissed"), a("onClick", ui.On(ui.JS.Push(p.Dismiss)))}),
		ui.El("button", []ui.Attr{a("id", "touch"), a("onClick", ui.On(p.Touch))}),
		ui.El("table", nil, ui.El("tbody", nil, rows...)))
}

func joinOrderRows(t *testing.T, v int) (*livetest.Client, *orderTable, *live.Session) {
	t.Helper()
	db := &orderTable{ids: []int{1, 2, 3}}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("orders", nil, &OrderRows{DB: db}) })
	c, err := livetest.Join(s, v)
	if err != nil {
		t.Fatal(err)
	}
	return c, db, s
}

// TestRemove: a remove ahead of a push, or in its Before, takes the row off
// at once; the push succeeding leaves it off, failing (an error, no reply
// in time) puts it back before Error runs, and KeepOnError keeps it off. A
// render while the push waits leaves it off; once the push has succeeded
// the server's render is the page's. Outside a push, or returned by a
// handler, it is a plain removal.
func TestRemove(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = false // the rows are read from the table, not state
	rows := func(c *livetest.Client) string {
		var out []string
		for _, id := range []int{1, 2, 3} {
			if c.Exists("#row-" + strconv.Itoa(id)) {
				out = append(out, strconv.Itoa(id))
			}
		}
		return fmt.Sprint(out)
	}
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			t.Run("success", func(t *testing.T) {
				c, db, _ := joinOrderRows(t, v)
				c.Hold()
				if err := c.Click("#row-2 .delete"); err != nil {
					t.Fatal(err)
				}
				if got := rows(c); got != "[1 3]" || c.Pending() != 1 {
					t.Fatalf("before the reply: %s, %d pending", got, c.Pending())
				}
				if err := c.Info("orders", nil); err != nil { // a render while the push waits
					t.Fatal(err)
				}
				if got := rows(c); got != "[1 3]" || c.Text("#infos") != "1" {
					t.Fatalf("a render while waiting brought it back: %s, %s", got, c.HTML())
				}
				if err := c.Release(); err != nil {
					t.Fatal(err)
				}
				if got := rows(c); got != "[1 3]" || fmt.Sprint(db.ids) != "[1 3]" {
					t.Fatalf("after the reply: %s, %v", got, db.ids)
				}
			})
			t.Run("error", func(t *testing.T) {
				c, db, _ := joinOrderRows(t, v)
				db.fail = true
				c.Hold()
				if err := c.Click("#row-2 .delete"); err != nil {
					t.Fatal(err)
				}
				if got := rows(c); got != "[1 3]" {
					t.Fatalf("before the reply: %s", got)
				}
				c.Release()
				if got := rows(c); got != "[1 2 3]" || c.Text("#err") != "still shipping" {
					t.Fatalf("after the error: %s, %q", got, c.Text("#err"))
				}
				if err := c.Click("#touch"); err != nil || rows(c) != "[1 2 3]" {
					t.Fatalf("the next render: %v %s", err, rows(c))
				}
			})
			t.Run("keep on error", func(t *testing.T) {
				c, db, _ := joinOrderRows(t, v)
				db.fail = true
				c.Click("#row-2 .keep")
				if got := rows(c); got != "[1 3]" {
					t.Fatalf("KeepOnError: %s", got)
				}
			})
			t.Run("timeout", func(t *testing.T) {
				c, _, _ := joinOrderRows(t, v)
				c.Hold()
				c.Click("#row-3 .slow")
				if got := rows(c); got != "[1 2]" {
					t.Fatalf("before the timeout: %s", got)
				}
				if err := c.Advance(2 * time.Second); err != nil {
					t.Fatal(err)
				}
				if got := rows(c); got != "[1 2 3]" {
					t.Fatalf("after the timeout: %s", got)
				}
			})
			t.Run("success the server doesn't render", func(t *testing.T) {
				c, db, _ := joinOrderRows(t, v)
				db.noop = true
				c.Click("#row-1 .delete")
				if got := rows(c); got != "[1 2 3]" {
					t.Fatalf("the server still renders the row: %s", got)
				}
			})
			t.Run("plain", func(t *testing.T) {
				c, _, _ := joinOrderRows(t, v)
				c.Click("#dismiss")
				if c.Exists("#notice") {
					t.Fatal("the notice is still there")
				}
				c.Click("#touch")
				if c.Exists("#notice") {
					t.Fatal("a render brought the notice back")
				}
				d, _, _ := joinOrderRows(t, v)
				if err := d.Click("#dismissed"); err != nil || d.Exists("#notice") {
					t.Fatalf("a handler's returned Remove: %v %v", err, d.Exists("#notice"))
				}
			})
		})
	}
}
