package ui

import (
	"bytes"
	"context"
	"sync"
)

type lazyKey struct{}
type lazyRender struct {
	first chan struct{}
	once  sync.Once
}

// LazyValue evaluates a deferred binding at its first rendered use. The
// compiler puts it inside a sync.OnceValue so later uses share its result.
func LazyValue[T any](ctx context.Context, run func(context.Context) (T, error)) T {
	state, _ := ctx.Value(lazyKey{}).(*lazyRender)
	ps := partsOf(ctx)
	if state == nil || ps == nil {
		return Must(run(ctx))
	}
	state.once.Do(func() { close(state.first) })
	if ps.o.Pending {
		<-ps.ctx.Done()
		panic(ps.ctx.Err())
	}
	select {
	case ps.sem <- struct{}{}:
		defer func() { <-ps.sem }()
	case <-ps.ctx.Done():
		panic(ps.ctx.Err())
	}
	return Must(run(ctx))
}

// LazyPart renders inline until a deferred binding is read. At that point
// the element becomes a part, its render continuing in the background.
// A render reaching no binding stays inline and starts no deferred work.
func LazyPart(fallback, failed Node, body func(context.Context) Node, group ...string) Node {
	if f, ok := fallback.(PartFallback); ok {
		fallback, failed = f.Pending, f.Error
	}
	return ctxNode(func(ctx context.Context) Node {
		ps := partsOf(ctx)
		if ps == nil {
			node, err := callBody(ctx, body)
			if err != nil {
				return partError(err, failed, nil)
			}
			return node
		}
		state := &lazyRender{first: make(chan struct{})}
		cctx, stop := context.WithCancel(context.WithValue(ctx, lazyKey{}, state))
		unhook := context.AfterFunc(ps.ctx, stop)
		done := make(chan struct{})
		var buf bytes.Buffer
		var err error
		go func() {
			defer close(done)
			defer stop()
			defer unhook()
			_, err = callPart(cctx, func(ctx context.Context) (any, error) {
				n := body(ctx)
				_, err := RenderPage(ctx, n, &buf)
				return nil, err
			})
		}()
		select {
		case <-state.first:
		case <-done:
		}
		select {
		case <-state.first:
			run := func(context.Context) (any, error) { <-done; return buf.Bytes(), err }
			show := func(v any) Node { return RawHTML(v.([]byte)) }
			m := partMethod{name: "Binding", deferred: true, independent: true, async: true}
			if len(group) > 0 {
				m.group = group[0]
			}
			return resolvePart(ctx, partKey{recv: new(byte)}, m, run, PartFallback{Pending: fallback, Error: failed}, show)
		default:
			if err != nil {
				return partError(err, failed, nil)
			}
			return RawHTML(buf.Bytes())
		}
	})
}
