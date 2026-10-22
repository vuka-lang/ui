package live_test

import (
	"context"
	"io"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/internal/demo"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/ui/v2/uigen"
)

// handCounter is demo.Counter written by hand: nodes, no frames.
type handCounter struct {
	ui.Live
	Label string
	n     uigen.Assign[int]
}

func (c *handCounter) Inc() { c.n.Update(func(n int) int { return n + 1 }) }

func (c *handCounter) Render() ui.Node {
	return ui.El("span", nil, ui.Child(c.Label), ui.Text(": "), ui.Child(c.n.Get()), ui.Text(" "),
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(c.Inc)}}, ui.Text("+")))
}

// handCounters is demo.Counters written by hand; opaque renders the list
// through a NodeFunc, which the tree can't see into.
func handCounters(n int, opaque bool) func(context.Context) ui.Node {
	return func(context.Context) ui.Node {
		list := ui.Nodes(func(add func(ui.Node)) {
			for i := range n {
				add(ui.Component("demo.vuka#3", i, &handCounter{Label: "c" + strconv.Itoa(i)}))
			}
		})
		if opaque {
			inner := list
			list = ui.NodeFunc(func(ctx context.Context, w io.Writer) error { return inner.Render(ctx, w) })
		}
		return ui.El("main", nil, list)
	}
}

type fallbackRun struct {
	html  []string // the page after each event
	bytes int
}

// fallbackSession sends steps random events to page over v1 and v2; each
// client holds the page the server rendered after every one. pick chooses
// the events: from the run's own page when nil.
func fallbackSession(t *testing.T, name string, page func(context.Context) ui.Node, seed uint64, steps int, pick []ui.Event) (fallbackRun, []ui.Event) {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, 9))
	s1, s2 := live.NewSession(context.Background(), page), live.NewSession(context.Background(), page)
	c1, err := livetest.Join(s1, 1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := livetest.Join(s2, 2)
	if err != nil {
		t.Fatal(err)
	}
	var run fallbackRun
	var sent []ui.Event
	for i := range steps {
		if c1.HTML() != c2.HTML() || c2.HTML() != s2.HTML() || c1.HTML() != s1.HTML() {
			t.Fatalf("%s seed %d step %d:\nv1     %s\nv2     %s\nserver %s", name, seed, i, c1.HTML(), c2.HTML(), s2.HTML())
		}
		run.html = append(run.html, c2.HTML())
		var ev ui.Event
		if pick != nil {
			if i >= len(pick) {
				break
			}
			ev = pick[i]
		} else {
			var ok bool
			if ev, ok = c2.RandomEvent(r, demo.Words); !ok {
				break
			}
		}
		sent = append(sent, ev)
		m1, err1 := c1.Event(ev)
		m2, err2 := c2.Event(ev)
		if err1 != nil || err2 != nil || m1.Type != m2.Type || m1.Error != m2.Error {
			t.Fatalf("%s seed %d step %d %+v:\nv1 %+v %v\nv2 %+v %v", name, seed, i, ev, m1, err1, m2, err2)
		}
	}
	run.bytes = c2.Bytes
	return run, sent
}

// TestWholeComponentFallback: pages rendered without render tracking, every
// instance one piece of markup, take the same events to the same pages as
// tracked ones, with larger patches.
func TestWholeComponentFallback(t *testing.T) {
	pages := map[string]func(context.Context) ui.Node{
		"page":     demo.Page,
		"counters": demo.Counters(20),
		"table": func(context.Context) ui.Node {
			return ui.Fragment(ui.Component("t#1", nil, &demo.Table{N: 30}), ui.Component("t#2", nil, &demo.Table{N: 5, Plain: true}),
				ui.Component("t#3", nil, &demo.CellTable{N: 20}))
		},
	}
	defer func(v bool) { live.Verify = v }(live.Verify)
	defer live.SetWholeTrees(false)
	for name, page := range pages {
		var tracked, whole int
		for seed := range uint64(8) {
			live.Verify = seed%2 == 0
			live.SetWholeTrees(false)
			want, events := fallbackSession(t, name, page, seed, 120, nil)
			live.SetWholeTrees(true)
			got, _ := fallbackSession(t, name, page, seed, 120, events)
			for i := range want.html {
				if i >= len(got.html) || got.html[i] != want.html[i] {
					t.Fatalf("%s seed %d step %d: without tracking\n%s\nwith\n%s", name, seed, i, got.html[i], want.html[i])
				}
			}
			tracked, whole = tracked+want.bytes, whole+got.bytes
		}
		if whole <= tracked {
			t.Errorf("%s: %d bytes without tracking, %d with", name, whole, tracked)
		}
		t.Logf("%s: %d bytes on v2 without tracking, %d with (%.1fx)", name, whole, tracked, float64(whole)/float64(tracked))
	}
}

// TestHandWrittenComponents: components written as nodes, no frames,
// directly and through an opaque NodeFunc, render and patch the page JSX's
// render does.
func TestHandWrittenComponents(t *testing.T) {
	for seed := range uint64(6) {
		want, events := fallbackSession(t, "jsx", demo.Counters(20), seed, 100, nil)
		for _, opaque := range []bool{false, true} {
			got, _ := fallbackSession(t, "hand", handCounters(20, opaque), seed, 100, events)
			for i := range want.html {
				if i >= len(got.html) || got.html[i] != want.html[i] {
					t.Fatalf("seed %d step %d opaque %v: hand-written\n%s\nJSX\n%s", seed, i, opaque, got.html[i], want.html[i])
				}
			}
		}
	}
}

// BenchmarkFallback is v2 events with render tracking and without (each
// instance one piece of markup): a 1000-row table rendered by one
// component and a page of 100 counters, one changing per event.
func BenchmarkFallback(b *testing.B) {
	pages := []struct {
		name   string
		page   func(context.Context) ui.Node
		target func(i int) string
	}{
		{"celltable", func(context.Context) ui.Node { return ui.Component("b#1", nil, &demo.CellTable{N: 1000}) },
			func(i int) string { return "c1:" + strconv.Itoa(i*37%1000) }},
		{"counters", demo.Counters(100), func(i int) string { return "c" + strconv.Itoa(1+i*7%100) + ":0" }},
	}
	for _, p := range pages {
		for _, whole := range []bool{false, true} {
			b.Run(p.name+"/"+map[bool]string{false: "tracked", true: "whole"}[whole], func(b *testing.B) {
				live.SetWholeTrees(whole)
				defer live.SetWholeTrees(false)
				bench(b, p.page, 2, p.target)
			})
		}
	}
}
