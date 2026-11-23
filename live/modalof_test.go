package live_test

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/uigen"
)

type syncTarget struct {
	ID   int
	Name string
}

// syncPage is a page with one dialog over one state: rows open it with a
// value, its content reads the value.
type syncPage struct {
	ui.Live
	sync ui.State[*syncTarget]
	ran  []int
	// variants: an OnClose handler, no ID (built in Go, not by a tag)
	withClose, noID bool
	closed          int
}

func (p *syncPage) Closed() { p.closed++ }

func init() { uigen.SyncFields[syncPage]("sync") }

func (p *syncPage) Pick(id int) {
	uigen.SrvSet(p.sync, &syncTarget{ID: id, Name: "row" + string(rune('0'+id))})
}

func (p *syncPage) RunSync(id int) { p.ran = append(p.ran, id) }

func (p *syncPage) Render() ui.Node {
	m := ui.ModalOf[syncTarget]{
		For:   p.sync,
		ID:    "sync",
		Title: ui.Text("Sync"),
		Children: func(t *syncTarget) ui.Node {
			return ui.El("div", nil,
				ui.El("input", []ui.Attr{a("name", "name"), a("value", t.Name)}),
				ui.El("button", []ui.Attr{a("id", "run"), a("onClick", ui.On(ui.JS.Push(p.RunSync, t.ID)))}, ui.Text("Sync now")),
			)
		},
	}
	if p.withClose {
		m.OnClose = p.Closed
	}
	if p.noID {
		m.ID = ""
	}
	return ui.El("main", nil,
		ui.El("button", []ui.Attr{a("id", "pick1"), a("onClick", ui.On(ui.JS.Push(p.Pick, 1)))}, ui.Text("one")),
		m,
	)
}

// modalSession renders a syncPage and returns its session, the page, and
// the key of its sync state.
func modalSession(t *testing.T, variant ...func(*syncPage)) (*live.Session, *syncPage, string) {
	t.Helper()
	p := &syncPage{}
	for _, v := range variant {
		v(p)
	}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("page.sync", nil, p) })
	out, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	key := regexp.MustCompile(`c\d+:f[0-9a-z]+:1`).FindString(html.UnescapeString(out))
	if key == "" {
		t.Fatalf("no state key in\n%s", out)
	}
	return s, s.Component("c1:0").(*syncPage), key
}

// modalShown reports whether the render shows the modal's root, and the
// value of its name field (empty when the content isn't there).
func modalShown(t *testing.T, s *live.Session) (bool, string) {
	t.Helper()
	out := html.UnescapeString(s.HTML())
	root := regexp.MustCompile(`<div class="vk-modal-root"[^>]*>`).FindString(out)
	if root == "" {
		t.Fatalf("no modal root in\n%s", out)
	}
	name := regexp.MustCompile(`<input name="name" value="([^"]*)"`).FindStringSubmatch(out)
	if name == nil {
		return !strings.Contains(root, " hidden"), ""
	}
	return !strings.Contains(root, " hidden"), name[1]
}

// carry is an event's dirty write of the sync state: v as JSON, write n.
func carry(key, v string, n uint64) map[string]ui.StateWrite {
	return map[string]ui.StateWrite{key: {V: json.RawMessage(v), N: n}}
}

// chain is the JS commands of the first element of the render matching
// sel (a regexp of its opening tag), for event.
func chain(t *testing.T, s *live.Session, sel, event string) [][]any {
	t.Helper()
	out := s.HTML()
	tag := regexp.MustCompile(sel).FindString(out)
	m := regexp.MustCompile(`data-vk-js-` + event + `="([^"]*)"`).FindStringSubmatch(tag)
	if m == nil {
		t.Fatalf("no %s commands on %s in\n%s", event, sel, out)
	}
	var steps [][]any
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &steps); err != nil {
		t.Fatal(err)
	}
	return steps
}

func ops(steps [][]any) []string {
	var out []string
	for _, s := range steps {
		out = append(out, s[0].(string))
	}
	return out
}

// TestModalOfHandlerOpen: a handler's write of a value opens the modal, its
// content rendered for the value; closed, the page carries no content.
func TestModalOfHandlerOpen(t *testing.T) {
	s, p, _ := modalSession(t)
	if shown, name := modalShown(t, s); shown || name != "" {
		t.Fatalf("a nil state shows the modal (%v) or its content (%q)", shown, name)
	}
	if _, err := s.Event(ui.Event{Target: p.ID() + ":0", Args: []json.RawMessage{json.RawMessage(`1`)}}); err != nil {
		t.Fatal(err)
	}
	if shown, name := modalShown(t, s); !shown || name != "row1" {
		t.Fatalf("after Pick(1): shown %v, name %q; want shown, row1", shown, name)
	}
	push := chain(t, s, `<button id="run"[^>]*>`, "click")[0][1].(map[string]any)
	if _, err := s.Event(ui.Event{Target: push["h"].(string), Args: []json.RawMessage{json.RawMessage(`1`)}}); err != nil {
		t.Fatal(err)
	}
	if len(p.ran) != 1 || p.ran[0] != 1 {
		t.Fatalf("the content's handler ran with %v, want [1]", p.ran)
	}
}

// TestModalOfMarkupOpen: a browser-only write opens the modal at once (its
// root follows the state) and its region asks for the content, the write
// riding that request, so the reply renders the content for the value.
func TestModalOfMarkupOpen(t *testing.T) {
	s, p, key := modalSession(t)
	out := html.UnescapeString(s.HTML())
	root := regexp.MustCompile(`<div class="vk-modal-root"[^>]*>`).FindString(out)
	if !strings.Contains(root, `data-vk-v="["!=",{"i":null,"s":"`+key+`","sy":1},null]"`) {
		t.Fatalf("the root doesn't follow the state in the browser:\n%s", root)
	}
	region := regexp.MustCompile(`data-vk-on-vkinit="([^"]+)"`).FindStringSubmatch(out)
	if region == nil || !strings.Contains(out, "data-vk-each") {
		t.Fatalf("no region asking for the content each time it shows:\n%s", out)
	}
	if _, err := s.Event(ui.Event{Target: region[1], Type: "vkinit", States: carry(key, `{"ID":2,"Name":"two"}`, 1)}); err != nil {
		t.Fatal(err)
	}
	if shown, name := modalShown(t, s); !shown || name != "two" {
		t.Fatalf("after the region's ask: shown %v, name %q; want shown, two", shown, name)
	}
	if got := p.sync.Get(); got == nil || got.ID != 2 {
		t.Fatalf("the server's value is %+v, want ID 2", got)
	}
}

// TestModalOfClose: the close button, the backdrop and Escape write nil in
// the browser first, give the focus back, and tell the server only once the
// fade is over — the markup, the last value's, stays as it was meanwhile;
// the server's render then removes the content.
func TestModalOfClose(t *testing.T) {
	s, p, key := modalSession(t)
	if _, err := s.Event(ui.Event{Target: p.ID() + ":0", Args: []json.RawMessage{json.RawMessage(`1`)}}); err != nil {
		t.Fatal(err)
	}
	fade := regexp.MustCompile(`"ms":(\d+)`).FindStringSubmatch(html.UnescapeString(s.HTML()))
	for _, c := range []struct{ sel, event string }{
		{`<button type="button" class="vk-modal__close"[^>]*>`, "click"},
		{`<div class="vk-modal-backdrop"[^>]*>`, "click"},
		{`<div id="sync" class="vk-modal"[^>]*>`, "keydown"},
	} {
		steps := chain(t, s, c.sel, c.event)
		got := strings.Join(ops(steps), " ")
		want := "state_set pop_focus debounce reload"
		if c.event == "keydown" {
			want = "key " + want
		}
		if got != want {
			t.Errorf("%s %s: %s, want %s", c.sel, c.event, got, want)
			continue
		}
		set := steps[len(steps)-4][1].(map[string]any)
		if set["s"] != key || set["v"] != nil {
			t.Errorf("%s: the write is %v, want nil into %s", c.sel, set, key)
		}
		if ms := steps[len(steps)-2][1].(map[string]any)["ms"]; fade == nil || fmt.Sprint(ms) != fade[1] {
			t.Errorf("%s: the server is told after %vms, not once the %v fade is over", c.sel, ms, fade)
		}
	}
	// the reload the close runs once faded carries the browser's nil
	if _, err := s.Event(ui.Event{Target: p.ID() + ":" + live.ReloadEvent, Type: live.ReloadEvent, States: carry(key, `null`, 1)}); err != nil {
		t.Fatal(err)
	}
	if shown, name := modalShown(t, s); shown || name != "" {
		t.Fatalf("after the close's reload: shown %v, name %q; want closed, no content", shown, name)
	}
}

// TestModalOfFreshPerOpen: each open is a new write, so a field in the
// content shows the new value's, closed in between or not.
func TestModalOfFreshPerOpen(t *testing.T) {
	s, p, key := modalSession(t)
	region := regexp.MustCompile(`data-vk-on-vkinit="([^"]+)"`).FindStringSubmatch(html.UnescapeString(s.HTML()))[1]
	steps := []struct {
		target, typ, v, want string
	}{
		{region, "vkinit", `{"ID":2,"Name":"two"}`, "two"},
		{p.ID() + ":" + live.ReloadEvent, live.ReloadEvent, `null`, ""},
		{region, "vkinit", `{"ID":3,"Name":"three"}`, "three"},
		{region, "vkinit", `{"ID":4,"Name":"four"}`, "four"}, // opened over an open one
	}
	for i, st := range steps {
		if _, err := s.Event(ui.Event{Target: st.target, Type: st.typ, States: carry(key, st.v, uint64(i+1))}); err != nil {
			t.Fatal(err)
		}
		if shown, name := modalShown(t, s); shown != (st.want != "") || name != st.want {
			t.Fatalf("step %d: shown %v, name %q; want %q", i, shown, name, st.want)
		}
	}
}

// TestModalOfFocus: each time it shows the modal remembers the focus and
// moves it into the dialog; its own closes give it back.
func TestModalOfFocus(t *testing.T) {
	s, _, _ := modalSession(t)
	if got := strings.Join(ops(chain(t, s, `<vk-lazy[^>]*>`, "vkinit")), " "); got != "push_focus focus focus_first" {
		t.Errorf("showing runs %s, want push_focus focus focus_first", got)
	}
}

// TestModalOfLabelled: the dialog is labelled by its title, by ids its tag
// gives it; the page carries no props attribute (no script reads them) and
// none of the content while closed.
func TestModalOfLabelled(t *testing.T) {
	s, _, _ := modalSession(t)
	out := html.UnescapeString(s.HTML())
	for _, want := range []string{`role="dialog" aria-modal="true" aria-labelledby="sync-title"`, `<h2 class="vk-modal__title" id="sync-title">Sync</h2>`, `<vk-teleport id="sync-layer" data-vk-teleport="body"`} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
	if strings.Contains(out, "data-vk-p") || strings.Contains(out, "Sync now") {
		t.Errorf("props or the content reached a closed modal's page:\n%s", out)
	}
}

// TestModalOfCloseVariants: with OnClose the faded-out close calls it (the
// write riding the call); a modal built in Go without an ID closes in the
// browser alone, the server told by the page's next event.
func TestModalOfCloseVariants(t *testing.T) {
	s, p, key := modalSession(t, func(p *syncPage) { p.withClose = true })
	steps := chain(t, s, `<button type="button" class="vk-modal__close"[^>]*>`, "click")
	if got := strings.Join(ops(steps), " "); got != "state_set pop_focus debounce push" {
		t.Fatalf("OnClose: %s, want state_set pop_focus debounce push", got)
	}
	if _, err := s.Event(ui.Event{Target: steps[3][1].(map[string]any)["h"].(string), States: carry(key, `null`, 1)}); err != nil {
		t.Fatal(err)
	}
	if shown, _ := modalShown(t, s); shown || p.closed != 1 {
		t.Fatalf("after OnClose: shown %v, called %d times", shown, p.closed)
	}

	s, _, _ = modalSession(t, func(p *syncPage) { p.noID = true })
	if got := strings.Join(ops(chain(t, s, `<button type="button" class="vk-modal__close"[^>]*>`, "click")), " "); got != "state_set pop_focus" {
		t.Errorf("no ID: %s, want state_set pop_focus", got)
	}
	if out := s.HTML(); strings.Contains(out, "aria-labelledby") || strings.Contains(out, `id="-`) {
		t.Errorf("an empty ID wrote ids:\n%s", out)
	}
}
