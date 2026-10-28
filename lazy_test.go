package ui_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

func TestLazyPartUntaken(t *testing.T) {
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{Limit: 1})
	defer ps.Stop()
	var calls atomic.Int32
	n := ui.LazyPart(ui.Text("loading"), nil, func(ctx context.Context) ui.Node {
		get := sync.OnceValue(func() int {
			return ui.LazyValue(ctx, func(context.Context) (int, error) { calls.Add(1); return 7, nil })
		})
		if false {
			return ui.Textf("%d", get())
		}
		return ui.Text("empty")
	})
	if got := renderIn(t, ctx, n); got != "empty" || calls.Load() != 0 || ps.Pending() {
		t.Fatalf("got %s; calls %d; pending %v", got, calls.Load(), ps.Pending())
	}
}

func TestLazyPartFirstUse(t *testing.T) {
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{Limit: 1})
	defer ps.Stop()
	var calls, ordinary atomic.Int32
	ready := make(chan struct{})
	n := ui.LazyPart(ui.Text("loading"), nil, func(ctx context.Context) ui.Node {
		ordinary.Add(1)
		get := sync.OnceValue(func() int {
			return ui.LazyValue(ctx, func(context.Context) (int, error) { calls.Add(1); <-ready; return 7, nil })
		})
		return ui.Textf("%d;%d", get(), get())
	})
	if got := renderIn(t, ctx, n); !strings.Contains(got, "loading") {
		t.Fatal(got)
	}
	close(ready)
	writes := stream(t, ps)
	if len(writes) != 1 || !strings.Contains(writes[0], "7;7") || calls.Load() != 1 || ordinary.Load() != 1 {
		t.Fatalf("writes %q; calls %d; ordinary %d", writes, calls.Load(), ordinary.Load())
	}
}

func TestLazyPartError(t *testing.T) {
	ctx, ps := ui.WithParts(context.Background(), ui.PartsOptions{Limit: 1})
	defer ps.Stop()
	n := ui.LazyPart(ui.PartFallback{Pending: ui.Text("loading"), Error: ui.Text("failed")}, nil, func(ctx context.Context) ui.Node {
		return ui.Text(ui.LazyValue(ctx, func(context.Context) (string, error) { return "", errors.New("down") }))
	})
	renderIn(t, ctx, n)
	if writes := stream(t, ps); len(writes) != 1 || !strings.Contains(writes[0], ">failed<") {
		t.Fatal(writes)
	}
}
