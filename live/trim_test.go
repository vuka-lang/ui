package live_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/internal/demo"
)

// A trimmed session answers every event as one never trimmed does, byte for
// byte, with and without verification, trimmed before any or all of them.
func TestTrimChangesNoReply(t *testing.T) {
	targets := []string{"c1:0", "c1:1", "c3:0", "c4:0", "c2:0", "c7:0"}
	words := []string{"add", "rotate", "rename", "big", "pop", "note", "front", "drop", "reverse", "clear", "mid", "swap"}
	for _, verify := range []bool{false, true} {
		for _, every := range []int{1, 2, 5} {
			t.Run(fmt.Sprintf("verify=%v/every=%d", verify, every), func(t *testing.T) {
				defer func(v bool) { live.Verify = v }(live.Verify)
				live.Verify = verify
				plain, trimmed := joined(t, demo.Page, 2), joined(t, demo.Page, 2)
				r := rand.New(rand.NewPCG(1, uint64(every)))
				for i := range 500 {
					if i%every == 0 {
						trimmed.Trim()
					}
					msg, _ := json.Marshal(live.ClientMessage{Type: live.EventMsg, Ref: i + 1, Event: ui.Event{
						Target: targets[r.IntN(len(targets))], Type: "input", Value: words[r.IntN(len(words))]}})
					a, b := plain.HandleJSON(msg), trimmed.HandleJSON(msg)
					if string(a) != string(b) {
						t.Fatalf("event %d %s:\nnot trimmed: %s\ntrimmed:     %s", i, msg, a, b)
					}
				}
			})
		}
	}
}

// failing counts messages taken while fail is set, and renders failing then
// unless it's steady.
type failing struct {
	ui.Live
	Steady bool
	n      int
	fail   *bool
}

func (f *failing) Update(string) {
	if *f.fail {
		f.n++
	}
}

func (f *failing) Render() ui.Node {
	if *f.fail && !f.Steady {
		ui.Must(0, errors.New("the render failed"))
	}
	return ui.El("b", nil, ui.Text(fmt.Sprint(f.n)))
}

// After a render that failed the trees aren't what the client holds, so Trim
// leaves the shadows, and the next reply still says what changed since the
// client's last.
func TestTrimAfterFailedRender(t *testing.T) {
	fail := false
	page := func(context.Context) ui.Node {
		return ui.Fragment(ui.Component("a", nil, &failing{Steady: true, fail: &fail}), ui.Component("b", nil, &failing{fail: &fail}))
	}
	plain, trimmed := live.NewSession(context.Background(), page), live.NewSession(context.Background(), page)
	for _, s := range []*live.Session{plain, trimmed} {
		if _, err := s.RenderTrees(); err != nil {
			t.Fatal(err)
		}
	}
	event := func(s *live.Session) (string, error) {
		p, err := s.Info("", "more")
		b, _ := json.Marshal(p)
		return string(b), err
	}
	fail = true
	if _, err := event(plain); err == nil {
		t.Fatal("no failure")
	}
	if _, err := event(trimmed); err == nil {
		t.Fatal("no failure")
	}
	trimmed.Trim()
	fail = false
	a, _ := event(plain)
	b, err := event(trimmed)
	if err != nil || a != b {
		t.Fatalf("err %v\nnot trimmed: %s\ntrimmed:     %s", err, a, b)
	}
}

// A join after Trim sends the whole page again, as any join does; a broadcast
// after it takes the same path as an event.
func TestTrimThenJoin(t *testing.T) {
	plain, trimmed := joined(t, demo.Page, 2), joined(t, demo.Page, 2)
	trimmed.Trim()
	join := []byte(`{"type":"join","ref":9,"v":2}`)
	if a, b := plain.HandleJSON(join), trimmed.HandleJSON(join); string(a) != string(b) {
		t.Fatalf("not trimmed: %s\ntrimmed:     %s", a, b)
	}
	trimmed.Trim()
	pa, errA := plain.Info("", "hello")
	pb, errB := trimmed.Info("", "hello")
	x, _ := json.Marshal(pa)
	y, _ := json.Marshal(pb)
	if string(x) != string(y) || (errA == nil) != (errB == nil) {
		t.Fatalf("info: %s (%v) / %s (%v)", x, errA, y, errB)
	}
}

// A navigation of a trimmed session, a patch keeping its instances or one
// mounting afresh, answers as one never trimmed does.
func TestTrimThenNavigate(t *testing.T) {
	route := func(page func(context.Context) ui.Node, keep bool) live.Route {
		return live.Route{Render: page, Keep: keep}
	}
	for _, keep := range []bool{false, true} {
		plain, trimmed := joined(t, demo.Page, 2), joined(t, demo.Page, 2)
		for _, page := range []func(context.Context) ui.Node{demo.Counters(5), demo.Page, demo.Page} {
			trimmed.Trim()
			pa, errA := plain.Navigate(route(page, keep), false)
			pb, errB := trimmed.Navigate(route(page, keep), false)
			x, _ := json.Marshal(pa)
			y, _ := json.Marshal(pb)
			if errA != nil || errB != nil || string(x) != string(y) {
				t.Fatalf("keep=%v: %v, %v\nnot trimmed: %s\ntrimmed:     %s", keep, errA, errB, x, y)
			}
		}
	}
}
