package live

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
)

// slowCard defers its rows; its loader returns when release is closed (at
// once when nil), counting its runs.
type slowCard struct {
	ui.Live
	wait    time.Duration
	delay   time.Duration
	minShow time.Duration
	release chan struct{}
	runs    *atomic.Int32
	rows    []string
}

func (c *slowCard) LiveSchema(l *ui.LiveSchema) {
	l.Field(&c.rows, ui.Defer("rows"))
	l.Loads("rows", c.load)
}

func (c *slowCard) load(ctx context.Context) error {
	c.runs.Add(1)
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.rows = []string{"a", "b"}
	return nil
}

func (c *slowCard) Render() ui.Node {
	section := ui.Await{Of: c, Key: "rows", Wait: c.wait, Delay: c.delay, MinShow: c.minShow,
		Pending: ui.HTML(`<i>loading</i>`),
		Children: func() ui.Node {
			return ui.HTML(`<ul>{rows}</ul>`, ui.HTML(`<li>`+strings.Join(c.rows, "</li><li>")+`</li>`))
		}}
	return ui.HTML(`<section>{section}</section>`, section)
}

func holdSession(cards ...*slowCard) *Session {
	return NewSession(context.Background(), func(ctx context.Context) ui.Node {
		var ns []ui.Node
		for i, c := range cards {
			ns = append(ns, ui.Component(string(rune('a'+i)), nil, c))
		}
		return ui.HTML(`<main>{ns}</main>`, ns)
	})
}

// A loader that finishes within Wait ships its content in the first render:
// the fallback never goes on the wire.
func TestWaitShipsContent(t *testing.T) {
	var runs atomic.Int32
	s := holdSession(&slowCard{wait: time.Second, runs: &runs})
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "loading") || !strings.Contains(html, "<li>a</li>") {
		t.Fatalf("got %s", html)
	}
	if ds := s.Deferred(); len(ds) != 0 {
		t.Fatalf("%d loads left to run", len(ds))
	}
	trees, err := s.RenderTrees()
	if err != nil {
		t.Fatal(err)
	}
	if got := encode(t, trees); strings.Contains(got, "loading") || !strings.Contains(got, "a") || runs.Load() != 1 {
		t.Fatalf("runs %d, join %s", runs.Load(), got)
	}
}

// A loader that misses Wait renders its fallback, keeps running, and is
// handed to the transport once, without starting again.
func TestWaitMissed(t *testing.T) {
	var runs atomic.Int32
	release := make(chan struct{})
	s := holdSession(&slowCard{wait: 30 * time.Millisecond, release: release, runs: &runs})
	start := time.Now()
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 25*time.Millisecond || took > 500*time.Millisecond {
		t.Fatalf("held %v", took)
	}
	if !strings.Contains(html, "loading") {
		t.Fatalf("got %s", html)
	}
	ds := s.Deferred()
	if len(ds) != 1 {
		t.Fatalf("%d loads", len(ds))
	}
	close(release)
	if err := ds[0].Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, err := s.Loaded(ds[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := encode(t, p); !strings.Contains(got, "a") || runs.Load() != 1 {
		t.Fatalf("runs %d, patch %s", runs.Load(), got)
	}
	if again := s.Deferred(); len(again) != 0 {
		t.Fatalf("%d loads start again", len(again))
	}
}

// Several sections of a page share one wait: the longest, not the sum.
func TestWaitIsOneBudget(t *testing.T) {
	var runs atomic.Int32
	never := make(chan struct{})
	defer close(never)
	s := holdSession(
		&slowCard{wait: 100 * time.Millisecond, release: never, runs: &runs},
		&slowCard{wait: 150 * time.Millisecond, release: never, runs: &runs},
		&slowCard{wait: 120 * time.Millisecond, release: never, runs: &runs},
	)
	defer s.Close()
	start := time.Now()
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 140*time.Millisecond || took > 300*time.Millisecond {
		t.Fatalf("held %v, want about the longest wait, 150ms", took)
	}
}

// A section with no Wait holds nothing, and a hold of 0 starts no loader.
func TestNoWaitHoldsNothing(t *testing.T) {
	var runs atomic.Int32
	never := make(chan struct{})
	defer close(never)
	s := holdSession(&slowCard{release: never, runs: &runs})
	defer s.Close()
	start := time.Now()
	html, err := s.Render()
	if err != nil || !strings.Contains(html, "loading") {
		t.Fatalf("got %s, %v", html, err)
	}
	if time.Since(start) > 100*time.Millisecond || runs.Load() != 0 {
		t.Fatalf("held, %d runs", runs.Load())
	}
}

// The wrapper the runtime holds: pending is hidden for Delay, and the same
// element carries the done state; without a session there is none.
func TestAwaitRegion(t *testing.T) {
	var runs atomic.Int32
	c := &slowCard{delay: 200 * time.Millisecond, minShow: 400 * time.Millisecond, release: make(chan struct{}), runs: &runs}
	s := holdSession(c)
	defer s.Close()
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<vk-await data-vk-await="pending" data-vk-delay="200" data-vk-min="400" style="display: contents; visibility: hidden;">`, `<i>loading</i>`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in %s", want, html)
		}
	}
	plain := &slowCard{runs: &runs}
	out, err := ui.String(context.Background(), ui.Component("p", nil, plain))
	if err != nil || strings.Contains(out, "vk-await") {
		t.Fatalf("got %s, %v", out, err)
	}
}

// ui.Instant, and no MinShow, leave no wrapper; the default Delay makes one.
func TestAwaitDefaultDelay(t *testing.T) {
	var runs atomic.Int32
	for delay, want := range map[time.Duration]string{0: `data-vk-delay="150"`, ui.Instant: "<i>loading</i></section>"} {
		s := holdSession(&slowCard{delay: delay, release: make(chan struct{}), runs: &runs})
		html, err := s.Render()
		s.Close()
		if err != nil || !strings.Contains(html, want) {
			t.Errorf("delay %v: got %s, %v", delay, html, err)
		}
	}
}

// pairCard shows two groups as one section.
type pairCard struct {
	ui.Live
	a, b []string
	bad  bool
}

func (c *pairCard) LiveSchema(l *ui.LiveSchema) {
	l.Field(&c.a, ui.Defer("a"))
	l.Field(&c.b, ui.Defer("b"))
	l.Loads("a", func(context.Context) error { c.a = []string{"x"}; return nil })
	l.Loads("b", func(context.Context) error {
		if c.bad {
			return shownErr{errors.New("b is down")}
		}
		c.b = []string{"y"}
		return nil
	})
}

func (c *pairCard) Render() ui.Node {
	return ui.Await{Of: c, Key: "a", And: []string{"b"}, Delay: ui.Instant, Pending: ui.HTML(`<i>loading</i>`),
		Error:    func(err error) ui.Node { return ui.HTML(`<b>` + err.Error() + `</b>`) },
		Children: func() ui.Node { return ui.HTML(`<p>` + c.a[0] + c.b[0] + `</p>`) }}
}

// A section over several groups is pending until the last has loaded, and
// shows the first failure.
func TestAwaitAnd(t *testing.T) {
	for _, bad := range []bool{false, true} {
		c := &pairCard{bad: bad}
		s := NewSession(context.Background(), func(ctx context.Context) ui.Node { return ui.Component("p", nil, c) })
		if html, err := s.Render(); err != nil || !strings.Contains(html, "loading") {
			t.Fatalf("%s, %v", html, err)
		}
		ds := s.Deferred()
		if len(ds) != 2 {
			t.Fatalf("%d loads", len(ds))
		}
		for i, d := range ds {
			d.Run(context.Background())
			if _, err := s.Loaded(d); err != nil {
				t.Fatal(err)
			}
			pending := strings.Contains(s.HTML(), "loading")
			if i == 0 && !pending {
				t.Fatalf("one group loaded: %s", s.HTML())
			}
		}
		want := "<p>xy</p>"
		if bad {
			want = "<b>b is down</b>"
		}
		if !strings.Contains(s.HTML(), want) {
			t.Fatalf("bad=%v: %s", bad, s.HTML())
		}
	}
}
