package live_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
)

func a(name string, v any) ui.Attr { return ui.Attr{Name: name, Value: v} }

type Counter struct {
	ui.Live
	Start  int
	Label  string
	n      int
	mounts int
	gone   *[]string
}

func (c *Counter) Mount(ctx context.Context) error { c.n = c.Start; c.mounts++; return nil }
func (c *Counter) Inc()                            { c.n++ }
func (c *Counter) Unmount() {
	if c.gone != nil {
		*c.gone = append(*c.gone, c.Label)
	}
}
func (c *Counter) Render() ui.Node {
	return ui.El("div", nil, ui.El("span", nil, ui.Text(c.Label), ui.Child(c.n)),
		ui.El("button", []ui.Attr{a("onClick", ui.On(c.Inc))}, ui.Text("+")))
}

// client is a reference client: it keeps the page's HTML and applies patches
// the way a browser runtime morphs them.
type client struct {
	t    *testing.T
	s    *live.Session
	html string
	ref  int
}

func join(t *testing.T, render func(context.Context) ui.Node) *client {
	c := &client{t: t, s: live.NewSession(context.Background(), render)}
	m := c.send(live.ClientMessage{Type: live.Join})
	if m.Type != live.Render {
		t.Fatalf("join: %+v", m)
	}
	c.html = m.HTML
	return c
}

func (c *client) send(m live.ClientMessage) live.ServerMessage {
	c.t.Helper()
	c.ref++
	m.Ref = c.ref
	data, _ := json.Marshal(m)
	var reply live.ServerMessage
	if err := json.Unmarshal(c.s.HandleJSON(data), &reply); err != nil {
		c.t.Fatal(err)
	}
	if reply.Ref != c.ref {
		c.t.Fatalf("reply to %d, want %d", reply.Ref, c.ref)
	}
	return reply
}

// handler finds the n-th handler of event in the page.
func (c *client) handler(event string, n int) string {
	c.t.Helper()
	ms := regexp.MustCompile(`data-vk-on-`+event+`="([^"]+)"`).FindAllStringSubmatch(c.html, -1)
	if n >= len(ms) {
		c.t.Fatalf("no %s handler %d in %s", event, n, c.html)
	}
	return ms[n][1]
}

func (c *client) event(ev ui.Event) live.ServerMessage {
	c.t.Helper()
	m := c.send(live.ClientMessage{Type: live.EventMsg, Event: ev})
	if m.Type == live.Error {
		c.t.Fatalf("event: %s", m.Error)
	}
	c.apply(m.Updates)
	return m
}

func (c *client) apply(us []live.Update) {
	for _, u := range us {
		if u.ID == live.RootID {
			c.html = u.HTML
			continue
		}
		start := strings.Index(c.html, `data-vk-id="`+u.ID+`"`)
		if start < 0 {
			c.t.Fatalf("no element %s in %s", u.ID, c.html)
		}
		start = strings.LastIndexByte(c.html[:start], '<')
		tag := c.html[start+1 : start+1+strings.IndexAny(c.html[start+1:], " >")]
		end := matchingEnd(c.html, start, tag)
		c.html = c.html[:start] + u.HTML + c.html[end:]
	}
}

func matchingEnd(s string, start int, tag string) int {
	depth, i := 0, start
	for {
		o := strings.Index(s[i:], "<"+tag)
		cl := strings.Index(s[i:], "</"+tag+">")
		if o >= 0 && o < cl {
			depth++
			i += o + 1
			continue
		}
		depth--
		i += cl + len(tag) + 3
		if depth == 0 {
			return i
		}
	}
}

func counters(cs ...*Counter) func(context.Context) ui.Node {
	return func(context.Context) ui.Node {
		var kids []ui.Node
		for _, c := range cs {
			kids = append(kids, ui.Component("t:1", nil, c))
		}
		return ui.El("main", nil, kids...)
	}
}

func TestClick(t *testing.T) {
	c := join(t, counters(&Counter{Label: "a", Start: 5}, &Counter{Label: "b"}))
	want := `<main><div data-vk-id="c1"><span>a5</span><button data-vk-on-click="c1:0">+</button></div>` +
		`<div data-vk-id="c2"><span>b0</span><button data-vk-on-click="c2:0">+</button></div></main>`
	if c.html != want {
		t.Fatalf("render:\n%s\nwant\n%s", c.html, want)
	}
	m := c.event(ui.Event{Target: c.handler("click", 1), Type: "click"})
	if len(m.Updates) != 1 || m.Updates[0].ID != "c2" {
		t.Fatalf("patch: %+v", m.Updates)
	}
	c.event(ui.Event{Target: c.handler("click", 1), Type: "click"})
	c.event(ui.Event{Target: c.handler("click", 0), Type: "click"})
	if !strings.Contains(c.html, "<span>a6</span>") || !strings.Contains(c.html, "<span>b2</span>") {
		t.Fatalf("after clicks: %s", c.html)
	}
	if m := c.send(live.ClientMessage{Type: live.EventMsg, Event: ui.Event{Target: "c9:0"}}); m.Type != live.Error {
		t.Fatalf("unknown handler: %+v", m)
	}
	if h, ok := c.s.Handler(c.handler("click", 1)).(func()); !ok || h == nil {
		t.Errorf("Handler(%s) = %T", c.handler("click", 1), c.s.Handler(c.handler("click", 1)))
	}
	if comp, ok := c.s.Component(c.handler("click", 1)).(*Counter); !ok || comp.Label != "b" {
		t.Errorf("Component(%s) = %#v", c.handler("click", 1), c.s.Component(c.handler("click", 1)))
	}
	for _, bad := range []string{"c9:0", "c1:7", "c1", "x"} {
		if bad != "c1:7" && bad != "c1" && c.s.Component(bad) != nil {
			t.Errorf("Component(%q) = %T", bad, c.s.Component(bad))
		}
		if h := c.s.Handler(bad); h != nil {
			t.Errorf("Handler(%q) = %T", bad, h)
		}
	}
}

func TestStatic(t *testing.T) {
	got, err := ui.String(context.Background(), counters(&Counter{Label: "a", Start: 2})(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	if want := `<main><div><span>a2</span><button>+</button></div></main>`; got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

type Search struct {
	ui.Live
	q, key string
	on     bool
	n      int
	err    error
}

type Signup struct {
	Name  string `form:"user_name"`
	Age   int
	Tags  []string `json:"tags"`
	Agree bool
}

func (s *Search) Render() ui.Node {
	return ui.El("form", []ui.Attr{a("onSubmit", ui.On(func(ctx context.Context, f Signup) error {
		if f.Name == "" {
			return errors.New("name is required")
		}
		s.q = f.Name + "/" + strings.Join(f.Tags, ",")
		s.n, s.on = f.Age, f.Agree
		return nil
	}))},
		ui.El("input", []ui.Attr{a("onInput", ui.On(func(v string) { s.q = v }))}),
		ui.El("input", []ui.Attr{a("onKeyDown", ui.On(func(k string) { s.key = k }))}),
		ui.El("input", []ui.Attr{a("onChange", ui.On(func(b bool) { s.on = b }))}),
		ui.El("input", []ui.Attr{a("onChange", ui.On(func(n int) { s.n = n }))}),
		ui.El("input", []ui.Attr{a("onChange", ui.On(func(f url.Values) { s.q = f.Get("raw") }))}),
		ui.El("p", nil, ui.Text(s.q), ui.Text("|"), ui.Text(s.key), ui.Text("|"), ui.Child(s.on), ui.Text("|"), ui.Child(s.n)))
}

func TestPayloads(t *testing.T) {
	c := join(t, func(context.Context) ui.Node { return ui.Component("t:1", nil, &Search{}) })
	p := func() string { return c.html[strings.Index(c.html, "<p>"):] }
	c.event(ui.Event{Target: c.handler("input", 0), Type: "input", Value: "go"})
	c.event(ui.Event{Target: c.handler("keydown", 0), Type: "keydown", Value: "go", Key: "Enter"})
	c.event(ui.Event{Target: c.handler("change", 0), Type: "change", Value: "true"})
	c.event(ui.Event{Target: c.handler("change", 1), Type: "change", Value: "42"})
	if got := p(); got != "<p>go|Enter|true|42</p></form></vk-c>" && !strings.HasPrefix(got, "<p>go|Enter|true|42</p>") {
		t.Fatalf("got %s", got)
	}
	m := c.event(ui.Event{Target: c.handler("change", 1), Type: "change", Value: "x"})
	if !strings.Contains(m.Error, "isn't a whole number") || len(m.Updates) != 0 {
		t.Fatalf("bad number: %+v", m)
	}
	m = c.event(ui.Event{Target: c.handler("submit", 0), Type: "submit", Form: url.Values{}})
	if m.Error != "name is required" {
		t.Fatalf("submit error: %+v", m)
	}
	c.event(ui.Event{Target: c.handler("submit", 0), Type: "submit",
		Form: url.Values{"user_name": {"ann"}, "AGE": {"7"}, "tags": {"x", "y"}, "agree": {"on"}}})
	if got := p(); !strings.HasPrefix(got, "<p>ann/x,y||true|7</p>") && !strings.HasPrefix(got, "<p>ann/x,y|Enter|true|7</p>") {
		t.Fatalf("got %s", got)
	}
	c.event(ui.Event{Target: c.handler("change", 2), Type: "change", Form: url.Values{"raw": {"r"}}})
	if got := p(); !strings.HasPrefix(got, "<p>r|") {
		t.Fatalf("got %s", got)
	}
}

type Reply struct {
	Text string `form:"text" json:"body" validate:"required,max=5"`
}

type Answers struct {
	ui.Live
	got []string
}

func (r *Answers) Render() ui.Node {
	return ui.El("form", []ui.Attr{a("onSubmit", ui.On(func(f Reply) { r.got = append(r.got, f.Text) }))},
		ui.El("button", []ui.Attr{a("onClick", ui.On(func(f *Reply) { r.got = append(r.got, f.Text) }))}),
		ui.El("p", nil, ui.Text(strings.Join(r.got, ","))))
}

func TestPayloadValidated(t *testing.T) {
	c := join(t, func(context.Context) ui.Node { return ui.Component("t:1", nil, &Answers{}) })
	submit := func(text string) live.ServerMessage {
		return c.event(ui.Event{Target: c.handler("submit", 0), Type: "submit", Form: url.Values{"text": {text}}})
	}
	for text, want := range map[string]string{"": "invalid: text: required", "shouting": "invalid: text: at most 5 characters"} {
		if m := submit(text); m.Error != want || len(m.Updates) != 0 {
			t.Errorf("submit %q: %+v, want the error %q", text, m, want)
		}
	}
	if m := c.event(ui.Event{Target: c.handler("click", 0), Type: "click", Data: []byte(`{"body":""}`)}); m.Error != "invalid: body: required" {
		t.Errorf("emit: %+v", m)
	}
	submit("hi")
	if !strings.Contains(c.html, "<p>hi</p>") {
		t.Fatalf("want only the valid reply: %s", c.html)
	}
}

type List struct {
	ui.Live
	items []string
	gone  *[]string
}

func (l *List) Remove(s string) {
	for i, x := range l.items {
		if x == s {
			l.items = append(l.items[:i:i], l.items[i+1:]...)
		}
	}
}

func (l *List) Reverse() {
	for i, j := 0, len(l.items)-1; i < j; i, j = i+1, j-1 {
		l.items[i], l.items[j] = l.items[j], l.items[i]
	}
}

func (l *List) Render() ui.Node {
	return ui.El("ul", nil, ui.Nodes(func(add func(ui.Node)) {
		for _, it := range l.items {
			add(ui.El("li", []ui.Attr{a("key", it)},
				ui.Component("t:2", nil, &Counter{Label: it, gone: l.gone}),
				ui.El("button", []ui.Attr{a("onClick", ui.On(func() { l.Remove(it) }))}, ui.Text("x"))))
		}
	}), ui.El("button", []ui.Attr{a("onClick", ui.On(l.Reverse))}))
}

func TestKeyedList(t *testing.T) {
	var gone []string
	l := &List{items: []string{"a", "b", "c"}, gone: &gone}
	c := join(t, func(context.Context) ui.Node { return ui.Component("t:1", "list", l) })
	idOf := func(label string) string {
		m := regexp.MustCompile(`data-vk-id="(c\d+)"><span>` + label).FindStringSubmatch(c.html)
		if m == nil {
			t.Fatalf("no %s in %s", label, c.html)
		}
		return m[1]
	}
	a, b := idOf("a"), idOf("b")
	// increment b, then reverse: b keeps its instance and its count
	c.event(ui.Event{Target: c.handler("click", 2), Type: "click"})
	if !strings.Contains(c.html, "<span>b1</span>") {
		t.Fatalf("b not incremented: %s", c.html)
	}
	last := strings.Count(c.html, "data-vk-on-click") - 1
	c.event(ui.Event{Target: c.handler("click", last), Type: "click"})
	if idOf("a") != a || idOf("b") != b || !strings.Contains(c.html, "<span>b1</span>") || strings.Index(c.html, ">c0<") > strings.Index(c.html, ">a0<") {
		t.Fatalf("after reverse: %s", c.html)
	}
	// remove b (now the middle item): its instance is dropped and unmounted
	c.event(ui.Event{Target: c.handler("click", 3), Type: "click"})
	if strings.Contains(c.html, "<span>b") {
		t.Fatalf("b still there: %s", c.html)
	}
	if len(gone) != 1 || gone[0] != "b" {
		t.Fatalf("unmounted %v", gone)
	}
	if got := c.html; strings.Contains(got, `"`+b+`"`) {
		t.Fatalf("b's id still rendered: %s", got)
	}
}

type Chat struct {
	ui.Live
	Room string
	log  []string
}

type NewMessage struct{ Text string }
type Typing struct{ Who string }

func (c *Chat) Mount() { c.Subscribe("room:" + c.Room) }

// Overloads of Update, as vuka names them: after the message's type.
func (c *Chat) UpdateNewMessage(m NewMessage) { c.log = append(c.log, "msg "+m.Text) }
func (c *Chat) UpdateTyping(ctx context.Context, m Typing) error {
	c.log = append(c.log, "typing "+m.Who)
	return nil
}
func (c *Chat) UpdateAny(m any) { c.log = append(c.log, "other") }

// UpdateTopic takes a string: an event, not an Update.
func (c *Chat) UpdateTopic(topic string) { c.Room = topic }

// OldChat's overloads are named as older vuka commands wrote them.
type OldChat struct {
	ui.Live
	log []string
}

func (c *OldChat) Update__NewMessage(m NewMessage) { c.log = append(c.log, "old "+m.Text) }
func (c *OldChat) Render() ui.Node                 { return ui.El("p", nil, ui.Text(strings.Join(c.log, ","))) }

func (c *Chat) Render() ui.Node {
	return ui.El("ul", nil, ui.Nodes(func(add func(ui.Node)) {
		for _, l := range c.log {
			add(ui.El("li", nil, ui.Text(l)))
		}
	}))
}

func TestInfo(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Fragment(ui.Component("t:1", nil, &Chat{Room: "go"}), ui.Component("t:1", nil, &Chat{Room: "rust"}))
	})
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.Topics(), ","); got != "room:go,room:rust" {
		t.Fatalf("topics %s", got)
	}
	p, err := s.Info("room:go", NewMessage{"hi"})
	if err != nil || len(p.Updates) != 1 || p.Updates[0].ID != "c1" || !strings.Contains(p.Updates[0].HTML, "<li>msg hi</li>") {
		t.Fatalf("info: %+v %v", p, err)
	}
	p, _ = s.Info("room:go", Typing{"ann"})
	if !strings.Contains(p.Updates[0].HTML, "<li>typing ann</li>") {
		t.Fatalf("typing: %+v", p)
	}
	p, _ = s.Info("", 42)
	if len(p.Updates) != 2 || !strings.Contains(p.Updates[1].HTML, "<li>other</li>") {
		t.Fatalf("broadcast: %+v", p)
	}
	if p, _ := s.Info("room:none", NewMessage{"x"}); len(p.Updates) != 0 {
		t.Fatalf("nobody subscribed: %+v", p)
	}
	if p, _ := s.Info("room:go", "rust"); len(p.Updates) != 1 || !strings.Contains(p.Updates[0].HTML, "<li>other</li>") {
		t.Fatalf("a string reached UpdateTopic: %+v", p)
	}

	old := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("t:1", nil, &OldChat{})
	})
	if _, err := old.Render(); err != nil {
		t.Fatal(err)
	}
	if p, _ := old.Info("", NewMessage{"hi"}); len(p.Updates) != 1 || !strings.Contains(p.Updates[0].HTML, "old hi") {
		t.Fatalf("Update__NewMessage: %+v", p)
	}
}

func TestIsUpdate(t *testing.T) {
	ct := reflect.TypeFor[*Chat]()
	for name, want := range map[string]bool{"UpdateNewMessage": true, "UpdateTyping": true, "UpdateAny": true, "UpdateTopic": false, "Render": false} {
		m, _ := ct.MethodByName(name)
		if got := ui.IsUpdate(m); got != want {
			t.Errorf("IsUpdate(%s) = %v", name, got)
		}
	}
	for _, c := range []struct {
		v    any
		want string
	}{
		{0, "Int"}, {(*NewMessage)(nil), "NewMessage"}, {[]Typing{}, "Typings"}, {map[string]int{}, "MapStringInt"},
		{[]*Chat{}, "Chats"}, {struct{}{}, "Struct"}, {(*error)(nil), "Error"}, {ui.State[[]Typing]{}, "StateTypings"},
	} {
		if got := ui.TypeName(reflect.TypeOf(c.v)); got != c.want {
			t.Errorf("TypeName(%T) = %s, want %s", c.v, got, c.want)
		}
	}
	if got := ui.TypeName(reflect.TypeFor[any]()); got != "Any" {
		t.Errorf("TypeName(any) = %s", got)
	}
}

func TestMountOnceAndProps(t *testing.T) {
	label := "a"
	var gone []string
	c := &Counter{Start: 3}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		if label == "" {
			return ui.El("main", nil)
		}
		cp := *c
		cp.Label, cp.gone = label, &gone
		return ui.El("main", nil, ui.Component("t:1", nil, &cp))
	})
	html, _ := s.Render()
	target := regexp.MustCompile(`data-vk-on-click="([^"]+)"`).FindStringSubmatch(html)[1]
	label = "b"
	p, err := s.Event(ui.Event{Target: target})
	if err != nil || len(p.Updates) != 1 || p.Updates[0].ID != "c1" || !strings.Contains(p.Updates[0].HTML, "<span>b4</span>") {
		t.Fatalf("props set again, state kept: %+v %v", p, err)
	}
	if c.mounts != 0 {
		t.Fatal("the tag's props were mounted")
	}
	label = ""
	p, _ = s.Event(ui.Event{Target: target})
	if len(p.Updates) != 1 || p.Updates[0].ID != live.RootID || len(gone) != 1 {
		t.Fatalf("removed: %+v %v", p, gone)
	}
}

func TestKeysRendered(t *testing.T) {
	c := join(t, func(context.Context) ui.Node {
		return ui.El("ul", nil,
			ui.El("li", []ui.Attr{a("key", 7)}, ui.Text("x")),
			ui.El("li", []ui.Attr{a("key", "q\"")}, ui.Text("y")),
			ui.Component("t:1", "row-1", &Counter{Label: "a"}),
			ui.El("li", []ui.Attr{a("key", nil)}))
	})
	for _, want := range []string{`<li data-vk-key="7">x</li>`, `<li data-vk-key="q&#34;">y</li>`, `data-vk-key="row-1"`, `<li></li>`} {
		if !strings.Contains(c.html, want) {
			t.Fatalf("no %s in %s", want, c.html)
		}
	}
	if !regexp.MustCompile(`data-vk-id="c\d+" data-vk-key="row-1"`).MatchString(c.html) {
		t.Fatalf("component root lacks its key: %s", c.html)
	}
}
