package ui

import (
	"context"
	"io"
	"strings"
	"sync"
)

// A lazy frame is a piece of JSX the compiler lowered with FT instead of F: it
// carries only the dynamics plus a way to rebuild the full node tree, and
// learns the frame's static HTML once, lazily. For a plain HTML render it then
// writes the statics straight through and renders only the dynamics, building
// no *Element/*Frame tree at all (the hot path that makes server-rendered
// pages cheap). Anything that needs the real tree — a live render, a non-HTML
// Renderer, RootAttrs, Walk by other code — gets it from build(), identical to
// what F would have produced, so HTML and the live protocol stay byte-for-byte
// the same as the F lowering.

// FT is a lazy Frame. Generated code calls it when the JSX target declares FT
// (ui does): build reconstructs the frame exactly as F would, from the
// dynamics; dyns are the dynamics in the order the shape lists them — a child
// expression, a block (ui.Nodes), a component tag, or an attribute value. The
// statics around them are derived once, by ui, from a sample tree build, so
// they match the live render tree the same source lowered with F.
func FT(fp uint64, shape string, build func([]any) Node, dyns ...any) Node {
	return &lazyFrame{fp: fp, shape: shape, build: build, dyns: dyns}
}

// lazyFrame is a piece of JSX lowered with FT, before its tree is built.
type lazyFrame struct {
	fp    uint64
	shape string
	build func([]any) Node
	dyns  []any
	built Node    // build(dyns), once materialized
	p     *ftPlan // a template's, compiled when it was parsed
}

func (n *lazyFrame) Render(ctx context.Context, w io.Writer) error {
	return renderHTML(ctx, w, n)
}

// frame is the lazy frame's real node tree, F would have built it: used by the
// live render-tree builder, a non-HTML Renderer, RootAttrs and the not-fast
// fallback. Built once per lazyFrame value.
// A template's frame is built each time instead, so a node Render returned
// renders concurrently without a race.
func (n *lazyFrame) frame() Node {
	if n.built != nil {
		return n.built
	}
	f := n.build(n.dyns)
	if n.p == nil {
		n.built = f
	}
	return f
}

// ftSlot is one dynamic of a lazy frame: an {expr}/block/component child, or an
// attribute value on an element of tag Tag.
type ftSlot struct {
	attr bool
	tag  string
	name string
}

// ftPlan is a lazy frame's statics and the kind of each dynamic, derived once
// from a sample tree build and shared by every frame with the same
// fingerprint. statics has one more entry than slots: the HTML is
// statics[0] + dyn[0] + statics[1] + … + statics[len(slots)]. fast is false
// when the fast HTML path can't render the frame on its own — a dynamic in a
// script or style element (the statics can't carry its content mode), a key
// attribute (data-vk-key depends on the render), or a tree that doesn't match
// its shape — so FT builds the tree and renders that instead, as F does.
type ftPlan struct {
	statics []string
	slots   []ftSlot
	fast    bool
	keyed   bool     // a key attribute: data-vk-key, which only a live render writes
	sites   []string // a template's slots' sites, where a block in one places its items
	sets    []bool   // a template's attribute set slots, a []Attr in one its attributes
}

// ftPlans caches one plan per fingerprint, like staticsCache: frames that share
// a fingerprint share their statics and so their plan.
var ftPlans sync.Map // uint64 → *ftPlan

func (n *lazyFrame) plan() *ftPlan {
	if n.p != nil {
		return n.p
	}
	if p, ok := ftPlans.Load(n.fp); ok {
		return p.(*ftPlan)
	}
	f, _ := n.frame().(*Frame)
	p := compilePlan(f, n.shape)
	actual, _ := ftPlans.LoadOrStore(n.fp, p)
	return actual.(*ftPlan)
}

func isTreeHost(h LiveHost) bool {
	_, ok := h.(TreeHost)
	return ok
}

// walk renders the lazy frame into r: the fast HTML path when r is an
// HTMLRenderer, no live session is building a tree, and the plan allows it;
// otherwise the real frame, walked as F's would be.
func (n *lazyFrame) walk(ctx context.Context, r Renderer) error {
	h, isHTML := r.(*HTMLRenderer)
	p := n.plan()
	if !isHTML || !p.fast || p.keyed && liveHost(ctx) != nil || isTreeHost(liveHost(ctx)) {
		return Walk(ctx, n.frame(), r)
	}
	return n.fastHTML(ctx, h, p)
}

// fastHTML writes the statics straight through and renders only the dynamics,
// building no tree. A frame reaches it only when every dynamic sits in HTML
// (not script or style) content, so a string child escapes and an attribute
// resolves exactly as the HTML renderer would in document context.
func (n *lazyFrame) fastHTML(ctx context.Context, h *HTMLRenderer, p *ftPlan) error {
	h.write(p.statics[0])
	for i, d := range n.dyns {
		s := p.slots[i]
		if set, ok := d.([]Attr); ok && s.attr && p.sets != nil && p.sets[i] {
			for _, a := range set {
				if err := h.attrNamed(s.tag, nil, a); err != nil {
					return err
				}
			}
		} else if s.attr {
			if err := h.attrNamed(s.tag, nil, Attr{Name: s.name, Value: d}); err != nil {
				return err
			}
		} else if ns, ok := d.([]Node); ok && p.sites != nil {
			// A template's block, walked as its list would be, without one.
			at := list{site: p.sites[i]}
			for _, c := range ns {
				if err := Walk(ctx, at.placed(c), h); err != nil {
					return err
				}
			}
		} else if err := h.child(ctx, d); err != nil {
			return err
		}
		if h.err != nil {
			return h.err
		}
		h.write(p.statics[i+1])
	}
	return h.err
}

// child renders a dynamic child value, as Child(v) walked into h would: a
// string escaped in document context (a fast frame has no dynamic elsewhere),
// anything else through Child and Walk.
func (h *HTMLRenderer) child(ctx context.Context, v any) error {
	if s, ok := v.(string); ok {
		h.writeEscaped(s)
		return h.err
	}
	return Walk(ctx, Child(v), h)
}

// compilePlan derives a frame's statics and slots from a sample tree, by
// writing its markup exactly as the HTML renderer does and noting where each
// dynamic falls. f is build(dyns); the dynamic values themselves are not
// rendered, only their positions and (for attributes) their element.
func compilePlan(f *Frame, shapeStr string) *ftPlan {
	notFast := &ftPlan{fast: false}
	if f == nil {
		return notFast
	}
	sh := shape{s: shapeStr}
	if !sh.fits(f.Kids, 0) || !sh.done() {
		return notFast
	}
	c := &ftCompiler{fast: true}
	c.h.w = &c.sb
	sh.i = 0
	if err := c.items(&sh, f.Kids); err != nil || c.h.err != nil {
		return notFast
	}
	c.statics = append(c.statics, c.sb.String())
	return &ftPlan{statics: c.statics, slots: c.slots, fast: c.fast, keyed: c.keyed}
}

// ftCompiler walks a sample frame once, writing its static markup into sb and
// recording a slot at every dynamic. It mirrors treeBuilder's static/dynamic
// split so the statics match the live render tree's, but it never renders a
// dynamic value.
type ftCompiler struct {
	h       HTMLRenderer // writes static markup into sb, unbuffered
	sb      strings.Builder
	statics []string
	slots   []ftSlot
	fast    bool
	keyed   bool
}

// dyn closes the current static piece and records a dynamic; a dynamic reached
// outside document content (a script or style element) can't be rendered on
// the fast path, since the statics don't carry the content mode.
func (c *ftCompiler) dyn(s ftSlot) {
	c.statics = append(c.statics, c.sb.String())
	c.sb.Reset()
	c.slots = append(c.slots, s)
	if c.h.mode != inHTML {
		c.fast = false
	}
	if s.attr && s.name == "key" {
		c.keyed = true
	}
}

func (c *ftCompiler) items(sh *shape, kids []Node) error {
	for _, n := range kids {
		switch sh.next() {
		case 't':
			if err := Walk(context.Background(), n, &c.h); err != nil {
				return err
			}
		case 'h', 'b', 'c':
			c.dyn(ftSlot{})
		case 'G':
			sh.next() // (
			if err := c.items(sh, n.(Group)); err != nil {
				return err
			}
			sh.next() // )
		case 'E':
			if err := c.elem(sh, n.(*Element)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *ftCompiler) elem(sh *shape, el *Element) error {
	flags := sh.flags()
	void := Void(el.Tag)
	if err := c.h.openTag(el); err != nil {
		return err
	}
	for i, a := range el.Attrs[:len(flags)] {
		if a.Name == "key" {
			c.keyed = true // data-vk-key depends on the render, not the statics
		}
		if flags[i] == 's' {
			if err := c.h.attr(el, a); err != nil {
				return err
			}
			continue
		}
		c.dyn(ftSlot{attr: true, tag: el.Tag, name: a.Name})
	}
	if err := c.h.endTag(el); err != nil {
		return err
	}
	sh.next() // (
	if err := c.items(sh, el.Children); err != nil {
		return err
	}
	sh.next() // )
	if void {
		return nil
	}
	return c.h.Close(el)
}
