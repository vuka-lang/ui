package live_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/uigen"
)

// The tags below are written as the compiler writes them: a func prop set to
// a method value is ui.MethodProps(&T{…}, ui.Method(field, receiver, method)).

// orders is a list of rows, each given a method of the list.
type orders struct {
	ui.Live
	N       int
	title   uigen.Assign[string]
	done    uigen.Assign[int]
	renders *int
}

func (p *orders) Rename()    { p.title.Set(p.title.Get() + "!") }
func (p *orders) Cancel()    { p.done.Update(func(n int) int { return n + 1 }) }
func (p *orders) Total() int { return p.done.Get() }
func (p *orders) Render() ui.Node {
	rows := []ui.Node{ui.El("button", []ui.Attr{a("onClick", ui.On(p.Rename))}, ui.Text(fmt.Sprint("rename ", p.title.Get(), " done ", p.done.Get())))}
	for i := range p.N {
		rows = append(rows, ui.Key(i, ui.MethodProps(&orderRow{ID: i, OnCancel: p.Cancel, renders: p.renders}, ui.Method("OnCancel", p, "Cancel"))))
	}
	return ui.El("div", nil, rows...)
}

// orderRow's state is all tracked, its func prop only a handler.
type orderRow struct {
	ui.Live
	ID       int
	OnCancel func()
	renders  *int `vuka:"-"`
	qty      uigen.Assign[int]
}

func (r *orderRow) Render() ui.Node {
	*r.renders++
	return ui.El("button", []ui.Attr{a("onClick", ui.On(r.OnCancel))}, ui.Text(fmt.Sprint(r.ID, ":", r.qty.Get())))
}

func newOrders(n int, renders *int) *live.Session {
	return live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("o", nil, &orders{N: n, renders: renders})
	})
}

// With the same method on the same receiver a tracked child is skipped, and a
// click on it still reaches the method.
func TestMethodPropSkipsChild(t *testing.T) {
	for _, verify := range []bool{false, true} {
		t.Run(fmt.Sprint("verify=", verify), func(t *testing.T) {
			defer func(v bool) { live.Verify = v }(live.Verify)
			live.Verify = verify
			renders := 0
			s := newOrders(3, &renders)
			if _, err := s.RenderTrees(); err != nil {
				t.Fatal(err)
			}
			if renders != 3 {
				t.Fatalf("first render: %d row renders", renders)
			}
			renders = 0
			if _, err := s.Event(ui.Event{Target: "c1:0", Type: "click"}); err != nil {
				t.Fatal(err)
			}
			want := 0
			if verify {
				want = 3 // verification renders what it would skip, to compare
			}
			if renders != want {
				t.Fatalf("after the parent's change: %d row renders, want %d", renders, want)
			}
			// the row's handler reaches the method through its stand-in
			if _, err := s.Event(ui.Event{Target: "c3:0", Type: "click"}); err != nil {
				t.Fatal(err)
			}
			if h := s.HTML(); !strings.Contains(h, "done 1") {
				t.Fatal(h)
			}
		})
	}
}

// totals' rows call its method to render: they must show what it says now.
type totals struct {
	ui.Live
	n uigen.Assign[int]
}

func (p *totals) Inc()         { p.n.Update(func(n int) int { return n + 1 }) }
func (p *totals) Show() string { return "n=" + strconv.Itoa(p.n.Get()) }
func (p *totals) Render() ui.Node {
	return ui.El("div", []ui.Attr{a("onClick", ui.On(p.Inc))},
		ui.Component("t", nil, ui.MethodProps(&totalRow{Label: p.Show}, ui.Method("Label", p, "Show"))))
}

type totalRow struct {
	ui.Live
	Label func() string
	st    uigen.Assign[int]
}

func (r *totalRow) Render() ui.Node { return ui.El("b", nil, ui.Text(r.helper())) }
func (r *totalRow) helper() string  { return r.Label() }

// A child calling the method while it renders is never skipped on it: without
// verification it shows the receiver's state, with it no event fails.
func TestMethodPropCalledInRender(t *testing.T) {
	for _, verify := range []bool{false, true} {
		t.Run(fmt.Sprint("verify=", verify), func(t *testing.T) {
			defer func(v bool) { live.Verify = v }(live.Verify)
			live.Verify = verify
			s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("p", nil, &totals{}) })
			if _, err := s.RenderTrees(); err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= 3; i++ {
				if _, err := s.Event(ui.Event{Target: "c1:0", Type: "click"}); err != nil {
					t.Fatal(err)
				}
				if h := s.HTML(); !strings.Contains(h, "n="+strconv.Itoa(i)) {
					t.Fatalf("after %d clicks: %s", i, h)
				}
			}
		})
	}
}

// A method of another receiver is another prop: the child renders, and its
// handler calls the new one.
type twoLists struct {
	ui.Live
	which uigen.Assign[int]
	a, b  *tray `vuka:"-"`
}
type tray struct{ hits int }

func (t *tray) Hit() { t.hits++ }

func (p *twoLists) Flip() { p.which.Update(func(n int) int { return n + 1 }) }
func (p *twoLists) Render() ui.Node {
	cur := p.a
	if p.which.Get()%2 == 1 {
		cur = p.b
	}
	return ui.El("div", []ui.Attr{a("onClick", ui.On(p.Flip))},
		ui.Component("h", nil, ui.MethodProps(&hitButton{OnHit: cur.Hit}, ui.Method("OnHit", cur, "Hit"))))
}

type hitButton struct {
	ui.Live
	OnHit func()
	st    uigen.Assign[int]
}

func (b *hitButton) Render() ui.Node {
	return ui.El("button", []ui.Attr{a("onClick", ui.On(b.OnHit))}, ui.Text("hit"))
}

func TestMethodPropOtherReceiver(t *testing.T) {
	x, y := &tray{}, &tray{}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("p", nil, &twoLists{a: x, b: y})
	})
	if _, err := s.RenderTrees(); err != nil {
		t.Fatal(err)
	}
	click := func(target string) {
		t.Helper()
		if _, err := s.Event(ui.Event{Target: target, Type: "click"}); err != nil {
			t.Fatal(err)
		}
	}
	click("c2:0")
	click("c1:0") // flips to y
	click("c2:0")
	if x.hits != 1 || y.hits != 1 {
		t.Fatalf("hits %d, %d", x.hits, y.hits)
	}
}

// A value receiver is bound by its value: rows passing a method of a per-row
// value are unchanged while the value is, changed when it isn't, and a value
// holding a pointer always counts as changed.
type priced struct {
	Name  string
	Cents int
}

func (p priced) Buy() {}

type holder struct {
	Name *string
}

func (h holder) Buy() {}

func TestSameReceiverValues(t *testing.T) {
	bound := func(c ui.Stateful) *hitButton { return c.(*hitButton) }
	mk := func(recv any) *hitButton {
		return ui.MethodProps(&hitButton{OnHit: func() {}}, ui.Method("OnHit", recv, "Buy"))
	}
	same := func(a, b any) bool { return live.SameProps(bound(mk(a)), bound(mk(b))) }
	name := "x"
	p1, p2 := &priced{}, &priced{}
	for _, c := range []struct {
		what string
		a, b any
		want bool
	}{
		{"same value", priced{"a", 1}, priced{"a", 1}, true},
		{"other value", priced{"a", 1}, priced{"a", 2}, false},
		{"other type", priced{"a", 1}, holder{}, false},
		{"same pointer", p1, p1, true},
		{"other pointer", p1, p2, false},
		{"value holding a pointer", holder{&name}, holder{&name}, false},
	} {
		if got := same(c.a, c.b); got != c.want {
			t.Errorf("%s: got %v", c.what, got)
		}
	}
	// another method of the receiver, or no record on one side
	a := ui.MethodProps(&hitButton{OnHit: func() {}}, ui.Method("OnHit", p1, "Buy"))
	b := ui.MethodProps(&hitButton{OnHit: func() {}}, ui.Method("OnHit", p1, "Sell"))
	if live.SameProps(a, b) {
		t.Error("another method counted as the same")
	}
	if live.SameProps(a, &hitButton{OnHit: func() {}}) || live.SameProps(&hitButton{OnHit: func() {}}, a) {
		t.Error("a closure counted as the same")
	}
}

// A closure is a change, as before.
type closureList struct {
	ui.Live
	N       int
	st      uigen.Assign[int]
	renders *int `vuka:"-"`
}

func (p *closureList) Inc() { p.st.Update(func(n int) int { return n + 1 }) }
func (p *closureList) Render() ui.Node {
	rows := []ui.Node{ui.El("button", []ui.Attr{a("onClick", ui.On(p.Inc))}, ui.Text("x"))}
	for i := range p.N {
		rows = append(rows, ui.Key(i, &orderRow{ID: i, OnCancel: func() { p.Inc() }, renders: p.renders}))
	}
	return ui.El("div", nil, rows...)
}

func TestClosurePropRendersChild(t *testing.T) {
	renders := 0
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("o", nil, &closureList{N: 3, renders: &renders})
	})
	if _, err := s.RenderTrees(); err != nil {
		t.Fatal(err)
	}
	renders = 0
	if _, err := s.Event(ui.Event{Target: "c1:0", Type: "click"}); err != nil {
		t.Fatal(err)
	}
	if renders != 3 {
		t.Fatalf("%d row renders, want 3", renders)
	}
}

// A transport names a handler by the method, and a variadic method works
// through its stand-in.
type vari struct {
	ui.Live
	Log func(xs ...string)
	st  uigen.Assign[int]
}

func (v *vari) Render() ui.Node {
	return ui.El("button", []ui.Attr{a("onClick", ui.On(v.Click))}, ui.Text("x"))
}
func (v *vari) Click() { v.Log("a", "b") }

type variHost struct {
	ui.Live
	got *[]string `vuka:"-"`
	st  uigen.Assign[int]
}

func (h *variHost) Add(xs ...string) { *h.got = append(*h.got, xs...) }
func (h *variHost) Render() ui.Node {
	return ui.Component("v", nil, ui.MethodProps(&vari{Log: h.Add}, ui.Method("Log", h, "Add")))
}

func TestMethodPropVariadic(t *testing.T) {
	var got []string
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("h", nil, &variHost{got: &got}) })
	if _, err := s.RenderTrees(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Event(ui.Event{Target: "c2:0", Type: "click"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a,b" {
		t.Fatalf("got %v", got)
	}
}

func TestHandlerNamesTheMethod(t *testing.T) {
	renders := 0
	s := newOrders(1, &renders)
	if _, err := s.RenderTrees(); err != nil {
		t.Fatal(err)
	}
	fn := s.Handler("c2:0")
	if name := live.FuncName(fn); !strings.Contains(name, "orders") || !strings.Contains(name, "Cancel") {
		t.Fatalf("named %q", name)
	}
}

// BenchmarkMethodPropRows: a parent re-rendering over 1000 rows, each given a
// method of the parent. "method" is a tag the compiler recorded; "closure" is
// one it can't: every row renders again. Both replies are small, which is what
// used to hide the cost; rows/event says how many rendered.
func BenchmarkMethodPropRows(b *testing.B) {
	for _, kind := range []string{"method", "closure"} {
		b.Run(kind, func(b *testing.B) {
			defer func(v bool) { live.Verify = v }(live.Verify)
			live.Verify = false
			renders := 0
			var page func(context.Context) ui.Node
			if kind == "method" {
				page = func(context.Context) ui.Node { return ui.Component("o", nil, &orders{N: 1000, renders: &renders}) }
			} else {
				page = func(context.Context) ui.Node { return ui.Component("o", nil, &closureList{N: 1000, renders: &renders}) }
			}
			s := live.NewSession(context.Background(), page)
			if _, err := s.RenderTrees(); err != nil {
				b.Fatal(err)
			}
			renders = 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := s.Event(ui.Event{Target: "c1:0", Type: "click"}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(renders)/float64(b.N), "rows/event")
		})
	}
}
