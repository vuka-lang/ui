package live_test

import (
	"context"
	"errors"
	"html"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/internal/demo"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/ui/v2/uigen"
)

// row is an item of a keyed list: Label says which row it is, whatever its key.
type row struct {
	Key   any
	Label string
}

// rowsState is what a KeyedRows renders and what its rows' clicks hit. The test
// changes it between events, as another visitor's change would.
type rowsState struct {
	rows []row
	hits []string
}

// KeyedRows is a list of keyed rows in plain Go: a refresh button (rendering
// again with what the test changed), its rows, each with a hit button, and
// one more button after them.
type KeyedRows struct {
	ui.Live
	St *rowsState
}

func (l *KeyedRows) Refresh() {}
func (l *KeyedRows) After()   { l.St.hits = append(l.St.hits, "after") }
func (l *KeyedRows) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{a("onClick", ui.On(l.Refresh))}, ui.Text("refresh")),
		ui.El("ul", nil, ui.Each(l.St.rows, func(r row) ui.Node { return l.item(r) })),
		ui.El("button", []ui.Attr{a("onClick", ui.On(l.After))}, ui.Text("after")))
}

func (l *KeyedRows) item(r row) ui.Node {
	return ui.El("li", []ui.Attr{a("key", r.Key)},
		ui.El("button", []ui.Attr{a("onClick", ui.On(func() { l.St.hits = append(l.St.hits, r.Label) }))}, ui.Text("hit "+r.Label)))
}

func rowsPage(st *rowsState) func(context.Context) ui.Node {
	return func(context.Context) ui.Node { return ui.Component("rows", nil, &KeyedRows{St: st}) }
}

func rowsOf(keys ...any) []row {
	out := make([]row, len(keys))
	for i, k := range keys {
		out[i] = row{k, "r" + strconv.Itoa(i)}
	}
	return out
}

// idOf is the handler id on the button labelled label in the page's HTML.
func idOf(t *testing.T, h, label string) string {
	t.Helper()
	m := regexp.MustCompile(`data-vk-on-click="([^"]*)"[^>]*>(?:<!--.*?-->)?` + regexp.QuoteMeta(label) + `<`).FindStringSubmatch(h)
	if m == nil {
		t.Fatalf("no button %q in %s", label, h)
	}
	return html.UnescapeString(m[1])
}

func clickID(c *livetest.Client, id string) (live.ServerMessage, error) {
	return c.Event(ui.Event{Target: id, Type: "click"})
}

// stale sends a click on id the way a browser that hasn't received the last
// patch does: straight to the session, after it rendered.
func stale(s *live.Session, id string) error {
	_, err := s.Event(ui.Event{Target: id, Type: "click"})
	return err
}

func refresh(t *testing.T, c *livetest.Client) {
	t.Helper()
	if m, err := clickID(c, idOf(t, c.HTML(), "refresh")); err != nil || m.Error != "" {
		t.Fatalf("refresh: %+v %v", m, err)
	}
	if c.HTML() != c.S.HTML() {
		t.Fatalf("client and server differ:\n%s\n%s", c.HTML(), c.S.HTML())
	}
}

func eachVersion(t *testing.T, f func(t *testing.T, v int)) {
	for _, v := range []int{1, 2} {
		t.Run("v"+strconv.Itoa(v), func(t *testing.T) { f(t, v) })
	}
}

func joinRows(t *testing.T, st *rowsState, v int) *livetest.Client {
	t.Helper()
	c, err := livetest.Join(live.NewSession(context.Background(), rowsPage(st)), v)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestKeyedClickRacedByPatch is a click on a row sent just before a patch
// inserting or removing rows: it reaches that row, or fails as a no-handler
// error, and never another row. The rows are the compiled shape's, the
// browser's copy never hearing of the patch.
func TestKeyedClickRacedByPatch(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		// c1's handlers outside its rows: insert, pop.
		join := func(t *testing.T) (*livetest.Client, *[]int) {
			hits := &[]int{}
			c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node {
				return ui.Component("keyed", nil, &demo.KeyedTable{N: 5, Hits: hits})
			}), v)
			if err != nil {
				t.Fatal(err)
			}
			return c, hits
		}
		t.Run("insert before it", func(t *testing.T) {
			c, hits := join(t)
			id := idOf(t, c.HTML(), "hit 3")
			for range 3 {
				if _, err := clickID(c, idOf(t, c.HTML(), "insert")); err != nil {
					t.Fatal(err)
				}
			}
			if err := stale(c.S, id); err != nil {
				t.Fatal(err)
			}
			if len(*hits) != 1 || (*hits)[0] != 3 {
				t.Fatalf("hits %v, want row 3", *hits)
			}
		})
		t.Run("remove before it", func(t *testing.T) {
			c, hits := join(t)
			id := idOf(t, c.HTML(), "hit 3")
			if _, err := clickID(c, idOf(t, c.HTML(), "del 1")); err != nil {
				t.Fatal(err)
			}
			if err := stale(c.S, id); err != nil {
				t.Fatal(err)
			}
			if len(*hits) != 1 || (*hits)[0] != 3 {
				t.Fatalf("hits %v, want row 3", *hits)
			}
		})
		t.Run("itself removed", func(t *testing.T) {
			c, hits := join(t)
			id := idOf(t, c.HTML(), "hit 3")
			if _, err := clickID(c, idOf(t, c.HTML(), "del 3")); err != nil {
				t.Fatal(err)
			}
			err := stale(c.S, id)
			var no *live.NoHandlerError
			if !errors.As(err, &no) {
				t.Fatalf("err = %v, want a no-handler error", err)
			}
			if len(*hits) != 0 {
				t.Fatalf("hits %v, want none", *hits)
			}
		})
	})
}

// TestKeyedInsertSendsNoLaterIDs: inserting a row at the top sends the new
// row, and none of the ids of the rows after it.
func TestKeyedInsertSendsNoLaterIDs(t *testing.T) {
	var hits []int
	c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("keyed", nil, &demo.KeyedTable{N: 200, Hits: &hits})
	}), 2)
	if err != nil {
		t.Fatal(err)
	}
	before := idOf(t, c.HTML(), "hit 150")
	m, err := clickID(c, idOf(t, c.HTML(), "insert"))
	if err != nil {
		t.Fatal(err)
	}
	reply := string(live.Marshal(m))
	if strings.Contains(reply, "c1:199") || strings.Count(reply, "c1:") != 2 || len(reply) > 300 {
		t.Fatalf("the insert carries more than its row's two ids: %s", reply)
	}
	if after := idOf(t, c.HTML(), "hit 150"); after != before || c.HTML() != c.S.HTML() {
		t.Fatalf("hit 150's id went from %s to %s", before, after)
	}
}

// TestKeyedOutsideHandlersKeepPosition: handlers outside any key count among
// themselves, the rows between them no matter.
func TestKeyedOutsideHandlersKeepPosition(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		st := &rowsState{rows: rowsOf(1, 2, 3)}
		c := joinRows(t, st, v)
		if r, after := idOf(t, c.HTML(), "refresh"), idOf(t, c.HTML(), "after"); r != "c1:0" || after != "c1:1" {
			t.Fatalf("refresh %s, after %s", r, after)
		}
		if id := idOf(t, c.HTML(), "hit r1"); id != "c1:2:0" {
			t.Fatalf("a keyed row's id is %s", id)
		}
		st.rows = rowsOf(9, 8, 7, 1, 2, 3)
		refresh(t, c)
		if after := idOf(t, c.HTML(), "after"); after != "c1:1" {
			t.Fatalf("after moved to %s", after)
		}
		if err := stale(c.S, "c1:1"); err != nil || st.hits[0] != "after" {
			t.Fatalf("%v %v", err, st.hits)
		}
	})
}

// hostileKeys are keys an app's data can hold, with ones meant to write like
// another's id.
var hostileKeys = []any{
	"a:b", "a:b:0", "a", "b", "a/b", "a%3Ab", `"`, `"><script>alert(1)</script>`, "'", "<", ">", "&", "&amp;", "a b", " ",
	"\x00", "\n", "é", "日本語", "\xff\xfe", "", "%", "%25", "~", "~7", "~~", "!", "!0", "^1", "7", "07", "-7", 7, -7, int64(7), uint8(7),
	7.5, true, "true", "c1:0", "c2:0", "Refresh", "After", "Render", "0p", "5p", "_set",
	strings.Repeat("k", 48), strings.Repeat("k", 49), strings.Repeat("k", 5000), strings.Repeat("k", 4999) + "x", strings.Repeat(":", 5000),
	"id-ABCDEFGHIJKLMNOPQRSTUVWXYZ", "id-", "xid-y", struct{ A, B int }{1, 2}, [2]string{"a", "b"},
}

var safeID = regexp.MustCompile(`^c\d+:[A-Za-z0-9_.~!^%/-]+:\d+$`)

// TestKeyedHostileKeys: whatever a key holds, its id needs no escaping, is
// no other key's, and a click on it reaches its row.
func TestKeyedHostileKeys(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		st := &rowsState{rows: rowsOf(hostileKeys...)}
		c := joinRows(t, st, v)
		seen := map[string]bool{}
		for i := range hostileKeys {
			label := "r" + strconv.Itoa(i)
			id := idOf(t, c.HTML(), "hit "+label)
			if !safeID.MatchString(id) || len(id) > 64 {
				t.Errorf("key %q: id %q", hostileKeys[i], id)
			}
			if seen[id] {
				t.Errorf("key %q: id %q is another's", hostileKeys[i], id)
			}
			seen[id] = true
			st.hits = nil
			if m, err := clickID(c, id); err != nil || m.Error != "" || len(st.hits) != 1 || st.hits[0] != label {
				t.Errorf("key %q: %q reached %v: %+v %v", hostileKeys[i], id, st.hits, m, err)
			}
		}
	})
}

// TestKeyedNested: the same inner key under two outer ones is two rows.
func TestKeyedNested(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		st := &groupsState{groups: map[string][]string{"a": {"1", "2"}, "b": {"1", "2"}}, order: []string{"a", "b"}}
		c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node {
			return ui.Component("groups", nil, &Groups{St: st})
		}), v)
		if err != nil {
			t.Fatal(err)
		}
		id := idOf(t, c.HTML(), "b/1")
		if strings.Count(id, "/") != 1 {
			t.Fatalf("id %s", id)
		}
		// a group and a row inserted before it
		st.groups["z"], st.order = []string{"1"}, []string{"z", "a", "b"}
		st.groups["b"] = []string{"0", "1", "2"}
		if _, err := clickID(c, idOf(t, c.HTML(), "refresh")); err != nil {
			t.Fatal(err)
		}
		if err := stale(c.S, id); err != nil {
			t.Fatal(err)
		}
		if len(st.hits) != 1 || st.hits[0] != "b/1" {
			t.Fatalf("hits %v", st.hits)
		}
		// the group's own handler is under its key alone
		if g := idOf(t, c.HTML(), "group b"); strings.Contains(g, "/") {
			t.Fatalf("group id %s", g)
		}
	})
}

type groupsState struct {
	order  []string
	groups map[string][]string
	hits   []string
}

type Groups struct {
	ui.Live
	St *groupsState
}

func (g *Groups) Refresh() {}
func (g *Groups) Render() ui.Node {
	return ui.El("div", nil, ui.El("button", []ui.Attr{a("onClick", ui.On(g.Refresh))}, ui.Text("refresh")),
		ui.Each(g.St.order, func(name string) ui.Node {
			hit := func(label string) ui.Node {
				return ui.El("button", []ui.Attr{a("onClick", ui.On(func() { g.St.hits = append(g.St.hits, label) }))}, ui.Text(label))
			}
			return ui.El("section", []ui.Attr{a("key", name)}, hit("group "+name),
				ui.Each(g.St.groups[name], func(r string) ui.Node {
					return ui.El("p", []ui.Attr{a("key", r)}, hit(name+"/"+r))
				}))
		}))
}

// TestKeyedDuplicates: rows sharing a key are told apart by position, as the
// morph pairs them, and a click raced by a patch changing which rows there
// are fails rather than reach the other.
func TestKeyedDuplicates(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		st := &rowsState{rows: []row{{"k", "a"}, {"k", "b"}, {"x", "x"}}}
		c := joinRows(t, st, v)
		ida, idb := idOf(t, c.HTML(), "hit a"), idOf(t, c.HTML(), "hit b")
		if ida == idb {
			t.Fatalf("both are %s", ida)
		}
		for _, click := range []struct{ id, want string }{{ida, "a"}, {idb, "b"}} {
			st.hits = nil
			if m, err := clickID(c, click.id); err != nil || m.Error != "" || len(st.hits) != 1 || st.hits[0] != click.want {
				t.Fatalf("%s reached %v: %+v %v", click.id, st.hits, m, err)
			}
		}
		// Rendering again changes no id: typing in such a row goes on working.
		refresh(t, c)
		if idOf(t, c.HTML(), "hit a") != ida || idOf(t, c.HTML(), "hit b") != idb {
			t.Fatalf("ids moved from %s %s:\n%s", ida, idb, c.HTML())
		}
		idx := idOf(t, c.HTML(), "hit x")
		// a is gone: its id must not reach b, now the first of the key.
		st.hits, st.rows = nil, st.rows[1:]
		refresh(t, c)
		var no *live.NoHandlerError
		if err := stale(c.S, ida); !errors.As(err, &no) {
			t.Fatalf("a's id: %v, hits %v", err, st.hits)
		}
		if err := stale(c.S, idb); !errors.As(err, &no) {
			t.Fatalf("b's old id: %v, hits %v", err, st.hits)
		}
		if err := stale(c.S, idx); err != nil || len(st.hits) != 1 || st.hits[0] != "x" {
			t.Fatalf("x's id, a unique key's, keeps working: %v, hits %v", err, st.hits)
		}
		st.hits = nil
		if m, err := clickID(c, idOf(t, c.HTML(), "hit b")); err != nil || m.Error != "" || len(st.hits) != 1 || st.hits[0] != "b" {
			t.Fatalf("b: %v %v %+v", st.hits, err, m)
		}
	})
}

// TestKeyedDuplicateAppears: a row inserted with another's key, before it: the
// click on the row that had the key alone must not reach the new one, though
// the new one is first met with the key still unique.
func TestKeyedDuplicateAppears(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		st := &rowsState{rows: []row{{"k", "old"}}}
		c := joinRows(t, st, v)
		id := idOf(t, c.HTML(), "hit old")
		st.rows = []row{{"k", "new"}, {"k", "old"}}
		refresh(t, c)
		var no *live.NoHandlerError
		if err := stale(c.S, id); !errors.As(err, &no) || len(st.hits) != 0 {
			t.Fatalf("%v, hits %v", err, st.hits)
		}
		for _, label := range []string{"new", "old"} {
			st.hits = nil
			if m, err := clickID(c, idOf(t, c.HTML(), "hit "+label)); err != nil || m.Error != "" || len(st.hits) != 1 || st.hits[0] != label {
				t.Fatalf("%s: %v %v %+v", label, st.hits, err, m)
			}
		}
		// Once the key is gone it can come back as a key, unique again.
		st.rows = []row{{"y", "y"}}
		refresh(t, c)
		st.rows = []row{{"k", "back"}}
		refresh(t, c)
		if id := idOf(t, c.HTML(), "hit back"); !safeID.MatchString(id) || strings.Contains(id, "!") {
			t.Fatalf("id %s", id)
		}
	})
}

// TestKeyedSameKeyTwoLists: a key of two lists names two rows.
func TestKeyedSameKeyTwoLists(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		st := &twoListsState{a: []string{"1", "2"}, b: []string{"1", "2"}}
		c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node {
			return ui.Component("two", nil, &TwoLists{St: st})
		}), v)
		if err != nil {
			t.Fatal(err)
		}
		ida, idb := idOf(t, c.HTML(), "a1"), idOf(t, c.HTML(), "b1")
		if ida == idb {
			t.Fatalf("both %s", ida)
		}
		st.a = []string{"2"}
		if _, err := clickID(c, idOf(t, c.HTML(), "refresh")); err != nil {
			t.Fatal(err)
		}
		if err := stale(c.S, ida); err == nil {
			t.Fatalf("a1's id reached %v", st.hits)
		}
	})
}

type twoListsState struct {
	a, b []string
	hits []string
}

type TwoLists struct {
	ui.Live
	St *twoListsState
}

func (l *TwoLists) Refresh() {}
func (l *TwoLists) Render() ui.Node {
	list := func(name string, keys []string) ui.Node {
		return ui.El("ul", nil, ui.Each(keys, func(k string) ui.Node {
			return ui.El("li", []ui.Attr{a("key", k)}, ui.El("button", []ui.Attr{a("onClick", ui.On(func() { l.St.hits = append(l.St.hits, name+k) }))}, ui.Text(name+k)))
		}))
	}
	return ui.El("div", nil, ui.El("button", []ui.Attr{a("onClick", ui.On(l.Refresh))}, ui.Text("refresh")), list("a", l.St.a), list("b", l.St.b))
}

// TestKeyedForgedTargets: an id the render didn't give out is refused as any
// unknown id is, saying nothing of the keys there are.
func TestKeyedForgedTargets(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		st := &rowsState{rows: []row{{"secret-row", "r"}, {7, "s"}}}
		c := joinRows(t, st, v)
		for _, target := range []string{
			"c1:secret-row:1", "c1:secret-row:-1", "c1:secret-row:00", "c1:secret-row", "c1:secret-row:0:0", "c1:other:0",
			"c1:7:9", "c1:~7:0", "c1:%37:0", "c1::0", "c1:::", "c1:", ":", "", "c1", "c9:7:0", "c0:7:0",
			"c1:!0:0", "c1:!:0", "c1:^1/7:0", "c1:Refresh:0", "c1:Render:0", `c1:"><script>:0`, "c1:\x00:0", "c1:7:0\n",
			"c1:" + strings.Repeat("k", 1<<16) + ":0",
		} {
			m, err := c.S.Event(ui.Event{Target: target, Type: "click"})
			var no *live.NoHandlerError
			if !errors.As(err, &no) {
				t.Fatalf("%.40q: %+v %v", target, m, err)
			}
			public, ok := live.PublicMessage(err)
			if want := "live: no handler " + strconv.Quote(target); !ok || public != want {
				t.Fatalf("%.40q: public %.60q", target, public)
			}
			_, sent, _ := strings.Cut(target, ":") // what the error may say back
			if strings.Contains(strings.ReplaceAll(strings.ReplaceAll(err.Error(), strconv.Quote(target), ""), sent, ""), "secret") {
				t.Fatalf("the error names a key: %s", err)
			}
			if len(st.hits) != 0 {
				t.Fatalf("%.40q reached %v", target, st.hits)
			}
		}
		// A method's name stays no key: refused unless AllowPush allows it.
		if h := c.S.Handler("c1:Refresh"); h != nil {
			t.Fatalf("c1:Refresh named a handler")
		}
	})
}

// TestKeyedKeptIDsAcrossRejoin: a session joined again gives out the same ids,
// and its clicks reach their rows.
func TestKeyedAcrossRejoin(t *testing.T) {
	st := &rowsState{rows: rowsOf("a", "b")}
	s := live.NewSession(context.Background(), rowsPage(st))
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	id := idOf(t, c.HTML(), "hit r1")
	again, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := idOf(t, again.HTML(), "hit r1"); got != id {
		t.Fatalf("%s after %s", got, id)
	}
	if _, err := clickID(again, id); err != nil || len(st.hits) != 1 || st.hits[0] != "r1" {
		t.Fatalf("%v %v", err, st.hits)
	}
}

// TestKeyedInfoPatch: a broadcast inserting a row leaves a row's id working.
func TestKeyedInfoPatch(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		st := &rowsState{rows: rowsOf("a", "b")}
		s := live.NewSession(context.Background(), func(context.Context) ui.Node {
			return ui.Component("sub", nil, &SubList{St: st})
		})
		c, err := livetest.Join(s, v)
		if err != nil {
			t.Fatal(err)
		}
		id := idOf(t, c.HTML(), "hit r1")
		if err := c.Info("rows", row{"new", "new"}); err != nil {
			t.Fatal(err)
		}
		if st.rows[0].Label != "new" || c.HTML() != s.HTML() {
			t.Fatalf("%+v\n%s", st.rows, c.HTML())
		}
		if err := stale(s, id); err != nil || st.hits[0] != "r1" {
			t.Fatalf("%v %v", err, st.hits)
		}
	})
}

type SubList struct {
	ui.Live
	St *rowsState
}

func (l *SubList) Refresh()                    {}
func (l *SubList) Render() ui.Node             { return (&KeyedRows{St: l.St}).Render() }
func (l *SubList) LiveSchema(s *ui.LiveSchema) { s.Mount(l.mount) }
func (l *SubList) mount(context.Context) error { l.Subscribe("rows"); return nil }
func (l *SubList) UpdateRow(r row)             { l.St.rows = append([]row{r}, l.St.rows...) }

// TestKeyedNavigateRoot: rows of the page's own render (c0) keep across
// navigations by nothing: another page's key 1 is not this page's row 1.
func TestKeyedNavigate(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		var hits []string
		page := func(name string) func(context.Context) ui.Node {
			return func(context.Context) ui.Node {
				return ui.El("ul", nil, ui.Each([]int{1, 2}, func(k int) ui.Node {
					label := name + strconv.Itoa(k)
					return ui.El("li", []ui.Attr{a("key", k)}, ui.El("button", []ui.Attr{a("onClick", ui.On(func() { hits = append(hits, label) }))}, ui.Text(label)))
				}))
			}
		}
		s := live.NewSession(context.Background(), page("pets"))
		s.SetRouter(func(u string) live.Route { return live.Route{Render: page(strings.TrimPrefix(u, "/"))} })
		c, err := livetest.Join(s, v)
		if err != nil {
			t.Fatal(err)
		}
		id := idOf(t, c.HTML(), "pets1")
		if _, err := c.Navigate("/orders"); err != nil {
			t.Fatal(err)
		}
		if err := stale(s, id); err == nil {
			t.Fatalf("pets1's id reached %v on /orders", hits)
		}
		if m, err := clickID(c, idOf(t, c.HTML(), "orders1")); err != nil || m.Error != "" || len(hits) != 1 || hits[0] != "orders1" {
			t.Fatalf("%v %v %+v", hits, err, m)
		}
	})
}

// TestKeyedAroundOtherShapes: what renders a key's handlers but a plain
// element: a component's own instance, an unkeyed list, an error boundary's
// recording (whose replay writes the attributes), ui.Keyed, an opaque node
// (a templ component) and a nil key.
func TestKeyedAroundOtherShapes(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		var hits []string
		on := func(label string) ui.Attr {
			return a("onClick", ui.On(func() { hits = append(hits, label) }))
		}
		btn := func(label string) ui.Node { return ui.El("button", []ui.Attr{on(label)}, ui.Text(label)) }
		templRow := func(label string) ui.Node {
			return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
				attrs := ui.TemplAttrs(ctx, nil, on(label))
				_, err := io.WriteString(w, `<button data-vk-on-click="`+attrs["data-vk-on-click"].(string)+`">`+label+`</button>`)
				return err
			})
		}
		page := func(front ...int) func(context.Context) ui.Node {
			return func(context.Context) ui.Node {
				keys := append(front, 1, 2)
				return ui.El("main", nil,
					ui.ErrorBoundary(func(error) ui.Node { return ui.Text("failed") },
						ui.Each(keys, func(k int) ui.Node {
							return ui.El("p", []ui.Attr{a("key", k), on("self" + strconv.Itoa(k))}, ui.Text("self"+strconv.Itoa(k)))
						})),
					ui.ErrorBoundary(func(error) ui.Node { return ui.Text("failed") },
						ui.Each(keys, func(k int) ui.Node {
							return ui.El("p", []ui.Attr{a("key", "b"+strconv.Itoa(k))}, btn("bound"+strconv.Itoa(k)), templRow("deep"+strconv.Itoa(k)))
						})),
					ui.Each(keys, func(k int) ui.Node { return ui.Keyed(100+k, btn("wrapped"+strconv.Itoa(k))) }),
					ui.Each(keys, func(k int) ui.Node {
						return ui.El("p", []ui.Attr{a("key", "t"+strconv.Itoa(k))}, templRow("templ"+strconv.Itoa(k)))
					}),
					ui.Each(keys, func(k int) ui.Node {
						return ui.El("p", []ui.Attr{a("key", nil)}, btn("nil"+strconv.Itoa(k)))
					}),
					ui.Each(keys, func(k int) ui.Node {
						return ui.El("p", []ui.Attr{a("key", "c"+strconv.Itoa(k))}, ui.Key(k, &Item{Name: "item" + strconv.Itoa(k)}))
					}),
				)
			}
		}
		route := map[string]func(context.Context) ui.Node{"/more": page(0)}
		s := live.NewSession(context.Background(), page())
		s.SetRouter(func(u string) live.Route { return live.Route{Render: route[u], Updates: map[ui.Stateful]any{nil: nil}} })
		c, err := livetest.Join(s, v)
		if err != nil {
			t.Fatal(err)
		}
		labels := []string{"self2", "bound2", "deep2", "wrapped2", "templ2"}
		ids := map[string]string{}
		for _, l := range labels {
			ids[l] = idOf(t, c.HTML(), l)
			if strings.Count(ids[l], ":") != 2 {
				t.Errorf("%s's id: %s", l, ids[l])
			}
		}
		if id := idOf(t, c.HTML(), "nil1"); strings.Count(id, ":") != 1 {
			t.Errorf("a nil key's handler: %s", id)
		}
		if _, err := c.Patch("/more"); err != nil {
			t.Fatal(err)
		}
		for _, l := range labels {
			if got := idOf(t, c.HTML(), l); got != ids[l] {
				t.Errorf("%s: %s after %s", l, got, ids[l])
			}
			hits = nil
			if m, err := clickID(c, ids[l]); err != nil || m.Error != "" || len(hits) != 1 || hits[0] != l {
				t.Errorf("%s: reached %v: %+v %v", l, hits, m, err)
			}
		}
	})
}

// TestKeyedPrivateKeys: a key the page doesn't show otherwise stays out of
// the ids naming it, and so does a key's Go type.
func TestKeyedPrivateKeys(t *testing.T) {
	type petID int64
	eachVersion(t, func(t *testing.T, v int) {
		var hits []string
		c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node {
			btn := func(l string) ui.Node {
				return ui.El("button", []ui.Attr{a("onClick", ui.On(func() { hits = append(hits, l) }))}, ui.Text(l))
			}
			return ui.El("main", nil,
				ui.Keyed("someone@example.com", btn("wrapped")),
				ui.El("p", []ui.Attr{a("key", petID(7))}, btn("typed")))
		}), v)
		if err != nil {
			t.Fatal(err)
		}
		if h := c.HTML(); strings.Contains(h, "someone") || strings.Contains(h, "example") || strings.Contains(h, "petID") {
			t.Fatalf("%s", h)
		}
		for _, l := range []string{"wrapped", "typed"} {
			if _, err := clickID(c, idOf(t, c.HTML(), l)); err != nil || hits[len(hits)-1] != l {
				t.Fatalf("%s: %v %v", l, err, hits)
			}
		}
	})
}

// TestKeyedPushCommands: a JS command's Push in a keyed row names its handler
// by the key too.
func TestKeyedPushCommands(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		var hits []string
		keys := []int{1, 2}
		c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node {
			return ui.El("ul", nil, ui.Each(keys, func(k int) ui.Node {
				return ui.El("li", []ui.Attr{a("key", k)}, ui.El("button", []ui.Attr{
					a("id", "b"+strconv.Itoa(k)),
					a("onClick", ui.JS.Push(func() { hits = append(hits, strconv.Itoa(k)) })),
				}, ui.Text("x")))
			}))
		}), v)
		if err != nil {
			t.Fatal(err)
		}
		attr, _ := c.Attr("#b2", "data-vk-js-click")
		if !strings.Contains(attr, `"h":"c0:2:0"`) {
			t.Fatalf("%s", attr)
		}
		if err := c.Click("#b2"); err != nil || len(hits) != 1 || hits[0] != "2" {
			t.Fatalf("%v %v", err, hits)
		}
	})
}

// TestKeyedKeptLayoutVerified: an instance a navigation keeps — tracked, so
// VUKA_LIVE_VERIFY compares its renders — takes the new page's ids at once.
func TestKeyedKeptLayoutVerified(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		hits := &[]string{}
		layout := func(page string) func(context.Context) ui.Node {
			return func(context.Context) ui.Node {
				return ui.El("div", nil, ui.Component("nav", nil, &TrackedNav{Hits: hits}), ui.Keyed(page, ui.Text(page)))
			}
		}
		s := live.NewSession(context.Background(), layout("/a"))
		s.SetRouter(func(u string) live.Route { return live.Route{Render: layout(u), Keep: true} })
		c, err := livetest.Join(s, v)
		if err != nil {
			t.Fatal(err)
		}
		id := idOf(t, c.HTML(), "go")
		if m, err := c.Navigate("/b"); err != nil || m.Error != "" {
			t.Fatalf("%+v %v", m, err)
		}
		if err := stale(s, id); err == nil {
			t.Fatalf("the last page's id reached %v", *hits)
		}
		if m, err := clickID(c, idOf(t, c.HTML(), "go")); err != nil || m.Error != "" || len(*hits) != 1 {
			t.Fatalf("%+v %v %v", m, err, *hits)
		}
		if c.HTML() != s.HTML() {
			t.Fatalf("%s\n%s", c.HTML(), s.HTML())
		}
	})
}

type TrackedNav struct {
	ui.Live
	Hits *[]string
	n    uigen.Assign[int]
}

func (n *TrackedNav) Render() ui.Node {
	return ui.El("nav", nil, ui.El("a", []ui.Attr{a("key", "home"), a("onClick", ui.On(func() { *n.Hits = append(*n.Hits, "home") }))}, ui.Text("go")), ui.Child(n.n.Get()))
}

// TestKeyedVerifyTracked: a tracked component with duplicate keys, kept
// across events, is checked against a full render by VUKA_LIVE_VERIFY without
// its ids telling them apart.
func TestKeyedVerifyTracked(t *testing.T) {
	eachVersion(t, func(t *testing.T, v int) {
		c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node {
			return ui.El("main", nil, ui.Component("t", nil, &TrackedDups{}), ui.Component("o", nil, &Counter{}))
		}), v)
		if err != nil {
			t.Fatal(err)
		}
		// The other component's button: this one is kept, and verified.
		hs := c.Handlers()
		for range 3 {
			if m, err := clickID(c, hs[len(hs)-1][1]); err != nil || m.Error != "" {
				t.Fatalf("%+v %v", m, err)
			}
		}
		if m, err := clickID(c, idOf(t, c.HTML(), "dup1")); err != nil || m.Error != "" {
			t.Fatalf("%+v %v", m, err)
		}
	})
}

type TrackedDups struct {
	ui.Live
	n uigen.Assign[int]
}

func (d *TrackedDups) Inc() { d.n.Set(d.n.Get() + 1) }
func (d *TrackedDups) Render() ui.Node {
	return ui.El("ul", nil, ui.Each([]string{"dup0", "dup1"}, func(l string) ui.Node {
		return ui.El("li", []ui.Attr{a("key", "same")}, ui.El("button", []ui.Attr{a("onClick", ui.On(d.Inc))}, ui.Text(l)), ui.Child(d.n.Get()))
	}))
}
