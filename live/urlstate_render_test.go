package live_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// Batches opens the batch its ?batch names, under which a DataTable keeps its
// parameters under the batch's own Key.
type Batches struct {
	ui.Live
	batch string
}

func (b *Batches) LiveSchema(l *ui.LiveSchema) { l.Field(&b.batch, ui.URL("batch")) }
func (b *Batches) Render() ui.Node {
	nodes := []ui.Node{}
	for _, id := range []string{"1", "2"} {
		nodes = append(nodes, ui.El("button", []ui.Attr{a("id", "open"+id), a("onClick", ui.On(func() { b.batch = id }))}))
	}
	nodes = append(nodes, ui.El("button", []ui.Attr{a("id", "close"), a("onClick", ui.On(func() { b.batch = "" }))}))
	if b.batch != "" {
		nodes = append(nodes, ui.DataTable[shelfBook]{
			ID: "t", Key: "b" + b.batch, Columns: shelfColumns(), Rows: shelf, Sizes: []int{2},
		})
	}
	return ui.El("div", nil, nodes...)
}

// The links an event's render builds from the page's URL carry the parameters
// that event wrote: read from the session right after the event's reply, before
// any navigation the client sends back re-renders the page.
func TestURLLinksSeeEventsEdits(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			render := func() ui.Node { return ui.El("main", nil, ui.Component("batches", nil, &Batches{})) }
			s, c := urlSession(t, v, "/p?advert=7", render)
			h, _ := c.Attr("#open1", "data-vk-on-click")
			if _, err := s.Event(ui.Event{Type: "click", Target: h}); err != nil {
				t.Fatal(err)
			}
			html := s.HTML()
			if !strings.Contains(html, "batch=1") || !strings.Contains(html, "b1.page=2") {
				t.Fatalf("pager links lack the new batch:\n%s", html)
			}
			for _, part := range strings.Split(html, `href="`)[1:] {
				href, _, _ := strings.Cut(part, `"`)
				if strings.Contains(href, "b1.page") && !strings.Contains(href, "batch=1") {
					t.Fatalf("link %q lacks batch=1", href)
				}
			}
		})
	}
}

// A DataTable's parameters leave the address when it stops rendering, and stay
// while it renders.
func TestURLTableParamsDropWithTable(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			render := func() ui.Node { return ui.El("main", nil, ui.Component("batches", nil, &Batches{})) }
			_, c := urlSession(t, v, "/p?batch=1&b1.page=2&b1.q=x", render)
			if err := c.Click("#open1"); err != nil {
				t.Fatal(err)
			}
			if got := c.URL(); got != "/p?batch=1&b1.page=2&b1.q=x" {
				t.Fatalf("still rendered, kept: %q", got)
			}
			if err := c.Click("#open2"); err != nil {
				t.Fatal(err)
			}
			if got := c.URL(); got != "/p?batch=2" {
				t.Fatalf("after switching batch: %q", got)
			}
			if err := c.Click("#close"); err != nil {
				t.Fatal(err)
			}
			if got := c.URL(); got != "/p" {
				t.Fatalf("after close: %q", got)
			}
		})
	}
}
