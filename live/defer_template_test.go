package live

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2/diag"
)

// statsCard is a component rendered by its own markup with two deferred
// groups: stats, an unexported field, and Rows, an exported one, which its
// tag never sets.
type statsCard struct {
	ui.Live
	Title  string
	Rows   []string
	stats  map[string]int
	clicks int
	fail   bool
	loads  *atomic.Int32
}

func (c *statsCard) LiveSchema(l *ui.LiveSchema) {
	l.Field(&c.stats, ui.Defer("stats"))
	l.Field(&c.Rows, ui.Defer("rows"))
	l.Loads("stats", c.loadStats)
	l.Loads("rows", c.loadRows)
}

func (c *statsCard) loadStats(ctx context.Context) error {
	if c.loads != nil {
		c.loads.Add(1)
	}
	if c.fail {
		return shownErr{errors.New("stats are down")}
	}
	c.stats = map[string]int{"open": 3}
	c.Title = "changed by a loader" // not the group's: dropped
	return nil
}

func (c *statsCard) loadRows(ctx context.Context) error {
	if c.loads != nil {
		c.loads.Add(1)
	}
	c.Rows = []string{"a", "b"}
	return nil
}

func (c *statsCard) Click() { c.clicks++ }

func (c *statsCard) Render() ui.Node {
	var stats, rows ui.Node
	switch {
	case c.Loading("stats"):
		stats = ui.Text("stats loading")
	case c.LoadErr("stats") != nil:
		stats = ui.Text("stats failed: " + c.LoadErr("stats").Error())
	default:
		stats = ui.Text(fmt.Sprintf("open %d", c.stats["open"]))
	}
	if c.Loading("rows") {
		rows = ui.Text("rows loading")
	} else {
		rows = ui.Text("rows " + strings.Join(c.Rows, ","))
	}
	return ui.El("section", nil, ui.El("h2", nil, ui.Text(c.Title)), ui.El("p", nil, stats), ui.El("p", nil, rows),
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(c.Click)}}, ui.Text("click")), ui.Child(c.clicks))
}

type shownErr struct{ error }

func (e shownErr) PublicMessage() string { return e.Error() }

func cardSession(card func() *statsCard) *Session {
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node {
		return ui.El("main", nil, ui.Component("card", nil, card()))
	})
	s.SetErrorText(func(error) string { return "Internal Server Error" })
	return s
}

func TestTemplateDeferred(t *testing.T) {
	s := cardSession(func() *statsCard { return &statsCard{Title: "Orders"} })
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "stats loading") || !strings.Contains(html, "rows loading") {
		t.Fatalf("first render: %s", html)
	}
	ds := s.Deferred()
	if len(ds) != 2 || len(s.Deferred()) != 0 {
		t.Fatalf("deferred %d, then more", len(ds))
	}
	for _, d := range ds {
		if err := d.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if ds[0].Group != "Rows" || ds[1].Group != "Stats" {
		t.Fatalf("groups %s, %s: in the order of their first fields", ds[0].Group, ds[1].Group)
	}
	p, err := s.Loaded(ds[0]) // each group applies on its own
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Updates) != 1 || !strings.Contains(p.Updates[0].HTML, "rows a,b") || !strings.Contains(p.Updates[0].HTML, "stats loading") {
		t.Fatalf("rows: %+v", p)
	}
	p, err = s.Loaded(ds[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Updates) != 1 || !strings.Contains(p.Updates[0].HTML, "open 3") || !strings.Contains(p.Updates[0].HTML, "<h2>Orders</h2>") {
		t.Fatalf("stats: %+v", p)
	}
}

func TestTemplateDeferredError(t *testing.T) {
	s := cardSession(func() *statsCard { return &statsCard{fail: true} })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	var stats *ui.DeferredLoad
	for _, d := range s.Deferred() {
		if d.Group == "Stats" {
			stats = d
		}
	}
	stats.Run(context.Background())
	p, err := s.Loaded(stats)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Updates) != 1 || !strings.Contains(p.Updates[0].HTML, "stats failed: stats are down") {
		t.Fatalf("patch: %+v", p)
	}
	c := s.Component("c1:0").(*statsCard)
	var de *ui.DeferError
	if err := c.LoadErr("stats"); !errors.As(err, &de) || de.Group != "stats" || errors.Unwrap(err) == nil || err.Error() != "stats are down" {
		t.Errorf("LoadErr: %#v", err)
	}
}

// The text of an error that isn't the app's to show is the session's.
func TestTemplateDeferredErrorText(t *testing.T) {
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node { return ui.Component("c", nil, &failCard{}) })
	s.SetErrorText(func(error) string { return "Internal Server Error" })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	ds := s.Deferred()
	ds[0].Run(context.Background())
	p, err := s.Loaded(ds[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Updates) != 1 || !strings.Contains(p.Updates[0].HTML, "failed: Internal Server Error") || strings.Contains(p.Updates[0].HTML, "refused") {
		t.Fatalf("patch: %+v", p)
	}
}

type failCard struct {
	ui.Live
	n int
}

func (c *failCard) LiveSchema(l *ui.LiveSchema) {
	l.Field(&c.n, ui.Defer("n"))
	l.Loads("n", c.load)
}

func (c *failCard) load(ctx context.Context) error { return errors.New("db: connection refused") }

func (c *failCard) Render() ui.Node {
	if err := c.LoadErr("n"); err != nil {
		return ui.El("p", nil, ui.Text("failed: "+err.Error()))
	}
	return ui.El("p", nil, ui.Child(c.n))
}

// A parent rendering a component's tag again never sets its deferred
// fields: they are the component's, set by its loader.
func TestTemplateDeferredKeptByParent(t *testing.T) {
	parent := 0
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node {
		return ui.El("main", nil, ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(func() { parent++ })}}), ui.Child(parent),
			ui.Component("card", nil, &statsCard{Title: "Orders"}))
	})
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range s.Deferred() {
		d.Run(context.Background())
		if _, err := s.Loaded(d); err != nil {
			t.Fatal(err)
		}
	}
	target := regexp.MustCompile(`data-vk-on-click="(c0:[^"]+)"`).FindStringSubmatch(html)
	if target == nil {
		t.Fatalf("no parent click: %s", html)
	}
	p, err := s.Event(ui.Event{Target: target[1], Type: "click"})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.HTML(); !strings.Contains(got, "rows a,b") || !strings.Contains(got, "open 3") {
		t.Fatalf("after the parent rendered again: %+v\n%s", p, got)
	}
}

// A load whose component is gone by the time it returns changes nothing.
func TestTemplateDeferredGone(t *testing.T) {
	show := true
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node {
		if !show {
			return ui.El("main", nil, ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(func() {})}}))
		}
		return ui.El("main", nil, ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(func() { show = false })}}), ui.Component("card", nil, &statsCard{}))
	})
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	ds := s.Deferred()
	target := regexp.MustCompile(`data-vk-on-click="(c0:[^"]+)"`).FindStringSubmatch(html)
	if _, err := s.Event(ui.Event{Target: target[1], Type: "click"}); err != nil {
		t.Fatal(err)
	}
	ds[0].Run(context.Background())
	if p, err := s.Loaded(ds[0]); err != nil || len(p.Updates) != 0 || len(p.Trees) != 0 {
		t.Fatalf("a gone component's load: %+v %v", p, err)
	}
}

// A component an event mounts starts its loads at the next Deferred.
func TestTemplateDeferredMountedLater(t *testing.T) {
	show := false
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node {
		kids := []ui.Node{ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(func() { show = true })}})}
		if show {
			kids = append(kids, ui.Component("card", nil, &statsCard{}))
		}
		return ui.El("main", nil, kids...)
	})
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if ds := s.Deferred(); len(ds) != 0 {
		t.Fatalf("deferred before the card: %d", len(ds))
	}
	target := regexp.MustCompile(`data-vk-on-click="(c0:[^"]+)"`).FindStringSubmatch(html)
	p, err := s.Event(ui.Event{Target: target[1], Type: "click"})
	if err != nil || !strings.Contains(fmt.Sprint(p.Updates), "stats loading") {
		t.Fatalf("the card mounts loading: %+v %v", p, err)
	}
	if ds := s.Deferred(); len(ds) != 2 {
		t.Fatalf("deferred after the card: %d", len(ds))
	}
}

// LoadDeferred loads every group before the page is sent: what a page
// rendered for a form posted without script shows.
func TestTemplateLoadDeferred(t *testing.T) {
	var n atomic.Int32
	s := cardSession(func() *statsCard { return &statsCard{loads: &n} })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadDeferred(context.Background()); err != nil {
		t.Fatal(err)
	}
	if html := s.HTML(); !strings.Contains(html, "open 3") || !strings.Contains(html, "rows a,b") || n.Load() != 2 {
		t.Fatalf("loaded %d: %s", n.Load(), html)
	}
	if ds := s.Deferred(); len(ds) != 0 {
		t.Errorf("started again: %d", len(ds))
	}
}

// Rendered without a session, a component loads its groups before its
// only render.
func TestTemplateDeferredStatic(t *testing.T) {
	html, err := ui.String(context.Background(), ui.Component("card", nil, &statsCard{Title: "Orders"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "open 3") || !strings.Contains(html, "rows a,b") || !strings.Contains(html, "<h2>Orders</h2>") {
		t.Fatalf("static: %s", html)
	}
	html, err = ui.String(context.Background(), ui.Component("card", nil, &statsCard{fail: true}))
	if err != nil || !strings.Contains(html, "stats failed: stats are down") {
		t.Fatalf("static, failing: %s %v", html, err)
	}
}

type typoCard struct {
	ui.Live
	n int
}

func (c *typoCard) LiveSchema(l *ui.LiveSchema) {
	l.Field(&c.n, ui.Defer("n"))
	l.Loads("n", c.load)
}

func (c *typoCard) load(ctx context.Context) error { return nil }

func (c *typoCard) Render() ui.Node {
	if c.Loading("stats") {
		return ui.Text("…")
	}
	return ui.Child(c.n)
}

func TestTemplateLoadingUndeclared(t *testing.T) {
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node { return ui.Component("c", nil, &typoCard{}) })
	_, err := s.Render()
	if diag.Of(err) != "E0325" || !strings.Contains(err.Error(), `typoCard defers no group "stats"`) {
		t.Fatalf("got %v", err)
	}
}

type noLoaderCard struct {
	ui.Live
	n int
}

func (c *noLoaderCard) LiveSchema(l *ui.LiveSchema) { l.Field(&c.n, ui.Defer("n")) }
func (c *noLoaderCard) Render() ui.Node             { return ui.Child(c.n) }

func TestTemplateDeferredNoLoader(t *testing.T) {
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node { return ui.Component("c", nil, &noLoaderCard{}) })
	_, err := s.Render()
	if diag.Of(err) != "E0324" {
		t.Fatalf("got %v", err)
	}
}
