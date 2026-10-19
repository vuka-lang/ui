package live_test

import (
	"context"
	"fmt"
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

// A component's scripts push the methods AllowPush lists by name, under
// either protocol; an unlisted one, a component with none listed, and
// Render, Mount, Update or a promoted method are refused like an unknown
// handler. PushEvent hands a payload to the scripts as a vk:event
// dispatched on its root.
func TestPushByName(t *testing.T) {
	typ := "github.com/vuka-lang/ui/live_test.Pushed"
	page := func(context.Context) ui.Node { return ui.Component("p", nil, &Pushed{}) }
	for _, v := range []int{1, 2} {
		t.Run("v"+strconv.Itoa(v), func(t *testing.T) {
			s := live.NewSession(context.Background(), page)
			ref := 0
			send := func(m live.ClientMessage) live.ServerMessage {
				ref++
				m.Ref = ref
				return s.Handle(m)
			}
			push := func(name string, value string) live.ServerMessage {
				return send(live.ClientMessage{Type: live.EventMsg, Event: ui.Event{Target: "c1:" + name, Type: "push", Value: value}})
			}
			if m := send(live.ClientMessage{Type: live.Join, V: v}); m.Type != live.Render {
				t.Fatalf("join: %+v", m)
			}
			live.AllowPush(typ)
			if m := push("Add", "1"); m.Type != live.Error || m.Error != `live: no handler "c1:Add"` {
				t.Errorf("a component with no pushes allowed took one: %+v", m)
			}
			live.AllowPush(typ, "Add", "Bumped", "Render", "Help")
			defer live.AllowPush(typ)
			m := push("Add", "5")
			if m.Type != live.PatchMsg || !strings.Contains(fmt.Sprint(m.Updates, string(treeJSON(m.Trees))), "5") {
				t.Fatalf("Add: %+v", m)
			}
			m = push("Bumped", "")
			if len(m.JS) != 1 || m.JS[0].ID != "c1" || !strings.Contains(string(m.JS[0].Ops), `"vk:event"`) ||
				!strings.Contains(string(m.JS[0].Ops), `{"name":"bumped","payload":{"n":5}}`) || !strings.Contains(string(m.JS[0].Ops), `"bubbles":false`) {
				t.Fatalf("PushEvent: %+v", m.JS)
			}
			for _, name := range []string{"Update", "Render", "Mount", "Help", "ID", "Subscribe", "PushJS", "add", "Missing"} {
				if m := push(name, ""); m.Type != live.Error || m.Error != `live: no handler "c1:`+name+`"` {
					t.Errorf("%s was pushed: %+v", name, m)
				}
			}
			if s.Handler("c1:Add") == nil || s.Handler("c1:Update") != nil {
				t.Error("Handler: by name")
			}
		})
	}
}

func treeJSON(ts []live.TreeUpdate) []byte {
	var b []byte
	for _, t := range ts {
		b = append(b, t.Tree...)
	}
	return b
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

type Searched struct {
	ui.Live
	Q string
}

func (s *Searched) LiveSchema(l *ui.LiveSchema) { l.Field(&s.Q, ui.Bind{}) }
func (s *Searched) Render() ui.Node             { return ui.El("p", nil, ui.Text("q="), ui.Child(s.Q)) }

// A page script's live.q.set(v) pushes _set: the page's bind: fields, as
// ui.SetBound sets them, once AllowPush lists it.
func TestPushSet(t *testing.T) {
	typ := "github.com/vuka-lang/ui/live_test.Searched"
	page := &Searched{}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("s", nil, page) })
	s.Handle(live.ClientMessage{Type: live.Join, V: 2, Ref: 1})
	ref := 1
	set := func() live.ServerMessage {
		ref++
		return s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: ref, Event: ui.Event{Target: "c1:_set", Type: "push", Data: []byte(`{"Q":"go"}`)}})
	}
	if m := set(); m.Type != live.Error {
		t.Fatalf("_set without AllowPush: %+v", m)
	}
	live.AllowPush(typ, "_set")
	defer live.AllowPush(typ)
	if m := set(); m.Type != live.PatchMsg || !strings.Contains(fmt.Sprint(m.Updates, string(treeJSON(m.Trees))), "go") {
		t.Fatalf("_set: %+v", m)
	}
}

type BoundSearch struct {
	ui.Live
	Q     string
	Limit int
	other string
}

func (s *BoundSearch) LiveSchema(l *ui.LiveSchema) {
	l.Field(&s.Q, ui.Bind{})
	l.Field(&s.Limit, ui.Bind{})
}

func (s *BoundSearch) Render() ui.Node {
	return ui.HTML(`<p><input {ui.BindValue(s, &s.Q)...}/><input {ui.BindValue(s, &s.Limit)...}/>{s.Q}/{s.Limit}</p>`,
		ui.BindValue(s, &s.Q), ui.BindValue(s, &s.Limit), s.Q, s.Limit)
}

// bind:value's input sets its bound field: the page renders again with
// what was typed, a number field taking a number.
func TestBindValue(t *testing.T) {
	page := &BoundSearch{Limit: 3}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("b", nil, page) })
	m := s.Handle(live.ClientMessage{Type: live.Join, V: 2, Ref: 1})
	if b := string(treeJSON(m.Trees)); !strings.Contains(b, `value=\"3\"`) || !strings.Contains(b, `debounce`) {
		t.Fatalf("join: %s", b)
	}
	ref := 1
	input := func(h, v string) live.ServerMessage {
		ref++
		return s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: ref, Event: ui.Event{Target: "c1:" + h, Type: "input", Value: v}})
	}
	if m := input("0", "go"); m.Type != live.PatchMsg || !strings.Contains(string(treeJSON(m.Trees)), `"go"`) || page.Q != "go" {
		t.Fatalf("Q: %+v", m)
	}
	if m := input("1", "7"); m.Type != live.PatchMsg || !strings.Contains(string(treeJSON(m.Trees)), `"7"`) || page.Limit != 7 {
		t.Fatalf("Limit: %+v", m)
	}
	if m := input("1", "x"); m.Error == "" {
		t.Fatalf("Limit x: %+v", m)
	}
	if _, err := ui.String(context.Background(), ui.HTML(`<input {a...}/>`, ui.BindValue(page, &page.other))); err == nil {
		t.Error("an unbound field took bind:value")
	}
}
