package live_test

import (
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/uigen"
)

// searchBox is a live component with a synced state (one state, §12): its
// Submit method reads q, so the compiler registers the field synced and the
// browser's writes of q ride each event.
type searchBox struct {
	ui.Live
	q    ui.State[string]
	open ui.State[bool] // never read by Go code: browser-local
	got  []string
}

func init() { uigen.SyncFields[searchBox]("q") }

func (b *searchBox) Submit() { b.got = append(b.got, b.q.Get()) }

func (b *searchBox) Clear() { uigen.SrvSet(b.q, "") }

func (b *searchBox) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("input", []ui.Attr{a("value", ui.BindExpr(b.q.Get(), `{"$":0}`, b.q))}),
		ui.El("ul", []ui.Attr{a("hidden", ui.BindExpr(!b.open.Get(), `["!",{"$":0}]`, b.open))}),
		ui.El("button", []ui.Attr{a("onClick", ui.On(b.Submit))}, ui.Text("go")),
		ui.El("button", []ui.Attr{a("onClick", ui.On(b.Clear))}, ui.Text("clear")),
	)
}

func syncSession(t *testing.T) (*live.Session, *searchBox, string) {
	t.Helper()
	b := &searchBox{}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("page.search", nil, b)
	})
	out, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	key := regexp.MustCompile(`c\d+:f[0-9a-z]+:1`)
	keys := key.FindAllString(html.UnescapeString(out), -1)
	if len(keys) == 0 {
		t.Fatalf("no state key in render:\n%s", out)
	}
	id, _, _ := strings.Cut(keys[0], ":")
	comp := s.Component(id + ":0")
	if comp == nil {
		t.Fatalf("no component for %s", keys[0])
	}
	return s, comp.(*searchBox), keys[0]
}

// TestSyncedRef: a synced state's reference carries "sy"; a browser-local
// one doesn't.
func TestSyncedRef(t *testing.T) {
	s, _, qKey := syncSession(t)
	out := html.UnescapeString(s.HTML())
	if !strings.Contains(out, `"s":"`+qKey+`","sy":1`) && !strings.Contains(out, `"sy":1`) {
		t.Errorf("q's reference isn't marked synced:\n%s", out)
	}
	if strings.Count(out, `"sy":1`) != 1 {
		t.Errorf("open is marked synced too (browser-local states carry no sy):\n%s", out)
	}
}

// TestEventCarriesState: an event carrying a dirty synced state applies it
// before the handler, so the handler reads the browser's truth.
func TestEventCarriesState(t *testing.T) {
	s, b, qKey := syncSession(t)
	_, err := s.Event(ui.Event{
		Target: b.ID() + ":0",
		States: map[string]ui.StateWrite{qKey: {V: json.RawMessage(`"orders"`), N: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.got) != 1 || b.got[0] != "orders" {
		t.Fatalf("the handler read %q, want [orders]", b.got)
	}
	// The echo: the render's reference carries the applied value, its
	// version and the write number, so the browser clears its dirty mark.
	out := html.UnescapeString(s.HTML())
	if !regexp.MustCompile(`"bn":1[,}]`).MatchString(out) || !strings.Contains(out, `"i":"orders"`) {
		t.Errorf("the echo doesn't carry bn and the value:\n%s", out)
	}
}

// TestStaleWriteSkipped: a write number at or below the last applied one is
// a repeated carry, not a newer write.
func TestStaleWriteSkipped(t *testing.T) {
	s, b, qKey := syncSession(t)
	ev := func(v string, n uint64) {
		if _, err := s.Event(ui.Event{Target: b.ID() + ":0", States: map[string]ui.StateWrite{qKey: {V: json.RawMessage(`"` + v + `"`), N: n}}}); err != nil {
			t.Fatal(err)
		}
	}
	ev("a", 2)
	ev("stale", 1)
	ev("stale", 2)
	if len(b.got) != 3 || b.got[1] != "a" || b.got[2] != "a" {
		t.Fatalf("stale writes applied: %q", b.got)
	}
}

// TestServerWriteSupersedes: a handler's own write (SrvSet) after the
// browser's base rides the render marked "sw", which the browser adopts
// whatever it wrote since.
func TestServerWriteSupersedes(t *testing.T) {
	s, b, qKey := syncSession(t)
	if _, err := s.Event(ui.Event{
		Target: b.ID() + ":1", // Clear: sets q to ""
		States: map[string]ui.StateWrite{qKey: {V: json.RawMessage(`"draft"`), N: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := b.q.Get(); got != "" {
		t.Fatalf("q is %q after Clear, want \"\"", got)
	}
	out := html.UnescapeString(s.HTML())
	if !strings.Contains(out, `"sw":1`) || !strings.Contains(out, `"i":""`) {
		t.Errorf("the server write isn't marked sw with its value:\n%s", out)
	}
}

// TestUnsyncedWriteRefused: a write naming a browser-local state is refused
// — the browser never sends one, so it is a forged message.
func TestUnsyncedWriteRefused(t *testing.T) {
	s, b, qKey := syncSession(t)
	openKey := ""
	for _, m := range regexp.MustCompile(`c\d+:f[0-9a-z]+:1`).FindAllString(html.UnescapeString(s.HTML()), -1) {
		if m != qKey {
			openKey = m
		}
	}
	if openKey == "" {
		t.Fatal("no browser-local state key found")
	}
	if _, err := s.Event(ui.Event{Target: b.ID() + ":0", States: map[string]ui.StateWrite{openKey: {V: json.RawMessage(`true`), N: 1}}}); err == nil {
		t.Fatal("a write to a browser-local state was applied")
	}
}

// TestBadValueRefused: a carried value that isn't the state's type is an
// error, not a write.
func TestBadValueRefused(t *testing.T) {
	s, b, qKey := syncSession(t)
	if _, err := s.Event(ui.Event{Target: b.ID() + ":0", States: map[string]ui.StateWrite{qKey: {V: json.RawMessage(`{"nope":1}`), N: 1}}}); err == nil {
		t.Fatal("a value of the wrong type was applied")
	}
}

// trackedSearch is tracked (its plain field is an Assign) with a synced
// state: an applied write alone must render it again.
type trackedSearch struct {
	ui.Live
	q ui.State[string]
	n uigen.Assign[int]
}

func init() { uigen.SyncFields[trackedSearch]("q") }

func (c *trackedSearch) Poke() {}

func (c *trackedSearch) Read() { _ = c.q.Get() }

func (c *trackedSearch) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("span", []ui.Attr{a("data-q", ui.BindExpr(c.q.Get(), `{"$":0}`, c.q))}, ui.Text(c.q.Get())),
		ui.El("button", []ui.Attr{a("onClick", ui.On(c.Poke))}, ui.Text("poke")),
	)
}

// TestSyncedWriteRendersTracked: a tracked component renders again when an
// event applied a synced write, and the patch carries the new value.
func TestSyncedWriteRendersTracked(t *testing.T) {
	c := &trackedSearch{}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.Component("page.t", nil, c)
	})
	out, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	qKey := regexp.MustCompile(`c\d+:f[0-9a-z]+:1`).FindString(html.UnescapeString(out))
	if qKey == "" {
		t.Fatalf("no state key:\n%s", out)
	}
	id, _, _ := strings.Cut(qKey, ":")
	p, err := s.Event(ui.Event{Target: id + ":0", States: map[string]ui.StateWrite{qKey: {V: json.RawMessage(`"pears"`), N: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Updates) == 0 || !strings.Contains(p.Updates[0].HTML, "pears") {
		t.Fatalf("the patch doesn't carry the applied value: %+v", p.Updates)
	}
	// Unchanged, the next event keeps the render (no update).
	p, err = s.Event(ui.Event{Target: id + ":0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Updates) != 0 {
		t.Fatalf("an event with no writes re-rendered a tracked component: %+v", p.Updates)
	}
}
