package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Renderer receives a node tree in document order; Walk drives it. The HTML
// renderer behind every Node's Render is one; package term is another.
type Renderer interface {
	Open(e *Element) error  // before children
	Close(e *Element) error // after children (not called for void elements)
	Text(s string) error
	Raw(html string) error                    // Safe content
	Opaque(ctx context.Context, n Node) error // a component that only renders HTML (templ, NodeFunc)
}

// Walk sends n's tree to r. Frames, Groups, Builders and Child conversions are
// flattened (r never sees them); a TryNode with an error stops the walk with
// it; a Boundary walks its children into a recording first, and replays it
// into r — or, when they fail, walks Fallback(err) instead, so r never sees
// the failed part. Inside a Boundary an opaque Node is rendered while
// recording and replayed as Opaque of its HTML (a RawHTML). Children on a void
// element are an error. The first error from r stops the walk.
func Walk(ctx context.Context, n Node, r Renderer) error {
	switch n := n.(type) {
	case nil:
		return nil
	case *Element:
		if n == nil {
			return nil
		}
		void := Void(n.Tag)
		if void && len(n.Children) > 0 {
			return fmt.Errorf("ui: <%s> is a void element and can't have children", n.Tag)
		}
		if k, ok := elementKey(n); ok {
			if h := liveHost(ctx); h != nil {
				keyed := h.Keyed(ctx, k)
				return inScope(keyed, r, func() error { return walkElement(keyed, n, r, void) })
			}
		}
		return walkElement(ctx, n, r, void)
	case *ComponentNode:
		if n == nil {
			return nil
		}
		if inStreamRow(ctx) != nil {
			return rowComponentErr(n)
		}
		if liveHost(ctx) != nil {
			break
		}
		c, err := n.static(ctx)
		if err != nil || c == nil {
			return err
		}
		return Walk(ctx, c.Render(), r)
	case *islandMount:
		return Walk(ctx, n.node(""), r)
	case *StreamBlock:
		if n == nil {
			return nil
		}
		return walkStream(ctx, n, r)
	case *list:
		if n.items == nil {
			for _, c := range n.nodes {
				if err := Walk(ctx, n.placed(c), r); err != nil {
					return err
				}
			}
			return nil
		}
		var err error
		n.items(func(c Node) {
			if err == nil {
				err = Walk(ctx, n.placed(c), r)
			}
		})
		return err
	case TextNode:
		return r.Text(string(n))
	case Binding:
		return Walk(ctx, n.node(), r)
	case RawHTML:
		return r.Raw(string(n))
	case *Frame:
		if n == nil {
			return nil
		}
		for _, c := range n.Kids {
			if err := Walk(ctx, c, r); err != nil {
				return err
			}
		}
		return nil
	case *lazyFrame:
		if n == nil {
			return nil
		}
		return n.walk(ctx, r)
	case Group:
		for _, c := range n {
			if err := Walk(ctx, c, r); err != nil {
				return err
			}
		}
		return nil
	case Builder:
		if n == nil {
			return nil
		}
		var err error
		n(func(c Node) {
			if err == nil {
				err = Walk(ctx, c, r)
			}
		})
		return err
	case TryNode:
		if n.Err != nil {
			return n.Err
		}
		return Walk(ctx, n.Node, r)
	case ctxNode:
		if n == nil {
			return nil
		}
		return Walk(ctx, ResolveContext(ctx, n), r)
	case keyedNode:
		ctx := n.in(ctx)
		if n.with != nil {
			return Walk(ctx, n.n, r)
		}
		return inScope(ctx, r, func() error { return Walk(ctx, n.n, r) })
	case Boundary:
		var rec recording
		for _, c := range n.Children {
			if err := Walk(ctx, c, &rec); err != nil {
				if n.Fallback == nil {
					return nil
				}
				return Walk(ctx, n.Fallback(err), r)
			}
		}
		return rec.replay(ctx, r)
	}
	if v, ok := n.(viewer); ok {
		return Walk(ctx, v.View(), r)
	}
	return r.Opaque(ctx, n)
}

// walkElement sends an element and what it holds to r.
func walkElement(ctx context.Context, n *Element, r Renderer, void bool) error {
	if _, html := r.(*HTMLRenderer); !html {
		n = withSets(n)
	}
	if err := r.Open(n); err != nil || void {
		return err
	}
	for _, c := range n.Children {
		if err := Walk(ctx, c, r); err != nil {
			return err
		}
	}
	return r.Close(n)
}

// A scopedRenderer writes attributes in a context of its own: a key's scope
// is entered here, so that the handlers of the keyed element's attributes
// and of those inside it are named by the key.
type scopedRenderer interface {
	swapScope(ctx context.Context) (prev context.Context)
}

func (h *HTMLRenderer) swapScope(ctx context.Context) context.Context {
	prev := h.ctx
	h.ctx = ctx
	return prev
}

// inScope runs walk with r's attributes in ctx, a key's scope in a live
// session. A page's probe names no handlers, and its context stays.
func inScope(ctx context.Context, r Renderer, walk func() error) error {
	s, ok := r.(scopedRenderer)
	if !ok {
		return walk()
	}
	switch liveHost(ctx).(type) {
	case nil, *pageProbe:
		return walk()
	}
	prev := s.swapScope(ctx)
	err := walk()
	s.swapScope(prev)
	return err
}

type event struct {
	kind uint8
	e    *Element
	s    string
	ctx  context.Context // evScope's
}

const (
	evOpen uint8 = iota
	evClose
	evText
	evRaw
	evOpaque
	evScope
)

// recording is a Renderer keeping what it receives, for a Boundary.
type recording struct {
	events []event
	scope  context.Context // the scope last entered; nil: the one it started in
}

func (r *recording) add(ev event) error { r.events = append(r.events, ev); return nil }

func (r *recording) Open(e *Element) error  { return r.add(event{kind: evOpen, e: e}) }
func (r *recording) Close(e *Element) error { return r.add(event{kind: evClose, e: e}) }
func (r *recording) Text(s string) error    { return r.add(event{kind: evText, s: s}) }
func (r *recording) Raw(s string) error     { return r.add(event{kind: evRaw, s: s}) }

func (r *recording) Opaque(ctx context.Context, n Node) error {
	var b strings.Builder
	if err := n.Render(ctx, &b); err != nil {
		return err
	}
	return r.add(event{kind: evOpaque, s: b.String()})
}

// swapScope keeps the scope the elements after it were walked in, for the
// replay, which writes their attributes.
func (r *recording) swapScope(ctx context.Context) context.Context {
	prev := r.scope
	r.scope = ctx
	r.add(event{kind: evScope, ctx: ctx})
	return prev
}

func (r *recording) replay(ctx context.Context, to Renderer) error {
	var base context.Context // to's scope before the first the recording entered
	entered := false
	for _, ev := range r.events {
		var err error
		switch ev.kind {
		case evOpen:
			err = to.Open(ev.e)
		case evClose:
			err = to.Close(ev.e)
		case evText:
			err = to.Text(ev.s)
		case evRaw:
			err = to.Raw(ev.s)
		case evOpaque:
			err = to.Opaque(ctx, RawHTML(ev.s))
		case evScope:
			if s, ok := to.(scopedRenderer); ok {
				scope := ev.ctx
				if scope == nil {
					scope = base
				}
				if prev := s.swapScope(scope); !entered {
					base, entered = prev, true
				}
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// withSets is e with the attributes of its attribute sets (a template's
// <input {attrs...}>) in their place, for a Renderer that isn't HTML's.
func withSets(e *Element) *Element {
	i := slices.IndexFunc(e.Attrs, func(a Attr) bool { _, ok := a.Value.(attrSet); return ok })
	if i < 0 {
		return e
	}
	cp := *e
	cp.Attrs = slices.Clone(e.Attrs[:i])
	for _, a := range e.Attrs[i:] {
		if set, ok := a.Value.(attrSet); ok {
			cp.Attrs = append(cp.Attrs, set...)
		} else {
			cp.Attrs = append(cp.Attrs, a)
		}
	}
	return &cp
}
