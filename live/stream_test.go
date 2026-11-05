package live_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/internal/demo"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// A chat in plain Go: its messages are a stream, loaded from the store at
// mount and changed by messages and its rows' pushes.
type chatMsg struct {
	ID   int
	Text string
}

type said struct{ chatMsg }
type unsaid struct{ ID int }
type pinned struct{ chatMsg }
type reload struct{}

type chatStore struct {
	msgs    []chatMsg
	removed []int
}

type StreamChat struct {
	ui.Live
	Store    *chatStore
	Max      int
	Title    string
	messages ui.Stream[chatMsg]
}

func (c *StreamChat) LiveSchema(l *ui.LiveSchema) { l.Mount(c.mount) }

func (c *StreamChat) mount(ctx context.Context) error {
	c.messages.Limit(c.Max)
	c.messages.Reset(c.Store.msgs...)
	c.Subscribe("chat")
	return nil
}

func (c *StreamChat) UpdateSaid(m said)     { c.messages.Insert(m.chatMsg) }
func (c *StreamChat) UpdatePinned(m pinned) { c.messages.Prepend(m.chatMsg) }

func (c *StreamChat) Remove(id int) error {
	c.Store.removed = append(c.Store.removed, id)
	c.messages.Delete(id)
	return nil
}

func (c *StreamChat) Render() ui.Node {
	return ui.El("section", nil,
		ui.El("h2", nil, ui.Text(c.Title)),
		c.messages.List("ul", []ui.Attr{{Name: "class", Value: "log"}}, func(m chatMsg) ui.Node {
			return ui.El("li", []ui.Attr{{Name: "key", Value: m.ID}},
				ui.Text(m.Text),
				ui.El("button", []ui.Attr{{Name: "class", Value: "del"}, {Name: "onClick", Value: ui.JS.Push(c.Remove, m.ID)}}, ui.Text("x")))
		}))
}

func msgs(ids ...int) []chatMsg {
	out := make([]chatMsg, len(ids))
	for i, id := range ids {
		out[i] = chatMsg{id, fmt.Sprintf("m%d", id)}
	}
	return out
}

func chatPage(st *chatStore, max int) func(context.Context) ui.Node {
	return func(context.Context) ui.Node {
		return ui.Component("chat", nil, &StreamChat{Store: st, Max: max, Title: "lobby"})
	}
}

func keysOf(rows []livetest.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Key
	}
	return out
}

func wantRows(t *testing.T, c *livetest.Client, want ...string) {
	t.Helper()
	if got := keysOf(c.Stream("c1", "messages")); !slices.Equal(got, want) {
		t.Fatalf("rows %v, want %v\n%s", got, want, c.HTML())
	}
}

func joinChat(t *testing.T, v int, st *chatStore, max int) (*live.Session, *livetest.Client) {
	t.Helper()
	s := live.NewSession(context.Background(), chatPage(st, max))
	c, err := livetest.Join(s, v)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func info(t *testing.T, c *livetest.Client, msg any) {
	t.Helper()
	if err := c.Info("chat", msg); err != nil {
		t.Fatal(err)
	}
}

func TestStreamOps(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			st := &chatStore{msgs: msgs(1, 2, 3)}
			_, c := joinChat(t, v, st, 0)
			wantRows(t, c, "1", "2", "3")
			if strings.Contains(c.HTML(), "data-vk-key") {
				t.Fatalf("a row carries data-vk-key: %s", c.HTML())
			}
			info(t, c, said{chatMsg{4, "m4"}})
			wantRows(t, c, "1", "2", "3", "4")
			info(t, c, said{chatMsg{2, "edited"}}) // in place
			wantRows(t, c, "1", "2", "3", "4")
			if r := c.Stream("c1", "messages")[1]; !strings.HasPrefix(r.Text, "edited") {
				t.Fatalf("row 2 is %q, want edited", r.Text)
			}
			info(t, c, unsaidMsg(3))
			wantRows(t, c, "1", "2", "4")
			info(t, c, pinned{chatMsg{9, "m9"}})
			wantRows(t, c, "9", "1", "2", "4")
			if err := c.Click(`li[data-vk-row="1"] .del`); err != nil {
				t.Fatal(err)
			}
			wantRows(t, c, "9", "2", "4")
			if !slices.Equal(st.removed, []int{1}) {
				t.Fatalf("removed %v, want [1]: the row's push carries its key", st.removed)
			}
		})
	}
}

func unsaidMsg(id int) any { return unsaid{id} }

func (c *StreamChat) UpdateUnsaid(m unsaid) { c.messages.Delete(m.ID) }

func (c *StreamChat) UpdateReload(reload) {
	c.messages.Reset(c.Store.msgs...)
}

func TestStreamLimit(t *testing.T) {
	st := &chatStore{msgs: msgs(1, 2, 3, 4, 5)}
	_, c := joinChat(t, 2, st, 3)
	wantRows(t, c, "1", "2", "3") // a reset keeps the first rows
	info(t, c, said{chatMsg{6, "m6"}})
	wantRows(t, c, "2", "3", "6") // an insert trims the start
	info(t, c, pinned{chatMsg{7, "m7"}})
	wantRows(t, c, "7", "2", "3") // a prepend trims the end
	info(t, c, said{chatMsg{2, "m2!"}})
	wantRows(t, c, "7", "2", "3") // in place: nothing trimmed
	st.msgs = msgs(10, 11)
	info(t, c, reload{})
	wantRows(t, c, "10", "11")
}

// The session holds no rows between replies: the stream's queue, and the
// block of the tree it keeps, are empty once the reply is written.
func TestStreamForgets(t *testing.T) {
	st := &chatStore{msgs: msgs(1, 2, 3)}
	s, c := joinChat(t, 2, st, 0)
	if h := s.HTML(); strings.Contains(h, "data-vk-row") {
		t.Fatalf("the session keeps the rows after the join: %s", h)
	}
	before := c.Bytes
	info(t, c, said{chatMsg{4, "m4"}})
	if h := s.HTML(); strings.Contains(h, "data-vk-row") {
		t.Fatalf("the session keeps the row after its reply: %s", h)
	}
	// the reply carries the one row, not the others
	if n := c.Bytes - before; n > 400 {
		t.Fatalf("an insert's reply is %d bytes", n)
	}
	wantRows(t, c, "1", "2", "3", "4")
}

// A container morphed again with a batch it applied (its instance renders
// for another reason) keeps its rows: a batch applies once.
type StreamChatWithTitle struct {
	StreamChat
}

func TestStreamBatchOnce(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			st := &chatStore{msgs: msgs(1, 2)}
			title := "lobby"
			s := live.NewSession(context.Background(), func(context.Context) ui.Node {
				return ui.El("main", nil,
					ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(func() { title += "!" })}}, ui.Text("rename")),
					ui.Component("chat", nil, &StreamChat{Store: st, Title: title}))
			})
			c, err := livetest.Join(s, v)
			if err != nil {
				t.Fatal(err)
			}
			info(t, c, said{chatMsg{3, "m3"}})
			for range 2 {
				if _, err := c.Event(ui.Event{Target: "c0:0", Type: "click"}); err != nil {
					t.Fatal(err)
				}
			}
			if !strings.Contains(c.HTML(), "lobby!!") {
				t.Fatalf("the chat didn't render again: %s", c.HTML())
			}
			wantRows(t, c, "1", "2", "3")
			if n := strings.Count(c.HTML(), "data-vk-row="); n != 3 {
				t.Fatalf("%d rows on the page, want 3: %s", n, c.HTML())
			}
		})
	}
}

// A reconnect mounts afresh: a new generation, whose first batch resets
// the rows to what the mount hook loaded.
func TestStreamReconnect(t *testing.T) {
	st := &chatStore{msgs: msgs(1, 2)}
	_, c := joinChat(t, 2, st, 0)
	info(t, c, said{chatMsg{3, "m3"}})
	wantRows(t, c, "1", "2", "3")
	st.msgs = msgs(2, 5)
	s2 := live.NewSession(context.Background(), chatPage(st, 0))
	if err := c.Reconnect(s2); err != nil {
		t.Fatal(err)
	}
	wantRows(t, c, "2", "5")
}

// The page's HTTP render sends the mount's batch; an adopted join renders
// the same batch with no rows, so nothing is sent twice.
func TestStreamAdoptedJoin(t *testing.T) {
	st := &chatStore{msgs: msgs(1, 2)}
	s := live.NewSession(context.Background(), chatPage(st, 0))
	page, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(page, "data-vk-row="); n != 2 {
		t.Fatalf("the HTTP render has %d rows: %s", n, page)
	}
	batch := attr(t, page, "data-vk-batch")
	if err := s.Adopt(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	ups, err := s.RenderTrees()
	if err != nil {
		t.Fatal(err)
	}
	j, _ := json.Marshal(ups)
	if strings.Contains(string(j), "data-vk-row") {
		t.Fatalf("the join sends the rows again: %s", j)
	}
	if !strings.Contains(string(j), `data-vk-batch=\"`+batch+`\"`) {
		t.Fatalf("the join's container isn't at batch %s: %s", batch, j)
	}
}

func attr(t *testing.T, h, name string) string {
	t.Helper()
	i := strings.Index(h, name+`="`)
	if i < 0 {
		t.Fatalf("no %s in %s", name, h)
	}
	rest := h[i+len(name)+2:]
	return rest[:strings.IndexByte(rest, '"')]
}

// The compiled form: rows are template frames, sent with their statics
// once; a row's pushes reach the table's methods with the row's id.
func TestStreamTemplate(t *testing.T) {
	var hits []int
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("t", nil, &demo.StreamTable{N: 3, Hits: &hits})
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	rows := func(want ...string) {
		t.Helper()
		if got := keysOf(c.Stream("c1", "rows")); !slices.Equal(got, want) {
			t.Fatalf("rows %v, want %v\n%s", got, want, c.HTML())
		}
	}
	rows("0", "1", "2")
	before := c.Bytes
	if err := c.Click(".insert"); err != nil {
		t.Fatal(err)
	}
	rows("3", "0", "1", "2")
	if n := c.Bytes - before; n > 500 {
		t.Fatalf("an insert's reply is %d bytes: the row's statics are the connection's already", n)
	}
	if err := c.Click(`tr[data-vk-row="1"] .hit`); err != nil {
		t.Fatal(err)
	}
	if err := c.Click(`tr[data-vk-row="0"] .del`); err != nil {
		t.Fatal(err)
	}
	rows("3", "1", "2")
	if !slices.Equal(hits, []int{1}) {
		t.Fatalf("hits %v", hits)
	}
	if h := s.HTML(); strings.Contains(h, "data-vk-row") {
		t.Fatalf("the session keeps rows: %s", h)
	}
}

// A deferred stream: the loader runs on a copy with nothing queued, and its
// operations go before those the page queued meanwhile.
type DeferredFeed struct {
	ui.Live
	Store  *chatStore
	events ui.Stream[chatMsg] `defer:"events"`
}

func (f *DeferredFeed) LiveSchema(l *ui.LiveSchema) {
	l.Loads("events", f.loadEvents)
	l.Mount(f.mount)
}

func (f *DeferredFeed) mount(context.Context) error { f.Subscribe("chat"); return nil }

func (f *DeferredFeed) loadEvents(context.Context) error {
	f.events.Reset(f.Store.msgs...)
	return nil
}

func (f *DeferredFeed) UpdateSaid(m said) { f.events.Insert(m.chatMsg) }

func (f *DeferredFeed) Render() ui.Node {
	if f.Loading("events") {
		return ui.El("div", nil, ui.Text("loading"))
	}
	return ui.El("div", nil, f.events.List("ol", nil, func(m chatMsg) ui.Node {
		return ui.El("li", []ui.Attr{{Name: "key", Value: m.ID}}, ui.Text(m.Text))
	}))
}

func TestStreamDeferred(t *testing.T) {
	st := &chatStore{msgs: msgs(1, 2)}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("f", nil, &DeferredFeed{Store: st})
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	ds := s.Deferred()
	if len(ds) != 1 {
		t.Fatalf("%d deferred loads", len(ds))
	}
	ds[0].Run(context.Background())
	info(t, c, said{chatMsg{3, "m3"}}) // arrives while the load runs
	p, err := s.Loaded(ds[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(live.Marshal(live.PatchMessage(0, p))); err != nil {
		t.Fatal(err)
	}
	if got := keysOf(c.Stream("c1", "events")); !slices.Equal(got, []string{"1", "2", "3"}) {
		t.Fatalf("rows %v, want the loader's then the message's\n%s", got, c.HTML())
	}
}

// BadRows renders its stream's rows as Row says, for the errors.
type BadRows struct {
	ui.Live
	Row  func(b *BadRows, id int) ui.Node
	rows ui.Stream[int]
}

func (b *BadRows) LiveSchema(l *ui.LiveSchema) { l.Mount(b.mount) }
func (b *BadRows) mount(context.Context) error { b.rows.Reset(1); return nil }
func (b *BadRows) Remove(id int)               {}
func (b *BadRows) Render() ui.Node {
	return ui.El("div", nil, b.rows.List("ul", nil, func(id int) ui.Node { return b.Row(b, id) }))
}

type Leaf struct{ ui.Live }

func (*Leaf) Render() ui.Node { return ui.El("i", nil) }

type ExportedStream struct {
	ui.Live
	Rows ui.Stream[int]
}

func (*ExportedStream) Render() ui.Node { return ui.El("i", nil) }

func TestStreamErrors(t *testing.T) {
	li := func(attrs ...ui.Attr) []ui.Attr { return append([]ui.Attr{{Name: "key", Value: 1}}, attrs...) }
	for _, tc := range []struct {
		name string
		row  func(b *BadRows, id int) ui.Node
		page ui.Stateful
		want string
	}{
		{"method value", func(b *BadRows, id int) ui.Node {
			return ui.El("li", li(ui.Attr{Name: "onClick", Value: ui.On(func() { b.Remove(id) })}))
		}, nil, "E0358"},
		{"closure push", func(b *BadRows, id int) ui.Node {
			return ui.El("li", li(ui.Attr{Name: "onClick", Value: ui.JS.Push(func() { b.Remove(id) })}))
		}, nil, "E0358"},
		{"no key", func(b *BadRows, id int) ui.Node { return ui.El("li", nil) }, nil, "E0355"},
		{"live component", func(b *BadRows, id int) ui.Node { return ui.Component("leaf", id, &Leaf{}) }, nil, "E0359"},
		{"exported", nil, &ExportedStream{}, "E0357"},
		{"push ok", func(b *BadRows, id int) ui.Node {
			return ui.El("li", li(ui.Attr{Name: "onClick", Value: ui.JS.Push(b.Remove, id)}))
		}, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := tc.page
			if page == nil {
				page = &BadRows{Row: tc.row}
			}
			s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("x", nil, page) })
			_, err := s.RenderTrees()
			switch {
			case tc.want == "" && err != nil:
				t.Fatal(err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("err %v, want %s", err, tc.want)
			}
		})
	}
}

// A container shown again after a render that didn't show it holds no rows
// in the browser: verification fails it unless the stream was reset.
type HidingFeed struct {
	ui.Live
	Shown *bool
	Reset *bool
	rows  ui.Stream[int]
}

func (h *HidingFeed) LiveSchema(l *ui.LiveSchema) { l.Mount(h.mount) }
func (h *HidingFeed) mount(context.Context) error { h.rows.Reset(1, 2); return nil }
func (h *HidingFeed) Render() ui.Node {
	if *h.Reset {
		h.rows.Reset(1, 2)
	}
	if !*h.Shown {
		return ui.El("p", nil)
	}
	return ui.El("div", nil, h.rows.List("ul", nil, func(id int) ui.Node { return ui.El("li", []ui.Attr{{Name: "key", Value: id}}) }))
}

func TestStreamShownAgain(t *testing.T) {
	for _, reset := range []bool{false, true} {
		shown := true
		s := live.NewSession(context.Background(), func(context.Context) ui.Node {
			return ui.El("main", nil,
				ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(func() { shown = !shown })}}),
				ui.Component("h", nil, &HidingFeed{Shown: &shown, Reset: &reset}))
		})
		c, err := livetest.Join(s, 2)
		if err != nil {
			t.Fatal(err)
		}
		c.Event(ui.Event{Target: "c0:0", Type: "click"}) // hidden
		m, err := c.Event(ui.Event{Target: "c0:0", Type: "click"})
		if failed := err != nil || m.Type == live.Error; failed == reset {
			t.Errorf("reset %v: shown again: %v %+v", reset, err, m)
		}
	}
}
