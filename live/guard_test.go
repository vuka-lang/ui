package live_test

import (
	"context"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/internal/demo"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/ui/v2/uigen"
)

func TestGuardKeepsLoop(t *testing.T) {
	for _, verify := range []bool{false, true} {
		t.Run("verify="+strconv.FormatBool(verify), func(t *testing.T) {
			defer func(v bool) { live.Verify = v }(live.Verify)
			live.Verify = verify
			built := 0
			s := joined(t, func(context.Context) ui.Node {
				return ui.Component("b#1", nil, &GuardTable{N: 5, Built: &built})
			}, 2)
			if built != 1 {
				t.Fatalf("join built the loop %d times", built)
			}
			want := 1
			for i := range 3 {
				event(t, s, "c1:0") // the title
				if verify {
					want++
				}
				if built != want {
					t.Fatalf("title event %d: built %d, want %d", i, built, want)
				}
			}
			event(t, s, "c1:3:0") // a row's cell
			if want++; built != want {
				t.Fatalf("cell event: built %d, want %d", built, want)
			}
			if !strings.Contains(s.HTML(), `<td class="n">1</td>`) {
				t.Fatalf("the cell didn't change: %s", s.HTML())
			}
			event(t, s, "c1:0")
			event(t, s, "c1:3:0") // the row's handler, named again by the kept loop
			if !strings.Contains(s.HTML(), `<td class="n">2</td>`) {
				t.Fatalf("the kept row's handler didn't run: %s", s.HTML())
			}
		})
	}
}

// guardPage has guarded parts with handlers in and around them: a branch
// with an unkeyed button, a keyed loop with buttons, a loop nested in a
// branch, and buttons before them that come and go.
type guardPage struct {
	ui.Live
	Guards bool
	built  *int `vuka:"-"`
	extra  uigen.Assign[int]
	title  uigen.Assign[int]
	on     uigen.Assign[bool]
	rows   uigen.Assign[[]int]
	tags   uigen.Assign[[]string]
}

func (p *guardPage) More()      { p.extra.Set(p.extra.Get() + 1) }
func (p *guardPage) Less()      { p.extra.Set(max(0, p.extra.Get()-1)) }
func (p *guardPage) Title()     { p.title.Set(p.title.Get() + 1) }
func (p *guardPage) Toggle()    { p.on.Set(!p.on.Get()) }
func (p *guardPage) Add()       { p.rows.Set(append(p.rows.Get(), len(p.rows.Get()))) }
func (p *guardPage) Tag()       { p.tags.Set(append(p.tags.Get(), "t"+strconv.Itoa(len(p.tags.Get())))) }
func (p *guardPage) Bump(i int) { p.rows.Update(func(r []int) []int { r[i]++; return r }) }
func (p *guardPage) Drop(i int) {
	p.tags.Update(func(t []string) []string { return append(t[:i:i], t[i+1:]...) })
}
func (p *guardPage) count()       { *p.built++ }
func (p *guardPage) guards() bool { return p.Guards }

func (p *guardPage) rowItems() []ui.Node {
	p.count()
	var trs []ui.Node
	for i, v := range p.rows.Get() {
		trs = append(trs, ui.HTML(`<li key={i}>{v} <button onClick={func() { p.Bump(i) }}>+</button></li>`, i, v, func() { p.Bump(i) }))
	}
	return trs
}

func (p *guardPage) tagItems() []ui.Node {
	p.count()
	var lis []ui.Node
	for i, t := range p.tags.Get() {
		lis = append(lis, ui.HTML(`<b onClick={func() { p.Drop(i) }}>{t}</b>`, func() { p.Drop(i) }, t))
	}
	return lis
}

func (p *guardPage) panel() ui.Node {
	p.count()
	if !p.on.Get() {
		return ui.HTML(`<p>off <button onClick={p.Tag}>tag</button></p>`, p.Tag)
	}
	var tags any
	if p.guards() {
		tags = ui.Guard(2, p.tagItems, p.tags.Version())
	} else {
		tags = p.tagItems()
	}
	return ui.HTML(`<section><button onClick={p.Tag}>tag</button>{tags}</section>`, p.Tag, tags)
}

func (p *guardPage) Render() ui.Node {
	var extras []ui.Node
	for range p.extra.Get() {
		extras = append(extras, ui.HTML(`<button onClick={p.Less}>less</button>`, p.Less))
	}
	var panel, rows any
	if p.guards() {
		panel = ui.GuardNode(1, p.panel, p.on.Version(), p.tags.Version())
		rows = ui.Guard(3, p.rowItems, p.rows.Version())
	} else {
		panel, rows = p.panel(), p.rowItems()
	}
	return ui.HTML(`<main>
<h1 onClick={p.Title}>{p.title.Get()}</h1>
<button onClick={p.More}>more</button>
<button onClick={p.Toggle}>toggle</button>
<button onClick={p.Add}>add</button>
{extras}
{panel}
<ul>{rows}</ul>
</main>`, p.Title, p.title.Get(), p.More, p.Toggle, p.Add, extras, panel, rows)
}

// TestGuardSameAsRender sends the same random events to a page with guarded
// parts and to the same page without, checking the clients hold the same
// HTML throughout: what a kept part gives back, its handlers' ids too, is
// what rendering it gives.
func TestGuardSameAsRender(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	for seed := range uint64(20) {
		live.Verify = seed%4 == 0 // verifying, nothing differs either
		built := [2]int{}
		var cs [2]*livetest.Client
		for i := range cs {
			s := live.NewSession(context.Background(), func(context.Context) ui.Node {
				return ui.Component("g#1", nil, &guardPage{Guards: i == 0, built: &built[i]})
			})
			if _, err := s.Render(); err != nil {
				t.Fatal(err)
			}
			c, err := livetest.Join(s, 2)
			if err != nil {
				t.Fatal(err)
			}
			cs[i] = c
		}
		r := rand.New(rand.NewPCG(seed, 7))
		for step := range 60 {
			if cs[0].HTML() != cs[1].HTML() {
				t.Fatalf("seed %d step %d: guarded\n%s\nunguarded\n%s", seed, step, cs[0].HTML(), cs[1].HTML())
			}
			ev, ok := cs[1].RandomEvent(r, demo.Words)
			if !ok {
				t.Fatal("no handlers")
			}
			for _, c := range cs {
				if m, err := c.Event(ev); err != nil || m.Type == live.Error {
					t.Fatalf("seed %d step %d: %s: %v %+v", seed, step, ev.Target, err, m)
				}
			}
		}
		if !live.Verify && built[0] >= built[1] {
			t.Errorf("seed %d: the guarded page built its parts %d times, the other %d", seed, built[0], built[1])
		}
	}
}

// wrongGuard's guard omits the field its loop reads.
type wrongGuard struct {
	ui.Live
	rows uigen.Assign[[]int]
}

func (w *wrongGuard) Add() { w.rows.Set(append(w.rows.Get(), 1)) }

func (w *wrongGuard) Render() ui.Node {
	items := ui.Guard(1, func() []ui.Node {
		var lis []ui.Node
		for range w.rows.Get() {
			lis = append(lis, ui.HTML(`<li>row</li>`))
		}
		return lis
	})
	return ui.HTML(`<ul onClick={w.Add}>{items}</ul>`, w.Add, items)
}

func TestGuardVerify(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = true
	s := joined(t, func(context.Context) ui.Node { return ui.Component("w#1", nil, &wrongGuard{}) }, 2)
	out := string(s.HandleJSON([]byte(`{"type":"event","target":"c1:0","event":"click"}`)))
	if !strings.Contains(out, "renders a guarded part differently") {
		t.Fatalf("a wrong guard passed verification: %s", out)
	}
}

// guardEffects' guarded parts do what a kept part can't give back.
type guardEffects struct {
	ui.Live
	built *int `vuka:"-"`
	n     uigen.Assign[int]
}

func (g *guardEffects) Inc() { g.n.Set(g.n.Get() + 1) }

func (g *guardEffects) Render() ui.Node {
	child := ui.GuardNode(1, func() ui.Node {
		*g.built++
		return ui.HTML(`<div>{counter}</div>`, &demo.Counter{})
	})
	random := ui.GuardNode(2, func() ui.Node {
		*g.built++
		return ui.HTML(`<p id={rid}>random</p>`, "id-ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	})
	keyed := ui.GuardNode(3, func() ui.Node {
		*g.built++
		return ui.HTML(`<p onClick={g.Inc}>in a key</p>`, g.Inc)
	})
	return ui.HTML(`<main onClick={g.Inc}>{g.n.Get()}{child}{random}<div key="k">{keyed}</div></main>`, g.Inc, g.n.Get(), child, random, keyed)
}

func TestGuardRendersWhatItCantKeep(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = false
	built := 0
	s := joined(t, func(context.Context) ui.Node { return ui.Component("e#1", nil, &guardEffects{built: &built}) }, 2)
	before := built
	event(t, s, "c1:0")
	if built-before != 3 {
		t.Fatalf("built %d of 3 parts that can't be kept", built-before)
	}
	if !strings.Contains(s.HTML(), `id-c1-1`) {
		t.Fatalf("the random id wasn't renamed: %s", s.HTML())
	}
}
