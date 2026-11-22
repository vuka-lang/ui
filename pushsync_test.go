package ui_test

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"reflect"
	"strconv"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/uigen"
)

// filterPage is a plain (non-live) page with a synced state: its Search
// method reads q over an HTTP push, so the browser's dirty value rides the
// request and the reply echoes it.
type filterPage struct {
	q    ui.State[string]
	open ui.State[bool] // browser-local: never read by Go code
	got  string
}

func init() { uigen.SyncFields[filterPage]("q") }

// qSite is q's site: the key a static render mints is "<site>:<n>".
func qSite(t *testing.T) string {
	t.Helper()
	p := &filterPage{}
	if err := ui.ApplyPushStates(p, nil); err != nil {
		t.Fatal(err)
	}
	out := ui.CollectPushStates(p, nil)
	if out != nil {
		t.Fatalf("an untouched page collects nothing, got %v", out)
	}
	// SrvSet to learn the site from the collected key.
	uigen.SrvSet(p.FieldQ(), "")
	for k := range ui.CollectPushStates(p, nil) {
		return k
	}
	t.Fatal("no synced state collected")
	return ""
}

// FieldQ exposes q to the test (a .vuka.html file's generated code reaches
// its own fields directly).
func (p *filterPage) FieldQ() ui.State[string] { return p.q }

func (p *filterPage) FieldOpen() ui.State[bool] { return p.open }

// TestApplyPushStates: a carried write lands on the fresh receiver before
// the method runs, matched by site, and the reply echoes it versioned.
func TestApplyPushStates(t *testing.T) {
	site := qSite(t)
	p := &filterPage{}
	key := site + ":7"
	writes := map[string]ui.StateWrite{key: {V: json.RawMessage(`"orders"`), N: 2}}
	if err := ui.ApplyPushStates(p, writes); err != nil {
		t.Fatal(err)
	}
	if got := p.FieldQ().Get(); got != "orders" {
		t.Fatalf("the method would read %q, want orders", got)
	}
	out := ui.CollectPushStates(p, writes)
	e, ok := out[key].(map[string]any)
	if !ok {
		t.Fatalf("the echo isn't under the carried key: %v", out)
	}
	if e["v"] != "orders" || e["bn"] != uint64(2) || e["ver"] == uint64(0) {
		t.Fatalf("echo %v", e)
	}
}

// TestPushServerWrite: a method's own write (SrvSet) rides the reply marked
// sw, under its site when the request carried nothing for it.
func TestPushServerWrite(t *testing.T) {
	site := qSite(t)
	p := &filterPage{}
	if err := ui.ApplyPushStates(p, nil); err != nil {
		t.Fatal(err)
	}
	uigen.SrvSet(p.FieldQ(), "reset")
	out := ui.CollectPushStates(p, nil)
	e, ok := out[site].(map[string]any)
	if !ok {
		t.Fatalf("the server write isn't under its site: %v", out)
	}
	if e["v"] != "reset" || e["sw"] != 1 {
		t.Fatalf("server write %v", e)
	}
}

// TestPushLocalRefused: a write naming a browser-local state is refused —
// the browser never sends one — and one naming no state of the page is
// dropped.
func TestPushLocalRefused(t *testing.T) {
	p := &filterPage{}
	// open's site, as initStates mints it.
	tp := reflect.TypeOf(*p)
	h := fnv.New64a()
	fmt.Fprintf(h, "%s.%s.%s", tp.PkgPath(), tp.Name(), "open")
	openSite := "f" + strconv.FormatUint(h.Sum64()&0xffffffffff, 36)
	err := ui.ApplyPushStates(p, map[string]ui.StateWrite{openSite + ":1": {V: json.RawMessage(`true`), N: 1}})
	if err == nil {
		t.Fatal("a write to a browser-local state was applied")
	}
	q := &filterPage{}
	if err := ui.ApplyPushStates(q, map[string]ui.StateWrite{"nosuch:1": {V: json.RawMessage(`true`), N: 1}}); err != nil {
		t.Fatal(err)
	}
	if q.FieldOpen().Get() {
		t.Fatal("a dropped write landed")
	}
}

// TestPushStale: a repeated carry (same write number) applies once.
func TestPushStale(t *testing.T) {
	site := qSite(t)
	p := &filterPage{}
	key := site + ":1"
	w := map[string]ui.StateWrite{key: {V: json.RawMessage(`"a"`), N: 3}}
	if err := ui.ApplyPushStates(p, w); err != nil {
		t.Fatal(err)
	}
	uigen.SrvSet(p.FieldQ(), "server") // ver moves on
	stale := map[string]ui.StateWrite{key: {V: json.RawMessage(`"a"`), N: 3}}
	if err := ui.ApplyPushStates(p, stale); err != nil {
		t.Fatal(err)
	}
	if got := p.FieldQ().Get(); got != "server" {
		t.Fatalf("a stale carry overwrote the server's write: %q", got)
	}
}
