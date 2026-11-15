package live_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// room is a page: it counts its mounts, subscribes to its topic, defers a
// group loading at once and one loading when seen, and pushes a command
// from Mount.
type room struct {
	ui.Live
	Name    string
	app     *aheadApp
	mounted int
	clicks  int
	log     []string
	members []string
	pins    []string
}

func (r *room) LiveSchema(l *ui.LiveSchema) {
	l.Mount(r.mount)
	l.Field(&r.members, ui.Defer("members"))
	l.Loads("members", r.loadMembers)
	l.Field(&r.pins, ui.Defer("pins").WhenSeen())
	l.Loads("pins", r.loadPins)
}

func (r *room) mount(context.Context) error {
	r.app.rooms++
	r.mounted = r.app.rooms
	r.Subscribe("room:" + r.Name)
	r.PushJS(ui.JS.AddClass(ui.This, "ready"))
	return nil
}
func (r *room) loadMembers(context.Context) error { r.members = []string{"ann", "bob"}; return nil }
func (r *room) loadPins(context.Context) error    { r.pins = []string{"rules"}; return nil }
func (r *room) Inc()                              { r.clicks++ }
func (r *room) UpdateNewMessage(m NewMessage)     { r.log = append(r.log, m.Text) }
func (r *room) Unmount()                          { r.app.gone = append(r.app.gone, "room "+r.Name) }
func (r *room) Render() ui.Node {
	members, pins := "members loading", ui.Node(ui.Skeleton{For: "pins"})
	if !r.Loading("members") {
		members = "members " + strings.Join(r.members, ",")
	}
	if !r.Loading("pins") {
		pins = ui.Text("pins " + strings.Join(r.pins, ","))
	}
	return ui.El("section", nil,
		ui.El("p", []ui.Attr{a("id", "room")}, ui.Text(fmt.Sprintf("%s mount %d clicks %d log %v", r.Name, r.mounted, r.clicks, r.log))),
		ui.El("button", []ui.Attr{a("id", "inc"), a("onClick", ui.On(r.Inc))}),
		ui.El("p", []ui.Attr{a("id", "members")}, ui.Text(members)),
		pins)
}

// lobby is another page, rendered at a room's place by its tag's site.
type lobby struct {
	ui.Live
	app *aheadApp
}

func (l *lobby) Unmount()        { l.app.gone = append(l.app.gone, "lobby") }
func (l *lobby) Render() ui.Node { return ui.El("p", []ui.Attr{a("id", "lobby")}, ui.Text("lobby")) }

// shell is a persistent layout: a component holding the page.
type shell struct {
	ui.Live
	Children ui.Node
	app      *aheadApp
	opened   int
}

func (s *shell) LiveSchema(l *ui.LiveSchema) { l.Mount(s.mount) }
func (s *shell) mount(context.Context) error { s.app.shells++; return nil }
func (s *shell) Toggle()                     { s.opened++ }
func (s *shell) Unmount()                    { s.app.gone = append(s.app.gone, "shell") }
func (s *shell) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("nav", []ui.Attr{a("id", "nav")}, ui.Text(fmt.Sprint("opened ", s.opened))),
		ui.El("button", []ui.Attr{a("id", "toggle"), a("onClick", ui.On(s.Toggle))}),
		s.Children)
}

// aheadApp is an app whose pages sit in a shell, each under a key of its
// own (as a router keys the page below a layout), counting what mounts and
// unmounts.
type aheadApp struct {
	rooms, shells int
	gone          []string
}

func (app *aheadApp) route(name string, key int, keep bool) live.Route {
	return live.Route{Keep: keep, URL: "/" + name, Render: func(context.Context) ui.Node {
		var page ui.Stateful = &room{Name: name, app: app}
		if name == "lobby" {
			page = &lobby{app: app}
		}
		return ui.El("main", nil, ui.Component("shell", nil, &shell{app: app,
			Children: ui.Keyed(key, ui.Component("page", nil, page))}))
	}}
}

// ahead renders rt in a session of its own, as a connection does ahead of a
// click.
func (app *aheadApp) ahead(t *testing.T, rt live.Route) *live.Session {
	t.Helper()
	s := live.NewSession(context.Background(), rt.Render)
	if err := s.RenderAhead(); err != nil {
		t.Fatal(err)
	}
	return s
}

func apply(t *testing.T, c *livetest.Client, p live.Patch) {
	t.Helper()
	if err := c.Apply(live.Marshal(live.PatchMessage(0, p))); err != nil {
		t.Fatal(err)
	}
	if c.HTML() != c.S.HTML() {
		t.Fatalf("client's page\n%s\nserver's\n%s", c.HTML(), c.S.HTML())
	}
}

// TestNavigateFrom: the page an ahead session mounted moves in without
// mounting again, its state as its Mount left it; the layout the navigation
// keeps stays the live one, its state too, and ahead's copy of it is
// unmounted with the old page.
func TestNavigateFrom(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			app := &aheadApp{}
			s := live.NewSession(context.Background(), app.route("go", 0, true).Render)
			c, err := livetest.Join(s, v)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Click("#toggle"); err != nil || c.Text("#nav") != "opened 1" {
				t.Fatalf("toggle: %v %s", err, c.Text("#nav"))
			}

			rt := app.route("rust", 1, true)
			ahead := app.ahead(t, rt)
			if app.rooms != 2 || app.shells != 2 {
				t.Fatalf("ahead mounted %d rooms, %d shells", app.rooms, app.shells)
			}
			p, err := s.NavigateFrom(rt, ahead, false)
			if err != nil {
				t.Fatal(err)
			}
			apply(t, c, p)
			if app.rooms != 2 || app.shells != 2 {
				t.Errorf("the commit mounted again: %d rooms, %d shells", app.rooms, app.shells)
			}
			if got := c.Text("#room"); got != "rust mount 2 clicks 0 log []" {
				t.Errorf("room: %s", got)
			}
			if got := c.Text("#nav"); got != "opened 1" {
				t.Errorf("the kept layout isn't the live one: %s", got)
			}
			if !slices.Equal(app.gone, []string{"room go", "shell"}) {
				t.Errorf("unmounted %v, want the old page and ahead's shell", app.gone)
			}
			id, _ := c.Attr("section", "data-vk-id")
			if !slices.ContainsFunc(p.JS, func(j live.JSPush) bool { return j.ID == id }) || !c.HasClass("section", "ready") {
				t.Errorf("the command its Mount pushed isn't the reply's under %s: %+v", id, p.JS)
			}

			// It is the live session's: its handlers, and the layout's.
			if err := c.Click("#inc"); err != nil || c.Text("#room") != "rust mount 2 clicks 1 log []" {
				t.Errorf("click: %v %s", err, c.Text("#room"))
			}
			if err := c.Click("#toggle"); err != nil || c.Text("#nav") != "opened 2" {
				t.Errorf("toggle: %v %s", err, c.Text("#nav"))
			}

			// Ahead is closed: closing it again unmounts nothing twice.
			ahead.Close()
			if len(app.gone) != 2 {
				t.Errorf("a second close unmounted %v", app.gone)
			}
			if _, err := ahead.RenderTrees(); err == nil {
				t.Errorf("ahead renders after the commit")
			}
		})
	}
}

// TestNavigateFromReply: the reply is Navigate's for the same move, byte
// for byte — what moved is sent as what mounted would be, against the
// statics the client holds.
func TestNavigateFromReply(t *testing.T) {
	for _, v := range []int{1, 2} {
		for _, keep := range []bool{true, false} {
			t.Run(fmt.Sprintf("v%d keep %v", v, keep), func(t *testing.T) {
				var replies [2][]byte
				for i := range replies {
					app := &aheadApp{}
					s := live.NewSession(context.Background(), app.route("go", 0, keep).Render)
					c, err := livetest.Join(s, v)
					if err != nil {
						t.Fatal(err)
					}
					rt := app.route("rust", 1, keep)
					var p live.Patch
					if i == 0 {
						p, err = s.Navigate(rt, false)
					} else {
						p, err = s.NavigateFrom(rt, app.ahead(t, rt), false)
					}
					if err != nil {
						t.Fatal(err)
					}
					apply(t, c, p)
					replies[i] = live.Marshal(live.PatchMessage(0, p))
				}
				if !bytes.Equal(replies[0], replies[1]) {
					t.Fatalf("Navigate's reply\n%s\nNavigateFrom's\n%s", replies[0], replies[1])
				}
				if v == 2 && strings.Contains(string(replies[1]), "opened") == keep {
					t.Errorf("the layout sent again: %v (%s)", keep, replies[1])
				}
				t.Logf("%d bytes", len(replies[1]))
			})
		}
	}
}

// TestNavigateFromElsewhere: an ahead instance at another place, or of
// another type, is never taken; the page mounts afresh and ahead's go.
func TestNavigateFromElsewhere(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ahead func(app *aheadApp) live.Route
	}{
		{"another key", func(app *aheadApp) live.Route { return app.route("rust", 2, true) }},
		{"another type", func(app *aheadApp) live.Route { return app.route("lobby", 1, true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &aheadApp{}
			s := live.NewSession(context.Background(), app.route("go", 0, true).Render)
			c, err := livetest.Join(s, 2)
			if err != nil {
				t.Fatal(err)
			}
			ahead := app.ahead(t, tc.ahead(app))
			rooms := app.rooms
			p, err := s.NavigateFrom(app.route("rust", 1, true), ahead, false)
			if err != nil {
				t.Fatal(err)
			}
			apply(t, c, p)
			if app.rooms != rooms+1 || c.Text("#room") != fmt.Sprintf("rust mount %d clicks 0 log []", app.rooms) {
				t.Errorf("not mounted afresh: %d rooms, %s", app.rooms, c.Text("#room"))
			}
			if len(app.gone) != 3 || ahead.Topics() != nil {
				t.Errorf("unmounted %v; ahead's topics %v", app.gone, ahead.Topics())
			}
		})
	}
}

// TestNavigateFromCarries: a moved instance's topics, deferred loads (the
// one ahead started, the one waiting to be seen) and messages are the live
// session's; ahead ends with nothing.
func TestNavigateFromCarries(t *testing.T) {
	app := &aheadApp{}
	s := live.NewSession(context.Background(), app.route("go", 0, true).Render)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if ds := s.Deferred(); len(ds) != 1 {
		t.Fatalf("go's loads: %d", len(ds))
	}
	rt := app.route("rust", 1, true)
	ahead := app.ahead(t, rt)
	if got := strings.Join(ahead.Topics(), ","); got != "room:rust" {
		t.Fatalf("ahead's topics %s", got)
	}
	ds := ahead.Deferred()
	if len(ds) != 1 {
		t.Fatalf("ahead started %d", len(ds))
	}
	p, err := s.NavigateFrom(rt, ahead, false)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, c, p)
	if got := strings.Join(s.Topics(), ","); got != "room:rust" || ahead.Topics() != nil {
		t.Errorf("topics: live %s, ahead %v", got, ahead.Topics())
	}
	if c.Text("#members") != "members loading" || len(s.Deferred()) != 0 || ahead.Deferred() != nil {
		t.Errorf("the load ahead started starts again: %s", c.Text("#members"))
	}

	ds[0].Run(context.Background())
	if _, err := ahead.Loaded(ds[0]); err == nil {
		t.Errorf("ahead took the load")
	}
	p, err = s.Loaded(ds[0])
	if err != nil {
		t.Fatal(err)
	}
	apply(t, c, p)
	if got := c.Text("#members"); got != "members ann,bob" {
		t.Errorf("after the load: %s", got)
	}

	seen := s.DeferredSeen([]string{"pins"})
	if len(seen) != 1 {
		t.Fatalf("seen: %d", len(seen))
	}
	seen[0].Run(context.Background())
	if p, err = s.Loaded(seen[0]); err != nil {
		t.Fatal(err)
	}
	apply(t, c, p)
	if !strings.Contains(c.HTML(), "pins rules") {
		t.Errorf("pins: %s", c.HTML())
	}

	if err := c.Info("room:rust", NewMessage{"hi"}); err != nil || c.Text("#room") != "rust mount 2 clicks 0 log [hi]" {
		t.Errorf("info: %v %s", err, c.Text("#room"))
	}
}

// TestNavigateFromKeptByPatch: an instance a patch keeps (its Update takes
// the URL) stays the live one, though ahead mounted one in its place.
func TestNavigateFromKeptByPatch(t *testing.T) {
	var gone []string
	router := shelfRouter(&gone)
	s := live.NewSession(context.Background(), router("/shelf?page=1").Render)
	s.SetRouter(router)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	first := mounts
	rt := router("/shelf?page=2")
	ahead := live.NewSession(context.Background(), rt.Render)
	if err := ahead.RenderAhead(); err != nil {
		t.Fatal(err)
	}
	p, err := s.NavigateFrom(rt, ahead, true)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, c, p)
	if got := c.Text("#at"); got != fmt.Sprintf("page 2, mount %d, updates [2]", first) {
		t.Errorf("at %s", got)
	}
	if !slices.Equal(gone, []string{"shelf"}) {
		t.Errorf("unmounted %v, want ahead's shelf", gone)
	}
}

// TestNavigateFromNothing: a nil or closed ahead takes nothing, and a
// session can't navigate from itself.
func TestNavigateFromNothing(t *testing.T) {
	app := &aheadApp{}
	s := live.NewSession(context.Background(), app.route("go", 0, true).Render)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.NavigateFrom(app.route("go", 0, true), s, false); err == nil {
		t.Errorf("navigated from itself")
	}
	rt := app.route("rust", 1, true)
	ahead := app.ahead(t, rt)
	ahead.Close()
	ahead.Close()
	if len(app.gone) != 2 {
		t.Errorf("closing twice: %v", app.gone)
	}
	for _, from := range []*live.Session{ahead, nil} {
		rooms := app.rooms
		p, err := s.NavigateFrom(rt, from, false)
		if err != nil {
			t.Fatal(err)
		}
		apply(t, c, p)
		if app.rooms != rooms+1 {
			t.Errorf("not mounted afresh from %v", from)
		}
		rt = app.route("rust", app.rooms+10, true)
	}
}

// TestNavigateFromStream: the rows a stream's Mount loaded ahead reach the
// client with the moved instance: RenderAhead wrote no reply carrying them.
func TestNavigateFromStream(t *testing.T) {
	store := &chatStore{msgs: msgs(1, 2)}
	page := func(context.Context) ui.Node {
		return ui.El("main", nil, ui.Component("chat", nil, &StreamChat{Store: store, Title: "chat"}))
	}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.El("main", nil) })
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	ahead := live.NewSession(context.Background(), page)
	if err := ahead.RenderAhead(); err != nil {
		t.Fatal(err)
	}
	p, err := s.NavigateFrom(live.Route{Render: page}, ahead, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(live.Marshal(live.PatchMessage(0, p))); err != nil {
		t.Fatal(err)
	}
	if got := c.Text("ul"); got != "m1xm2x" {
		t.Fatalf("rows %q: %s", got, c.HTML())
	}
}
