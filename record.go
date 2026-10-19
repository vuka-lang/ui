package ui

import (
	"context"
	"hash/maphash"
	"io"
	"sync"
	"sync/atomic"

	templruntime "github.com/a-h/templ/runtime"
)

// A templ component compiled by the vuka command records itself into a live
// render tree the way JSX does: the markup its source fixes as statics,
// what its expressions write as dynamics, each loop as a block of item
// frames, and each branch and component call as a frame of its own. The
// generated code calls Record once per component (and once per children
// block), then writes its statics with S, ends each run of expression output
// with D, renders components with Render and brackets branches with
// Open/Close and loops with For/Item/End. Outside a live render — an HTTP
// page, a v1 session, a component rendered into a buffer of its own — the Rec
// is the zero value, and the code writes what templ's own would: the same
// bytes.
//
// A recorded frame's statics are identified by their text: two frames whose
// statics are the same strings share a Key, so the same layout keeps its
// frames from one page to the next.

// Rec is what generated templ code records a render through. Its zero value
// only writes. Not for hand-written code.
type Rec struct{ r *recorder }

// recorders counts the renders recording now, so a Record outside one doesn't
// look in the context.
var recorders atomic.Int64

type recorderKey struct{}

// Record is the recorder of the live render ctx belongs to, when w is the
// writer it records: the component renders into a frame of the tree. Else
// the zero Rec. Generated code calls it.
func Record(ctx context.Context, w io.Writer) Rec {
	if recorders.Load() == 0 {
		return Rec{}
	}
	return record(ctx, w)
}

func record(ctx context.Context, w io.Writer) Rec {
	r, _ := ctx.Value(recorderKey{}).(*recorder)
	if r == nil || w != io.Writer(r.w) {
		return Rec{}
	}
	r.on = true
	return Rec{r}
}

// The methods below keep the zero Rec's path short enough to inline: outside
// a live render, generated code costs about what templ's own does.

// S writes s, the n-th static of its file: a static of the frame.
func (c Rec) S(w io.Writer, n int, s string) error {
	if c.r != nil {
		c.r.static(s)
		return nil
	}
	return templruntime.WriteString(w, n, s)
}

// D ends a run of expression output: what was written since the last static
// or frame is a dynamic, empty if nothing was.
func (c Rec) D() {
	if c.r != nil {
		c.r.d()
	}
}

// Render renders n into w, as n.Render(ctx, w) does; recording, n is a
// dynamic of the frame, as a JSX child is: a frame of its own when it has
// one (JSX, a recorded templ component), a stateful component's instance,
// else its HTML.
func (c Rec) Render(n Node, ctx context.Context, w io.Writer) error {
	if c.r != nil {
		return c.r.render(ctx, n)
	}
	return n.Render(ctx, w)
}

// Open starts a branch's frame (an if or switch); Close ends it, a dynamic of
// the frame around it.
func (c Rec) Open() {
	if c.r != nil {
		c.r.open(false)
	}
}

func (c Rec) Close() {
	if c.r != nil {
		c.r.close()
	}
}

// For starts a loop, Item each of its iterations, End the loop: a block of
// one frame per iteration, a dynamic of the frame around it.
func (c Rec) For() {
	if c.r != nil {
		c.r.open(true)
	}
}

func (c Rec) Item() {
	if c.r != nil {
		c.r.item()
	}
}

func (c Rec) End() {
	if c.r != nil {
		c.r.end()
	}
}

//go:noinline
func (r *recorder) d() { r.top().dynamic(r.take()) }

//go:noinline
func (r *recorder) static(s string) {
	r.flush()
	r.top().static(s)
}

//go:noinline
func (r *recorder) render(ctx context.Context, n Node) error {
	r.flush()
	v, err := r.b.value(ctx, n)
	if err != nil {
		return err
	}
	r.top().dynamic(v)
	return nil
}

//go:noinline
func (r *recorder) open(list bool) {
	r.flush()
	r.push(list)
}

func (r *recorder) close() {
	if len(r.stack) < 2 || r.top().list {
		return
	}
	r.flush()
	f := r.pop()
	r.top().dynamic(f.tree())
}

func (r *recorder) item() {
	r.endItem()
	if r.top().list {
		r.push(false)
	}
}

func (r *recorder) end() {
	r.endItem()
	if len(r.stack) < 2 || !r.top().list {
		return
	}
	l := r.pop()
	r.top().dynamic(&TreeList{Items: l.items})
}

// recorder builds the tree of one opaque node's render: generated code
// writes its dynamics into w, and tells it where the rest goes.
type recorder struct {
	b     *treeBuilder
	out   byteBuf
	w     *templruntime.Buffer
	on    bool // a component recorded into it
	stack []*recFrame
}

// recFrame is a frame, or a loop (list), being recorded.
type recFrame struct {
	list    bool
	statics []string
	dyn     []any
	last    byte // what was added last: 0 nothing, 's' a static, 'd' a dynamic
	items   []*Tree
}

func (b *byteBuf) Write(p []byte) (int, error) { *b = append(*b, p...); return len(p), nil }

var recorderPool = sync.Pool{New: func() any { return new(recorder) }}

func newRecorder(b *treeBuilder) *recorder {
	r := recorderPool.Get().(*recorder)
	r.b, r.on = b, false
	r.out = r.out[:0]
	r.w, _ = templruntime.GetBuffer(&r.out)
	r.stack = append(r.stack[:0], &recFrame{})
	return r
}

func (r *recorder) release() {
	_ = templruntime.ReleaseBuffer(r.w)
	r.w, r.b = nil, nil
	clear(r.stack)
	r.stack = r.stack[:0]
	if cap(r.out) > 64<<10 {
		r.out = nil
	}
	recorderPool.Put(r)
}

func (r *recorder) top() *recFrame { return r.stack[len(r.stack)-1] }

func (r *recorder) push(list bool) { r.stack = append(r.stack, &recFrame{list: list}) }

func (r *recorder) pop() *recFrame {
	f := r.top()
	r.stack = r.stack[:len(r.stack)-1]
	return f
}

// take is what was written since the last take.
func (r *recorder) take() string {
	_ = r.w.Flush()
	if len(r.out) == 0 {
		return ""
	}
	s := string(r.out)
	r.out = r.out[:0]
	return s
}

// flush makes what was written outside the generated code's own statements
// — by a hand-written component around a recorded one — a dynamic.
func (r *recorder) flush() {
	if s := r.take(); s != "" {
		r.top().dynamic(s)
	}
}

// endItem closes the open item of the innermost loop, if one is open.
func (r *recorder) endItem() {
	if len(r.stack) < 2 || r.top().list || !r.stack[len(r.stack)-2].list {
		return
	}
	r.flush()
	f := r.pop()
	l := r.top()
	l.items = append(l.items, f.tree())
}

// result is the render's value as a dynamic: its HTML when no component
// recorded into it, else its frame.
func (r *recorder) result() any {
	r.flush()
	for len(r.stack) > 1 { // left open by a failed render; not reached on success
		f := r.pop()
		if f.list {
			r.top().dynamic(&TreeList{Items: f.items})
		} else {
			r.top().dynamic(f.tree())
		}
	}
	root := r.stack[0]
	if !r.on {
		switch len(root.dyn) {
		case 0:
			return ""
		case 1:
			return root.dyn[0]
		}
	}
	t := root.tree()
	if len(t.Dyn) == 1 && t.Key == opaqueKey(1) {
		return t.Dyn[0]
	}
	return t
}

func (f *recFrame) static(s string) {
	if f.last == 's' {
		f.statics[len(f.statics)-1] += s
	} else {
		f.statics = append(f.statics, s)
	}
	f.last = 's'
}

func (f *recFrame) dynamic(v any) {
	if f.last != 's' {
		f.statics = append(f.statics, "")
	}
	f.dyn = append(f.dyn, v)
	f.last = 'd'
}

// tree is the frame as a Tree. One whose statics are all empty is a frame of
// its dynamics alone, as OpaqueTree is — or, around a single frame, that
// frame.
func (f *recFrame) tree() *Tree {
	if f.last != 's' {
		f.statics = append(f.statics, "")
	}
	if allEmpty(f.statics) {
		if len(f.dyn) == 1 {
			if t, ok := f.dyn[0].(*Tree); ok {
				return t
			}
		}
		return OpaqueTree(f.dyn...)
	}
	key := staticsKey(f.statics)
	if s, ok := recStatics.Load(key); ok {
		return &Tree{Key: key, Statics: s.([]string), Dyn: f.dyn}
	}
	s, _ := recStatics.LoadOrStore(key, f.statics)
	return &Tree{Key: key, Statics: s.([]string), Dyn: f.dyn}
}

func allEmpty(statics []string) bool {
	for _, s := range statics {
		if s != "" {
			return false
		}
	}
	return true
}

var (
	recStatics sync.Map // key → []string: the statics of recorded frames
	recSeed    = maphash.MakeSeed()
)

// staticsKey identifies statics by their text.
func staticsKey(statics []string) uint64 {
	var h maphash.Hash
	h.SetSeed(recSeed)
	var n [8]byte
	for _, s := range statics {
		l := uint64(len(s))
		for i := range n {
			n[i] = byte(l >> (8 * i))
		}
		h.Write(n[:])
		h.WriteString(s)
	}
	return mix(h.Sum64() ^ 0x7465_6d70_6c00_0000) // "templ"
}

// record renders an opaque node, recording what generated templ code in it
// records.
func (b *treeBuilder) record(ctx context.Context, n Node) (any, error) {
	r := newRecorder(b)
	defer r.release()
	recorders.Add(1)
	err := n.Render(context.WithValue(ctx, recorderKey{}, r), r.w)
	recorders.Add(-1)
	if err != nil {
		return nil, err
	}
	return r.result(), nil
}

// opaque reports whether n is a Node Walk can't see into: a templ
// component, a NodeFunc.
func opaque(n Node) bool {
	switch n.(type) {
	case *Element, TextNode, RawHTML, Group, Builder, TryNode, Boundary, *Frame, *lazyFrame, *ComponentNode, *islandMount, *list, ctxNode, viewer:
		return false
	}
	return true
}
