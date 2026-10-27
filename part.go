package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/vuka-lang/vuka/v2"
	"github.com/vuka-lang/vuka/v2/diag"
)

// Parts of stateless pages. A method of a page or component marked
// @ui.Defer (vuka.RegisterFunc((*Index).Stats, ui.Defer(""))) runs after the
// page's first bytes are sent, and the markup calling it — its part — is
// sent once it has: the page shows at once, its slow parts filling in.
//
//	@ui.Defer
//	func (p *Index) Stats(ctx context.Context) (Stats, error) { … }
//
//	<p>{Stats(ctx).Open} open</p>
//	<ui.Fallback for={Stats}><span class="skeleton"></span></ui.Fallback>
//
// is, in the generated Go,
//
//	ui.Part(p.Stats, ui.HTML(`<span class="skeleton"></span>`), func(stats Stats) ui.Node {
//		return ui.HTML(`<p>{stats.Open} open</p>`, stats.Open)
//	})
//
// Part is what markup written in Go calls too. A page rendered for a
// request web streams (WithParts) gets the fallback in the part's place,
// then each part as its method returns; anywhere else — a live page, a
// fragment, a test rendering a Node — the method runs as the part renders,
// in place.

// Part is the part of a page or component that method m's result renders:
// render's markup once m returns, fallback until then. m is a method of the
// page or component, as a method value (p.Stats), returning (V, error); its
// attributes say when it runs (Defer). A part whose method fails renders the
// fallback's Error (PartFallback), else a short message with role="alert";
// the rest of the page is unaffected. A nil fallback is an empty <span
// aria-busy="true">.
//
// One request calls a method once, however many parts name it: parts of the
// same method of the same value (a pointer receiver) share its result.
func Part[V any](m func(context.Context) (V, error), fallback Node, render func(V) Node) Node {
	if m == nil || render == nil {
		panic("ui: Part needs a method and a render function")
	}
	key, method := partKeyOf(unsafe.Pointer(&m), m)
	run := func(ctx context.Context) (any, error) { return m(ctx) }
	show := func(v any) Node { return render(v.(V)) }
	return ctxNode(func(ctx context.Context) Node {
		return resolvePart(ctx, key, method, run, fallback, show)
	})
}

// PartFallback is what a part shows while its method runs (Pending) and when
// it fails (Error): Part's fallback, for one with an error of its own. It
// renders as Pending.
type PartFallback struct{ Pending, Error Node }

func (f PartFallback) Render(ctx context.Context, w io.Writer) error {
	if f.Pending == nil {
		return nil
	}
	return f.Pending.Render(ctx, w)
}

// partKey names a method of a value: its code, and the receiver of a method
// value of a pointer receiver.
type partKey struct {
	pc   uintptr
	recv unsafe.Pointer
	own  unsafe.Pointer // a func value that isn't a pointer's method value: itself, shared with no other
}

// partKeyOf is the key of the func value ptr points to, fn, and what Part
// knows of it. A method value of a pointer receiver is a closure whose word
// after its code is the receiver, as the gc compiler makes it (cmd/compile's
// method value wrappers, -fm).
func partKeyOf(ptr unsafe.Pointer, fn any) (partKey, partMethod) {
	clo := *(*unsafe.Pointer)(ptr)
	pc := *(*uintptr)(clo)
	k, m := partKey{pc: pc}, partMethodOf(pc, fn)
	if m.ptrMethod {
		k.recv = *(*unsafe.Pointer)(unsafe.Add(clo, unsafe.Sizeof(uintptr(0))))
	} else {
		k.own = clo
	}
	return k, m
}

// partMethod is what Part knows of a method from its code: its name, and
// when it runs.
type partMethod struct {
	name      string // "Index.Stats"
	ptrMethod bool   // a method value of a pointer receiver
	deferred  bool   // @ui.Defer
	group     string // its Defer's group: "" for its own
}

var partMethods sync.Map // pc → partMethod

// partMethodOf is what Part knows of fn, whose code is at pc: its name, and
// its timing attribute (Defer), registered with vuka.RegisterFunc.
func partMethodOf(pc uintptr, fn any) partMethod {
	if m, ok := partMethods.Load(pc); ok {
		return m.(partMethod)
	}
	var m partMethod
	if f := runtime.FuncForPC(pc); f != nil {
		full := f.Name()
		m.ptrMethod = strings.HasSuffix(full, "-fm") && strings.Contains(full, ".(*")
		m.name = shortFuncName(full)
	}
	var d Defer
	if ref, err := vuka.FuncOf(fn); err == nil && ref.Attrs.Attr(&d) {
		m.deferred, m.group = true, string(d)
	}
	partMethods.Store(pc, m)
	return m
}

// shortFuncName is a method's name as messages give it: "Index.Stats" for
// "example.com/app/pages.(*Index).Stats-fm".
func shortFuncName(s string) string {
	s = strings.TrimSuffix(s, "-fm")
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	if _, rest, ok := strings.Cut(s, "."); ok {
		s = rest
	}
	return strings.NewReplacer("(*", "", ")", "").Replace(s)
}

// resolvePart is a Part as ctx renders it: its content once its method has
// returned, else — on a page whose parts stream — its fallback in a slot
// the stream fills.
func resolvePart(ctx context.Context, key partKey, m partMethod, run func(context.Context) (any, error), fallback Node, show func(any) Node) Node {
	pending, failed := fallback, Node(nil)
	if f, ok := fallback.(PartFallback); ok {
		pending, failed = f.Pending, f.Error
	}
	ps := partsOf(ctx)
	if ps == nil || !m.deferred {
		v, err := callPart(ctx, run)
		if err != nil {
			slog.ErrorContext(ctx, "ui: a part's method failed", "method", m.name, "err", err)
		}
		return partContent(v, err, show, failed, nil)
	}
	c, slot := ps.slot(ctx, key, m, run, show, failed)
	if slot == nil {
		return partContent(c.v, c.err, show, failed, ps.errText)
	}
	if pending == nil {
		pending = RawHTML(`<span aria-busy="true"></span>`)
	}
	return Group{RawHTML("<!--vk-part:" + slot.id + "-->"), pending, RawHTML("<!--/vk-part:" + slot.id + "-->")}
}

// callPart calls a part's method, a panic its error.
func callPart(ctx context.Context, run func(context.Context) (any, error)) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := Failed(r); ok {
				err = e
				return
			}
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return run(ctx)
}

// partContent is a part's markup for its method's result: render's, or
// the error's.
func partContent(v any, err error, show func(any) Node, failed Node, text func(error) string) Node {
	if err != nil {
		return partError(err, failed, text)
	}
	return show(v)
}

func partError(err error, failed Node, text func(error) string) Node {
	if failed != nil {
		return failed
	}
	return RawHTML(`<span role="alert">` + html.EscapeString(partErrorText(err, text)) + `</span>`)
}

// Parts are the deferred parts of a page rendered for one request: what
// WithParts puts in its context. The page renders with each part's fallback
// in a slot; Stream then runs their methods and writes each part as it
// arrives.
type Parts struct {
	mu      sync.Mutex
	calls   map[partKey]*partCall
	order   []*partCall // in the order the page met them
	slots   []*partSlot // not written yet
	n       int         // slots so far, naming them
	running bool        // Stream has started: a call met now starts at once
	start   func(*partCall)
	errText func(error) string
}

type partCall struct {
	m     partMethod
	ctx   context.Context
	run   func(context.Context) (any, error)
	state int // 0 waiting, 1 running, 2 done
	v     any
	err   error
	slots []*partSlot
}

type partSlot struct {
	id     string
	ctx    context.Context
	call   *partCall
	show   func(any) Node
	failed Node
}

type partsKey struct{}

// WithParts is ctx for rendering a page whose deferred parts stream: a part
// renders its fallback, its method run by Stream.
func WithParts(ctx context.Context) (context.Context, *Parts) {
	p := &Parts{calls: map[partKey]*partCall{}}
	return context.WithValue(ctx, partsKey{}, p), p
}

func partsOf(ctx context.Context) *Parts {
	p, _ := ctx.Value(partsKey{}).(*Parts)
	return p
}

// slot is the call of the method key names and, while it runs, a new slot
// for its part; nil when it has returned.
func (p *Parts) slot(ctx context.Context, key partKey, m partMethod, run func(context.Context) (any, error), show func(any) Node, failed Node) (*partCall, *partSlot) {
	p.mu.Lock()
	c := p.calls[key]
	if c == nil {
		c = &partCall{m: m, ctx: ctx, run: run}
		p.calls[key] = c
		p.order = append(p.order, c)
	}
	if c.state == 2 {
		p.mu.Unlock()
		return c, nil
	}
	p.n++
	s := &partSlot{id: "vk-p" + strconv.Itoa(p.n), ctx: ctx, call: c, show: show, failed: failed}
	c.slots = append(c.slots, s)
	p.slots = append(p.slots, s)
	start := p.running && c.state == 0
	if start {
		c.state = 1
	}
	p.mu.Unlock()
	if start {
		p.start(c)
	}
	return c, s
}

// Pending reports whether the page has parts to stream.
func (p *Parts) Pending() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.slots) > 0
}

// StreamOptions are how Stream runs a page's parts.
type StreamOptions struct {
	// Limit is how many methods run at once: DefaultPartLimit when zero.
	Limit int
	// Observe, if set, is called as each method starts, with its context
	// and name ("Index.Stats"); the context it returns is the one the
	// method gets, and done is called when it returns.
	Observe func(ctx context.Context, name string) (context.Context, func(err error))
	// ErrorText is the message of a part whose method fails and that has
	// no Error fallback: "" for the default.
	ErrorText func(error) string
	// Written, if set, is called as each part is written, with its
	// method's name and error (or its markup's): nil for one that rendered.
	Written func(name string, err error)
}

// DefaultPartLimit is how many of a page's deferred methods run at once.
const DefaultPartLimit = 8

// PartsScript is the script the stream starts with: what moves each part
// into its place as it arrives (and the mark a navigation's reply is cut
// at: the page before it, its parts after).
const PartsScript = `<script data-vk-parts>window.__vkP=function(i){var t=document.querySelector('template[data-vk-part="'+i+'"]');if(!t)return;var w=document.createTreeWalker(document,128),s=null,n,x;while(n=w.nextNode())if(n.data==="vk-part:"+i){s=n;break}if(s){var p=s.parentNode;for(n=s.nextSibling;n&&!(n.nodeType===8&&n.data==="/vk-part:"+i);n=x){x=n.nextSibling;p.removeChild(n)}p.insertBefore(t.content,s);p.removeChild(s);if(n)p.removeChild(n)}t.parentNode.removeChild(t);document.dispatchEvent(new CustomEvent("vk:part",{detail:{id:i}}))}</script>`

// Stream writes PartsScript and flushes, then runs the page's deferred
// methods, Limit at once, and writes each part to w as its method returns — those of a group (Defer("stats"))
// together — flushing after each: a <template data-vk-part> holding its
// markup and the call of the script moving it into place. A part whose
// markup meets another part's method running gets that part's slot, filled
// later in the stream. It returns when every part is written, or ctx (the
// request's) is done, its methods' contexts canceled.
func (p *Parts) Stream(ctx context.Context, w io.Writer, flush func() error, o StreamOptions) error {
	if !p.Pending() {
		return nil
	}
	limit := o.Limit
	if limit <= 0 {
		limit = DefaultPartLimit
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, limit)
	wake := make(chan struct{}, 1)
	p.mu.Lock()
	p.errText = o.ErrorText
	p.start = func(c *partCall) {
		go func() {
			var err error
			var v any
			select {
			case sem <- struct{}{}:
				cctx, stop := context.WithCancel(c.ctx)
				unhook := context.AfterFunc(ctx, stop)
				end := func(error) {}
				if o.Observe != nil {
					cctx, end = o.Observe(cctx, c.m.name)
				}
				v, err = callPart(cctx, c.run)
				end(err)
				unhook()
				stop()
				<-sem
			case <-ctx.Done():
				err = ctx.Err()
			}
			p.mu.Lock()
			c.v, c.err, c.state = v, err, 2
			p.mu.Unlock()
			select {
			case wake <- struct{}{}:
			default:
			}
		}()
	}
	p.running = true
	var waiting []*partCall
	for _, c := range p.order {
		if c.state == 0 {
			c.state = 1
			waiting = append(waiting, c)
		}
	}
	p.mu.Unlock()
	if _, err := io.WriteString(w, PartsScript); err != nil {
		return err
	}
	if flush != nil {
		// The page so far and the script: a navigation reading the reply
		// shows the page once it has the script.
		if err := flush(); err != nil {
			return err
		}
	}
	for _, c := range waiting {
		p.start(c)
	}
	for {
		p.mu.Lock()
		left := len(p.slots)
		p.mu.Unlock()
		if left == 0 {
			return nil
		}
		select {
		case <-wake:
		case <-ctx.Done():
			return ctx.Err()
		}
		wrote, err := p.writeReady(w, o)
		if err != nil {
			return err
		}
		if wrote && flush != nil {
			if err := flush(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// writeReady writes the slots whose groups have all returned.
func (p *Parts) writeReady(w io.Writer, o StreamOptions) (bool, error) {
	wrote := false
	for {
		p.mu.Lock()
		var ready []*partSlot
		groups := map[string]bool{} // groups with a call still running
		for _, c := range p.order {
			if c.state != 2 && c.m.group != "" {
				groups[c.m.group] = true
			}
		}
		rest := p.slots[:0]
		for _, s := range p.slots {
			if c := s.call; c.state == 2 && (c.m.group == "" || !groups[c.m.group]) {
				ready = append(ready, s)
			} else {
				rest = append(rest, s)
			}
		}
		p.slots = rest
		p.mu.Unlock()
		if len(ready) == 0 {
			return wrote, nil
		}
		var buf bytes.Buffer
		for _, s := range ready {
			html, err := s.render(p.errText)
			if o.Written != nil {
				o.Written(s.call.m.name, err)
			}
			buf.WriteString(`<template data-vk-part="` + s.id + `">`)
			buf.Write(html)
			buf.WriteString(`</template><script>__vkP("` + s.id + `")</script>`)
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			return wrote, err
		}
		wrote = true
	}
}

// errLivePart is the error of a part whose markup holds a live component.
var errLivePart = errors.New("ui: a deferred part's markup holds a live component or event handler, which a page that isn't live can't run")

// render is the slot's part as HTML: its content, or its error's markup;
// err is the method's or the render's error.
func (s *partSlot) render(text func(error) string) (out []byte, err error) {
	c := s.call
	var buf bytes.Buffer
	if c.err == nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			var info PageInfo
			info, err = RenderPage(s.ctx, s.show(c.v), &buf)
			if err == nil && info.Live {
				err = errLivePart
			}
		}()
		if err == nil {
			return buf.Bytes(), nil
		}
	} else {
		err = c.err
	}
	buf.Reset()
	if _, e := RenderPage(s.ctx, partError(err, s.failed, text), &buf); e != nil {
		buf.Reset()
		buf.WriteString(`<span role="alert">` + html.EscapeString(partErrorText(err, text)) + `</span>`)
	}
	return buf.Bytes(), err
}

func partErrorText(err error, text func(error) string) string {
	if text != nil {
		if t := text(err); t != "" {
			return t
		}
	}
	return "This part couldn't load."
}

// CheckParts checks the timing attributes on the methods of t, a page or
// component type (or a pointer to one): a deferred method takes a
// context.Context and returns (T, error), has one timing attribute, and is
// a stateless page's (live is whether t is live). Package web runs it on
// each page it routes, so a misregistered method fails the app's start.
func CheckParts(t reflect.Type, live bool) error {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() == "" {
		return nil
	}
	pt := reflect.PointerTo(t)
	for i := range pt.NumMethod() {
		m := pt.Method(i)
		n := 0
		var group Defer
		for _, a := range vuka.MethodAttrs(t, m.Name) {
			if d, ok := a.(Defer); ok {
				n++
				group = d
			}
		}
		if n == 0 {
			continue
		}
		name := t.Name() + "." + m.Name
		ft := m.Type // with its receiver
		switch {
		case n > 1:
			return diag.New("E0345", "%s has @ui.Defer twice: one timing attribute says when it runs", name).WithHelp("keep one: @ui.Defer, or @ui.Defer(%q)", string(group))
		case live:
			return diag.New("E0346", "@ui.Defer is for stateless pages; %s is live: a live page loads in its @ui.Mount hook and pushes with Update", name).WithHelp("drop the attribute and load %s in @ui.Mount into a field the markup reads", m.Name)
		case ft.NumIn() != 2 || ft.In(1) != contextType || ft.NumOut() != 2 || ft.Out(1) != errorType:
			return diag.New("E0347", "@ui.Defer: %s is %s; a deferred method takes a context.Context and returns (T, error)", name, strings.Replace(ft.String(), "func(*"+t.String()+", ", "func(", 1)).
				WithHelp("func (p *%s) %s(ctx context.Context) (T, error)", t.Name(), m.Name)
		}
	}
	return nil
}

var (
	contextType = reflect.TypeFor[context.Context]()
	errorType   = reflect.TypeFor[error]()
)
