package live_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/ui/v2/uigen"
)

// Tabs keeps its choice in the URL's ?tab, "a" when the URL has none.
type Tabs struct {
	ui.Live
	ID     string
	active string
	mounts int
}

func (t *Tabs) LiveSchema(l *ui.LiveSchema) {
	l.Field(&t.active, ui.URL("tab").Default("a"))
	l.Mount(t.mount)
}
func (t *Tabs) mount(context.Context) error { t.mounts++; return nil }
func (t *Tabs) Pick(v string)               { t.active = v }
func (t *Tabs) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("p", []ui.Attr{a("id", t.ID)}, ui.Text(fmt.Sprintf("%s mount %d", t.active, t.mounts))),
		ui.El("button", []ui.Attr{a("id", t.ID+"-b"), a("onClick", ui.On(func() { t.Pick("b") }))}),
		ui.El("button", []ui.Attr{a("id", t.ID+"-a"), a("onClick", ui.On(func() { t.Pick("a") }))}),
		ui.El("button", []ui.Attr{a("id", t.ID+"-push"), a("onClick", ui.On(func(ctx context.Context) {
			ui.EditURL(ctx, func(q url.Values) { q.Set("tab", "c") }, true)
		}))}))
}

// Pager keeps its page (an Assign, as a .vuka.html file's state is) in
// ?page, 1 when the URL has none, and its sort as an exported prop.
type Pager struct {
	ui.Live
	ID   string
	Sort string
	page uigen.Assign[int]
}

func (p *Pager) LiveSchema(l *ui.LiveSchema) {
	l.Field(&p.page, ui.URL("page").Default(1))
	l.Field(&p.Sort, ui.URL("sort"))
}
func (p *Pager) Next()   { p.page.Set(p.page.Get() + 1) }
func (p *Pager) First()  { p.page.Set(1) }
func (p *Pager) ByName() { p.Sort = "name" }
func (p *Pager) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("p", []ui.Attr{a("id", p.ID)}, ui.Text(fmt.Sprintf("page %d sort %s", p.page.Get(), p.Sort))),
		ui.El("button", []ui.Attr{a("id", p.ID+"-next"), a("onClick", ui.On(p.Next))}),
		ui.El("button", []ui.Attr{a("id", p.ID+"-first"), a("onClick", ui.On(p.First))}),
		ui.El("button", []ui.Attr{a("id", p.ID+"-sort"), a("onClick", ui.On(p.ByName))}))
}

// urlRouter renders the page of a URL: render builds its components; a
// patch keeps them mounted.
func urlRouter(render func() ui.Node) live.Router {
	return func(u string) live.Route {
		pu, _ := url.Parse(u)
		ctx := ui.WithPageURL(context.Background(), pu, nil)
		return live.Route{Ctx: ctx, Render: func(context.Context) ui.Node { return render() }, Keep: true}
	}
}

func urlSession(t *testing.T, v int, start string, render func() ui.Node) (*live.Session, *livetest.Client) {
	t.Helper()
	u, _ := url.Parse(start)
	s := live.NewSession(ui.WithPageURL(context.Background(), u, nil), func(context.Context) ui.Node { return render() })
	s.SetRouter(urlRouter(render))
	c, err := livetest.Join(s, v)
	if err != nil {
		t.Fatal(err)
	}
	c.SetURL(start)
	return s, c
}

func TestURLFieldReadWrite(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			render := func() ui.Node { return ui.El("main", nil, ui.Component("tabs", nil, &Tabs{ID: "t"})) }
			s, c := urlSession(t, v, "/p?tab=c&x=1", render)
			check := func(url, text string) {
				t.Helper()
				if c.URL() != url || c.Text("#t") != text || c.HTML() != s.HTML() {
					t.Fatalf("at %q %q, want %q %q\n%s", c.URL(), c.Text("#t"), url, text, c.HTML())
				}
			}
			check("/p?tab=c&x=1", "c mount 1")

			// Assigning the field writes the URL, the other parameters kept,
			// in place of the current history entry; the instance stays.
			if err := c.Click("#t-b"); err != nil {
				t.Fatal(err)
			}
			check("/p?tab=b&x=1", "b mount 1")
			if h, i := c.History(); len(h) != 1 || i != 0 {
				t.Fatalf("history %v %d: a URL field's write takes the entry's place", h, i)
			}
			// Its default leaves the parameter out.
			if err := c.Click("#t-a"); err != nil {
				t.Fatal(err)
			}
			check("/p?x=1", "a mount 1")

			// An edit asking for a history entry adds one; back refills the
			// field from the URL it returns to, forward again.
			if err := c.Click("#t-push"); err != nil {
				t.Fatal(err)
			}
			check("/p?tab=c&x=1", "c mount 1")
			if _, err := c.Back(); err != nil {
				t.Fatal(err)
			}
			check("/p?x=1", "a mount 1")
			if _, err := c.Forward(); err != nil {
				t.Fatal(err)
			}
			check("/p?tab=c&x=1", "c mount 1")

			// A patch from elsewhere (a link) fills it too.
			if _, err := c.Patch("/p?tab=z"); err != nil {
				t.Fatal(err)
			}
			check("/p?tab=z", "z mount 1")
		})
	}
}

func TestURLFieldPrecedence(t *testing.T) {
	render := func() ui.Node {
		return ui.El("main", nil,
			ui.Component("free", nil, &Pager{ID: "free"}),
			ui.Component("set", nil, &Pager{ID: "set", Sort: "date"}))
	}
	_, c := urlSession(t, 2, "/p?sort=size&page=3", render)
	// The parent's prop wins; one it leaves out is the URL's.
	if c.Text("#free") != "page 3 sort size" || c.Text("#set") != "page 3 sort date" {
		t.Fatalf("free %q, set %q", c.Text("#free"), c.Text("#set"))
	}
	// An event setting a prop the parent leaves out writes the URL, and
	// the parent's next render doesn't take it back.
	if err := c.Click("#free-sort"); err != nil {
		t.Fatal(err)
	}
	if c.URL() != "/p?page=3&sort=name" || c.Text("#free") != "page 3 sort name" || c.Text("#set") != "page 3 sort date" {
		t.Fatalf("at %q: free %q, set %q", c.URL(), c.Text("#free"), c.Text("#set"))
	}
	// A prop the parent passes is never written.
	if err := c.Click("#set-sort"); err != nil {
		t.Fatal(err)
	}
	if c.URL() != "/p?page=3&sort=name" {
		t.Fatalf("at %q: the parent's prop was written", c.URL())
	}
}

func TestURLKey(t *testing.T) {
	render := func() ui.Node {
		return ui.El("main", nil,
			ui.URLKey("requests", ui.Component("a", nil, &Pager{ID: "requests"})),
			ui.URLKey("users", ui.Component("b", nil, &Pager{ID: "users"})))
	}
	s, c := urlSession(t, 2, "/p?requests.page=2&users.page=5", render)
	if c.Text("#requests") != "page 2 sort " || c.Text("#users") != "page 5 sort " {
		t.Fatalf("requests %q, users %q", c.Text("#requests"), c.Text("#users"))
	}
	if err := c.Click("#requests-next"); err != nil {
		t.Fatal(err)
	}
	if c.URL() != "/p?requests.page=3&users.page=5" || c.Text("#requests") != "page 3 sort " || c.Text("#users") != "page 5 sort " {
		t.Fatalf("at %q: requests %q, users %q", c.URL(), c.Text("#requests"), c.Text("#users"))
	}
	if err := c.Click("#users-first"); err != nil {
		t.Fatal(err)
	}
	if c.URL() != "/p?requests.page=3" || c.HTML() != s.HTML() {
		t.Fatalf("at %q", c.URL())
	}
}

func TestURLFieldStatic(t *testing.T) {
	u, _ := url.Parse("/p?requests.page=4&tab=b")
	ctx := ui.WithPageURL(context.Background(), u, nil)
	out, err := ui.String(ctx, ui.El("main", nil,
		ui.Component("t", nil, &Tabs{ID: "t"}),
		ui.URLKey("requests", ui.Component("p", nil, &Pager{ID: "p"}))))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "b mount 1") || !strings.Contains(out, "page 4 sort") {
		t.Fatal(out)
	}
}

// Invoice binds the route's path parameter {id}.
type Invoice struct {
	ui.Live
	id int64
}

func (o *Invoice) LiveSchema(l *ui.LiveSchema) { l.Field(&o.id, *new(ui.URL)) }
func (o *Invoice) Render() ui.Node             { return ui.El("p", nil, ui.Text(fmt.Sprint("order ", o.id))) }

func TestURLFieldPathParam(t *testing.T) {
	u, _ := url.Parse("/orders/42")
	ctx := ui.WithPageURL(context.Background(), u, func(n string) string {
		if strings.EqualFold(n, "id") {
			return "42"
		}
		return ""
	})
	out, err := ui.String(ctx, ui.Component("o", nil, &Invoice{}))
	if err != nil || out != "<p>order 42</p>" {
		t.Fatal(out, err)
	}
}

// Bad keeps a map in the URL, which can't hold one.
type Bad struct {
	ui.Live
	m map[string]int
}

func (b *Bad) LiveSchema(l *ui.LiveSchema) { l.Field(&b.m, ui.URL("m")) }
func (b *Bad) Render() ui.Node             { return ui.Text("") }

func TestURLFieldChecked(t *testing.T) {
	_, err := ui.String(context.Background(), ui.Component("b", nil, &Bad{}))
	if err == nil || !strings.Contains(err.Error(), "E0335") && !strings.Contains(err.Error(), "URL can't hold") {
		t.Fatal(err)
	}
}
