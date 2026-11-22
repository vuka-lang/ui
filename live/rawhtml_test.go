package live_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

type articlePage struct {
	ui.Live
	title string
	body  ui.RawHTML
	note  string
	grown int
}

func (p *articlePage) Edit() {
	p.body = ui.Sanitize(`<p>second <b>draft</b></p><img src=x onerror="alert(1)">`)
	p.note = "<i>edited</i>"
}

func (p *articlePage) Grow() {
	p.grown++
	p.body = ui.Sanitize(strings.Repeat("<p>a paragraph of the post</p>", 40) + "<p>edit " + strconv.Itoa(p.grown) + "</p>" + strings.Repeat("<p>more</p>", 40))
}

func (p *articlePage) Render() ui.Node {
	return ui.HTML(`<article onClick={p.Edit}><h1 onDblclick={p.Grow}>{p.title}</h1><div class="body">{p.body}</div><p>{p.note}</p></article>`, p.Edit, p.Grow, p.title, p.body, p.note)
}

// TestRawHTMLPatch: a RawHTML value is one dynamic of the render tree: a
// change sends its markup, as is, in the place of the old, and leaves its
// siblings alone; a string beside it is still escaped.
func TestRawHTMLPatch(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("p#1", nil, &articlePage{title: "Hello", body: ui.Sanitize(`<p>first</p>`), note: "draft"})
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if h := c.HTML(); !strings.Contains(h, `<div class="body"><p>first</p></div>`) {
		t.Fatalf("first render: %s", h)
	}
	m, err := c.Event(ui.Event{Target: "c1:0", Type: "click"})
	if err != nil || len(m.Trees) != 1 {
		t.Fatalf("patch: %+v %v", m.Trees, err)
	}
	if tree := string(m.Trees[0].Tree); tree != `{"u":{"4":"<p>second <b>draft</b></p><img src=\"x\"/>","5":"&lt;i&gt;edited&lt;/i&gt;"}}` {
		t.Fatalf("patch %s: the body's markup as one dynamic, the note's text beside it", tree)
	}
	h := c.HTML()
	for _, want := range []string{`>Hello</h1>`, `<div class="body"><p>second <b>draft</b></p><img src="x"/></div>`, `<p>&lt;i&gt;edited&lt;/i&gt;</p>`} {
		if !strings.Contains(h, want) {
			t.Fatalf("client %s\nwants %s", h, want)
		}
	}
	if h != s.HTML() {
		t.Fatalf("client %s\nserver %s", h, s.HTML())
	}
	for i := range 2 { // long markup, then a change in its middle: a token patch
		if m, err = c.Event(ui.Event{Target: c.Handlers()[1][1], Type: "dblclick"}); err != nil || len(m.Trees) != 1 {
			t.Fatalf("grow: %+v %v", m.Trees, err)
		}
		if tree := string(m.Trees[0].Tree); i == 1 && (!strings.Contains(tree, `"p":`) || strings.Contains(tree, "paragraph")) {
			t.Fatalf("a change inside long markup: %s", tree)
		}
		if c.HTML() != s.HTML() {
			t.Fatalf("client %s\nserver %s", c.HTML(), s.HTML())
		}
	}
}
