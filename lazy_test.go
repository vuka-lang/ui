package ui_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestLazyPartInsideSelectiveBoundary(t *testing.T) {
	request, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx, ps := ui.WithParts(request, ui.PartsOptions{Limit: 1})
	defer ps.Stop()
	ready := make(chan struct{})
	n := ui.Suspense(ui.Text("outer"), nil, func(ctx context.Context) ui.Node {
		return ui.Group{ui.Text("ready"), ui.LazyPart(ui.Text("inner"), nil, func(ctx context.Context) ui.Node {
			return ui.Text(ui.LazyValue(ctx, func(ctx context.Context) (string, error) {
				select {
				case <-ready:
					return "done", nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}))
		})}
	})
	renderIn(t, ctx, n)
	var buf bytes.Buffer
	var writes []string
	var once sync.Once
	if err := ps.Stream(&buf, func() error {
		text := buf.String()
		buf.Reset()
		writes = append(writes, text)
		if strings.Contains(text, "ready") {
			once.Do(func() { close(ready) })
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	writes = writes[1:]
	if len(writes) != 2 || !strings.Contains(writes[0], "ready") || !strings.Contains(writes[0], "inner") || !strings.Contains(writes[1], ">done<") {
		t.Fatalf("nested writes: %q", writes)
	}
}
