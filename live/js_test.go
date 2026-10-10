package live_test

import (
	"context"
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/live/livetest"
)

// Menu opens with commands alone, tells the server with a Push, and is
// closed by the server pushing commands.
type Menu struct {
	ui.Live
	opens  int
	closes int
	picked string
}

func (m *Menu) Mount()        { m.PushJS(ui.JS.AddClass(ui.This, "mounted")) }
func (m *Menu) Opened()       { m.opens++ }
func (m *Menu) Pick(v string) { m.picked = v }
func (m *Menu) Close() {
	m.closes++
	m.PushJS(ui.JS.On(ui.Ref("list")).AddClass("hidden").Push(m.Closed))
}
func (m *Menu) Closed() { m.closes += 10 }
func (m *Menu) Render() ui.Node {
	on := func(ch ui.JSChain) ui.Attr { return a("onClick", ui.On(ch)) }
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{a("id", "open"), on(ui.JS.On(ui.Ref("list")).RemoveClass("hidden").SetAttr("aria-expanded", "true").Push(m.Opened))}),
		ui.El("ul", []ui.Attr{a("ref", "list"), a("class", "hidden")},
			ui.El("li", []ui.Attr{a("id", "a"), on(ui.JS.Push(m.Pick, ui.Value("apples")))}),
			ui.El("li", []ui.Attr{a("id", "b"), a("onClick", ui.On(m.Close))})),
		ui.El("p", nil, ui.Child(m.opens), ui.Text("/"), ui.Child(m.closes), ui.Text("/"), ui.Text(m.picked)))
}

func menus(context.Context) ui.Node {
	return ui.El("main", nil, ui.Component("m#1", nil, &Menu{}), ui.Component("m#2", nil, &Menu{}))
}

func jsOf(t *testing.T, page, event string, n int) string {
	t.Helper()
	parts := strings.Split(page, `data-vk-js-`+event+`="`)
	if n+1 >= len(parts) {
		t.Fatalf("no %s commands %d in %s", event, n, page)
	}
	return html.UnescapeString(parts[n+1][:strings.IndexByte(parts[n+1], '"')])
}

func TestJSPushIDs(t *testing.T) {
	s := live.NewSession(context.Background(), menus)
	page, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := jsOf(t, page, "click", 2), `[["remove_class",{"to":{"ref":"list"},"names":["hidden"]}],["set_attr",{"to":{"ref":"list"},"name":"aria-expanded","value":"true"}],["push",{"h":"c2:0"}]]`; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if got, want := jsOf(t, page, "click", 3), `[["push",{"h":"c2:1","value":"apples"}]]`; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if !strings.Contains(page, `data-vk-on-click="c2:2"`) || !strings.Contains(page, `<ul data-vk-ref="list" class="hidden">`) {
		t.Fatalf("handler ids count pushes and data-vk-on together; ref renders: %s", page)
	}
	js := s.JS()
	if len(js) != 2 || js[0].ID != "c1" || string(js[0].Ops) != `[["add_class",{"names":["mounted"]}]]` || s.JS() != nil {
		t.Fatalf("Mount's pushes go with the join, once: %+v", js)
	}
	for range 2 { // the ids stay across renders
		p, err := s.Event(ui.Event{Target: "c2:0", Type: "click"})
		if err != nil || len(p.Updates) != 1 {
			t.Fatalf("%+v %v", p, err)
		}
		if !strings.Contains(s.HTML(), `data-vk-js-click="[[&#34;remove_class&#34;,{&#34;to&#34;:{&#34;ref&#34;:&#34;list&#34;},&#34;names&#34;:[&#34;hidden&#34;]}],[&#34;set_attr&#34;,{&#34;to&#34;:{&#34;ref&#34;:&#34;list&#34;},&#34;name&#34;:&#34;aria-expanded&#34;,&#34;value&#34;:&#34;true&#34;}],[&#34;push&#34;,{&#34;h&#34;:&#34;c2:0&#34;}]]"`) {
			t.Fatalf("ids moved: %s", s.HTML())
		}
	}
	if _, err := s.Event(ui.Event{Target: "c2:1", Type: "click", Value: "apples"}); err != nil || !strings.Contains(s.HTML(), "<p>2/0/apples</p>") {
		t.Fatalf("pushes call their handlers: %v %s", err, s.HTML())
	}
}

func TestPushJS(t *testing.T) {
	for _, v := range []int{1, 2} {
		s := live.NewSession(context.Background(), menus)
		join := live.Marshal(s.Handle(live.ClientMessage{Type: live.Join, V: v}))
		if !strings.Contains(string(join), `"js":[{"id":"c1","ops":[["add_class",{"names":["mounted"]}]]},{"id":"c2","ops":[["add_class",{"names":["mounted"]}]]}]`) {
			t.Fatalf("v%d join: %s", v, join)
		}
		reply := s.Handle(live.ClientMessage{Type: live.EventMsg, Event: ui.Event{Target: "c1:2", Type: "click"}})
		if len(reply.JS) != 1 || reply.JS[0].ID != "c1" ||
			string(reply.JS[0].Ops) != `[["add_class",{"to":{"ref":"list"},"names":["hidden"]}],["push",{"h":"c1:3"}]]` {
			t.Fatalf("v%d patch: %s", v, live.Marshal(reply))
		}
		if v == 2 && len(reply.Trees) != 1 || v == 1 && len(reply.Updates) != 1 {
			t.Fatalf("v%d patch: %s", v, live.Marshal(reply))
		}
		// The pushed handler is registered after the render's own.
		if p, err := s.Event(ui.Event{Target: "c1:3", Type: "click"}); err != nil || p.JS != nil {
			t.Fatalf("v%d: %+v %v", v, p, err)
		}
		if !strings.Contains(s.HTML(), "<p>0/11/</p>") {
			t.Fatalf("v%d: %s", v, s.HTML())
		}
	}
}

// TestLivetestCommands drives the menu through the reference client, which
// runs the commands on its model of the page.
func TestLivetestCommands(t *testing.T) {
	for _, v := range []int{1, 2} {
		c, err := livetest.Join(live.NewSession(context.Background(), menus), v)
		if err != nil {
			t.Fatal(err)
		}
		first := `[data-vk-id="c1"]`
		if !c.HasClass(first, "mounted") || !c.HasClass(first+" ul", "hidden") {
			t.Fatalf("v%d join: %s", v, c.HTML())
		}
		if err := c.Click(first + " #open"); err != nil {
			t.Fatal(err)
		}
		if c.HasClass(first+" ul", "hidden") || !c.HasClass(`[data-vk-id="c2"] ul`, "hidden") {
			t.Fatalf("v%d: a ref is its instance's", v)
		}
		if e, _ := c.Attr(first+" ul", "aria-expanded"); e != "true" || c.Text(first+" p") != "1/0/" {
			t.Fatalf("v%d: the push ran, and the commands' changes survived its patch: %q %q", v, e, c.Text(first+" p"))
		}
		if err := c.Click(first + " #a"); err != nil || c.Text(first+" p") != "1/0/apples" || c.HasClass(first+" ul", "hidden") {
			t.Fatalf("v%d: %v %s", v, err, c.Text(first+" p"))
		}
		if err := c.Click(first + " #b"); err != nil || !c.HasClass(first+" ul", "hidden") || c.Text(first+" p") != "1/11/apples" {
			t.Fatalf("v%d: the server's commands ran, and their push: %v %s", v, err, c.HTML())
		}
	}
}

// Toggle's class is the server's: the browser's change holds until the
// server renders the attribute differently.
type Toggle struct {
	ui.Live
	on bool
	n  int
}

func (g *Toggle) Flip() { g.on = !g.on }
func (g *Toggle) Inc()  { g.n++ }
func (g *Toggle) Render() ui.Node {
	cls := "off"
	if g.on {
		cls = "on"
	}
	return ui.El("div", nil,
		ui.El("span", []ui.Attr{a("class", cls), a("style", "color: red")}, ui.Child(g.n)),
		ui.El("button", []ui.Attr{a("id", "js"), a("onClick", ui.On(ui.JS.On("span").AddClass("lit").Hide()))}),
		ui.El("button", []ui.Attr{a("id", "flip"), a("onClick", ui.On(g.Flip))}),
		ui.El("button", []ui.Attr{a("id", "inc"), a("onClick", ui.On(g.Inc))}),
		ui.El("input", []ui.Attr{a("type", "checkbox"), a("id", "box")}),
		ui.El("button", []ui.Attr{a("id", "cond"), a("onKeyDown", ui.On(ui.JS.Key("Enter").If(ui.Sel("#box").Checked()).SetAttr("#box", "data-x", "yes").Else().SetValue("#box", "true").End()))}),
	)
}

func TestLivetestReconcile(t *testing.T) {
	c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("t#1", nil, &Toggle{})
	}), 2)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Visible("span") {
		t.Fatal("hidden at first")
	}
	c.Click("#js")
	if !c.HasClass("span", "lit") || c.Visible("span") {
		t.Fatalf("commands: %s", c.HTML())
	}
	c.Click("#inc")
	if !c.HasClass("span", "lit") || c.Visible("span") || c.Text("span") != "1" {
		t.Fatal("a patch leaving the class and style as they were keeps the commands' changes")
	}
	c.Click("#flip")
	if c.HasClass("span", "lit") || !c.HasClass("span", "on") || c.Visible("span") {
		t.Fatal("the server's class wins once it changes; the style is still the browser's")
	}
	c.Fire("#cond", ui.Event{Type: "keydown", Key: "a"})
	if _, ok := c.Attr("#box", "checked"); ok {
		t.Fatal("a key filter stops the chain")
	}
	c.Fire("#cond", ui.Event{Type: "keydown", Key: "Enter"})
	if _, ok := c.Attr("#box", "checked"); !ok {
		t.Fatal("else branch: set_value checks the box")
	}
	c.Fire("#cond", ui.Event{Type: "keydown", Key: "Enter"})
	if x, _ := c.Attr("#box", "data-x"); x != "yes" {
		t.Fatal("if branch")
	}
}

func TestJSStaticPush(t *testing.T) {
	out, err := ui.String(context.Background(), menus(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	var ops [][]json.RawMessage
	if err := json.Unmarshal([]byte(jsOf(t, out, "click", 1)), &ops); err != nil || string(ops[0][1]) != `{}` {
		t.Fatalf("a static push has no handler id: %s", out)
	}
	if strings.Contains(out, "data-vk-id") || !strings.Contains(out, `data-vk-ref="list"`) {
		t.Fatal(out)
	}
}

// Panel exercises the rest of the commands the reference client runs.
type Panel struct {
	ui.Live
	got []string
}

func (p *Panel) Got(v string) { p.got = append(p.got, v) }
func (p *Panel) Render() ui.Node {
	on := func(event string, ch ui.JSChain) ui.Attr { return a(event, ui.On(ch)) }
	return ui.El("section", []ui.Attr{on("onPicked", ui.JS.Push(p.Got, ui.Value("picked")))},
		ui.El("div", []ui.Attr{a("class", "card"), a("data-close", `[["add_class",{"names":["closed"]}]]`)},
			ui.El("button", []ui.Attr{a("id", "tog"), on("onClick", ui.JS.ToggleAttr(ui.Closest(".card"), "aria-expanded", "true", "false").AddClass(ui.Inner("i"), "x"))}, ui.El("i", nil)),
			ui.El("button", []ui.Attr{a("id", "exec"), on("onClick", ui.JS.Exec(ui.Closest(".card"), "data-close"))}),
			ui.El("button", []ui.Attr{a("id", "fire"), on("onClick", ui.JS.Dispatch(ui.This, "picked"))}),
			ui.El("button", []ui.Attr{a("id", "ask"), on("onClick", ui.JS.Confirm("sure?").CopyText("yes"))}),
			ui.El("input", []ui.Attr{a("ref", "q"), a("value", "hello"), a("onInput", ui.On(p.Got))}),
			ui.El("button", []ui.Attr{a("id", "set"), on("onClick", ui.JS.SetValue(ui.Ref("q"), "typed").Copy(ui.Ref("q")))}),
			ui.El("button", []ui.Attr{a("id", "if"), on("onClick", ui.JS.If(ui.Ref("q").Value().Eq("typed")).If(ui.Ref("q").Attr("value")).Hide(ui.Sel("i")).End().ElseIf(ui.Ref("q").Exists()).Push(p.Got, ui.Value(1)).End())}),
		),
		ui.El("p", nil, ui.Text(strings.Join(p.got, ","))))
}

func TestLivetestOps(t *testing.T) {
	c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("p#1", nil, &Panel{}) }), 2)
	if err != nil {
		t.Fatal(err)
	}
	step := func(sel string, check func() bool) {
		t.Helper()
		if err := c.Click(sel); err != nil || !check() {
			t.Fatalf("%s: %v\n%s", sel, err, c.HTML())
		}
	}
	step("#tog", func() bool { v, _ := c.Attr(".card", "aria-expanded"); return v == "true" && c.HasClass("i", "x") })
	step("#tog", func() bool { v, _ := c.Attr(".card", "aria-expanded"); return v == "false" })
	step("#exec", func() bool { return c.HasClass(".card", "closed") })
	step("#fire", func() bool { return c.Text("p") == "picked" })
	c.Confirm = func(string) bool { return false }
	step("#ask", func() bool { return c.Clipboard == "" })
	c.Confirm = nil
	step("#ask", func() bool { return c.Clipboard == "yes" })
	step("#if", func() bool { return c.Visible("i") && c.Text("p") == "picked,1" })
	step("#set", func() bool { return c.Clipboard == "typed" && c.Text("p") == "picked,1,typed" })
	step("#if", func() bool { return !c.Visible("i") && c.Text("p") == "picked,1,typed" })
}
