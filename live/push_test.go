package live_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
)

type Pushed struct {
	ui.Live
	helper
	n int
}

type helper struct{}

func (helper) Help() {}

func (p *Pushed) Render() ui.Node { return ui.El("p", nil, ui.Text("n"), ui.Child(p.n)) }

func (p *Pushed) Add(n int)         { p.n += n }
func (p *Pushed) Bumped()           { p.PushEvent("bumped", map[string]int{"n": p.n}) }
func (p *Pushed) Update(n int) bool { p.n = n; return true }

// A component's script pushes its methods by name (ctx.push): one it
// declares, but not Render, Mount, Update or one it embeds; PushEvent hands
// a payload to the script as a vk:event dispatched on its root.
func TestPushByName(t *testing.T) {
	c := join(t, func(context.Context) ui.Node { return ui.Component("p", nil, &Pushed{}) })
	m := c.send(live.ClientMessage{Type: live.EventMsg, Event: ui.Event{Target: "c1:Add", Type: "push", Value: "5"}})
	if m.Type != live.PatchMsg || len(m.Updates) == 0 || !strings.Contains(m.Updates[0].HTML, "n5") {
		t.Fatalf("Add: %+v", m)
	}
	m = c.send(live.ClientMessage{Type: live.EventMsg, Event: ui.Event{Target: "c1:Bumped", Type: "push"}})
	if len(m.JS) != 1 || m.JS[0].ID != "c1" || !strings.Contains(string(m.JS[0].Ops), `"vk:event"`) ||
		!strings.Contains(string(m.JS[0].Ops), `{"name":"bumped","payload":{"n":5}}`) || !strings.Contains(string(m.JS[0].Ops), `"bubbles":false`) {
		t.Fatalf("PushEvent: %+v", m.JS)
	}
	for _, name := range []string{"Render", "Mount", "Update", "Help", "ID", "Subscribe", "PushJS", "add", "Missing"} {
		if m := c.send(live.ClientMessage{Type: live.EventMsg, Event: ui.Event{Target: "c1:" + name, Type: "push"}}); m.Type != live.Error {
			t.Errorf("%s was pushed: %+v", name, m)
		}
	}
	if c.s.Handler("c1:Add") == nil || c.s.Handler("c1:Render") != nil {
		t.Error("Handler: by name")
	}
}

// Scoped is a component as the transpiler lowers it for a scoped
// declaration: its elements carry data-vk-s, a static of their frame.
type Scoped struct {
	ui.Live
	n int
}

func (s *Scoped) Inc() { s.n++ }
func (s *Scoped) Render() ui.Node {
	return ui.F(0x5c0, "Es(Eds(t)Es(h))", ui.El("div", []ui.Attr{{Name: "data-vk-s", Value: "k1"}},
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(s.Inc)}, {Name: "data-vk-s", Value: "k1"}}, ui.Text("+")),
		ui.El("output", []ui.Attr{{Name: "data-vk-s", Value: "k1"}}, ui.Child(s.n))))
}

// A component's scope is in the statics its first render sends, and in no
// change after it: scoping costs a live page nothing per event.
func TestScopeIsStatic(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("s", nil, &Scoped{}) })
	trees, err := s.RenderTrees()
	if err != nil {
		t.Fatal(err)
	}
	first := ""
	for _, u := range trees {
		first += string(u.Tree)
	}
	if strings.Count(first, `data-vk-s=\"k1\"`) != 3 {
		t.Fatalf("join: %s", first)
	}
	for i := range 3 {
		p, err := s.Event(ui.Event{Target: "c1:0", Type: "click"})
		if err != nil || len(p.Trees) != 1 {
			t.Fatalf("event %d: %+v %v", i, p, err)
		}
		if tr := string(p.Trees[0].Tree); strings.Contains(tr, "data-vk-s") || !strings.Contains(tr, strconv.Itoa(i+1)) {
			t.Fatalf("event %d's change carries the scope: %s", i, tr)
		}
	}
}
