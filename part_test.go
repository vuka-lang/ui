package ui_test

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2"
)

type partPage struct {
	calls atomic.Int32
	wait  chan struct{}
	fail  bool
}

func (p *partPage) Stats(ctx context.Context) (int, error) {
	p.calls.Add(1)
	if p.wait != nil {
		select {
		case <-p.wait:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if p.fail {
		return 0, errors.New("stats: down")
	}
	return 7, nil
}

func (p *partPage) A(ctx context.Context) (string, error) { return "a", nil }
func (p *partPage) B(ctx context.Context) (string, error) {
	time.Sleep(20 * time.Millisecond)
	return "b", nil
}
func (p *partPage) Now(ctx context.Context) (string, error) { return "now", nil }

func init() {
	vuka.RegisterFunc((*partPage).Stats, ui.Defer(""))
	vuka.RegisterFunc((*partPage).A, ui.Defer("g"))
	vuka.RegisterFunc((*partPage).B, ui.Defer("g"))
}

func (p *partPage) View() ui.Node {
	return ui.Group{
		ui.El("h1", nil, ui.Text("Title")),
		ui.Part(p, (*partPage).Stats, ui.PartFallback{Pending: ui.Text("…"), Error: ui.Text("no stats")}, func(n int) ui.Node {
			return ui.El("p", nil, ui.Textf("%d open", n))
		}),
		ui.Part(p, (*partPage).Stats, nil, func(n int) ui.Node { return ui.El("b", nil, ui.Text(n)) }),
	}
}

func renderIn(t *testing.T, ctx context.Context, n ui.Node) string {
	t.Helper()
	var b bytes.Buffer
	if err := n.Render(ctx, &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestPartInPlace(t *testing.T) {
	p := &partPage{}
	got := renderIn(t, context.Background(), p.View())
	if want := "<h1>Title</h1><p>7 open</p><b>7</b>"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	p.fail = true
	got = renderIn(t, context.Background(), p.View())
	if want := `<h1>Title</h1>no stats<span role="alert">This part couldn&#39;t load.</span>`; got != want {
		t.Errorf("failing: got %s, want %s", got, want)
	}
	// a method without a timing attribute runs in place even where parts stream
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	got = renderIn(t, ctx, ui.Part(p, (*partPage).Now, nil, func(s string) ui.Node { return ui.Text(s) }))
	if got != "now" || ps.Pending() {
		t.Errorf("an undeferred part: %q, pending %v", got, ps.Pending())
	}
}

var slotRE = regexp.MustCompile(`<!--vk-part:(vk-p\d+)-->(.*?)<!--/vk-part:vk-p\d+-->`)

func TestPartStream(t *testing.T) {
	p := &partPage{wait: make(chan struct{})}
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	shell := renderIn(t, ctx, p.View())
	if want := `<h1>Title</h1><!--vk-part:vk-p1-->…<!--/vk-part:vk-p1--><!--vk-part:vk-p2--><span aria-busy="true"></span><!--/vk-part:vk-p2-->`; shell != want {
		t.Fatalf("shell: got %s, want %s", shell, want)
	}
	if !ps.Pending() {
		t.Fatal("the shell has no parts")
	}
	var out bytes.Buffer
	var flushed []string
	done := make(chan error)
	first := make(chan struct{})
	go func() {
		done <- ps.Stream(&out, func() error {
			if flushed = append(flushed, out.String()); len(flushed) == 1 {
				close(first)
			}
			return nil
		})
	}()
	<-first // the script is sent before the method returns
	close(p.wait)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 1 {
		t.Errorf("Stats called %d times, want once for its two parts", p.calls.Load())
	}
	s := out.String()
	if !strings.HasPrefix(s, ui.PartsScript) {
		t.Errorf("the stream doesn't start with the parts script: %s", s)
	}
	s = strings.TrimPrefix(s, ui.PartsScript)
	if want := `<template data-vk-part="vk-p1"><p>7 open</p></template><script>__vkP("vk-p1")</script><template data-vk-part="vk-p2"><b>7</b></template><script>__vkP("vk-p2")</script>`; s != want {
		t.Errorf("parts: got %s, want %s", s, want)
	}
	if len(flushed) != 2 || flushed[0] != ui.PartsScript {
		t.Errorf("flushed %q, want the script, then the parts", flushed)
	}
}

func TestPartStreamError(t *testing.T) {
	p := &partPage{fail: true}
	var failed []string
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{
		ErrorText: func(err error) string { return "down: " + err.Error() },
		Written: func(name string, err error) {
			if err != nil {
				failed = append(failed, name+": "+err.Error())
			}
		},
	})
	renderIn(t, ctx, p.View())
	var out bytes.Buffer
	err := ps.Stream(&out, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimPrefix(out.String(), ui.PartsScript)
	if want := `<template data-vk-part="vk-p1">no stats</template><script>__vkP("vk-p1")</script><template data-vk-part="vk-p2"><span role="alert">down: stats: down</span></template><script>__vkP("vk-p2")</script>`; s != want {
		t.Errorf("got %s, want %s", s, want)
	}
	if want := "partPage.Stats: stats: down"; len(failed) != 2 || failed[0] != want {
		t.Errorf("Written: %q", failed)
	}
}

func TestPartGroupAndNesting(t *testing.T) {
	p := &partPage{}
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	page := ui.Group{
		ui.Part(p, (*partPage).A, nil, func(a string) ui.Node {
			// a part inside a part: B's, of A's group, arrives with it
			return ui.El("div", nil, ui.Text(a), ui.Part(p, (*partPage).B, nil, func(b string) ui.Node { return ui.Text(b) }))
		}),
		ui.Part(p, (*partPage).B, nil, func(b string) ui.Node { return ui.El("i", nil, ui.Text(b)) }),
	}
	renderIn(t, ctx, page)
	var out bytes.Buffer
	var writes []string
	err := ps.Stream(&out, func() error {
		writes = append(writes, out.String())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 2 {
		t.Errorf("a group's parts came in %d flushes after the script's, want 1: %q", len(writes)-1, writes)
	}
	s := strings.TrimPrefix(out.String(), ui.PartsScript)
	if want := `<template data-vk-part="vk-p1"><div>ab</div></template><script>__vkP("vk-p1")</script><template data-vk-part="vk-p2"><i>b</i></template><script>__vkP("vk-p2")</script>`; s != want {
		t.Errorf("got %s, want %s", s, want)
	}
}

func TestPartPerValue(t *testing.T) {
	a, b := &partPage{}, &partPage{}
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	show := func(n int) ui.Node { return ui.Text(n) }
	renderIn(t, ctx, ui.Group{ui.Part(a, (*partPage).Stats, nil, show), ui.Part(b, (*partPage).Stats, nil, show), ui.Part(a, (*partPage).Stats, nil, show)})
	if err := ps.Stream(&bytes.Buffer{}, nil); err != nil {
		t.Fatal(err)
	}
	if a.calls.Load() != 1 || b.calls.Load() != 1 {
		t.Errorf("calls: a %d, b %d; want one each", a.calls.Load(), b.calls.Load())
	}
}

func TestPartStreamCanceled(t *testing.T) {
	p := &partPage{wait: make(chan struct{})}
	rctx, cancel := context.WithCancel(context.Background())
	ctx, ps := ui.WithParts(rctx, ui.PartsOptions{})
	renderIn(t, ctx, p.View())
	done := make(chan error)
	go func() { done <- ps.Stream(&bytes.Buffer{}, nil) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Stream: %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stream didn't return when its context was canceled")
	}
}

// A method starts as the render meets its part, before the stream.
func TestPartStartsAtOnce(t *testing.T) {
	p := &partPage{wait: make(chan struct{})}
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	renderIn(t, ctx, p.View())
	for i := 0; i < 500 && p.calls.Load() == 0; i++ {
		time.Sleep(time.Millisecond)
	}
	if p.calls.Load() != 1 {
		t.Fatalf("Stats called %d times before the stream, want 1", p.calls.Load())
	}
	close(p.wait)
	if err := ps.Stream(&bytes.Buffer{}, nil); err != nil {
		t.Fatal(err)
	}
	pending := &partPage{}
	ctx, ps = ui.WithParts(context.Background(), ui.PartsOptions{Pending: true})
	renderIn(t, ctx, pending.View())
	time.Sleep(20 * time.Millisecond)
	if pending.calls.Load() != 0 || !ps.Pending() {
		t.Errorf("Pending: %d calls", pending.calls.Load())
	}
	ps.Stop()
}

// valuePage has a method of a value receiver: equal values share a call.
type valuePage struct{ id int }

var valueCalls atomic.Int32

func (v valuePage) Count(ctx context.Context) (int, error) { valueCalls.Add(1); return v.id, nil }

func init() { vuka.RegisterFunc(valuePage.Count, ui.Defer("")) }

func TestPartValueReceiver(t *testing.T) {
	valueCalls.Store(0)
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	show := func(n int) ui.Node { return ui.Text(n) }
	a, b := valuePage{1}, valuePage{2}
	renderIn(t, ctx, ui.Group{ui.Part(a, valuePage.Count, nil, show), ui.Part(b, valuePage.Count, nil, show), ui.Part(a, valuePage.Count, nil, show)})
	var out bytes.Buffer
	if err := ps.Stream(&out, nil); err != nil {
		t.Fatal(err)
	}
	if valueCalls.Load() != 2 {
		t.Errorf("Count called %d times, want once per value", valueCalls.Load())
	}
	if s := strings.TrimPrefix(out.String(), ui.PartsScript); !strings.Contains(s, `"vk-p1">1<`) || !strings.Contains(s, `"vk-p2">2<`) || !strings.Contains(s, `"vk-p3">1<`) {
		t.Errorf("parts: %s", s)
	}
}
