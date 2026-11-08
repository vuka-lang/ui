package ui_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

var lazyRenders int

// orderList is a region's content: its render is the server work a lazy
// region holds back.
func orderList() ui.Node {
	return ui.Ctx(func(ctx context.Context) ui.Node {
		lazyRenders++
		return ui.El("ul", nil, ui.El("li", nil, ui.Text("order 7")))
	})
}

type lazyPage struct {
	ui.Live
	each, open bool
	fail       bool
}

var lazyPushes int

func (p *lazyPage) Edit(ctx context.Context) error { lazyPushes++; return nil }

func (p *lazyPage) Render() ui.Node {
	content := orderList()
	if p.fail {
		content = ui.Try(nil, errors.New("no orders"))
	}
	return ui.El("main", nil,
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(p.Edit)}}, ui.Text("edit")),
		ui.Lazy(ui.LazyProps{When: p.open, Each: p.each, Children: content}))
}

func joinLazy(t *testing.T, p *lazyPage) *livetest.Client {
	t.Helper()
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "lazy", Props: p, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var (
	initOn  = regexp.MustCompile(`<vk-lazy[^>]*data-vk-on-vkinit="([^"]+)"`)
	clickOn = regexp.MustCompile(`data-vk-on-click="([^"]+)"`)
)

// TestLazyLive: a hidden region renders an empty, busy shell, its content
// not rendered, until its vkinit; then the content stays, rendered with the
// page; Each renders it again at each vkinit; a region shown at the first
// render renders its content then.
func TestLazyLive(t *testing.T) {
	lazyRenders = 0
	c := joinLazy(t, &lazyPage{})
	html := c.HTML()
	m := initOn.FindStringSubmatch(html)
	if lazyRenders != 0 || m == nil || strings.Contains(html, "order 7") || !strings.Contains(html, `data-vk-init aria-busy="true"`) || !strings.Contains(html, `class="vk-lazy-loading" role="status" aria-busy="true" aria-label="Loading…"`) {
		t.Fatalf("a hidden region rendered its content, or isn't marked: %d renders\n%s", lazyRenders, html)
	}
	if _, err := c.Event(ui.Event{Target: m[1], Type: "vkinit"}); err != nil {
		t.Fatal(err)
	}
	html = c.HTML()
	if lazyRenders != 1 || !strings.Contains(html, "order 7") || strings.Contains(html, "data-vk-init") || strings.Contains(html, "vkinit") {
		t.Fatalf("vkinit: %d renders\n%s", lazyRenders, html)
	}
	click := clickOn.FindStringSubmatch(html)
	if _, err := c.Event(ui.Event{Target: click[1], Type: "click"}); err != nil {
		t.Fatal(err)
	}
	if html := c.HTML(); !strings.Contains(html, "order 7") {
		t.Errorf("the content went away:\n%s", html)
	}

	lazyRenders = 0
	c = joinLazy(t, &lazyPage{each: true})
	m = initOn.FindStringSubmatch(c.HTML())
	for range 2 {
		if _, err := c.Event(ui.Event{Target: m[1], Type: "vkinit"}); err != nil {
			t.Fatal(err)
		}
		if m = initOn.FindStringSubmatch(c.HTML()); m == nil || !strings.Contains(c.HTML(), "data-vk-each") {
			t.Fatalf("each: no vkinit handler once shown\n%s", c.HTML())
		}
	}
	if lazyRenders != 2 {
		t.Errorf("each: %d renders", lazyRenders)
	}

	lazyRenders = 0
	c = joinLazy(t, &lazyPage{open: true})
	if html := c.HTML(); lazyRenders != 1 || !strings.Contains(html, "order 7") || strings.Contains(html, "data-vk-init") {
		t.Errorf("shown at first: %d renders\n%s", lazyRenders, html)
	}
}

// TestLazyShownWithEvent: a region the browser showed as it sent another
// event (Event.Shown) renders in that event's reply, after its handler.
func TestLazyShownWithEvent(t *testing.T) {
	lazyRenders, lazyPushes = 0, 0
	c := joinLazy(t, &lazyPage{})
	html := c.HTML()
	init, click := initOn.FindStringSubmatch(html)[1], clickOn.FindStringSubmatch(html)[1]
	if _, err := c.Event(ui.Event{Target: click, Type: "click", Shown: []string{init, click}}); err != nil {
		t.Fatal(err)
	}
	if html := c.HTML(); lazyPushes != 1 || lazyRenders != 1 || !strings.Contains(html, "order 7") {
		t.Errorf("shown with a click: %d pushes (a shown target that isn't an InitHandler doesn't run), %d renders\n%s", lazyPushes, lazyRenders, html)
	}
}

// TestLazyError: an error rendering a region's content shows in the region.
func TestLazyError(t *testing.T) {
	c := joinLazy(t, &lazyPage{fail: true})
	if _, err := c.Event(ui.Event{Target: initOn.FindStringSubmatch(c.HTML())[1], Type: "vkinit"}); err != nil {
		t.Fatal(err)
	}
	if html := c.HTML(); !strings.Contains(html, `<vk-lazy hidden><span role="alert">`) || !strings.Contains(html, "<button") {
		t.Errorf("the region's error:\n%s", html)
	}
}

func lazyHTTPPage() ui.Node {
	return ui.El("main", nil,
		ui.Lazy(ui.LazyProps{When: false, Children: orderList()}),
		ui.Lazy(ui.LazyProps{When: false, Loading: ui.Text("…"), Children: ui.Fragment(
			ui.Text("outer"),
			ui.Lazy(ui.LazyProps{When: false, Children: ui.Text("inner")}),
		)}),
	)
}

// TestLazyRegion: on a page that isn't live a hidden region renders an
// empty shell naming its region, the page needing the runtime; the page
// renders again for one region (RenderRegion) gives that region's HTML
// with its content, a nested one's inside its own; a static render and
// WithEager render the content at once.
func TestLazyRegion(t *testing.T) {
	ctx := context.Background()
	lazyRenders = 0
	var b strings.Builder
	info, err := ui.RenderPage(ctx, lazyHTTPPage(), &b)
	html := b.String()
	if err != nil || info.Live || !info.JS || lazyRenders != 0 || !strings.Contains(html, `<vk-lazy hidden data-vk-region="0" data-vk-init aria-busy="true">`) || !strings.Contains(html, `data-vk-region="1"`) || strings.Contains(html, "outer") {
		t.Fatalf("a page's hidden regions: %v %+v, %d renders\n%s", err, info, lazyRenders, html)
	}
	got, err := ui.RenderRegion(ctx, lazyHTTPPage(), "0")
	if err != nil || lazyRenders != 1 || got != `<vk-lazy hidden data-vk-region="0"><ul><li>order 7</li></ul></vk-lazy>` {
		t.Errorf("region 0: %v, %d renders: %s", err, lazyRenders, got)
	}
	got, err = ui.RenderRegion(ctx, lazyHTTPPage(), "1")
	if err != nil || !strings.Contains(got, `outer<vk-lazy hidden data-vk-region="1.0" data-vk-init`) || lazyRenders != 1 {
		t.Errorf("region 1: %v: %s", err, got)
	}
	got, err = ui.RenderRegion(ctx, lazyHTTPPage(), "1.0")
	if err != nil || got != `<vk-lazy hidden data-vk-region="1.0">inner</vk-lazy>` {
		t.Errorf("region 1.0: %v: %s", err, got)
	}
	if _, err := ui.RenderRegion(ctx, lazyHTTPPage(), "5"); !errors.Is(err, ui.ErrNoRegion) {
		t.Errorf("a region the page hasn't: %v", err)
	}

	lazyRenders = 0
	if s, err := ui.String(ctx, lazyHTTPPage()); err != nil || lazyRenders != 1 || !strings.Contains(s, "order 7") || !strings.Contains(s, "inner") {
		t.Errorf("static: %v, %d renders: %s", err, lazyRenders, s)
	}
	b.Reset()
	if _, err := ui.RenderPage(ui.WithEager(ctx), lazyHTTPPage(), &b); err != nil || !strings.Contains(b.String(), "inner") || strings.Contains(b.String(), "data-vk-init") {
		t.Errorf("eager: %v: %s", err, b.String())
	}
}

// formsPage holds a form type's form hidden, on a page that isn't live.
func formsPage(f *productForm) ui.Node {
	return ui.El("div", []ui.Attr{{Name: "hidden", Value: true}},
		ui.Form{For: ui.For(f), Bound: []string{"Name", "Price"}, Children: ui.Fragment(
			ui.Field{For: ui.For(&f.Value.Name, f)},
			ui.Field{For: ui.For(&f.Value.Price, f)},
		)})
}

// TestFormInitRegion: a hidden form type's form on a page that isn't live
// waits for its Init as a region does; the page rendered again for that
// region runs Init and gives the form.
func TestFormInitRegion(t *testing.T) {
	ctx := context.Background()
	inits = 0
	var b strings.Builder
	info, err := ui.RenderPage(ctx, formsPage(&productForm{}), &b)
	if err != nil || info.Live || inits != 0 || !strings.Contains(b.String(), `data-vk-init aria-busy="true" data-vk-region="0"`) {
		t.Fatalf("a hidden form: %v, %d inits\n%s", err, inits, b.String())
	}
	got, err := ui.RenderRegion(ctx, formsPage(&productForm{}), "0")
	if err != nil || inits != 1 || !strings.HasPrefix(got, "<form ") || !strings.Contains(got, `data-vk-region="0"`) || !strings.Contains(got, `value="10"`) || strings.Contains(got, "data-vk-init") {
		t.Errorf("its region: %v, %d inits: %s", err, inits, got)
	}
}
