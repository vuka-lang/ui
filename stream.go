package ui

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	"github.com/vuka-lang/vuka/v2/diag"
)

// Stream is a list a live page sends and then forgets: an unexported field
// of a live shape (messages ui.Stream[Message]) whose methods queue
// operations — insert these rows, delete that key, start over — that the
// next reply carries. Once the reply is written the session drops them,
// and the browser keeps the rows, applying each batch of operations once,
// by key. Between replies a stream costs the server a few words, however
// many rows the browser holds.
//
// A row's key is the key attribute of the row's element, as for any keyed
// loop; the browser matches rows by the key's text (fmt.Sprint's), so
// Delete takes the key's value and the server never needs the row. Its
// markup is a loop over Rows() that is the only content of its element,
// the container:
//
//	<ul class="log">
//	  for _, m := range messages.Rows() {
//	    <li key={m.ID}>{m.Text} <button onClick={ui.JS.Push(Remove, m.ID)}>Delete</button></li>
//	  }
//	</ul>
//
// or, in Go, List. A row's server handler is a Push of a method of the
// stream's owner with the row's values as arguments: it names the method,
// not the row, so a row rendered an hour ago still works. A live
// component in a row, a closure or a plain method value as a row's
// handler fail the render. A <ui.Form for> in a row submits as a Push of
// a method taking the form; failing its checks, it answers with the form's
// report, which the browser puts in place on that row's form.
//
// The container's onViewportTop and onViewportBottom are ordinary
// handlers the browser calls when its first or last row comes into view,
// the event's value that row's key — the edge the browser holds, which
// the server doesn't track: an infinite list, in either direction.
//
// A stream is tracked state, as an Assign is: a component whose state is
// all tracked renders again when one of its streams has operations
// queued, and is skipped otherwise. The server can't count, sort or
// filter what the browser holds; keep counts in fields beside the stream.
// The zero Stream is ready to use.
type Stream[T any] struct {
	name   string
	gen    string // the generation: random per stream per mount
	n      int    // the batches of gen the browser was sent
	limit  int
	trim   byte // the end Limit trims: 0 the end away from where rows went in, else trimStart or trimEnd
	ops    []streamOp[T]
	dirty  bool
	shown  bool // a render since the last flush rendered the block
	last   StreamBatch
	mark   int // the ops queued when a deferred load of the stream started
	markN  int // and the batches sent then
	warned bool

	// for verification (CheckStreams): whether the render under way showed
	// the container, whether one since it was last shown didn't, and
	// whether Reset ran since
	rendered, hidden, ever, reset bool
}

type streamOp[T any] struct {
	kind  byte // opReset, opDelete, opEnd (Insert), opStart (Prepend), opAt (InsertAt)
	key   string
	row   T
	at    int  // an InsertAt's position
	reset bool // one of a Reset's rows
}

// The end a limit trims, when TrimStart or TrimEnd fixed it.
const (
	trimStart = 's'
	trimEnd   = 'e'
)

const (
	opReset  = 'r'
	opDelete = 'd'
	opEnd    = 'e'
	opStart  = 's'
	opAt     = 'a'
)

// StreamWarnAt is how many queued rows of a stream without a limit make the
// session warn about it (in development and tests): a container that isn't
// rendered while messages arrive queues them all.
const StreamWarnAt = 10000

// begin starts the stream's generation, if it hasn't one: its first batch
// starts with a reset, so a browser holding another generation's rows (a
// page that reconnected) drops them.
func (s *Stream[T]) begin() {
	if s.gen == "" {
		s.gen = newGeneration()
		s.n = 0
		s.ops = append(s.ops[:0:0], streamOp[T]{kind: opReset})
	}
}

func newGeneration() string {
	var b [5]byte
	rand.Read(b[:])
	return strconv.FormatUint(uint64(b[0])|uint64(b[1])<<8|uint64(b[2])<<16|uint64(b[3])<<24|uint64(b[4])<<32, 36)
}

// Insert queues rows at the end; a row whose key the browser already holds
// is morphed in place instead, and stays where it is. Insert is how a row
// is updated.
func (s *Stream[T]) Insert(rows ...T) {
	s.begin()
	for _, r := range rows {
		s.ops = append(s.ops, streamOp[T]{kind: opEnd, row: r})
		s.bound(opEnd)
	}
}

// Prepend queues rows at the start, in the order given; a row whose key the
// browser holds is morphed in place instead.
func (s *Stream[T]) Prepend(rows ...T) {
	s.begin()
	for i := len(rows) - 1; i >= 0; i-- {
		s.ops = append(s.ops, streamOp[T]{kind: opStart, row: rows[i]})
		s.bound(opStart)
	}
}

// InsertAt queues rows before the browser's row i (from 0), in the order
// given; past the last row, at the end, and a negative i is 0. A row whose
// key the browser holds is morphed in place instead, and stays where it
// is. A limit trims from the end, as for Prepend: InsertAt(0, rows...) is
// Prepend(rows...).
func (s *Stream[T]) InsertAt(i int, rows ...T) {
	s.begin()
	i = max(i, 0)
	for j := len(rows) - 1; j >= 0; j-- {
		s.ops = append(s.ops, streamOp[T]{kind: opAt, row: rows[j], at: i})
	}
}

// Delete queues the removal of the rows with these keys; a key the browser
// doesn't hold is ignored. A key is matched by its text, as fmt.Sprint
// writes it.
func (s *Stream[T]) Delete(keys ...any) {
	s.begin()
	for _, k := range keys {
		s.ops = append(s.ops, streamOp[T]{kind: opDelete, key: fmt.Sprint(k)})
	}
}

// Reset queues replacing every row the browser holds with rows, in order:
// those whose keys it holds are morphed in place, the others go. With a
// limit, only the first limit rows are kept — the last, when the limit
// trims the start (TrimStart).
func (s *Stream[T]) Reset(rows ...T) {
	s.begin()
	s.ops = append(s.ops[:0:0], streamOp[T]{kind: opReset})
	s.reset = true
	for _, r := range rows {
		s.ops = append(s.ops, streamOp[T]{kind: opEnd, row: r, reset: true})
	}
	s.keepReset()
}

// Limit is the most rows the browser keeps (0, the default: no limit). A
// batch adding rows trims from the end away from where they went in:
// Insert trims from the start, Prepend and InsertAt from the end. A row
// replaced in place trims nothing. It takes effect at the next batch.
//
// The limit it returns fixes the end it trims instead, wherever rows go
// in: a newest-first feed keeps its newest rows at the top whether it
// prepends what arrives or inserts older rows at the bottom.
//
//	f.events.Limit(50).TrimEnd()
func (s *Stream[T]) Limit(n int) StreamLimit {
	s.begin()
	s.limit = max(n, 0)
	s.trim = 0
	s.limited()
	return StreamLimit{s}
}

// limited applies the limit and its end to what is queued: a Reset's rows
// past the limit, and inserts the browser would trim anyway.
func (s *Stream[T]) limited() {
	if s.limit == 0 {
		return
	}
	s.keepReset()
	s.bound(opEnd)
	s.bound(opStart)
}

// keepReset drops a queued Reset's rows past the limit: from the end the
// limit trims, the end by default.
func (s *Stream[T]) keepReset() {
	n := 0
	for _, op := range s.ops {
		if op.reset {
			n++
		}
	}
	if s.limit == 0 || n <= s.limit {
		return
	}
	drop := n - s.limit // the first ones, trimming the start; else the last
	kept := s.ops[:0]
	seen := 0
	for _, op := range s.ops {
		if op.reset {
			seen++
			if s.trim == trimStart && seen <= drop || s.trim != trimStart && seen > s.limit {
				continue
			}
		}
		kept = append(kept, op)
	}
	s.ops = kept
}

func (s *Stream[T]) setTrim(end byte) {
	s.trim = end
	s.limited()
}

// StreamLimit is a stream's limit as Limit set it, to fix the end it
// trims.
type StreamLimit struct{ s interface{ setTrim(byte) } }

// TrimStart has the limit trim the stream's start (its first rows),
// wherever a batch's rows went in. A Reset with more rows than the limit
// keeps the last.
func (l StreamLimit) TrimStart() { l.s.setTrim(trimStart) }

// TrimEnd has the limit trim the stream's end (its last rows), wherever a
// batch's rows went in: rows inserted at the end of a full stream are
// dropped, unless the browser holds their keys already.
func (l StreamLimit) TrimEnd() { l.s.setTrim(trimEnd) }

// bound keeps the queue of a stream with a limit from growing past it at
// one end: older inserts there would be trimmed by the browser anyway —
// or, when the limit trims that same end (TrimStart, TrimEnd), newer
// ones, which go in past the rows already there. InsertAt's rows aren't
// bounded: where they end up depends on the rows the browser holds.
func (s *Stream[T]) bound(kind byte) {
	if s.limit == 0 {
		return
	}
	n, first, last := 0, -1, -1
	for i, op := range s.ops {
		if op.kind == kind {
			if first < 0 {
				first = i
			}
			last = i
			n++
		}
	}
	if n <= s.limit {
		return
	}
	drop := first
	if kind == opEnd && s.trim == trimEnd || kind == opStart && s.trim == trimStart {
		drop = last
	}
	s.ops = append(s.ops[:drop], s.ops[drop+1:]...)
}

// Rows is the rows queued since the last reply, in the order their
// operations apply (a Prepend's or an InsertAt's last row first, each
// going before the one queued after it): what the next render
// sends. A stream's loop ranges over it, and nothing else may read it — it
// isn't what the browser shows.
func (s *Stream[T]) Rows() []T {
	var out []T
	for _, op := range s.ops {
		if op.kind == opEnd || op.kind == opStart || op.kind == opAt {
			out = append(out, op.row)
		}
	}
	return out
}

// AssignDirty is the stream's changed mark: set while it has operations
// queued.
func (s *Stream[T]) AssignDirty() *bool {
	if len(s.ops) > 0 {
		s.dirty = true
	}
	return &s.dirty
}

// List is the stream's container: an element tag with attrs and the
// stream's attributes, holding the queued rows, each what row makes of it
// — one element with a key. It is what a stream's markup lowers to.
//
//	c.messages.List("ul", []ui.Attr{{Name: "class", Value: "log"}}, func(m Message) ui.Node {
//		return ui.El("li", []ui.Attr{{Name: "key", Value: m.ID}}, ui.Text(m.Text))
//	})
func (s *Stream[T]) List(tag string, attrs []Attr, row func(T) Node) Node {
	rows := s.Rows()
	nodes := make([]Node, len(rows))
	for i, r := range rows {
		nodes[i] = row(r)
	}
	b := s.Block(nodes)
	all := append(attrs[:len(attrs):len(attrs)], Attr{Name: "data-vk-stream", Value: attrSet(b.Attrs())})
	shape := "E" + strings.Repeat("d", len(all)) + "(h)"
	return newFrame(streamFP(tag, len(all)), shape, []Node{newElement(tag, all, []Node{b})})
}

// streamFP is the fingerprint of List's container: its statics are its
// tag's, every attribute a dynamic.
func streamFP(tag string, attrs int) uint64 {
	h := uint64(0x73747265616d0000)
	for i := 0; i < len(tag); i++ {
		h = mix(h ^ uint64(tag[i]))
	}
	return mix(h ^ uint64(attrs)<<40)
}

// Block is the stream's rows block for this render: rows are what the
// stream's loop rendered, one per row of Rows, in its order. Its Attrs go on
// the container, the element it is the only content of. The compiler
// writes it; Go writes List.
func (s *Stream[T]) Block(rows []Node) *StreamBlock {
	s.begin()
	b := &StreamBlock{name: s.name, limit: s.limit, trim: s.trim, shown: &s.shown, rendered: &s.rendered}
	batch := s.n
	if len(s.ops) > 0 {
		batch++
	}
	b.batch = s.gen + "." + strconv.Itoa(batch)
	if want := len(s.Rows()); len(rows) != want {
		b.err = fmt.Errorf("ui: stream %s rendered %d rows for the %d queued: its loop renders one element per row of Rows()", s.name, len(rows), want)
		return b
	}
	at := map[string]int{} // a key's last op in b.ops
	i := 0
	for _, op := range s.ops {
		switch op.kind {
		case opReset:
			b.ops = b.ops[:0]
			clear(at)
			b.ops = append(b.ops, blockOp{kind: opReset})
		case opDelete:
			if j, ok := at[op.key]; ok {
				if b.ops[j].kind == opDelete {
					continue // deleted already
				}
				b.ops[j].kind = 0 // an insert the delete undoes
			}
			at[op.key] = len(b.ops)
			b.ops = append(b.ops, blockOp{kind: opDelete, key: op.key})
		default:
			n := rows[i]
			i++
			key, f, err := rowKey(n)
			if err != nil {
				b.err = fmt.Errorf("stream %s: %w", s.name, err)
				return b
			}
			if j, ok := at[key]; ok && b.ops[j].kind != opDelete {
				b.ops[j].node, b.ops[j].frame = n, f // a second insert of a key replaces the first's row, in its place
				continue
			}
			at[key] = len(b.ops)
			b.ops = append(b.ops, blockOp{kind: op.kind, key: key, at: op.at, node: n, frame: f})
		}
	}
	kept := b.ops[:0]
	for _, op := range b.ops {
		if op.kind != 0 {
			kept = append(kept, op)
		}
	}
	b.ops = kept
	s.last = b.info()
	return b
}

// StreamBlock is a stream's rows in one render (Stream.Block): the batch of
// operations its container applies, and the rows they insert.
type StreamBlock struct {
	name  string
	batch string
	limit int
	trim  byte
	ops   []blockOp
	err   error
	shown *bool

	rendered *bool
}

type blockOp struct {
	kind  byte
	key   string
	at    int    // an InsertAt's position
	node  Node   // an insert's row
	frame *Frame // its frame, when rowKey built it
}

func (b *StreamBlock) Render(ctx context.Context, w io.Writer) error { return renderHTML(ctx, w, b) }

// Attrs is the container's stream attributes: data-vk-stream (the field's
// name), data-vk-batch (generation.number), data-vk-limit, and, when the
// render has operations, data-vk-ops — with, for a batch starting with a
// reset, the rows it leaves (data-vk-rows, data-vk-empty). The attributes
// a batch adds come last, from data-vk-ops on: the session takes them off
// the render it keeps once the reply is written.
func (b *StreamBlock) Attrs() []Attr {
	attrs := []Attr{{"data-vk-stream", b.name}, {"data-vk-batch", b.batch}}
	if b.limit > 0 {
		attrs = append(attrs, Attr{"data-vk-limit", b.limit})
		if b.trim != 0 {
			attrs = append(attrs, Attr{"data-vk-trim", trimName(b.trim)})
		}
	}
	if len(b.ops) == 0 {
		return attrs
	}
	ops := make([][]any, len(b.ops))
	rows := 0
	for i, op := range b.ops {
		switch op.kind {
		case opReset:
			ops[i] = []any{"reset"}
		case opDelete:
			ops[i] = []any{"delete", op.key}
		case opEnd:
			ops[i] = []any{"insert", op.key, "end"}
			rows++
		case opStart:
			ops[i] = []any{"insert", op.key, "start"}
			rows++
		case opAt:
			ops[i] = []any{"insert", op.key, op.at}
			rows++
		}
	}
	j, _ := json.Marshal(ops)
	attrs = append(attrs, Attr{"data-vk-ops", string(j)})
	if b.ops[0].kind == opReset {
		if b.limit > 0 {
			rows = min(rows, b.limit)
		}
		attrs = append(attrs, Attr{"data-vk-rows", rows})
		if rows == 0 {
			attrs = append(attrs, Attr{"data-vk-empty", true})
		}
	}
	return attrs
}

func trimName(end byte) string {
	if end == trimStart {
		return "start"
	}
	return "end"
}

// StreamBatchAttr starts the attributes a batch adds to a container's: what
// FlushedAttrs takes off.
const StreamBatchAttr = " data-vk-ops="

// FlushedAttrs is a container's stream attributes as rendered (one dynamic
// of a render tree, starting with data-vk-stream) as they render once the
// batch is sent: without what the batch added.
func FlushedAttrs(s string) string {
	if i := strings.Index(s, StreamBatchAttr); i >= 0 {
		return s[:i]
	}
	return s
}

// IsStreamAttrs reports whether s, a dynamic of a render tree, is a
// container's stream attributes.
func IsStreamAttrs(s string) bool { return strings.HasPrefix(s, ` data-vk-stream="`) }

func (b *StreamBlock) info() StreamBatch {
	in := StreamBatch{Name: b.name, Batch: b.batch, Limit: b.limit}
	if b.limit > 0 && b.trim != 0 {
		in.Trim = trimName(b.trim)
	}
	for _, op := range b.ops {
		switch op.kind {
		case opReset:
			in.Reset = true
		case opDelete:
			in.Deletes++
		default:
			in.Inserts++
		}
	}
	return in
}

// StreamBatch is what a stream's batch did, for a development toolbar.
type StreamBatch struct {
	Name             string
	Batch            string
	Reset            bool
	Inserts, Deletes int
	Limit            int
	Trim             string // the end the limit trims, when TrimStart or TrimEnd fixed it: "start", "end"
}

// rowKey is the key of a row: its element's key attribute, as text; and
// the row's frame, when it is one.
func rowKey(n Node) (string, *Frame, error) {
	n = unview(n)
	if lf, ok := n.(*lazyFrame); ok && lf != nil {
		n = lf.frame()
	}
	f, _ := n.(*Frame)
	k, err := elemKey(n)
	return k, f, err
}

func elemKey(n Node) (string, error) {
	if f, ok := n.(*Frame); ok && f != nil {
		var kids []Node
		for _, k := range f.Kids {
			if t, ok := k.(TextNode); ok && strings.TrimSpace(string(t)) == "" {
				continue
			}
			kids = append(kids, k)
		}
		if len(kids) != 1 {
			return "", streamRowErr()
		}
		n = kids[0]
	}
	switch n := n.(type) {
	case *Element:
		if k, ok := elementKey(n); ok && k != nil {
			return fmt.Sprint(k), nil
		}
	case *ComponentNode:
		return "", rowComponentErr(n)
	}
	return "", streamRowErr()
}

func streamRowErr() error {
	return diag.New("E0355", "a stream row is one element with a key").
		WithHelp("render each row as one element keyed by the record: <li key={m.ID}>…</li>")
}

func rowComponentErr(n *ComponentNode) error {
	return diag.New("E0359", "a stream row renders the live component %T", n.Props).
		WithHelp("render a stateless shape in the row, and push to the owner's methods: ui.JS.Push(Remove, m.ID)")
}

// A stream row is rendered in a context saying so: its first key attribute
// writes data-vk-row, and its server handlers must be pushes of the
// owner's methods, which a host names by the method (StreamPusher).
type streamRowKey struct{}

type streamRow struct {
	key  string
	done bool // its key attribute was written
}

func inStreamRow(ctx context.Context) *streamRow {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(streamRowKey{}).(*streamRow)
	return r
}

// StreamPusher is a live host that names a Push in a stream row by the
// method it calls, which its owner's instance takes by name: the row
// holds no handler. Package live implements it.
type StreamPusher interface {
	StreamPush(ctx context.Context, fn any) (string, error)
}

// rowHandlerErr is the error of a handler in a stream row that isn't a Push.
func rowHandlerErr(attr string) error {
	return diag.New("E0358", "%s in a stream row is a handler the server would have to keep for the row", attr).
		WithHelp("push a method of the stream's owner with the row's key: %s={ui.JS.Push(Remove, m.ID)}", attr)
}

// walkStream sends a block's rows to r, each in its row's context.
func walkStream(ctx context.Context, b *StreamBlock, r Renderer) error {
	if b.err != nil {
		return b.err
	}
	*b.shown, *b.rendered = true, true
	sr, _ := r.(scopedRenderer)
	for _, op := range b.ops {
		if op.node == nil {
			continue
		}
		rctx := context.WithValue(ctx, streamRowKey{}, &streamRow{key: op.key})
		var prev context.Context
		if sr != nil {
			prev = sr.swapScope(rctx)
		}
		err := Walk(rctx, op.node, r)
		if sr != nil {
			sr.swapScope(prev)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// streamTree is a block's rows as a render tree's list, marked a stream's:
// the session empties it once its reply is written.
func (b *treeBuilder) streamTree(ctx context.Context, sb *StreamBlock) (*TreeList, error) {
	if sb.err != nil {
		return nil, sb.err
	}
	*sb.shown, *sb.rendered = true, true
	l := &TreeList{Stream: true}
	for _, op := range sb.ops {
		if op.node == nil {
			continue
		}
		rctx := context.WithValue(ctx, streamRowKey{}, &streamRow{key: op.key})
		prev := b.h.swapScope(rctx)
		f := op.frame
		if f == nil {
			f = newFrame(itemFP, "h", []Node{unview(op.node)})
		}
		t, err := b.frame(rctx, f)
		b.h.swapScope(prev)
		if err != nil {
			return nil, err
		}
		l.Items = append(l.Items, t)
		l.Keys = append(l.Keys, "row="+op.key)
	}
	return l, nil
}

// The session's view of a component's streams.
type streamer interface {
	streamInit(name string)
	streamCheck() bool
	streamFlush() (StreamBatch, bool)
	streamQueued() int
	streamWarn() bool
	deferStart()
	deferCopy()
	deferMerge(from any)
}

func (s *Stream[T]) streamInit(name string) {
	s.name = name
	s.begin()
}

func (s *Stream[T]) streamFlush() (StreamBatch, bool) {
	if !s.shown {
		return StreamBatch{}, false
	}
	s.shown = false
	if len(s.ops) == 0 {
		return StreamBatch{}, false
	}
	s.n++
	s.ops = s.ops[:0:0]
	s.dirty = false
	return s.last, true
}

func (s *Stream[T]) streamQueued() int { return len(s.ops) }

// streamCheck ends a render of the stream's owner, reporting false when
// it showed the container again after a render that didn't, with no Reset
// between: the browser dropped the rows with the container, and the
// stream has nothing to show.
func (s *Stream[T]) streamCheck() bool {
	ok := true
	switch {
	case s.rendered:
		ok = !s.hidden || s.reset
		s.hidden, s.ever = false, true
	case s.ever && !s.hidden:
		s.hidden, s.reset = true, false
	}
	s.rendered = false
	return ok
}

// streamWarn reports, once, whether the stream queued more rows than
// StreamWarnAt with no limit, or with InsertAt's rows, which a limit
// doesn't bound.
func (s *Stream[T]) streamWarn() bool {
	if s.warned || len(s.ops) <= StreamWarnAt || s.limit > 0 && !slices.ContainsFunc(s.ops, func(op streamOp[T]) bool { return op.kind == opAt }) {
		return false
	}
	s.warned = true
	return true
}

func (s *Stream[T]) deferStart() { s.mark, s.markN = len(s.ops), s.n }

func (s *Stream[T]) deferCopy() { s.ops = nil }

// deferMerge takes what a deferred load of the stream queued on its copy:
// before what the stream queued meanwhile, which is newer than the
// loader's snapshot.
func (s *Stream[T]) deferMerge(from any) {
	f := from.(*Stream[T])
	mark := min(s.mark, len(s.ops))
	if s.n != s.markN {
		mark = 0 // what was queued when the load started has been sent
	}
	ops := append(append(append([]streamOp[T](nil), s.ops[:mark]...), f.ops...), s.ops[mark:]...)
	for i := len(ops) - 1; i > 0; i-- {
		if ops[i].kind == opReset {
			ops = ops[i:]
			break
		}
	}
	s.ops = ops
	if f.limit != s.limit || f.trim != s.trim {
		s.limit, s.trim = f.limit, f.trim
	}
}

var streamerType = reflect.TypeFor[streamer]()

// streamsOf calls each with c's stream fields and their names; c is a
// pointer to a struct.
func streamsOf(v reflect.Value, each func(name string, s streamer)) {
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return
	}
	v = v.Elem()
	for _, i := range stateOf(v.Type()).streams {
		f := v.Field(i)
		each(v.Type().Field(i).Name, reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Interface().(streamer))
	}
}

// initStreams names c's streams after their fields and starts their
// generations: a fresh mount's first batch resets what a browser holds. A
// stream a parent could set (exported) fails the mount.
func initStreams(p reflect.Value) error {
	if p.Kind() != reflect.Pointer || p.IsNil() || p.Elem().Kind() != reflect.Struct {
		return nil
	}
	t := p.Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); f.IsExported() && !f.Anonymous && reflect.PointerTo(f.Type).Implements(streamerType) {
			return diag.New("E0357", "%s.%s is an exported ui.Stream: a parent can't own rows the server forgets", t.Name(), f.Name).
				WithHelp("make it unexported: %s", strings.ToLower(f.Name[:1])+f.Name[1:])
		}
	}
	streamsOf(p, func(name string, s streamer) { s.streamInit(name) })
	return nil
}

// FlushStreams ends the batches of c's streams that its last render sent,
// once the reply carrying them is written: their operations are dropped,
// and the next render starts the next batch. A stream whose container
// that render didn't show keeps its operations. It returns what each
// flushed batch did.
func FlushStreams(c Stateful) []StreamBatch {
	var out []StreamBatch
	streamsOf(reflect.ValueOf(c), func(_ string, s streamer) {
		if b, ok := s.streamFlush(); ok {
			out = append(out, b)
		}
	})
	return out
}

// HasStreams reports whether c has a stream field.
func HasStreams(c Stateful) bool {
	v := reflect.ValueOf(c)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return false
	}
	return len(stateOf(v.Elem().Type()).streams) > 0
}

// StreamWarnings names c's streams that queued more than StreamWarnAt rows
// that no limit bounds (none set, or InsertAt's rows), each once.
func StreamWarnings(c Stateful) []string {
	var out []string
	streamsOf(reflect.ValueOf(c), func(name string, s streamer) {
		if s.streamWarn() {
			out = append(out, fmt.Sprintf("%s.%s has %d operations queued that no limit bounds: is its container rendered? (a stream whose container isn't shown keeps what it queues)", reflect.TypeOf(c).Elem().Name(), name, s.streamQueued()))
		}
	})
	return out
}

// FlushedHTML is markup holding stream containers (an instance's render
// as one piece of HTML) as it renders once their batches are sent: each
// container without what its batch added to its attributes, and empty.
func FlushedHTML(s string) string {
	const mark = ` data-vk-stream="`
	if !strings.Contains(s, mark) {
		return s
	}
	var b strings.Builder
	i := 0
	for {
		at := strings.Index(s[i:], mark)
		if at < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		at += i
		start := strings.LastIndexByte(s[:at], '<')
		gt := strings.IndexByte(s[at:], '>')
		if start < i || gt < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		gt += at
		tag := s[start+1:]
		tag = tag[:strings.IndexAny(tag, " \t\n/>")]
		end := closeOf(s, gt+1, tag)
		if end < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		open := s[start:gt]
		if j := strings.Index(open, StreamBatchAttr); j >= 0 {
			open = open[:j]
		}
		b.WriteString(s[i:start])
		b.WriteString(open + "></" + tag + ">")
		i = end
	}
}

// closeOf is where the element of tag whose content starts at from ends:
// past its closing tag, nested elements of the tag counted.
func closeOf(s string, from int, tag string) int {
	depth := 1
	for i := from; i < len(s); {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			return -1
		}
		i += lt
		rest := s[i+1:]
		switch {
		case strings.HasPrefix(rest, "/"+tag+">"):
			depth--
			i += len(tag) + 3
			if depth == 0 {
				return i
			}
			continue
		case strings.HasPrefix(rest, tag) && len(rest) > len(tag) && strings.IndexByte(" \t\n/>", rest[len(tag)]) >= 0:
			depth++
		}
		i++
	}
	return -1
}

// CheckStreams ends a render of c under verification (VUKA_LIVE_VERIFY, on
// in tests): it fails when the render showed a stream's container again
// after a render of c that didn't, with no Reset between — the browser
// dropped the rows with the container.
func CheckStreams(c Stateful) error {
	var err error
	streamsOf(reflect.ValueOf(c), func(name string, s streamer) {
		if !s.streamCheck() && err == nil {
			err = fmt.Errorf("ui: %s shows the container of its stream %s again, after a render that didn't, with no Reset between: the browser dropped the rows with the container (Reset the stream when it is shown again, or keep the container rendered)", reflect.TypeOf(c).Elem().Name(), name)
		}
	})
	return err
}
