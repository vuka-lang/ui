package ui

import (
	"context"
	"time"
)

// ResolveContext evaluates leading Ctx nodes with this render's context.
// Renderers use it before inspecting a root, preserving frame trees and root
// attributes. A live host can implement ContextRead() to prevent state-only
// reuse of output that depends on context. No context is retained in the node.
//
// Under such a host build gets the context through a readCtx, which tells
// the host what build read: a value a node of the same render set for the
// nodes it renders (a renderLocal key) is LocalContextRead, anything else —
// the request's values, the page's URL, cancellation — ContextRead.
func ResolveContext(ctx context.Context, n Node) Node {
	for {
		build, ok := n.(ctxNode)
		if !ok {
			return n
		}
		if build == nil {
			return nil
		}
		if host, ok := liveHost(ctx).(contextReader); ok {
			n = build(readCtx{ctx, host})
		} else {
			n = build(ctx)
		}
	}
}

type contextReader interface{ ContextRead() }

// renderLocal is a context key whose value only a node of a render sets,
// for the nodes it renders (an ActionMenu's menu, set by the row menu it is
// in), and never a request or a page: a component's render that reads it
// reads what its own render, from its props and state, set — or nothing,
// whatever renders around it — so the read doesn't stop a live host from
// keeping its last render. A key is renderLocal only if no code puts it on
// a context that outlives one render (TestRenderLocalKeysSetInRenderOnly).
type renderLocal interface{ renderLocal() }

// readCtx is a render's context as a Ctx build sees it, telling host what
// the build reads of it, whenever it reads it.
type readCtx struct {
	context.Context
	host contextReader
}

func (c readCtx) Value(key any) any {
	if _, ok := key.(renderLocal); ok {
		if h, ok := c.host.(interface{ LocalContextRead() }); ok {
			h.LocalContextRead()
		} else {
			c.host.ContextRead()
		}
	} else {
		c.host.ContextRead()
	}
	return c.Context.Value(key)
}

func (c readCtx) Deadline() (time.Time, bool) {
	c.host.ContextRead()
	return c.Context.Deadline()
}

func (c readCtx) Done() <-chan struct{} {
	c.host.ContextRead()
	return c.Context.Done()
}

func (c readCtx) Err() error {
	c.host.ContextRead()
	return c.Context.Err()
}
