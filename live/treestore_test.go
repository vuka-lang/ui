package live_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/internal/demo"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

func joinIn(t testing.TB, st *live.Store, page func(context.Context) ui.Node) (*live.Session, *livetest.Client) {
	t.Helper()
	s := live.NewSession(context.Background(), page)
	live.UseStore(s, st)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func TestIdenticalSessionsShare(t *testing.T) {
	st := live.NewStore()
	var ss []*live.Session
	var cs []*livetest.Client
	var base [2]int
	for i := range 6 {
		s, c := joinIn(t, st, demo.Page)
		ss, cs = append(ss, s), append(cs, c)
		if i == 0 {
			base[0], base[1] = live.StoreCounts(st)
			if base[0] == 0 {
				t.Fatal("the first session stored nothing")
			}
		}
	}
	if tr, li := live.StoreCounts(st); tr != base[0] || li != base[1] {
		t.Errorf("six identical sessions store %d trees, %d lists; one stores %d, %d", tr, li, base[0], base[1])
	}
	if live.RootTree(ss[0]) != live.RootTree(ss[5]) {
		t.Error("two identical pages don't share their root tree")
	}
	if err := live.CheckStore(st, ss...); err != nil {
		t.Fatal(err)
	}
	before := live.TreeDump(ss[0])
	ev, ok := cs[4].RandomEvent(rand.New(rand.NewPCG(1, 2)), demo.Words)
	if !ok {
		t.Fatal("no handler")
	}
	if _, err := cs[4].Event(ev); err != nil {
		t.Fatal(err)
	}
	if live.TreeDump(ss[0]) != before {
		t.Error("an event of one session changed another's tree")
	}
	if cs[0].HTML() != ss[0].HTML() {
		t.Error("the first session's page changed")
	}
	if err := live.CheckStore(st, ss...); err != nil {
		t.Fatal(err)
	}
	for _, s := range ss {
		s.Close()
	}
	if tr, li := live.StoreCounts(st); tr != 0 || li != 0 {
		t.Errorf("closed sessions left %d trees, %d lists", tr, li)
	}
	if err := live.CheckStore(st); err != nil {
		t.Fatal(err)
	}
}

type scenario struct {
	name  string
	lossy bool // the server forgets what it sent (streams, Temporary): the client's page isn't its HTML
	page  func(i int) func(context.Context) ui.Node
	info  func(r *rand.Rand) (topic string, msg any)
}

func scenarios() []scenario {
	return []scenario{
		{name: "boards", page: func(int) func(context.Context) ui.Node { return demo.Page }},
		{name: "counters", page: func(i int) func(context.Context) ui.Node { return demo.Counters(10 + i%3*5) }},
		{name: "tables", page: func(int) func(context.Context) ui.Node {
			return func(context.Context) ui.Node {
				return ui.Fragment(ui.Component("t#1", nil, &demo.Table{N: 30}), ui.Component("t#2", nil, &demo.Table{N: 5, Plain: true}),
					ui.Component("t#3", nil, &demo.CellTable{N: 20}))
			}
		}},
		{name: "temporary", lossy: true, page: func(int) func(context.Context) ui.Node { return reportPage(false) }},
		{name: "stream", lossy: true, page: func(i int) func(context.Context) ui.Node {
			return chatPage(&chatStore{msgs: msgs(1, 2, 3, i%2)}, 6)
		}, info: func(r *rand.Rand) (string, any) {
			id := r.IntN(12)
			switch r.IntN(4) {
			case 0:
				return "chat", unsaid{id}
			case 1:
				return "chat", pinned{chatMsg{id + 100, fmt.Sprint("pin", id)}}
			}
			return "chat", said{chatMsg{id, fmt.Sprint("say", id)}}
		}},
	}
}

var batchID = regexp.MustCompile(`data-vk-batch=(\\?")[^"\\]*(\\?")`)

var tempMark = regexp.MustCompile(`,t="[^"]*"`)

// same is whether two pages are alike but for the random ids a stream's batch
// and a Temporary region carry.
func same(a, b string) bool {
	strip := func(s string) string { return tempMark.ReplaceAllString(batchID.ReplaceAllString(s, ""), "") }
	return strip(a) == strip(b)
}

type pair struct {
	sh, ref *live.Session
	csh     *livetest.Client
	cref    *livetest.Client
}

// Sessions that share trees answer every event and message, byte for byte, as
// sessions that don't, and nothing one does changes another's trees.
func TestSharedTreesMatchUnshared(t *testing.T) {
	for _, sc := range scenarios() {
		for seed := range uint64(3) {
			t.Run(fmt.Sprintf("%s/%d", sc.name, seed), func(t *testing.T) {
				defer func(v bool) { live.ShareTrees = v }(live.ShareTrees)
				st := live.NewStore()
				r := rand.New(rand.NewPCG(seed, 7))
				n := 0
				mk := func() pair {
					var p pair
					var err error
					live.ShareTrees = true
					p.sh = live.NewSession(context.Background(), sc.page(n))
					live.UseStore(p.sh, st)
					live.ShareTrees = false
					p.ref = live.NewSession(context.Background(), sc.page(n))
					n++
					if p.csh, err = livetest.Join(p.sh, 2); err != nil {
						t.Fatal(err)
					}
					if p.cref, err = livetest.Join(p.ref, 2); err != nil {
						t.Fatal(err)
					}
					return p
				}
				ps := make([]pair, 6)
				for i := range ps {
					ps[i] = mk()
				}
				sessions := func() (out []*live.Session) {
					for _, p := range ps {
						out = append(out, p.sh)
					}
					return out
				}
				for step := range 150 {
					i := r.IntN(len(ps))
					p := ps[i]
					switch k := r.IntN(40); {
					case k == 0:
						var err error
						if p.csh, err = livetest.Join(p.sh, 2); err != nil {
							t.Fatal(err)
						}
						if p.cref, err = livetest.Join(p.ref, 2); err != nil {
							t.Fatal(err)
						}
						ps[i] = p
					case k == 1:
						p.sh.Close()
						p.ref.Close()
						ps[i] = mk()
					case k == 2:
						p.sh.Trim()
						p.ref.Trim()
					case k < 12 && sc.info != nil:
						topic, msg := sc.info(r)
						e1, e2 := p.csh.Info(topic, msg), p.cref.Info(topic, msg)
						if (e1 == nil) != (e2 == nil) {
							t.Fatalf("step %d info %v: %v, %v", step, msg, e1, e2)
						}
					default:
						ev, ok := p.cref.RandomEvent(r, demo.Words)
						if !ok {
							continue
						}
						m1, e1 := p.csh.Event(ev)
						m2, e2 := p.cref.Event(ev)
						j1, _ := json.Marshal(m1)
						j2, _ := json.Marshal(m2)
						if (e1 == nil) != (e2 == nil) || !same(string(j1), string(j2)) {
							t.Fatalf("step %d %+v:\nshared   %s %v\nunshared %s %v", step, ev, j1, e1, j2, e2)
						}
					}
					for j, q := range ps {
						if !same(live.TreeDump(q.sh), live.TreeDump(q.ref)) || !same(q.csh.HTML(), q.cref.HTML()) || !same(q.sh.HTML(), q.ref.HTML()) {
							t.Fatalf("step %d: session %d differs from its unshared twin after session %d acted:\n%s\n%s", step, j, i, live.TreeDump(q.sh), live.TreeDump(q.ref))
						}
					}
					if err := live.CheckStore(st, sessions()...); err != nil {
						t.Fatalf("step %d: %v", step, err)
					}
				}
				for _, p := range ps {
					p.sh.Close()
					p.ref.Close()
				}
				if tr, li := live.StoreCounts(st); tr != 0 || li != 0 {
					t.Errorf("closed sessions left %d trees, %d lists", tr, li)
				}
			})
		}
	}
}

// Sessions on many goroutines store into one table.
func TestSharedTreesConcurrent(t *testing.T) {
	st := live.NewStore()
	scs := scenarios()
	const n = 24
	ss := make([]*live.Session, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := scs[i%len(scs)]
			s, c := joinIn(t, st, sc.page(i%3))
			ss[i] = s
			r := rand.New(rand.NewPCG(uint64(i), 3))
			for range 120 {
				if sc.info != nil && r.IntN(3) == 0 {
					topic, msg := sc.info(r)
					if err := c.Info(topic, msg); err != nil {
						t.Error(err)
						return
					}
					continue
				}
				ev, ok := c.RandomEvent(r, demo.Words)
				if !ok {
					break
				}
				if _, err := c.Event(ev); err != nil {
					t.Error(err)
					return
				}
				if r.IntN(25) == 0 {
					s.Trim()
				}
			}
			if !sc.lossy && c.HTML() != s.HTML() {
				t.Errorf("session %d: the client's page is not the server's", i)
			}
		}()
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	if err := live.CheckStore(st, ss...); err != nil {
		t.Fatal(err)
	}
	for i, s := range ss {
		if i%2 == 0 {
			s.Close()
		}
	}
	var rest []*live.Session
	for i, s := range ss {
		if i%2 == 1 {
			rest = append(rest, s)
		}
	}
	if err := live.CheckStore(st, rest...); err != nil {
		t.Fatal(err)
	}
	var wg2 sync.WaitGroup
	for _, s := range rest {
		wg2.Add(1)
		go func() { defer wg2.Done(); s.Close() }()
	}
	wg2.Wait()
	if tr, li := live.StoreCounts(st); tr != 0 || li != 0 {
		t.Errorf("closed sessions left %d trees, %d lists", tr, li)
	}
}

// FuzzSharedTrees is FuzzTreePatch over sessions that share trees: each
// client's page is its server's, and a session closing leaves the rest whole.
func FuzzSharedTrees(f *testing.F) {
	for seed := range uint64(4) {
		f.Add(seed, uint8(seed), uint8(50))
	}
	f.Fuzz(func(t *testing.T, seed uint64, which, steps uint8) {
		sc := scenarios()[int(which)%len(scenarios())]
		st := live.NewStore()
		r := rand.New(rand.NewPCG(seed, uint64(which)))
		var ss []*live.Session
		var cs []*livetest.Client
		for i := range 4 {
			s, c := joinIn(t, st, sc.page(i))
			ss, cs = append(ss, s), append(cs, c)
		}
		for i := range int(steps) {
			k := r.IntN(len(ss))
			if r.IntN(20) == 0 {
				ss[k].Close()
				ss[k], cs[k] = joinIn(t, st, sc.page(k))
				continue
			}
			if sc.info != nil && r.IntN(3) == 0 {
				topic, msg := sc.info(r)
				if err := cs[k].Info(topic, msg); err != nil {
					t.Fatal(err)
				}
			} else if ev, ok := cs[k].RandomEvent(r, demo.Words); ok {
				if _, err := cs[k].Event(ev); err != nil {
					t.Fatal(err)
				}
			}
			for j := range ss {
				if !sc.lossy && cs[j].HTML() != ss[j].HTML() {
					t.Fatalf("step %d: client %d holds another page than its server", i, j)
				}
			}
			if err := live.CheckStore(st, ss...); err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
		}
		for _, s := range ss {
			s.Close()
		}
		if tr, li := live.StoreCounts(st); tr != 0 || li != 0 {
			t.Errorf("closed sessions left %d trees, %d lists", tr, li)
		}
	})
}

func shareModes(b *testing.B, f func(b *testing.B)) {
	for _, share := range []bool{false, true} {
		b.Run(fmt.Sprintf("share=%v", share), func(b *testing.B) {
			defer func(s, v bool) { live.ShareTrees, live.Verify = s, v }(live.ShareTrees, live.Verify)
			live.ShareTrees, live.Verify = share, false
			f(b)
		})
	}
}

// An event's reply on a page 50 other sessions render alike: what hashing and
// finding the page's nodes adds to a render, and takes from its diff.
func BenchmarkSharedRender(b *testing.B) {
	pages := []struct {
		name   string
		page   func(context.Context) ui.Node
		target func(i int) string
	}{
		{"celltable", func(context.Context) ui.Node { return ui.Component("b#1", nil, &demo.CellTable{N: 1000}) },
			func(i int) string { return "c1:" + strconv.Itoa(i*37%1000) + ":0" }},
		{"counters", demo.Counters(100), func(i int) string { return "c" + strconv.Itoa(1+i*7%100) + ":0" }},
		{"boards", demo.Page, func(i int) string { return []string{"c1:0", "c1:1", "c3:0", "c4:0"}[i%4] }},
	}
	for _, p := range pages {
		b.Run(p.name, func(b *testing.B) {
			shareModes(b, func(b *testing.B) {
				var others []*live.Session
				for range 50 {
					others = append(others, joined(b, p.page, 2))
				}
				s := joined(b, p.page, 2)
				b.ResetTimer()
				for i := 0; b.Loop(); i++ {
					event(b, s, p.target(i))
				}
				b.StopTimer()
				runtime.KeepAlive(others)
			})
		})
	}
}

// A joined session's heap, with 200 sessions on the same page.
func BenchmarkSessionBytes(b *testing.B) {
	pages := map[string]func(context.Context) ui.Node{
		"celltable": func(context.Context) ui.Node { return ui.Component("b#1", nil, &demo.CellTable{N: 200}) },
		"counters":  demo.Counters(100),
		"boards":    demo.Page,
	}
	for name, page := range pages {
		b.Run(name, func(b *testing.B) {
			shareModes(b, func(b *testing.B) {
				const n = 200
				heap := func() uint64 {
					runtime.GC()
					runtime.GC()
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					return m.HeapAlloc
				}
				for b.Loop() {
					before := heap()
					ss := make([]*live.Session, n)
					for i := range ss {
						ss[i] = joined(b, page, 2)
					}
					after := heap()
					b.ReportMetric(float64(after-before)/n, "B/session")
					for _, s := range ss {
						s.Close()
					}
				}
			})
		})
	}
}

// Pages that move in from an ahead session, navigations and a close leave the
// table holding exactly what the session reaches.
func TestStoreAcrossNavigation(t *testing.T) {
	st := live.NewStore()
	app := &aheadApp{}
	s := live.NewSession(context.Background(), app.route("go", 0, true).Render)
	live.UseStore(s, st)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		if err := live.CheckStore(st, s); err != nil {
			t.Fatal(err)
		}
	}
	check()
	for i, name := range []string{"rust", "go", "lobby"} {
		rt := app.route(name, i+1, true)
		ahead := live.NewSession(context.Background(), rt.Render)
		live.UseStore(ahead, st)
		if err := ahead.RenderAhead(); err != nil {
			t.Fatal(err)
		}
		p, err := s.NavigateFrom(rt, ahead, false)
		if err != nil {
			t.Fatal(err)
		}
		apply(t, c, p)
		check()
		if err := c.Click("#toggle"); err != nil {
			t.Fatal(err)
		}
		check()
	}
	p, err := s.Navigate(app.route("rust", 9, false), false)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, c, p)
	check()
	s.Close()
	if tr, li := live.StoreCounts(st); tr != 0 || li != 0 {
		t.Errorf("a closed session left %d trees, %d lists", tr, li)
	}
}

// Events on 400 sessions of one page from every core at once: the store's
// locks and counts on nodes all of them hold.
func BenchmarkSharedRenderParallel(b *testing.B) {
	shareModes(b, func(b *testing.B) {
		ss := make([]*live.Session, 400)
		for i := range ss {
			ss[i] = joined(b, demo.Counters(100), 2)
		}
		var n atomic.Int64
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				i := int(n.Add(1))
				event(b, ss[i%len(ss)], "c"+strconv.Itoa(1+i*7%100)+":0")
			}
		})
	})
}

// A shared fragment's frames and snapshots, and the pages that show it, are
// the same bytes with trees stored once as without.
func TestSharedFragmentsThroughStore(t *testing.T) {
	defer func(v bool) { live.ShareTrees = v }(live.ShareTrees)
	run := func(share bool) (out []string) {
		live.ShareTrees = share
		set := newSet(true)
		var pages []*live.Session
		for range 3 {
			s := sharedPage(set, &links{}, func() ui.Node {
				return ui.El("main", nil, ui.Component("b", nil, &PriceBoard{}))
			})
			m := s.Handle(live.ClientMessage{Type: live.Join, V: 2})
			out = append(out, sj(m.Trees))
			pages = append(pages, s)
		}
		f := set.Fragments()[0]
		for n := 1; n <= 3; n++ {
			err := f.Publish([]live.Delivery{live.Message(n)}, func(fr live.SharedFrame) {
				out = append(out, string(fr.Msg), string(fr.Snapshot().Msg))
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, p := range pages {
			p.Close()
		}
		return out
	}
	on, off := run(true), run(false)
	if strings.Join(on, "\n") != strings.Join(off, "\n") {
		t.Errorf("shared fragments differ with the store:\n%q\n%q", on, off)
	}
}
