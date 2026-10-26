package live_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// Counted starts a count from its Mount hook by pushing a command that
// calls an unexported method its markup never names, as a data table
// starting an async total does. Its markup has handlers of its own, so a
// pushed handler's id comes after theirs.
type Counted struct {
	ui.Live
	clicks  int
	started int
	ticks   int
}

type countTick struct{}

func (c *Counted) LiveSchema(l *ui.LiveSchema) { l.Mount(c.mount) }
func (c *Counted) mount(context.Context) error {
	c.Subscribe("ticks")
	c.PushJS(ui.JS.Push(c.start))
	return nil
}
func (c *Counted) start()           { c.started++ }
func (c *Counted) Click()           { c.clicks++; c.PushJS(ui.JS.Push(c.start)) }
func (c *Counted) Update(countTick) { c.ticks++; c.PushJS(ui.JS.Push(c.start)) }
func (c *Counted) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{a("id", "click"), a("onClick", ui.On(c.Click))}),
		ui.El("button", []ui.Attr{a("id", "other"), a("onClick", ui.On(func() {}))}),
		ui.El("p", []ui.Attr{a("id", "out")}, ui.Child(c.clicks), ui.Text("/"), ui.Child(c.started), ui.Text("/"), ui.Child(c.ticks)))
}

// CountedOf is Counted as a generic component.
type CountedOf[T any] struct {
	ui.Live
	Rows    []T
	started int
}

func (c *CountedOf[T]) LiveSchema(l *ui.LiveSchema) { l.Mount(c.mount) }
func (c *CountedOf[T]) mount(context.Context) error {
	c.PushJS(ui.JS.Push(c.startCount))
	return nil
}
func (c *CountedOf[T]) startCount(context.Context) error { c.started++; return nil }
func (c *CountedOf[T]) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{a("id", "g-other"), a("onClick", ui.On(func() {}))}),
		ui.El("p", []ui.Attr{a("id", "g-out")}, ui.Child(len(c.Rows)), ui.Text(" rows, started "), ui.Child(c.started)))
}

func countedPage(context.Context) ui.Node {
	return ui.El("main", nil, ui.Component("c", nil, &Counted{}), ui.Component("g", nil, &CountedOf[string]{Rows: []string{"a", "b"}}))
}

// TestPushedHandlerAfterHTTPRender: the page's HTTP render mounts its
// components, their Mount pushing a command that calls a method no
// markup names; the join adopts that session and renders again before the
// command reaches the browser. The pushed handler still answers.
func TestPushedHandlerAfterHTTPRender(t *testing.T) {
	for _, v := range []int{1, 2} {
		s := live.NewSession(context.Background(), countedPage)
		if _, err := s.Render(); err != nil {
			t.Fatal(err)
		}
		if err := s.Adopt(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
		c, err := livetest.Join(s, v)
		if err != nil {
			t.Fatalf("v%d: %v", v, err)
		}
		if got := c.Text("#out"); got != "0/1/0" {
			t.Fatalf("v%d: Counted's pushed start ran %q, want 0/1/0", v, got)
		}
		if got := c.Text("#g-out"); got != "2 rows, started 1" {
			t.Fatalf("v%d: the generic component's pushed start: %q", v, got)
		}
	}
}

// TestPushedHandlerFromJoin: a Mount connected at the join pushes the same
// command; it answers too.
func TestPushedHandlerFromJoin(t *testing.T) {
	for _, v := range []int{1, 2} {
		c, err := livetest.Join(live.NewSession(context.Background(), countedPage), v)
		if err != nil {
			t.Fatalf("v%d: %v", v, err)
		}
		if got := c.Text("#out") + " " + c.Text("#g-out"); got != "0/1/0 2 rows, started 1" {
			t.Fatalf("v%d: %q", v, got)
		}
	}
}

// TestPushedHandlerAcrossRenders: commands pushed from Mount reach the
// server after their instances rendered again (events, a message); their
// handlers still answer, the generic component's too, and no id a later
// push or render takes names another handler.
func TestPushedHandlerAcrossRenders(t *testing.T) {
	for _, v := range []int{1, 2} {
		s := live.NewSession(context.Background(), countedPage)
		if _, err := s.Render(); err != nil {
			t.Fatal(err)
		}
		targets := map[string]string{}
		for _, p := range s.JS() {
			targets[p.ID] = between(string(p.Ops), `"h":"`, `"`)
		}
		if targets["c1"] != "c1:0p" || targets["c2"] != "c2:0p" {
			t.Fatalf("v%d: pushed handler ids %v", v, targets)
		}
		for range 2 {
			p, err := s.Event(ui.Event{Target: handlerOf(t, s.HTML(), "click"), Type: "click"})
			if err != nil || len(p.JS) != 1 || between(string(p.JS[0].Ops), `"h":"`, `"`) == targets["c1"] {
				t.Fatalf("v%d: a later push takes an id of its own: %+v %v", v, p.JS, err)
			}
		}
		if _, err := s.Info("ticks", countTick{}); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"c1", "c2"} {
			if _, err := s.Event(ui.Event{Target: targets[id], Type: "push"}); err != nil {
				t.Fatalf("v%d: %s's Mount push after renders: %v", v, id, err)
			}
		}
		if !strings.Contains(s.HTML(), `<p id="out">2/1/1</p>`) || !strings.Contains(s.HTML(), "2 rows, started 1") {
			t.Fatalf("v%d: %s", v, s.HTML())
		}
	}
}

// TestPushedHandlerExpiry: an instance keeps the handlers of its last 64
// pushed commands; an older one, or one of an instance gone, fails saying
// whose it was.
func TestPushedHandlerExpiry(t *testing.T) {
	s := live.NewSession(context.Background(), countedPage)
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	for range 64 {
		if _, err := s.Event(ui.Event{Target: handlerOf(t, s.HTML(), "click"), Type: "click"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Event(ui.Event{Target: "c1:0p", Type: "push"})
	if want := `live: no handler "c1:0p": live_test.Counted (c1) keeps the handlers of its last 64 pushed commands, and this one is older`; err == nil || err.Error() != want {
		t.Fatalf("got  %v\nwant %s", err, want)
	}
	if _, err := s.Event(ui.Event{Target: "c1:1p", Type: "push"}); err != nil {
		t.Fatalf("the 64 latest stay: %v", err)
	}
	_, err = s.Event(ui.Event{Target: "c2:9", Type: "push"})
	if want := `live: no handler "c2:9": live_test.CountedOf[…] (c2): its last render has 1`; err == nil || err.Error() != want {
		t.Fatalf("got  %v\nwant %s", err, want)
	}
	_, err = s.Event(ui.Event{Target: "c7:0p", Type: "push"})
	if want := `live: no handler "c7:0p": no component c7 is mounted (it was unmounted, or the page navigated)`; err == nil || err.Error() != want {
		t.Fatalf("got  %v\nwant %s", err, want)
	}
}

// TestNoHandlerHidesDetail: with a transport's error text that shows the
// browser only public messages, a no-handler reply is the short form,
// naming no Go type; the full reason is what the hook (and so the log)
// gets.
func TestNoHandlerHidesDetail(t *testing.T) {
	var logged []string
	hide := func(err error) string {
		logged = append(logged, err.Error())
		if m, ok := live.PublicMessage(err); ok {
			return m
		}
		return "internal error"
	}
	for _, v := range []int{1, 2} {
		s := live.NewSession(context.Background(), countedPage)
		s.SetErrorText(hide)
		if m := s.Handle(live.ClientMessage{Type: live.Join, V: v}); m.Type != live.Render {
			t.Fatal(m.Error)
		}
		for _, tc := range []struct{ target, full string }{
			{"c2:9", `live: no handler "c2:9": live_test.CountedOf[…] (c2): its last render has 1`},
			{"c1:Secret", `live: no handler "c1:Secret": live_test.Counted (c1) takes no push named Secret`},
			{"c7:0p", `live: no handler "c7:0p": no component c7 is mounted (it was unmounted, or the page navigated)`},
		} {
			logged = nil
			got := string(s.HandleJSON([]byte(`{"type":"event","ref":7,"target":"` + tc.target + `","event":"push"}`)))
			want := `{"type":"error","ref":7,"error":"live: no handler \"` + tc.target + `\""}`
			if got != want || strings.Contains(got, "live_test") {
				t.Errorf("v%d %s: got %s, want %s", v, tc.target, got, want)
			}
			if len(logged) != 1 || logged[0] != tc.full {
				t.Errorf("v%d %s: the hook got %q, want %q", v, tc.target, logged, tc.full)
			}
		}
		var nh *live.NoHandlerError
		_, err := s.Event(ui.Event{Target: "c2:9", Type: "push"})
		if !errors.As(err, &nh) || nh.Target != "c2:9" || nh.Component != "live_test.CountedOf[…]" {
			t.Errorf("v%d: %#v", v, err)
		}
	}
}

// TestPushedHandlerFromEventAndInfo: pushes from an event and from an Info
// (held while other events re-render the instance) still answer.
func TestPushedHandlerFromEventAndInfo(t *testing.T) {
	for _, v := range []int{1, 2} {
		c, err := livetest.Join(live.NewSession(context.Background(), countedPage), v)
		if err != nil {
			t.Fatal(err)
		}
		c.Hold()
		if err := c.Click("#click"); err != nil {
			t.Fatal(err)
		}
		if err := c.Deliver(); err != nil { // the click; its reply pushes start, held
			t.Fatalf("v%d: %v", v, err)
		}
		if err := c.Info("ticks", countTick{}); err != nil { // renders again, pushes start
			t.Fatalf("v%d: %v", v, err)
		}
		if err := c.Click("#other"); err != nil {
			t.Fatal(err)
		}
		if err := c.Release(); err != nil {
			t.Fatalf("v%d: the held pushes: %v", v, err)
		}
		if got := c.Text("#out"); got != "1/3/1" {
			t.Fatalf("v%d: %q, want 1/3/1", v, got)
		}
	}
}

func between(s, from, to string) string {
	_, rest, ok := strings.Cut(s, from)
	if !ok {
		return ""
	}
	v, _, _ := strings.Cut(rest, to)
	return v
}

func handlerOf(t *testing.T, page, id string) string {
	t.Helper()
	_, rest, ok := strings.Cut(page, `id="`+id+`"`)
	if !ok {
		_, rest, ok = strings.Cut(page, `id="`+id+`" `)
	}
	h := between(rest, `data-vk-on-click="`, `"`)
	if h == "" {
		t.Fatalf("no click handler on #%s: %s", id, page)
	}
	return h
}
