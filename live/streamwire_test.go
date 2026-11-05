package live_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// WireFeed is a stream written as the compiler writes a .vuka.html file's
// (its rows a loop over Rows(), the container's attributes and rows from
// the stream's Block), with every kind of operation and a limit trimming
// the end.
type WireFeed struct {
	ui.Live
	Title string
	next  int `vuka:"-"`
	items ui.Stream[chatMsg]
}

func (f *WireFeed) LiveSchema(l *ui.LiveSchema) { l.Mount(f.mount) }

func (f *WireFeed) mount(context.Context) error {
	f.items.Limit(4).TrimEnd()
	f.items.Reset(msgs(1, 2, 3)...)
	f.next = 10
	return nil
}

func (f *WireFeed) item() chatMsg {
	f.next++
	return chatMsg{f.next, fmt.Sprintf("<m%d & \"q\">", f.next)}
}

func (f *WireFeed) Add()          { f.items.Insert(f.item()) }
func (f *WireFeed) Top()          { f.items.Prepend(f.item(), f.item()) }
func (f *WireFeed) At()           { f.items.InsertAt(1, f.item()) }
func (f *WireFeed) Clear()        { f.items.Reset() }
func (f *WireFeed) Reload()       { f.items.Reset(msgs(2, 3, 4, 5, 6)...) }
func (f *WireFeed) Rename()       { f.Title += "!" }
func (f *WireFeed) Remove(id int) { f.items.Delete(id) }
func (f *WireFeed) Edit(id int)   { f.items.Insert(chatMsg{id, "edited"}) }

func (f *WireFeed) Render() ui.Node {
	var rows []ui.Node
	for _, m := range f.items.Rows() {
		rows = append(rows, ui.HTML(`<li key={id}><span>{text}</span><button class="edit" onClick={edit}>e</button><button class="del" onClick={del}>x</button></li>`,
			m.ID, m.Text, ui.JS.Push(f.Edit, m.ID), ui.JS.Push(f.Remove, m.ID)))
	}
	b := f.items.Block(rows)
	return ui.HTML(`<main><h1>{title}</h1>
<button id="add" onClick={add}>add</button><button id="top" onClick={top}>top</button><button id="at" onClick={at}>at</button>
<button id="clear" onClick={clear}>clear</button><button id="reload" onClick={reload}>reload</button><button id="rename" onClick={rename}>rename</button>
<ul class="feed" {attrs...}>{rows}</ul></main>`, f.Title, f.Add, f.Top, f.At, f.Clear, f.Reload, f.Rename, b.Attrs(), b)
}

// The compact form of a container's attributes ({"st":…}, live.ExtStream)
// builds the very HTML the attribute form does, operation by operation —
// so the browser's batch rule, rows and counts are the same — and costs
// fewer bytes.
func TestStreamCompactWire(t *testing.T) {
	join := func(ext ...string) *livetest.Client {
		s := live.NewSession(context.Background(), func(context.Context) ui.Node {
			return ui.Component("feed", nil, &WireFeed{Title: "feed"})
		})
		c, err := livetest.JoinExt(s, 2, ext...)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	compact, attrs := join(live.ExtStream), join()
	steps := []string{"#add", "#top", "#at", `li[data-vk-row="1"] .edit`, `li[data-vk-row="13"] .del`, "#rename", "#add", "#reload", "#top", "#clear", "#add", "#rename"}
	for _, sel := range append([]string{""}, steps...) {
		for _, c := range []*livetest.Client{compact, attrs} {
			if sel == "" {
				continue
			}
			if err := c.Click(sel); err != nil {
				t.Fatalf("%s: %v", sel, err)
			}
		}
		if a, b := sameGen(compact.HTML()), sameGen(attrs.HTML()); a != b {
			t.Fatalf("after %q the compact form builds\n%s\nand the attributes\n%s", sel, a, b)
		}
		if a, b := keysOf(compact.Stream("c1", "items")), keysOf(attrs.Stream("c1", "items")); fmt.Sprint(a) != fmt.Sprint(b) {
			t.Fatalf("after %q rows %v and %v", sel, a, b)
		}
	}
	if got := keysOf(compact.Stream("c1", "items")); len(got) != 1 {
		t.Fatalf("rows %v", got)
	}
	t.Logf("compact %d bytes, attributes %d", compact.Bytes, attrs.Bytes)
	if compact.Bytes >= attrs.Bytes {
		t.Fatalf("the compact form took %d bytes, the attributes %d", compact.Bytes, attrs.Bytes)
	}
}

var genRe = regexp.MustCompile(`data-vk-batch="\w+\.`)

// sameGen is h with every stream's generation the same: a random one per
// mount.
func sameGen(h string) string { return genRe.ReplaceAllString(h, `data-vk-batch="g.`) }

// A container's attributes go compact only to a client joining with the
// extension, and never in v1; the compact dynamic carries the operations as
// plain JSON.
func TestStreamCompactNegotiated(t *testing.T) {
	for _, tc := range []struct {
		v       int
		ext     []string
		compact bool
	}{{2, []string{live.ExtStream}, true}, {2, nil, false}, {2, []string{"other"}, false}, {1, []string{live.ExtStream}, false}} {
		s := live.NewSession(context.Background(), func(context.Context) ui.Node {
			return ui.Component("feed", nil, &WireFeed{Title: "feed"})
		})
		m := live.ClientMessage{Type: live.Join, V: tc.v, Ext: tc.ext, Ref: 1}
		join := string(live.Marshal(s.Handle(m)))
		ev := string(live.Marshal(s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: 2, Event: ui.Event{Target: "c1:1", Type: "click"}})))
		for _, msg := range []string{join, ev} {
			if got := strings.Contains(msg, `{"st":{"n":"items"`); got != tc.compact {
				t.Fatalf("v%d ext %v: compact %v in %s", tc.v, tc.ext, got, msg)
			}
			if tc.compact && strings.Contains(msg, "data-vk-ops") {
				t.Fatalf("a compact reply carries data-vk-ops: %s", msg)
			}
		}
		if tc.compact && !strings.Contains(ev, `"o":[["s","12"],["s","11"]]`) {
			t.Fatalf("a prepend's operations: %s", ev)
		}
	}
}
