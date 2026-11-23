package live_test

import (
	"cmp"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// confirm is a component with an AutoID prop, its markup naming its parts
// by it.
type confirm struct {
	ID    ui.AutoID
	Title string
}

func (c confirm) Render() ui.Node {
	return ui.El("dialog", []ui.Attr{a("id", c.ID), a("aria-labelledby", c.ID.Suffix("title"))},
		ui.El("h2", []ui.Attr{a("id", c.ID.Suffix("title"))}, ui.Text(c.Title)))
}

// confirmTag is <confirm title={t}/> as the compiler writes it: the id the
// tag leaves out is NewAutoID's, in the context the tag renders in.
func confirmTag(title string) ui.Node {
	return ui.Ctx(func(ctx context.Context) ui.Node {
		return confirm{ID: ui.NewAutoID(ctx), Title: title}.Render()
	})
}

// confirmSet is <confirm id?={id} title={t}/>: a zero id is the tag's own.
func confirmSet(id ui.AutoID, title string) ui.Node {
	return ui.Ctx(func(ctx context.Context) ui.Node {
		return confirm{ID: cmp.Or[ui.AutoID](id, ui.NewAutoID(ctx)), Title: title}.Render()
	})
}

type dialogs struct {
	ui.Live
	extra bool
	rows  []string
	n     int
}

func (d *dialogs) Toggle() { d.extra = !d.extra }
func (d *dialogs) Drop()   { d.rows = d.rows[1:] }
func (d *dialogs) Inc()    { d.n++ }

func (d *dialogs) Render() ui.Node {
	var rows []ui.Node
	for _, r := range d.rows {
		rows = append(rows, ui.El("li", []ui.Attr{a("key", r)}, confirmTag("row "+r)))
	}
	var extra ui.Node
	if d.extra {
		extra = ui.El("p", nil, ui.Text("extra"))
	}
	return ui.El("section", nil,
		ui.El("button", []ui.Attr{a("id", "toggle"), a("onClick", ui.On(d.Toggle))}, ui.Text("t")),
		ui.El("button", []ui.Attr{a("id", "drop"), a("onClick", ui.On(d.Drop))}, ui.Text("d")),
		ui.El("button", []ui.Attr{a("id", "inc"), a("onClick", ui.On(d.Inc))}, ui.Text("+")),
		extra,
		ui.GuardNode(1, func() ui.Node { return confirmTag("guarded") }),
		confirmTag("first"),
		confirmTag("second"),
		confirmSet("", "unset"),
		confirmSet("confirm", "set"),
		ui.El("ul", nil, rows...),
		ui.Text(strings.Repeat("+", d.n)),
	)
}

var dialogID = regexp.MustCompile(`<dialog id="([^"]+)" aria-labelledby="([^"]+)"><h2 id="([^"]+)">([^<]+)</h2>`)

// dialogIDs are the ids of the page's dialogs, by title, checking that each
// one's parts are named by its id.
func dialogIDs(t *testing.T, html string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range dialogID.FindAllStringSubmatch(html, -1) {
		if m[2] != m[1]+"-title" || m[3] != m[1]+"-title" {
			t.Fatalf("the parts aren't named by the id: %s", m[0])
		}
		out[m[4]] = m[1]
	}
	return out
}

// TestAutoIDs: an AutoID the tag leaves out is unique on the page and the
// same in the HTTP render, the join and every patch — whether a branch
// before it shows, a guarded part around it is kept, or a keyed row before
// it goes; a value the tag sets is the id.
func TestAutoIDs(t *testing.T) {
	page := func(context.Context) ui.Node {
		return ui.Component("d", nil, &dialogs{rows: []string{"a", "b", "c"}})
	}
	s := live.NewSession(context.Background(), page)
	first, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	ids := dialogIDs(t, first)
	if len(ids) != 8 {
		t.Fatalf("%d dialogs in %s", len(ids), first)
	}
	seen := map[string]string{}
	for title, id := range ids {
		if other, ok := seen[id]; ok {
			t.Fatalf("%s and %s share the id %s", title, other, id)
		}
		seen[id] = title
	}
	if ids["set"] != "confirm" {
		t.Errorf("the id the tag sets is %q", ids["set"])
	}
	if ids["unset"] == "" || ids["unset"] == "confirm" {
		t.Errorf("a zero id set with ?= is %q", ids["unset"])
	}
	again, _ := s.Render()
	if again != first {
		t.Fatalf("the same state renders differently:\n%s\n%s", first, again)
	}
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if c.HTML() != first {
		t.Fatalf("the join differs from the HTTP render:\n%s\n%s", c.HTML(), first)
	}
	same := func(step string, drop ...string) {
		t.Helper()
		now := dialogIDs(t, c.HTML())
		for title, id := range ids {
			if dropped := len(drop) > 0 && title == drop[0]; dropped {
				if _, ok := now[title]; ok {
					t.Errorf("%s: %s is still shown", step, title)
				}
				continue
			}
			if now[title] != id {
				t.Errorf("%s: %s's id is %q, was %q", step, title, now[title], id)
			}
		}
	}
	for _, ev := range []struct{ step, button string }{
		{"a patch beside them", "#inc"},
		{"a branch before them", "#toggle"},
		{"the branch gone again", "#toggle"},
	} {
		if _, err := c.Event(click(c, ev.button)); err != nil {
			t.Fatal(err)
		}
		same(ev.step)
	}
	if _, err := c.Event(click(c, "#drop")); err != nil {
		t.Fatal(err)
	}
	same("a keyed row before them gone", "row a")
}

// TestAutoIDsStatic: a page that isn't live names its ids the same in each
// render, so a lazy region rendered later agrees with the page.
func TestAutoIDsStatic(t *testing.T) {
	page := ui.El("main", nil, confirmTag("one"), confirmTag("two"))
	var a, b strings.Builder
	if _, err := ui.RenderPage(context.Background(), page, &a); err != nil {
		t.Fatal(err)
	}
	if _, err := ui.RenderPage(context.Background(), page, &b); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Fatalf("two renders differ:\n%s\n%s", a.String(), b.String())
	}
	ids := dialogIDs(t, a.String())
	if len(ids) != 2 || ids["one"] == ids["two"] {
		t.Fatalf("ids: %v", ids)
	}
	// with no page around it, each id is still unique
	x, _ := ui.String(context.Background(), confirmTag("x"))
	y, _ := ui.String(context.Background(), confirmTag("x"))
	if x == y {
		t.Errorf("two renders outside a page share an id: %s", x)
	}
}

// TestAutoIDUnset: an AutoID no tag set leaves its attributes out.
func TestAutoIDUnset(t *testing.T) {
	s, err := ui.String(context.Background(), confirm{Title: "plain"}.Render())
	if err != nil {
		t.Fatal(err)
	}
	if want := `<dialog><h2>plain</h2></dialog>`; s != want {
		t.Errorf("%s, want %s", s, want)
	}
	if got := ui.AutoID("dlg").Suffix("title"); got != "dlg-title" {
		t.Errorf("Suffix: %s", got)
	}
}

// panel is a live component with AutoID props.
type panel struct {
	ui.Live
	ID    ui.AutoID
	Help  ui.AutoID
	Label string
	n     int
}

func (p *panel) Inc() { p.n++ }

func (p *panel) Render() ui.Node {
	return ui.El("section", []ui.Attr{a("id", p.ID), a("aria-describedby", p.Help)},
		ui.El("button", []ui.Attr{a("id", "inc-"+p.Label), a("onClick", ui.On(p.Inc))}, ui.Text(p.Label)),
		ui.El("p", []ui.Attr{a("id", p.Help)}, ui.Text(strings.Repeat("+", p.n))))
}

type panels struct {
	ui.Live
	extra bool
}

func (p *panels) Toggle() { p.extra = !p.extra }

func (p *panels) Render() ui.Node {
	var extra ui.Node
	if p.extra {
		extra = ui.Component("x", nil, &panel{Label: "extra"})
	}
	return ui.El("main", nil,
		ui.El("button", []ui.Attr{a("id", "toggle"), a("onClick", ui.On(p.Toggle))}, ui.Text("t")),
		extra,
		ui.Component("a", nil, &panel{Label: "a"}),
		ui.Component("b", nil, &panel{ID: "mine", Label: "b"}))
}

var panelID = regexp.MustCompile(`<section id="([^"]*)" aria-describedby="([^"]*)"[^>]*><button id="inc-(\w+)"`)

// TestAutoIDsLive: a live component's AutoID its tag leaves empty is its
// instance's, the same while the instance lives, whatever its parent's
// patches; one its tag sets is that value.
func TestAutoIDsLive(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("p", nil, &panels{}) })
	first, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	ids := func(html string) map[string][2]string {
		out := map[string][2]string{}
		for _, m := range panelID.FindAllStringSubmatch(html, -1) {
			out[m[3]] = [2]string{m[1], m[2]}
		}
		return out
	}
	was := ids(first)
	if a := was["a"]; a[0] == "" || a[1] != a[0]+"-help" {
		t.Fatalf("a's ids: %q in %s", a, first)
	}
	if b := was["b"]; b[0] != "mine" || b[1] == "" || b[1] == was["a"][1] {
		t.Fatalf("b's ids: %q", b)
	}
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"#toggle", "#inc-a", "#toggle"} {
		if _, err := c.Event(click(c, step)); err != nil {
			t.Fatal(err)
		}
		now := ids(c.HTML())
		for _, k := range []string{"a", "b"} {
			if now[k] != was[k] {
				t.Errorf("after %s: %s's ids %q, were %q", step, k, now[k], was[k])
			}
		}
		if x, ok := now["extra"]; ok && (x[0] == was["a"][0] || x[0] == "") {
			t.Errorf("extra's id %q", x[0])
		}
	}
}
