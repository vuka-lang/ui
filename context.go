package ui

import "context"

// ResolveContext evaluates leading Ctx nodes with this render's context.
// Renderers use it before inspecting a root, preserving frame trees and root
// attributes. A live host can implement ContextRead() to prevent state-only
// reuse of output that depends on context. No context is retained in the node.
func ResolveContext(ctx context.Context, n Node) Node {
	for {
		build, ok := n.(ctxNode)
		if !ok {
			return n
		}
		if host, ok := liveHost(ctx).(interface{ ContextRead() }); ok {
			host.ContextRead()
		}
		if build == nil {
			return nil
		}
		n = build(ctx)
	}
}
