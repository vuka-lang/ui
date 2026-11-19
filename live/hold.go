package live

import (
	"context"
	"sync"
	"time"

	"github.com/vuka-lang/ui/v2"
)

// A page's first render, the HTTP one and the join's, holds for the loaders
// of the groups its sections ask to wait for (ui.Await.Wait): they start
// once the page rendered, and when they all return, or the longest Wait has
// passed, those that returned are applied and the page renders again, so a
// loader that was quick ships its content in the first reply and the
// fallback never goes on the wire. The others keep running, held for the
// transport (Deferred) as any started load is.

// holdRender waits, with the session locked, for the loads the render just
// done asked for, and reports whether it applied any: the page must render
// again. The lock is released while it waits.
func (s *Session) holdRender() bool {
	ds := s.startWaited()
	if len(ds) == 0 {
		return false
	}
	var budget time.Duration
	for _, d := range ds {
		budget = max(budget, d.Wait())
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(s.ctx))
	s.stops = append(s.stops, cancel)
	parent := s.ctx
	s.mu.Unlock()
	hold(ctx, parent, ds, budget)
	s.mu.Lock()
	if s.closed {
		return false
	}
	applied := false
	for _, d := range ds {
		select {
		case <-d.Done():
			applied = s.apply(d) || applied
		default:
			s.held = append(s.held, d)
		}
	}
	return applied
}

// hold runs ds with ctx and returns when they have all returned, budget has
// passed or parent is done.
func hold(ctx, parent context.Context, ds []*ui.DeferredLoad, budget time.Duration) {
	var wg sync.WaitGroup
	for _, d := range ds {
		wg.Go(func() { d.Run(ctx) })
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	t := time.NewTimer(budget)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	case <-parent.Done():
	}
}

// startWaited starts the waited groups of every instance not started yet,
// and returns them with the ones an earlier hold left running.
func (s *Session) startWaited() []*ui.DeferredLoad {
	ds := s.deferredLocked(ui.StartWaited, false)
	for _, d := range s.held {
		if _, ok := s.loading[d]; ok {
			ds = append(ds, d)
		}
	}
	s.held = nil
	return ds
}
