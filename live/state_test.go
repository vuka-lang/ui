package live_test

import (
	"context"
	"encoding/json"
	"html"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/uigen"
)

// menu declares its state in Render, as ui.State(false) in a .vuka file
// does, and hands it to a child.
type menu struct {
	ui.Live
	Label string
	n     int
}

func (m *menu) Bump() { m.n++ }

func (m *menu) Render() ui.Node {
	open := uigen.State("menu.open", false)
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{a("onClick", ui.On(uigen.Toggle(ui.JS, open))), a("aria-expanded", ui.BindExpr(open.Get(), `{"$":0}`, open))}, ui.Text(m.Label)),
		ui.El("button", []ui.Attr{a("onClick", ui.On(m.Bump))}, ui.Child(m.n)),
		ui.Component("menu.list", nil, &menuList{Open: open}))
}

// menuList reads the state its parent passes: the same state.
type menuList struct {
	ui.Live
	Open ui.State[bool]
}

func (l *menuList) Render() ui.Node {
	return ui.El("ul", []ui.Attr{a("hidden", ui.BindExpr(!l.Open.Get(), `["!",{"$":0}]`, l.Open))})
}

var stateKeys = regexp.MustCompile(`c\d+:menu\.open:\d+`)

func keysIn(s string) []string {
	ks := stateKeys.FindAllString(html.UnescapeString(s), -1)
	slices.Sort(ks)
	return slices.Compact(ks)
}

func TestStatePerInstance(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Fragment(ui.Component("page.a", nil, &menu{Label: "A"}), ui.Component("page.b", nil, &menu{Label: "B"}))
	})
	out, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keysIn(out), []string{"c1:menu.open:1", "c3:menu.open:1"}; !slices.Equal(got, want) {
		t.Fatalf("keys %v, want %v (each instance's own, the child's its parent's):\n%s", got, want, out)
	}
	// The child's binding reads its parent's state.
	if !strings.Contains(html.UnescapeString(out), `<ul hidden data-vk-b-hidden="["!",{"i":false,"s":"c1:menu.open:1"}]" data-vk-id="c2">`) {
		t.Errorf("the child doesn't read its parent's state:\n%s", html.UnescapeString(out))
	}
	// A render after an event keeps the keys: the states are the same.
	if _, err := s.Event(ui.Event{Target: "c1:0"}); err != nil {
		t.Fatal(err)
	}
	if got := keysIn(s.HTML()); !slices.Equal(got, []string{"c1:menu.open:1", "c3:menu.open:1"}) {
		t.Errorf("keys after an event: %v", got)
	}
}

func TestStateTrees(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("page.a", nil, &menu{Label: "A"})
	})
	ups, err := s.RenderTrees()
	if err != nil {
		t.Fatal(err)
	}
	j, _ := json.Marshal(ups)
	if got := keysIn(string(j)); !slices.Equal(got, []string{"c1:menu.open:1"}) {
		t.Errorf("tree keys %v:\n%s", got, j)
	}
}

// pusher sets a state from the server, with PushJS.
type pusher struct {
	ui.Live
	tab ui.State[string]
}

func (p *pusher) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }
func (p *pusher) mount(context.Context) error { p.tab = uigen.State("pusher.tab", "logs"); return nil }
func (p *pusher) ShowEnv()                    { p.PushJS(p.tab.Set("env")) }
func (p *pusher) Render() ui.Node {
	return ui.El("p", []ui.Attr{a("onClick", ui.On(p.ShowEnv)), a("title", ui.BindExpr(p.tab.Get(), `{"$":0}`, p.tab))})
}

func TestStatePushed(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("page", nil, &pusher{}) })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	p, err := s.Event(ui.Event{Target: "c1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.JS) != 1 || string(p.JS[0].Ops) != `[["state_set",{"s":"c1:pusher.tab:1","i":"logs","v":"env"}]]` {
		t.Errorf("pushed %+v", p.JS)
	}
}
