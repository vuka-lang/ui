package live

import (
	"context"
	"log"
	"os"
	"reflect"
	"runtime"
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
			emptyStreams(in.tree, in.shadow)
		} else {
			in.own = ui.FlushedHTML(in.own)
		}
	}
	return out
}

// emptyStreams empties the stream blocks of t, and of sh, its shadow, and
// takes what each batch added off its container's attributes.
func emptyStreams(t *ui.Tree, sh *sframe) {
	if t == nil {
		return
	}
	if sh != nil && (sh.key != t.Key || len(sh.d) != len(t.Dyn)) {
		sh = nil
	}
	for i, d := range t.Dyn {
		var sd any
		if sh != nil {
			sd = sh.d[i]
		}
		switch d := d.(type) {
		case string:
			// a container in markup rendered as one piece: a component in
			// Go, a templ component
			if f := ui.FlushedHTML(d); f != d {
				t.Dyn[i] = f
				if sh != nil {
					sh.d[i] = shadowDyn(f)
				}
			}
		case *ui.Tree:
			f, _ := sd.(*sframe)
			emptyStreams(d, f)
		case *ui.TreeList:
			if !d.Stream {
				l, _ := sd.(*slist)
				for j, it := range d.Items {
					var f *sframe
					if l != nil && j < len(l.items) {
						f = l.items[j]
					}
					emptyStreams(it, f)
				}
				continue
			}
			t.Dyn[i] = &ui.TreeList{Stream: true}
			if sh != nil {
				sh.d[i] = &slist{}
			}
			for j := i - 1; j >= 0; j-- {
				if a, ok := t.Dyn[j].(string); ok && ui.IsStreamAttrs(a) {
					a = ui.FlushedAttrs(a)
					t.Dyn[j] = a
					if sh != nil {
						sh.d[j] = shadowDyn(a)
					}
					break
				}
			}
		}
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
