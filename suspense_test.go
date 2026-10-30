package ui_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// suspensePage has deferred methods and one that isn't.
type suspensePage struct {
	plain atomic.Int32
	wait  chan struct{}
}

// Plain has no timing attribute.
func (p *suspensePage) Plain(ctx context.Context) (string, error) {
	p.plain.Add(1)
	if p.wait != nil {
		<-p.wait
	}
	return "plain", nil
}

func stream(t *testing.T, ps *ui.Parts) []string {
	t.Helper()
	var out bytes.Buffer
	var writes []string
	err := ps.Stream(&out, func() error {
		writes = append(writes, plainIDs(strings.TrimPrefix(out.String(), ui.PartsScript)))
		out.Reset()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return writes[1:] // after the script's
}

// Two deferred methods in a boundary reveal together: one slot, sent once
// both have returned.
func TestSuspenseRevealsTogether(t *testing.T) {
	p := &partPage{}
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	page := ui.Group{ui.Text("shell"), ui.Suspense(ui.Text("…"), nil, func(ctx context.Context) ui.Node {
		return ui.Group{
			ui.Part(p, (*partPage).Stats, nil, func(n int) ui.Node { return ui.Textf("%d;", n) }),
			ui.Part(p, (*partPage).B, nil, func(b string) ui.Node { return ui.Text(b) }),
		}
	})}
	if shell := renderIn(t, ctx, page); shell != `shell<!--vk-part:vk-p1-->…<!--/vk-part:vk-p1-->` {
		t.Fatalf("shell: %s", shell)
	}
	writes := stream(t, ps)
	if want := `<template data-vk-part="vk-p1">7;b</template><script>__vkP("vk-p1")</script>`; len(writes) != 1 || writes[0] != want {
		t.Errorf("got %q, want one write %s", writes, want)
	}
}

// Inside a boundary a method without a timing attribute runs after the
// shell, with the boundary; outside one it runs in place.
func TestSuspensePlainStreams(t *testing.T) {
	p := &suspensePage{wait: make(chan struct{})}
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	body := func(ctx context.Context) ui.Node { return ui.Text(ui.Must(p.Plain(ctx))) }
	shell := renderIn(t, ctx, ui.Suspense(nil, nil, body))
	if !strings.Contains(shell, `<span aria-busy="true"></span>`) || strings.Contains(shell, "plain") {
		t.Errorf("shell: %s", shell)
	}
	close(p.wait)
	if writes := stream(t, ps); len(writes) != 1 || !strings.Contains(writes[0], ">plain<") {
		t.Errorf("stream: %q", writes)
	}
	p = &suspensePage{}
	ctx, ps = ui.WithParts(context.Background(), ui.PartsOptions{})
	if got := renderIn(t, ctx, ui.Text(ui.Must(p.Plain(ctx)))); got != "plain" || ps.Pending() {
		t.Errorf("outside a boundary: %q, pending %v", got, ps.Pending())
	}
}

// Nested boundaries: the outer is sent first, the inner after it, though
// the inner returns first.
func TestSuspenseNested(t *testing.T) {
	p := &partPage{wait: make(chan struct{})}
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	innerDone := make(chan struct{})
	page := ui.Suspense(nil, nil, func(ctx context.Context) ui.Node {
		return ui.Group{
			ui.Suspense(nil, nil, func(ctx context.Context) ui.Node {
				defer close(innerDone)
				return ui.Text("inner")
			}),
			ui.Part(p, (*partPage).Stats, nil, func(n int) ui.Node { return ui.Textf("outer %d", n) }),
		}
	})
	renderIn(t, ctx, page)
	go func() {
		<-innerDone
		close(p.wait)
	}()
	writes := strings.Join(stream(t, ps), "")
	o, i := strings.Index(writes, `"vk-p1"><!--vk-part:vk-p1.1-->`), strings.Index(writes, `"vk-p1.1">inner<`)
	if o < 0 || i < 0 || i < o {
		t.Errorf("want the outer, then the inner: %s", writes)
	}
}

// A call the render never reaches doesn't run, and doesn't hold the
// boundary.
func TestSuspenseUntakenBranch(t *testing.T) {
	p := &suspensePage{wait: make(chan struct{})} // would block forever
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	more := false
	renderIn(t, ctx, ui.Suspense(nil, nil, func(ctx context.Context) ui.Node {
		if more {
			return ui.Text(ui.Must(p.Plain(ctx)))
		}
		return ui.Text("less")
	}))
	if writes := stream(t, ps); len(writes) != 1 || !strings.Contains(writes[0], ">less<") || p.plain.Load() != 0 {
		t.Errorf("stream %q, %d calls", writes, p.plain.Load())
	}
}

// A boundary's error is its own: its error slot, the rest of the page and
// another boundary unaffected; in place too.
func TestSuspenseErrorLocal(t *testing.T) {
	failing := func(ctx context.Context) ui.Node { panic(ui.Must(0, errors.New("down"))) }
	fine := func(ctx context.Context) ui.Node { return ui.Text("fine") }
	page := ui.Group{
		ui.Suspense(nil, ui.Text("no categories"), failing),
		ui.Suspense(nil, nil, fine),
		ui.Suspense(nil, nil, failing),
	}
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{})
	renderIn(t, ctx, page)
	writes := strings.Join(stream(t, ps), "")
	for _, want := range []string{`"vk-p1">no categories<`, `"vk-p2">fine<`, `"vk-p3"><span role="alert">`} {
		if !strings.Contains(writes, want) {
			t.Errorf("stream lacks %s: %s", want, writes)
		}
	}
	if got := renderIn(t, context.Background(), page); !strings.HasPrefix(got, "no categoriesfine<span role=\"alert\">") {
		t.Errorf("in place: %s", got)
	}
}
