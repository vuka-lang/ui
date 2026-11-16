package ui

import (
	"context"
	"io"
)

// A guarded part of a render is a block of a component's markup — a loop, a
// branch — that reads nothing but tracked state and props: the compiler
// writes it as Guard (a loop's items) or GuardNode (a branch's node), with
// the versions of the Assigns it reads and the values of the props, its
// deps. Under a live session building a render tree, a guarded part whose
// deps are what they were at the instance's last render keeps the frame it
// built then, without running the block; anywhere else it renders as the
// block would. In tests (or with VUKA_LIVE_VERIFY=1) the session renders
// it anyway and fails when the result differs.

// guarded is a guarded part: a block's items (nodes) or node.
type guarded struct {
	site  uint64
	deps  []any
	nodes func() []Node
	node  func() Node
	slot  string // the site of the template hole it fills, as a []Node's
}

// Guard is the items a loop of a component's render builds, guarded by
// deps: comparable values, the versions of the Assigns it reads and the
// props it reads. site identifies the loop in its source. Generated code
// calls it.
func Guard(site uint64, nodes func() []Node, deps ...any) Node {
	return &guarded{site: site, deps: deps, nodes: nodes}
}

// GuardNode is the node a branch of a component's render picks, guarded by
// deps, as Guard is. Generated code calls it.
func GuardNode(site uint64, node func() Node, deps ...any) Node {
	return &guarded{site: site, deps: deps, node: node}
}

func (g *guarded) Render(ctx context.Context, w io.Writer) error { return renderHTML(ctx, w, g) }

// View is what the block builds, as its value in a template's hole is: a
// loop's items as the hole's block, a branch's node as a child.
func (g *guarded) View() Node {
	if g.nodes != nil {
		return &list{site: g.slot, nodes: g.nodes()}
	}
	return Child(g.node())
}

// GuardHost is a TreeHost that keeps what guarded parts built. Package live
// implements it: render builds the part's value in the tree; Guarded returns
// it, or the value the part built at the instance's last render when it can
// stand for it.
type GuardHost interface {
	Guarded(ctx context.Context, site uint64, deps []any, render func() (any, error)) (any, error)
}
