package live

import (
	"context"
	"log"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2/diag"
)

// Streams (ui.Stream): a row's server handler is a Push of a method of the
// stream's owner, named by the method ("c1:Remove"), since the session
// keeps nothing of the row; once a reply carrying a batch is written, the
// session drops the batch from the stream and from the render it keeps
// (the instance's tree and its shadow), so the next render diffs against
// no rows.

// StreamPush names fn, a Push in one of the instance's stream rows: a
// method of its component, which the instance then takes by name.
func (sc *scope) StreamPush(_ context.Context, fn any) (string, error) {
	in := sc.in
	in.effects++
	name, ok := ownerMethod(in, fn)
	if !ok {
		return "", diag.New("E0358", "a Push in a stream row calls %s, which isn't a method of the stream's owner", funcText(fn)).
			WithHelp("push a method of %s with the row's key: ui.JS.Push(Remove, m.ID)", typeName(in.typ))
	}
	allowStreamPush(in.typ, name)
	if pushable(in.comp, name) == nil {
		return "", diag.New("E0358", "a Push in a stream row calls %s.%s, which can't be pushed", typeName(in.typ), name).
			WithHelp("push an exported method that isn't a hook, Render or an Update")
	}
	return in.id + ":" + name, nil
}

// ownerMethod is the name of the method of the instance's component fn is
// a method value of.
func ownerMethod(in *instance, fn any) (string, bool) {
	v := reflect.ValueOf(fn)
	if in == nil || in.comp == nil || v.Kind() != reflect.Func || v.IsNil() {
		return "", false
	}
	f := runtime.FuncForPC(v.Pointer())
	if f == nil {
		return "", false
	}
	full, ok := strings.CutSuffix(f.Name(), "-fm") // a method value's wrapper
	if !ok {
		return "", false
	}
	t := in.typ
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	base, _, _ := strings.Cut(t.Name(), "[")
	for _, recv := range []string{".(*" + base, "." + base} {
		rest, ok := strings.CutPrefix(full, t.PkgPath()+recv)
		if !ok {
			continue
		}
		if strings.HasPrefix(rest, "[") { // a generic type's arguments
			if i := strings.IndexByte(rest, ']'); i >= 0 {
				rest = rest[i+1:]
			}
		}
		rest = strings.TrimPrefix(rest, ")")
		if name, ok := strings.CutPrefix(rest, "."); ok && !strings.Contains(name, ".") {
			return name, true
		}
	}
	return "", false
}

func funcText(fn any) string {
	v := reflect.ValueOf(fn)
	if v.Kind() == reflect.Func && !v.IsNil() {
		if f := runtime.FuncForPC(v.Pointer()); f != nil {
			return strings.TrimSuffix(f.Name(), "-fm")
		}
	}
	return reflect.TypeOf(fn).String()
}

var streamPushes sync.Map // reflect.Type (the component's pointer type) → *sync.Map of method names

// allowStreamPush lets instances of typ take pushes of the method name:
// its stream rows push it.
func allowStreamPush(typ reflect.Type, name string) {
	m, _ := streamPushes.LoadOrStore(typ, &sync.Map{})
	m.(*sync.Map).Store(name, true)
}

func streamPushAllowed(typ reflect.Type, name string) bool {
	m, ok := streamPushes.Load(typ)
	if !ok {
		return false
	}
	_, ok = m.(*sync.Map).Load(name)
	return ok
}

// StreamBatch is a stream's batch a reply carried, for a development
// toolbar: the instance, its component and what the batch did.
type StreamBatch struct {
	ID        string
	Component string
	ui.StreamBatch
}

// flushStreams drops the batches the reply just written carried: from the
// streams, and from the trees and shadows the session keeps.
func (s *Session) flushStreams() []StreamBatch {
	var out []StreamBatch
	for _, in := range s.order() {
		if in.comp == nil || !ui.HasStreams(in.comp) {
			continue
		}
		bs := ui.FlushStreams(in.comp)
		if len(bs) == 0 {
			continue
		}
		for _, b := range bs {
			out = append(out, StreamBatch{ID: in.id, Component: typeName(in.typ), StreamBatch: b})
		}
		if s.trees {
			in.emptied(emptyStreams)
		} else {
			in.own = ui.FlushedHTML(in.own)
		}
	}
	return out
}

// emptyStreams is t with the stream blocks emptied, and what each batch added
// taken off its container's attributes: t itself when it holds none, else a
// copy of the frames down to them. A stored tree is never changed.
func emptyStreams(t *ui.Tree) *ui.Tree {
	if t == nil {
		return nil
	}
	var dyn []any
	cur := func(i int) any {
		if dyn != nil {
			return dyn[i]
		}
		return t.Dyn[i]
	}
	set := func(i int, v any) {
		if dyn == nil {
			dyn = slices.Clone(t.Dyn)
		}
		dyn[i] = v
	}
	for i, d := range t.Dyn {
		switch d := d.(type) {
		case string:
			// a container in markup rendered as one piece: a component in
			// Go, a templ component
			if f := ui.FlushedHTML(d); f != d {
				set(i, f)
			}
		case *ui.Tree:
			if n := emptyStreams(d); n != d {
				set(i, n)
			}
		case *ui.TreeList:
			if !d.Stream {
				var items []*ui.Tree
				for j, it := range d.Items {
					if n := emptyStreams(it); n != it {
						if items == nil {
							items = slices.Clone(d.Items)
						}
						items[j] = n
					}
				}
				if items != nil {
					set(i, &ui.TreeList{Items: items, Keys: d.Keys, Temp: d.Temp})
				}
				continue
			}
			if len(d.Items) > 0 || d.Keys != nil {
				set(i, &ui.TreeList{Stream: true})
			}
			for j := i - 1; j >= 0; j-- {
				if a, ok := cur(j).(string); ok && ui.IsStreamAttrs(a) {
					set(j, ui.FlushedAttrs(a))
					break
				}
			}
		}
	}
	if dyn == nil {
		return t
	}
	return &ui.Tree{Key: t.Key, Statics: t.Statics, Dyn: dyn}
}

// emptied makes empty, applied to the instance's tree and to its shadow, their
// replacements.
func (in *instance) emptied(empty func(*ui.Tree) *ui.Tree) {
	t := empty(in.tree)
	sh := in.shadow
	switch {
	case sh == nil:
	case sh == in.tree:
		sh = t
	default:
		sh = empty(sh)
	}
	if t != in.tree {
		in.setTree(t)
	}
	if sh != in.shadow {
		in.setShadow(sh)
	}
}

// streamChecks reports, in development and tests, what the instances'
// streams queued past ui.StreamWarnAt rows.
func (s *Session) streamChecks() {
	if !s.verify && os.Getenv("VUKA_DEV_DIR") == "" && os.Getenv("VUKA_ENV") != "development" {
		return
	}
	for _, in := range s.byID {
		if in.comp == nil || !ui.HasStreams(in.comp) {
			continue
		}
		for _, w := range ui.StreamWarnings(in.comp) {
			log.Printf("live: %s (%s): %s", typeName(in.typ), in.id, w)
		}
	}
}
