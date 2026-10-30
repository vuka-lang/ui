package ui

import (
	"bytes"
	"context"
)

// Suspense is a boundary of a stateless page: body, the markup of a
// <ui.Suspense>, renders after the page's first bytes, every call it makes
// with it, and is sent at once when all of it has returned — one reveal,
// not piece by piece. fallback shows until then; failed when the body
// fails (a panic of Must, a deferred method's error), else a short message
// with role="alert". A nil fallback is an empty <span aria-busy="true">.
//
//	ui.Suspense(ui.Skeleton{Lines: 3}, ui.Text("Couldn't load categories."), func(ctx context.Context) ui.Node {
//		categories := ui.Must(p.GetCategories(ctx))
//		return ui.El("h3", nil, ui.Textf("%d categories", len(categories)))
//	})
//
// It is a Part whose method is its body. Inside it a deferred method runs
// where its part is, held by the boundary's reveal; a boundary inside is
// a part of its own, sent after the outer one. Where parts don't stream
// (a live page, a fragment, a test), the body renders in place, failed in
// its place when it fails.
func Suspense(fallback, failed Node, body func(ctx context.Context) Node) Node {
	if body == nil {
		panic("ui: Suspense needs a body")
	}
	m := partMethod{name: "Suspense", deferred: true, boundary: true}
	key := partKey{recv: new(byte)} // shared with no other part
	run := func(ctx context.Context) (any, error) {
		var buf bytes.Buffer
		n, err := callBody(ctx, body)
		if err != nil {
			return nil, err
		}
		info, err := RenderPage(ctx, n, &buf)
		if err == nil && info.Live {
			err = errLivePart
		}
		return buf.Bytes(), err
	}
	show := func(v any) Node { return RawHTML(v.([]byte)) }
	return ctxNode(func(ctx context.Context) Node {
		if partsOf(ctx) == nil {
			n, err := callBody(ctx, body)
			if err != nil {
				return partError(err, failed, nil)
			}
			return n
		}
		return resolvePart(ctx, key, m, run, PartFallback{Pending: fallback, Error: failed}, show)
	})
}

// callBody builds a boundary's body, a panic its error.
func callBody(ctx context.Context, body func(context.Context) Node) (Node, error) {
	v, err := callPart(ctx, func(ctx context.Context) (any, error) { return body(ctx), nil })
	if err != nil {
		return nil, err
	}
	n, _ := v.(Node)
	return n, nil
}

type boundaryKey struct{}

// boundaryOf is the boundary whose body ctx renders, or nil.
func boundaryOf(ctx context.Context) *partCall {
	c, _ := ctx.Value(boundaryKey{}).(*partCall)
	return c
}
