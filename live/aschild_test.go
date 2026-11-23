package live_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// donor is a component as vuka writes one: its button takes its
// Fallthrough, and asChild replaces the button with the tag's child.
type donor struct {
	Fallthrough ui.Fallthrough `json:"-"`
	Children    ui.Node
}

func (d donor) View() ui.Node {
	return ui.HTML(`<button {d.Fallthrough.Attrs()...} class="btn px-4{d.Fallthrough.Classes()}" data-kind="donor">{d.Children}</button>`,
		d.Fallthrough.Attrs(), d.Fallthrough.Classes(), d.Children)
}

func (d donor) Render(ctx context.Context, w io.Writer) error { return d.View().Render(ctx, w) }

type Reports struct {
	ui.Live
	n int
}

func (p *Reports) Inc() { p.n++ }

func (p *Reports) Render() ui.Node {
	child := ui.HTML(`<a id="report" href="/reports/42" class="px-2" onClick={p.Inc}>Report {p.n}</a>`, p.Inc, p.n)
	return ui.HTML(`<main>{x}<p>{p.n}</p></main>`, ui.AsChild(donor{Fallthrough: ui.Fallthrough{{Name: "aria-label", Value: "Report"}, ui.AsChildAttr(child)}}), p.n)
}

// An asChild child is the live page's element: its handler runs, the page
// renders again, and the element keeps its tag, id and donated attributes
// in each render, the forward element never written.
func TestAsChildLive(t *testing.T) {
	page := &Reports{}
	c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("r", nil, page) }), 2)
	if err != nil {
		t.Fatal(err)
	}
	check := func(n string) {
		t.Helper()
		html := c.HTML()
		for _, want := range []string{`class="btn px-2"`, `data-kind="donor"`, `aria-label="Report"`, `href="/reports/42"`} {
			if v, ok := c.Attr("a#report", strings.SplitN(want, "=", 2)[0]); !ok || `"`+v+`"` != strings.SplitN(want, "=", 2)[1] {
				t.Errorf("render %s: <a> has no %s: %s", n, want, html)
			}
		}
		if c.Exists("button") || strings.Contains(html, "aschild") || strings.Contains(html, "Fallthrough") {
			t.Errorf("render %s: the forward element or its Fallthrough reached the page: %s", n, html)
		}
		if got := c.Text("a#report"); got != "Report "+n {
			t.Errorf("render %s: the link says %q", n, got)
		}
	}
	check("0")
	for _, n := range []string{"1", "2"} {
		if err := c.Click("a#report"); err != nil {
			t.Fatal(err)
		}
		check(n)
		if got := c.Text("p"); got != n {
			t.Errorf("the page didn't render again: %q", got)
		}
	}
}
