package live

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2/diag"
)

// ordersCard defers its orders; await renders them through ui.Await, else
// through the branches a component writes itself.
type ordersCard struct {
	ui.Live
	await    bool
	fail     bool
	orders   []string
	clicks   int
	children *atomic.Int32
}

func (c *ordersCard) LiveSchema(l *ui.LiveSchema) {
	l.Field(&c.orders, ui.Defer("orders"))
	l.Loads("orders", c.load)
}

func (c *ordersCard) load(ctx context.Context) error {
	if c.fail {
		return shownErr{errors.New("orders are down")}
	}
	c.orders = []string{"#1", "#2"}
	return nil
}

func (c *ordersCard) Click() { c.clicks++ }

func (c *ordersCard) pending() ui.Node { return ui.HTML(`<i aria-busy="true">loading</i>`) }

func (c *ordersCard) failed(err error) ui.Node {
	return ui.HTML(`<b role="alert">{err.Error()}</b>`, err.Error())
}

func (c *ordersCard) rows() ui.Node {
	if c.children != nil {
		c.children.Add(1)
	}
	var lis []ui.Node
	for _, o := range c.orders {
		lis = append(lis, ui.HTML(`<li key={o}>{o}</li>`, o))
	}
	return ui.HTML(`<ul>{lis}</ul>`, lis)
}

func (c *ordersCard) Render() ui.Node {
	var section ui.Node
	switch {
	case c.await:
		section = ui.Await{Of: c, Key: "orders", Pending: c.pending(), Error: c.failed, Children: c.rows}
	case c.Loading("orders"):
		section = c.pending()
	default:
		if err := c.LoadErr("orders"); err != nil {
			section = c.failed(err)
		} else {
			section = c.rows()
		}
	}
	return ui.HTML(`<section>{section}<button onClick={c.Click}>+</button>{c.clicks}</section>`, section, c.Click, c.clicks)
}

// awaitRun is what a page sends over a v2 connection: the join's trees,
// the load's patch, an event's patch.
func awaitRun(t *testing.T, card *ordersCard) []string {
	t.Helper()
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node {
		return ui.HTML(`<main>{card}</main>`, ui.Component("card", nil, card))
	})
	s.SetErrorText(func(error) string { return "Internal Server Error" })
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	trees, err := s.RenderTrees()
	if err != nil {
		t.Fatal(err)
	}
	out := []string{html, encode(t, trees)}
	for _, d := range s.Deferred() {
		d.Run(context.Background())
		p, err := s.Loaded(d)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, encode(t, p), s.HTML())
	}
	target := regexp.MustCompile(`data-vk-on-click="([^"]+)"`).FindStringSubmatch(s.HTML())
	p, err := s.Event(ui.Event{Target: target[1], Type: "click"})
	if err != nil {
		t.Fatal(err)
	}
	return append(out, encode(t, p), s.HTML())
}

func encode(t *testing.T, v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// An Await renders and patches as the branches it stands for: the join's
// trees, the load's patch and an event's after it.
func TestAwaitAsBranches(t *testing.T) {
	for _, fail := range []bool{false, true} {
		branches := awaitRun(t, &ordersCard{fail: fail})
		awaited := awaitRun(t, &ordersCard{fail: fail, await: true})
		if len(branches) != len(awaited) {
			t.Fatalf("fail=%v: %d messages, branches %d", fail, len(awaited), len(branches))
		}
		for i := range branches {
			if branches[i] != awaited[i] {
				t.Errorf("fail=%v, message %d:\nawait   %s\nbranches %s", fail, i, awaited[i], branches[i])
			}
		}
		want := `>#2</li>`
		if fail {
			want = `<b role="alert">orders are down</b>`
		}
		if !strings.Contains(branches[0], "loading") || !strings.Contains(awaited[3], want) {
			t.Errorf("fail=%v: first render %s\nloaded %s", fail, branches[0], awaited[3])
		}
	}
}

// Children runs once the group has loaded, never while it loads.
func TestAwaitChildrenOnceLoaded(t *testing.T) {
	var n atomic.Int32
	card := &ordersCard{await: true, children: &n}
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node { return ui.Component("card", nil, card) })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 0 {
		t.Fatalf("Children ran %d times while loading", n.Load())
	}
	for _, d := range s.Deferred() {
		d.Run(context.Background())
		if _, err := s.Loaded(d); err != nil {
			t.Fatal(err)
		}
	}
	if n.Load() == 0 || !strings.Contains(s.HTML(), `>#1</li>`) {
		t.Fatalf("Children ran %d times: %s", n.Load(), s.HTML())
	}
}

// A key naming no group of the component fails the render, as Loading does.
func TestAwaitUndeclared(t *testing.T) {
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node { return ui.Component("c", nil, &typoAwait{}) })
	if _, err := s.Render(); diag.Of(err) != "E0325" || !strings.Contains(err.Error(), `typoAwait defers no group "order"`) {
		t.Fatalf("got %v", err)
	}
}

type typoAwait struct {
	ui.Live
	orders []string
}

func (c *typoAwait) LiveSchema(l *ui.LiveSchema) {
	l.Field(&c.orders, ui.Defer("orders"))
	l.Loads("orders", func(context.Context) error { return nil })
}

func (c *typoAwait) Render() ui.Node { return ui.Await{Of: c, Key: "order"} }

// Rendered without a session, a component loads its groups first: its
// Await shows them.
func TestAwaitStatic(t *testing.T) {
	html, err := ui.String(context.Background(), ui.Component("card", nil, &ordersCard{await: true}))
	if err != nil || !strings.Contains(html, `>#1</li>`) {
		t.Fatalf("static: %s %v", html, err)
	}
	html, err = ui.String(context.Background(), ui.Component("card", nil, &ordersCard{await: true, fail: true}))
	if err != nil || !strings.Contains(html, `<b role="alert">orders are down</b>`) {
		t.Fatalf("static, failing: %s %v", html, err)
	}
}
