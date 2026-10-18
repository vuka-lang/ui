package live_test

import (
	"context"
	"math/rand/v2"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/live/internal/demo"
	"github.com/vuka-lang/ui/live/livetest"
)

// FuzzTreePatch drives a page with a run of random events the fuzzer picks:
// after each, the page a client patched with the server's tree diffs is the
// page the server rendered, patch(old, diff(old, new)) == new, over both
// protocol versions and through reconnects.
func FuzzTreePatch(f *testing.F) {
	pages := []func(context.Context) ui.Node{
		demo.Page,
		demo.Counters(20),
		func(context.Context) ui.Node {
			return ui.Fragment(ui.Component("t#1", nil, &demo.Table{N: 30}), ui.Component("t#2", nil, &demo.Table{N: 5, Plain: true}),
				ui.Component("t#3", nil, &demo.CellTable{N: 20}))
		},
	}
	for seed := range uint64(6) {
		f.Add(seed, uint8(seed), uint8(60), seed%2 == 0)
	}
	f.Fuzz(func(t *testing.T, seed uint64, which, steps uint8, verify bool) {
		defer func(v bool) { live.Verify = v }(live.Verify)
		live.Verify = verify
		page := pages[int(which)%len(pages)]
		r := rand.New(rand.NewPCG(seed, uint64(which)))
		s1, s2 := live.NewSession(context.Background(), page), live.NewSession(context.Background(), page)
		c1, err := livetest.Join(s1, 1)
		if err != nil {
			t.Fatal(err)
		}
		c2, err := livetest.Join(s2, 2)
		if err != nil {
			t.Fatal(err)
		}
		for i := range int(steps) {
			if c1.HTML() != s1.HTML() || c2.HTML() != s2.HTML() || c1.HTML() != c2.HTML() {
				t.Fatalf("step %d:\nv1     %s\nv2     %s\nserver %s", i, c1.HTML(), c2.HTML(), s2.HTML())
			}
			if r.IntN(30) == 0 {
				if c2, err = livetest.Join(s2, 2); err != nil {
					t.Fatal(err)
				}
				continue
			}
			ev, ok := c2.RandomEvent(r, demo.Words)
			if !ok {
				return
			}
			m1, err1 := c1.Event(ev)
			m2, err2 := c2.Event(ev)
			if err1 != nil || err2 != nil || m1.Type != m2.Type || m1.Error != m2.Error {
				t.Fatalf("step %d %+v:\nv1 %+v %v\nv2 %+v %v", i, ev, m1, err1, m2, err2)
			}
		}
	})
}
