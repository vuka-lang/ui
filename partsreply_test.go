package ui_test

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2"
)

var markRE = regexp.MustCompile(`<!--vk-part:([^>]+?)-->`)

// shown is the page as its request sends it, its methods unrun, and the
// ids of the parts it shows.
func shown(t *testing.T, page ui.Node) (string, []string) {
	t.Helper()
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{Pending: true})
	defer ps.Stop()
	var b bytes.Buffer
	if _, err := ui.RenderPage(ctx, page, &b); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range markRE.FindAllStringSubmatch(b.String(), -1) {
		ids = append(ids, m[1])
	}
	return b.String(), ids
}

// partsReply is what a request for want, rendering page again, answers.
func partsReply(t *testing.T, page ui.Node, want []string) string {
	t.Helper()
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{Want: want})
	if _, err := ui.RenderPage(ctx, page, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ps.StreamParts(&out, nil); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// A page's parts on a request of their own: the page shows the fallbacks
// with no method run; the request for its parts, rendering it again, gets
// each part it asks for by id, with no script.
func TestPartsReply(t *testing.T) {
	p := &partPage{}
	html, ids := shown(t, p.View())
	if p.calls.Load() != 0 || len(ids) != 2 || !strings.Contains(html, "…") {
		t.Fatalf("the page: %d calls, ids %q: %s", p.calls.Load(), ids, html)
	}
	got := partsReply(t, p.View(), ids)
	want := `<template data-vk-part="` + ids[0] + `"><p>7 open</p></template><!--vk-parts:` + ids[0] + `-->` +
		`<template data-vk-part="` + ids[1] + `"><b>7</b></template><!--vk-parts:` + ids[1] + `-->`
	if got != want || p.calls.Load() != 1 {
		t.Errorf("got %s (%d calls), want %s", got, p.calls.Load(), want)
	}
}

// Only the parts asked for run; one the page no longer has fails; another
// method's part in its place isn't taken for it.
func TestPartsReplyWant(t *testing.T) {
	p := &partPage{}
	page := ui.Group{
		ui.Part(p, (*partPage).Stats, nil, func(n int) ui.Node { return ui.Text(n) }),
		ui.Part(p, (*partPage).A, nil, func(a string) ui.Node { return ui.Text(a) }),
	}
	_, ids := shown(t, page)
	got := partsReply(t, page, ids[1:])
	if p.calls.Load() != 0 || !strings.Contains(got, `"`+ids[1]+`">a<`) || strings.Contains(got, ids[0]) {
		t.Errorf("asked for A only: %d Stats calls: %s", p.calls.Load(), got)
	}
	// The page changed: A's place now holds Stats's part.
	swapped := ui.Group{
		ui.Part(p, (*partPage).A, nil, func(a string) ui.Node { return ui.Text(a) }),
		ui.Part(p, (*partPage).Stats, nil, func(n int) ui.Node { return ui.Text(n) }),
	}
	got = partsReply(t, swapped, ids)
	for _, id := range ids {
		if !strings.Contains(got, `"`+id+`"><span role="alert">This part couldn&#39;t load.</span></template><!--vk-parts:`+id+`-->`) {
			t.Errorf("%s isn't failed: %s", id, got)
		}
	}
}

// A boundary inside a boundary: the page shows the outer; asked for it,
// the reply has the inner too, after it.
func TestPartsReplyNested(t *testing.T) {
	page := ui.Suspense(nil, nil, func(ctx context.Context) ui.Node {
		return ui.Group{ui.Text("outer"), ui.Suspense(nil, nil, func(ctx context.Context) ui.Node { return ui.Text("inner") })}
	})
	_, ids := shown(t, page)
	if len(ids) != 1 {
		t.Fatalf("ids %q", ids)
	}
	got := partsReply(t, page, ids)
	o, i := strings.Index(got, `"`+ids[0]+`">outer<!--vk-part:`), strings.Index(got, ">inner</template>")
	if o < 0 || i < o {
		t.Errorf("want the outer, then the inner: %s", got)
	}
}

// card is a component rendered with props, its method on a value
// receiver.
type card struct{ ID int }

var cardCalls atomic.Int32

func (c card) Count(ctx context.Context) (int, error) { cardCalls.Add(1); return c.ID * 10, nil }

func init() { vuka.RegisterFunc(card.Count, ui.Defer("")) }

func cards(ids ...int) ui.Node {
	return ui.Nodes(func(add func(ui.Node)) {
		for _, id := range ids {
			c := card{id}
			add(ui.El("li", []ui.Attr{{Name: "key", Value: id}}, ui.Part(c, card.Count, nil, func(n int) ui.Node { return ui.Text(n) })))
		}
	})
}

// The parts of components in a keyed list are named by their keys: an
// item added before them since the page was sent doesn't move them.
func TestPartsReplyKeyed(t *testing.T) {
	_, ids := shown(t, cards(1, 2))
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("ids %q", ids)
	}
	got := partsReply(t, cards(0, 1, 2), ids)
	if !strings.Contains(got, `"`+ids[0]+`">10<`) || !strings.Contains(got, `"`+ids[1]+`">20<`) || strings.Contains(got, ">0<") {
		t.Errorf("by key: %s", got)
	}
}

// A group waits only for the parts asked for: B, of A's group, isn't.
func TestPartsReplyGroup(t *testing.T) {
	p := &partPage{}
	page := ui.Group{
		ui.Part(p, (*partPage).A, nil, func(a string) ui.Node { return ui.Text(a) }),
		ui.Part(p, (*partPage).B, nil, func(b string) ui.Node { return ui.Text(b) }),
	}
	_, ids := shown(t, page)
	got := partsReply(t, page, ids[:1])
	if !strings.Contains(got, ">a<") || strings.Contains(got, ">b<") {
		t.Errorf("the group: %s", got)
	}
}

// A binding's element: the page shows its fallback, its method unrun; the
// reply has it.
func TestPartsReplyBinding(t *testing.T) {
	var calls atomic.Int32
	page := ui.Group{ui.Text("shell"), ui.LazyPart(ui.Text("loading"), nil, func(ctx context.Context) ui.Node {
		return ui.Textf("%d categories", ui.LazyValue(ctx, func(context.Context) (int, error) { calls.Add(1); return 3, nil }))
	})}
	html, ids := shown(t, page)
	if len(ids) != 1 || calls.Load() != 0 || !strings.Contains(html, "loading") {
		t.Fatalf("the page: %d calls, %q: %s", calls.Load(), ids, html)
	}
	if got := partsReply(t, page, ids); !strings.Contains(got, `"`+ids[0]+`">3 categories<`) || calls.Load() != 1 {
		t.Errorf("the reply: %s", got)
	}
}
