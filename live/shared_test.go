package live_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

var boardDraws atomic.Int32

type SRow struct {
	ui.Live
	N, Seq int
}

func (r *SRow) Render() ui.Node {
	return ui.El("li", nil, ui.Text(fmt.Sprintf("%d:%d", r.N, r.Seq)))
}

type PriceBoard struct {
	ui.SharedLive
	seq int
}

func (b *PriceBoard) UpdateInt(n int) { b.seq = n }

func (b *PriceBoard) Render() ui.Node {
	boardDraws.Add(1)
	rows := make([]ui.Node, 3)
	for i := range rows {
		rows[i] = ui.Key(i, &SRow{N: i, Seq: b.seq})
	}
	return ui.El("ul", nil, rows...)
}

type withHandler struct{ ui.SharedLive }

func (w *withHandler) Pick() {}
func (w *withHandler) Render() ui.Node {
	return ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(w.Pick)}}, ui.Text("x"))
}

type reader struct{ ui.SharedLive }

func (r *reader) LiveSchema(l *ui.LiveSchema) { l.Mount(r.mount) }
func (r *reader) mount(ctx context.Context) error {
	ui.ViewerRead(ctx, "the session")
	return nil
}
func (r *reader) Render() ui.Node { return ui.Text("x") }

type withProp struct {
	ui.SharedLive
	User string
}

func (w *withProp) Render() ui.Node { return ui.Text(w.User) }

// links is a page's connection: what it attached and what it was handed.
type links struct {
	got   map[string]*live.SharedSnapshot
	trees []live.TreeUpdate
}

func (l *links) Attach(sh *live.Shared) []live.TreeUpdate {
	var out []live.TreeUpdate
	sh.Do(func(snap func() *live.SharedSnapshot) {
		if l.got == nil {
			l.got = map[string]*live.SharedSnapshot{}
		}
		s := snap()
		l.got[sh.Name()] = s
		out = s.Trees
	})
	return out
}
func (l *links) Detach(sh *live.Shared) { delete(l.got, sh.Name()) }

func newSet(strict bool) *live.SharedSet {
	return live.NewSharedSet(func(string) context.Context { return context.Background() }, strict)
}

func sharedPage(set *live.SharedSet, l *links, page func() ui.Node) *live.Session {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return page() })
	s.SetShared(set, l)
	return s
}

func TestSharedRendersOnceForManyPages(t *testing.T) {
	set := newSet(true)
	var pages []*live.Session
	var ls []*links
	for range 5 {
		l := &links{}
		s := sharedPage(set, l, func() ui.Node {
			return ui.El("main", nil, ui.Component("b", nil, &PriceBoard{}))
		})
		m := s.Handle(live.ClientMessage{Type: live.Join, V: 2})
		if m.Type != live.Render {
			t.Fatalf("join: %+v", m)
		}
		if strings.Contains(sj(m.Trees), "SRow") || !strings.Contains(sj(m.Trees), `"id":"s1.c1"`) {
			t.Fatalf("the page holds a reference to the fragment, and the fragment's trees ride along: %s", sj(m.Trees))
		}
		pages = append(pages, s)
		ls = append(ls, l)
	}
	frags := set.Fragments()
	if len(frags) != 1 {
		t.Fatalf("fragments: %d", len(frags))
	}
	before := boardDraws.Load()
	var frames []live.SharedFrame
	err := frags[0].Publish([]live.Delivery{live.Message(1), live.Message(2)}, func(f live.SharedFrame) { frames = append(frames, f) })
	if err != nil {
		t.Fatal(err)
	}
	if n := boardDraws.Load() - before; n != 1 {
		t.Errorf("rendered %d times for 5 pages and 2 messages, want 1", n)
	}
	if len(frames) != 1 || frames[0].Ver != 2 || frames[0].Base != 1 {
		t.Fatalf("frames: %+v", frames)
	}
	if !strings.Contains(string(frames[0].Msg), "2") || strings.Contains(string(frames[0].Msg), `"s":[`) {
		t.Errorf("a frame carries no statics: %s", frames[0].Msg)
	}
	if snap := frames[0].Snapshot(); snap.Ver != 2 || !strings.Contains(string(snap.Msg), `"s":[`) {
		t.Errorf("snapshot: %s", snap.Msg)
	}
	_ = ls
	_ = pages
}

func TestSharedRefusesWhatIsPerViewer(t *testing.T) {
	cases := []struct {
		name string
		node func() ui.Node
		want string
	}{
		{"handler", func() ui.Node { return ui.Component("h", nil, &withHandler{}) }, "event handler"},
		{"viewer", func() ui.Node { return ui.Component("r", nil, &reader{}) }, "differs per viewer"},
		{"props", func() ui.Node { return ui.Component("p", nil, &withProp{User: "ann"}) }, "takes no props"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := sharedPage(newSet(true), &links{}, func() ui.Node { return ui.El("main", nil, c.node()) })
			m := s.Handle(live.ClientMessage{Type: live.Join, V: 2})
			if m.Type != live.Error || !strings.Contains(m.Error, c.want) {
				t.Errorf("join: %+v, want an error with %q", m, c.want)
			}
		})
	}
}

func TestSharedEventIsRefused(t *testing.T) {
	set := newSet(true)
	s := sharedPage(set, &links{}, func() ui.Node { return ui.El("main", nil, ui.Component("b", nil, &PriceBoard{})) })
	s.Handle(live.ClientMessage{Type: live.Join, V: 2})
	for _, target := range []string{"s1.c1:0", "s1.c1:Pick", "s1.c2:0"} {
		m := s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: 1, Event: ui.Event{Target: target, Type: "click"}})
		if m.Type != live.Error || !strings.Contains(m.Error, "no handler") {
			t.Errorf("%s: %+v", target, m)
		}
		_, err := s.Event(ui.Event{Target: target})
		var se *live.SharedEventError
		if !errorsAs(err, &se) {
			t.Errorf("%s: err %v is not a SharedEventError", target, err)
		}
	}
	if !strings.Contains(set.Fragments()[0].HTML(), "0:0") {
		t.Errorf("state changed: %s", set.Fragments()[0].HTML())
	}
}

func errorsAs(err error, target **live.SharedEventError) bool {
	e, ok := err.(*live.SharedEventError)
	if ok {
		*target = e
	}
	return ok
}

// A per-page component wrapping the shared one: its event changes only its page.
type Wrapper struct {
	ui.Live
	sort string
}

func (w *Wrapper) SortIt() { w.sort = "name" }
func (w *Wrapper) Render() ui.Node {
	return ui.El("main", []ui.Attr{{Name: "data-sort", Value: w.sort}},
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(w.SortIt)}}, ui.Text("sort")),
		ui.Component("b", nil, &PriceBoard{}))
}

func TestPerPageWrapperEventTouchesOnlyItsPage(t *testing.T) {
	set := newSet(true)
	mk := func() (*live.Session, *links) {
		l := &links{}
		s := sharedPage(set, l, func() ui.Node { return ui.Component("w", nil, &Wrapper{}) })
		if m := s.Handle(live.ClientMessage{Type: live.Join, V: 2}); m.Type != live.Render {
			t.Fatalf("join: %+v", m)
		}
		return s, l
	}
	a, _ := mk()
	b, _ := mk()
	before := boardDraws.Load()
	m := a.Handle(live.ClientMessage{Type: live.EventMsg, Ref: 1, Event: ui.Event{Target: "c1:0", Type: "click"}})
	if m.Type != live.PatchMsg || m.Error != "" {
		t.Fatalf("event: %+v", m)
	}
	if !strings.Contains(sj(m.Trees), "name") || strings.Contains(sj(m.Trees), "s1.") && strings.Contains(sj(m.Trees), `"full":true`) {
		t.Errorf("the page's patch: %s", sj(m.Trees))
	}
	if boardDraws.Load() != before {
		t.Errorf("the shared fragment rendered for a page's event")
	}
	if strings.Contains(b.HTML(), `data-sort="name"`) {
		t.Errorf("the other page changed: %s", b.HTML())
	}
	if !strings.Contains(a.HTML(), `data-sort="name"`) {
		t.Errorf("page a: %s", a.HTML())
	}
}

func sj(ts []live.TreeUpdate) string { b, _ := json.Marshal(ts); return string(b) }

type Toggler struct {
	ui.Live
	on bool
}

func (w *Toggler) Flip() { w.on = !w.on }
func (w *Toggler) Render() ui.Node {
	kids := []ui.Node{ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(w.Flip)}}, ui.Text("flip"))}
	if w.on {
		kids = append(kids, ui.Component("b", nil, &PriceBoard{}))
	}
	return ui.El("main", nil, kids...)
}

func TestSharedAttachesAndDetachesWithThePage(t *testing.T) {
	set := newSet(true)
	l := &links{}
	s := sharedPage(set, l, func() ui.Node { return ui.Component("t", nil, &Toggler{}) })
	if m := s.Handle(live.ClientMessage{Type: live.Join, V: 2}); m.Type != live.Render || len(l.got) != 0 {
		t.Fatalf("join: %+v, attached %d", m, len(l.got))
	}
	m := s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: 1, Event: ui.Event{Target: "c1:0", Type: "click"}})
	if len(l.got) != 1 || !strings.Contains(sj(m.Trees), `"id":"s1.c1"`) || !strings.Contains(sj(m.Trees), `"full":true`) {
		t.Fatalf("shown: attached %d, %s", len(l.got), sj(m.Trees))
	}
	m = s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: 2, Event: ui.Event{Target: "c1:0", Type: "click"}})
	if len(l.got) != 0 || m.Error != "" {
		t.Fatalf("hidden: attached %d, %+v", len(l.got), m)
	}
	s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: 3, Event: ui.Event{Target: "c1:0", Type: "click"}})
	s.Close()
	if len(l.got) != 0 {
		t.Errorf("a closed page stays attached")
	}
}

type subscriber struct{ ui.SharedLive }

func (b *subscriber) LiveSchema(l *ui.LiveSchema) { l.Mount(b.mount) }
func (b *subscriber) mount(context.Context) error { b.Subscribe("x"); return nil }
func (b *subscriber) Render() ui.Node             { return ui.Text("x") }

func TestSharedTopicRules(t *testing.T) {
	s := sharedPage(newSet(true), &links{}, func() ui.Node { return ui.El("main", nil, ui.Component("s", nil, &subscriber{})) })
	if m := s.Handle(live.ClientMessage{Type: live.Join, V: 2}); m.Type != live.Error || !strings.Contains(m.Error, "does not call Subscribe") {
		t.Errorf("subscribe: %+v", m)
	}
}

func TestSharedMountsOnBroadcastAndAppliesTheMessage(t *testing.T) {
	set := newSet(true)
	if len(set.Fragments()) != 0 {
		t.Fatal("mounted before anything asked")
	}
	sh, err := set.ForType(reflect.TypeFor[*PriceBoard]())
	if err != nil {
		t.Fatal(err)
	}
	if err := sh.Publish([]live.Delivery{live.Message(7)}, func(live.SharedFrame) {}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sh.HTML(), "0:7") {
		t.Errorf("the message was not applied: %s", sh.HTML())
	}
	if n, _ := ui.SharedByName(ui.SharedName(reflect.TypeFor[*PriceBoard]())); n != reflect.TypeFor[*PriceBoard]() {
		t.Errorf("not registered by name")
	}
}
