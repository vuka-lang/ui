package ui_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
)

// keyRow is a struct component written by hand: View is its markup, and
// Render makes the struct a ui.Node.
type keyRow struct{ ID int }

func (r keyRow) View() ui.Node {
	return ui.HTML(`<li key={r.ID}>{r.ID}</li>`, r.ID)
}

func (r keyRow) Render(ctx context.Context, w io.Writer) error { return r.View().Render(ctx, w) }

// A struct component is a Node: rendered by templ's method or as a child,
// it writes its View.
func TestStructComponentIsNode(t *testing.T) {
	card := ui.Card{Title: ui.Text("Members"), NoPad: true, Children: ui.Text("body")}
	var b strings.Builder
	if err := card.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if want := render(t, card.View()); b.String() != want || render(t, card) != want {
		t.Fatalf("Render wrote %s, View renders %s", b.String(), want)
	}
	inner := render(t, ui.HTML(`<main>{c}</main>`, ui.Node(card)))
	if inner != "<main>"+b.String()+"</main>" {
		t.Fatalf("as a child: %s", inner)
	}
}

// In a live session a struct component is its View's tree: the stateful
// components inside it keep their instance and events, and its items key
// as their markup does.
func TestStructComponentLive(t *testing.T) {
	c := join(t, 1, func(context.Context) ui.Node {
		return ui.Card{Title: ui.Text("Tabs"), Children: ui.Component("tabs#1", nil, &tabsHost{})}
	})
	r := root("c1")
	if err := c.Click(r + "#tab-b"); err != nil {
		t.Fatal(err)
	}
	if !c.HasClass(r+"#tab-b", "vk-tab--active") {
		t.Fatalf("tab B not active after the click inside a Card: %s", c.HTML())
	}

	rows := []ui.Node{keyRow{1}, keyRow{2}}
	l := join(t, 1, func(context.Context) ui.Node { return ui.HTML(`<ul>{rows}</ul>`, rows) })
	contains(t, l.HTML(), `<li data-vk-key="1">1</li>`, `<li data-vk-key="2">2</li>`)
	direct := join(t, 1, func(context.Context) ui.Node {
		views := []ui.Node{keyRow{1}.View(), keyRow{2}.View()}
		return ui.HTML(`<ul>{views}</ul>`, views)
	})
	if l.HTML() != direct.HTML() {
		t.Fatalf("struct items render %s, their Views %s", l.HTML(), direct.HTML())
	}
}

// viewCount counts its View's calls.
type viewCount struct{ n *int }

func (c viewCount) View() ui.Node {
	*c.n++
	return ui.HTML(`<b>x</b>`)
}

func (c viewCount) Render(ctx context.Context, w io.Writer) error { return c.View().Render(ctx, w) }

// A struct component's View is built once per render, statically and live.
func TestStructComponentViewOnce(t *testing.T) {
	n := 0
	c := viewCount{&n}
	for _, tc := range []struct {
		node  ui.Node
		views int
	}{
		{ui.HTML(`<p>{c}</p>`, ui.Node(c)), 1},
		{ui.Actions(c, c), 2},
		{ui.Card{Children: c}, 1},
	} {
		n = 0
		render(t, tc.node)
		if n != tc.views {
			t.Errorf("rendering built %d views, want %d", n, tc.views)
		}
		n = 0
		join(t, 1, func(context.Context) ui.Node { return tc.node })
		if n != tc.views {
			t.Errorf("a live join built %d views, want %d", n, tc.views)
		}
	}
}
