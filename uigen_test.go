package ui_test

import (
	"context"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/uigen"
)

// TestStateAPI: a State's API is Get and Set; what the compiler lowers to
// is package uigen's.
func TestStateAPI(t *testing.T) {
	var names []string
	m := reflect.TypeFor[ui.State[bool]]()
	for i := 0; i < m.NumMethod(); i++ {
		names = append(names, m.Method(i).Name)
	}
	if !slices.Equal(names, []string{"Get", "Set"}) {
		t.Errorf("ui.State's methods: %v, want [Get Set]", names)
	}
}

// TestChainOrder: writes chain in the order an on… attribute writes them,
// whatever their kind.
func TestChainOrder(t *testing.T) {
	open := uigen.State("o", false)
	tab := uigen.State("t", "a")
	n := uigen.State("n", 1)
	keys := regexp.MustCompile(`"s":"(\w+):\w+"`)
	got := keys.ReplaceAllString(encode(t, uigen.Inc(uigen.Toggle(ui.JS.Set(tab, "b"), open), n).Push(func() {})), `"s":"$1"`)
	want := `[["state_set",{"s":"t","i":"a","v":"b"}],["state_toggle",{"s":"o","i":false}],["state_inc",{"s":"n","i":1,"by":1}],["push",{}]]`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// fields keeps its states in fields, declared by their type alone.
type fields struct {
	ui.Live
	open   ui.State[bool]
	tab    ui.State[string]
	Shared ui.State[int] // a parent may pass one
	pushed string
}

func (f *fields) LiveSchema(l *ui.LiveSchema) { l.Mount(f.mount) }

func (f *fields) mount(ctx context.Context) error {
	f.PushJS(f.tab.Set("env")) // a write before the first render
	return nil
}

func (f *fields) Render() ui.Node {
	hidden := uigen.Not(f.open)
	return ui.HTML(`<p hidden={hidden} title={f.tab}></p>`, hidden, f.tab)
}

// TestFieldStates: a live component's zero ui.State fields get states of
// their own before it mounts, one per field; a state its parent set stays.
func TestFieldStates(t *testing.T) {
	shared := uigen.State("shared", 3)
	c := &fields{Shared: shared}
	if err := ui.MountLive(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Shared, shared) {
		t.Errorf("the parent's state was replaced")
	}
	out, _ := page(t, c.Render())
	if !strings.Contains(out, "hidden") || strings.Count(out, `&#34;s&#34;:&#34;c0:f`) != 2 {
		t.Errorf("the fields' states:\n%s", out)
	}
	again := &fields{}
	uigen.States(again)
	before := again.open
	uigen.States(again)
	if !reflect.DeepEqual(again.open, before) {
		t.Errorf("States replaced a state it had given")
	}
}

// static is a struct component: its View gives its fields states first.
type static struct {
	on ui.State[bool]
}

func (s static) View() ui.Node {
	uigen.States(&s)
	return ui.HTML(`<b data-on={s.on}></b>`, s.on)
}

func TestStaticStates(t *testing.T) {
	out, _ := page(t, static{}.View())
	if !strings.Contains(out, `&#34;s&#34;:&#34;c0:f`) {
		t.Errorf("a static component's state:\n%s", out)
	}
}

// TestAssign: uigen.Assign is tracked state: a set marks it changed.
func TestAssign(t *testing.T) {
	var a uigen.Assign[[]int]
	a.Set([]int{1})
	a.Update(func(v []int) []int { return append(v, 2) })
	if got := a.Get(); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("Get: %v", got)
	}
	if !*a.AssignDirty() {
		t.Error("a set didn't mark it changed")
	}
}

// tracked has tracked server state beside client state.
type tracked struct {
	ui.Live
	n    uigen.Assign[int]
	open ui.State[bool]
}

func (t *tracked) Render() ui.Node { return ui.Text("") }

// TestTrackedWithStates: a ui.State field leaves a component's server state
// tracked.
func TestTrackedWithStates(t *testing.T) {
	c := &tracked{}
	if tracked, changed := ui.TrackedState(c); !tracked || changed {
		t.Errorf("tracked %v, changed %v; want true, false", tracked, changed)
	}
	c.n.Set(1)
	if _, changed := ui.TrackedState(c); !changed {
		t.Error("a set isn't a change")
	}
}

// cart derives its total from its states, and a flag from the total, as
// the compiler writes @ui.From.
type cart struct {
	ui.Live
	count ui.State[int]
	price ui.State[int]
	total ui.Derived[int]
	over  ui.Derived[bool]
	n     uigen.Assign[int]
}

func init() {
	uigen.Derive(func(c *cart) {
		c.total = uigen.From(uigen.Mul(c.count, c.price))
		c.over = uigen.From(uigen.Gt(uigen.Read(c.total), 100))
	})
}

func (c *cart) Render() ui.Node {
	return ui.HTML(`<p data-over={c.over}>{c.total}</p>`, c.over, c.total)
}

// TestDerived: a Derived field is set once the component's states are,
// renders with their initial values, and is a state of its own in the
// browser, keyed, with the expression it is computed from; it leaves the
// component's server state tracked.
func TestDerived(t *testing.T) {
	c := &cart{}
	if err := ui.MountLive(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if c.total.Get() != 0 || c.over.Get() {
		t.Errorf("initial values: %v %v", c.total.Get(), c.over.Get())
	}
	before := c.total
	uigen.States(c)
	if !reflect.DeepEqual(c.total, before) {
		t.Error("States derived a Derived it had")
	}
	out, _ := page(t, c.Render())
	out = strings.ReplaceAll(out, "&#34;", `"`)
	for _, want := range []string{
		`"s":"c0:s`, // the total's own key
		`"x":["*",{"i":0,"s":"c0:f`,
		`"x":["&gt;",{"i":0,"s":"c0:s`, // over reads the total by its key
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
	if tracked, _ := ui.TrackedState(c); !tracked {
		t.Error("a Derived field leaves the component untracked")
	}
	defer func() {
		if recover() == nil {
			t.Error("setting a Derived didn't panic")
		}
	}()
	ui.JS.Set(c.total, 3)
}

func TestDerivedValues(t *testing.T) {
	n := uigen.State("n", 3)
	p := uigen.State("p", 2.5)
	if got := uigen.Mul(n, 4).Get(); got != 12 {
		t.Errorf("3 * 4 = %v", got)
	}
	if got := uigen.Mul(p, n).Get(); got != 7.5 {
		t.Errorf("2.5 * 3 = %v", got)
	}
	if got := uigen.From(uigen.Mul(n, n)).Get(); got != 9 {
		t.Errorf("a Derived of 3 * 3 = %v", got)
	}
}

// search notifies its handler of its query's browser writes.
type search struct {
	ui.Live
	q    ui.State[string]
	page ui.State[int]
	got  []string
}

func (s *search) Search(q string) { s.got = append(s.got, q) }
func (s *search) Page(n int)      {}

func init() {
	uigen.Derive(func(s *search) {
		uigen.With(s.q, ui.Notify(s.Search).Debounce(300*time.Millisecond))
		uigen.With(s.page, ui.Notify(s.Page))
	})
}

func (s *search) Render() ui.Node {
	set := ui.JS.Set(s.q, "x")
	return ui.HTML(`<p title={s.q} onClick={set}>{s.page}</p>`, s.q, set, s.page)
}

// TestNotify: a notified state's references carry the push its writes
// run — the handler, paced as declared — in bindings and in commands.
func TestNotify(t *testing.T) {
	c := &search{}
	if err := ui.MountLive(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := c.Render().Render(ui.WithLiveHost(context.Background(), handlers{}), &b); err != nil {
		t.Fatal(err)
	}
	out := strings.ReplaceAll(b.String(), "&#34;", `"`)
	for _, want := range []string{
		`"w":[["debounce",{"ms":300}],["push",{"h":`,
		`["state_set",{"s":"f`,
		`"args":[""]`,
		`"args":[0]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
	if n := strings.Count(out, `"w":`); n != 3 {
		t.Errorf("%d references carry their push, want 3:\n%s", n, out)
	}
}

// handlers is a live host giving every handler the id h.
type handlers struct{}

func (handlers) Component(ctx context.Context, n *ui.ComponentNode, w io.Writer) error { return nil }
func (handlers) Handler(context.Context, any) string                                   { return "h" }
func (handlers) Keyed(ctx context.Context, _ any) context.Context                      { return ctx }

// prefs keeps its theme and shares its sidebar.
type prefs struct {
	theme   ui.State[string]
	sidebar ui.State[bool]
}

func init() {
	uigen.Derive(func(p *prefs) {
		uigen.With(p.theme, ui.Keep("theme"))
		uigen.With(p.sidebar, ui.Keep("sidebar"))
		uigen.With(p.sidebar, ui.Shared{})
	})
}

// TestKeepShared: a kept state's references name its storage key, a
// shared one's the channel name its writes go by, in bindings and
// commands.
func TestKeepShared(t *testing.T) {
	p := &prefs{}
	uigen.States(p)
	set := ui.JS.Set(p.sidebar, true)
	out, _ := page(t, ui.HTML(`<p title={p.theme} onClick={set}>{p.sidebar}</p>`, p.theme, set, p.sidebar))
	out = strings.ReplaceAll(out, "&#34;", `"`)
	for _, want := range []string{`"k":"theme"`, `"i":false,"k":"sidebar"`, `"sh":"k:sidebar"`, `"v":true`} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
	if n := strings.Count(out, `"sh":`); n != 2 {
		t.Errorf("%d references are shared, want 2:\n%s", n, out)
	}
}
