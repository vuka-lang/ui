package ui

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vuka-lang/vuka"
)

type ordersPage struct {
	Live
	Q       string         `url:"q" bind:"q"`
	Stats   map[string]int `json:"stats" defer:""`
	Rows    []string       `json:"rows" defer:"lists"`
	Recent  []string       `json:"recent,omitempty" defer:"lists"`
	Title   string         `json:"title"`
	running *atomic.Int32
	fail    bool
}

func (p *ordersPage) Render() Node { return IslandPage(p) }

func (p *ordersPage) LoadStats(ctx context.Context) error {
	if p.running != nil {
		p.running.Add(1)
		time.Sleep(20 * time.Millisecond)
	}
	if p.fail {
		return publicError{errors.New("stats are down")}
	}
	p.Stats = map[string]int{"open": 3}
	p.Title = "changed by a loader" // not the group's: dropped
	return nil
}

func (p *ordersPage) LoadLists(ctx context.Context) error {
	if p.running != nil {
		p.running.Add(1)
		time.Sleep(20 * time.Millisecond)
	}
	p.Rows = []string{"a", "b"}
	return nil
}

func (p *ordersPage) Cancel(id int64) error { return nil }

func init() { Island("islands/Orders.vue")(vuka.TypeOf[ordersPage]("app.ordersPage")) }

func TestDeferFields(t *testing.T) {
	fields, err := IslandFields(reflect.TypeFor[*ordersPage]())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range fields {
		got = append(got, f.Handle+":"+f.Defer)
	}
	if want := "q: stats:Stats rows:Lists recent:Lists title:"; strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
	if err := CheckIsland(reflect.TypeFor[*ordersPage]()); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range IslandMethods(reflect.TypeFor[*ordersPage]()) {
		names = append(names, m.Name)
	}
	if strings.Join(names, " ") != "cancel" {
		t.Errorf("callable: %v (a loader isn't)", names)
	}
}

type deferNoLoader struct {
	Stats int `defer:""`
}
type deferBind struct {
	Q string `bind:"q" defer:""`
}
type deferBadName struct {
	Q string `defer:"two words"`
}
type deferBadLoader struct {
	Stats int `defer:""`
}

func (p *deferBadLoader) LoadStats(n int) error { return nil }

func TestCheckIslandDefer(t *testing.T) {
	for _, c := range []struct {
		t    reflect.Type
		want string
	}{
		{reflect.TypeFor[deferNoLoader](), "deferNoLoader.Stats is deferred: its loader is a method LoadStats(ctx context.Context) error"},
		{reflect.TypeFor[deferBind](), "drop bind: or defer:"},
		{reflect.TypeFor[deferBadName](), `defer:"two words": a group is a name`},
		{reflect.TypeFor[deferBadLoader](), "LoadStats is LoadStats() or LoadStats(ctx context.Context)"},
	} {
		err := CheckIsland(c.t)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want %q", c.t, err, c.want)
		}
	}
}

func islandAttr(t *testing.T, s, name string) string {
	t.Helper()
	i := strings.Index(s, " "+name+`="`)
	if i < 0 {
		return ""
	}
	rest := s[i+len(name)+3:]
	return html.UnescapeString(rest[:strings.IndexByte(rest, '"')])
}

func TestDeferredFirstRender(t *testing.T) {
	p := &ordersPage{Q: "x", Title: "Orders", Stats: map[string]int{"stale": 1}}
	s := renderIsland(t, IslandPage(p))
	if got := islandAttr(t, s, "data-vk-props"); got != `{"Q":"x","title":"Orders"}` {
		t.Errorf("props %s", got)
	}
	if got := islandAttr(t, s, "data-vk-defer"); got != `{"e":{},"p":["stats","rows","recent"]}` {
		t.Errorf("defer %s", got)
	}
	if got := islandAttr(t, s, "data-vk-fields"); !strings.Contains(got, `"stats":{"p":1,"w":"stats"}`) {
		t.Errorf("fields %s", got)
	}
}

func TestDeferredLoad(t *testing.T) {
	var running atomic.Int32
	p := &ordersPage{Title: "Orders", running: &running}
	ds := StartDeferred(p)
	if len(ds) != 2 || ds[0].Group != "Stats" || ds[1].Group != "Lists" {
		t.Fatalf("loads %+v", ds)
	}
	if again := StartDeferred(p); len(again) != 0 {
		t.Errorf("started twice: %d", len(again))
	}
	done := make(chan struct{})
	start := time.Now()
	for _, d := range ds {
		go func() { d.Run(context.Background()); done <- struct{}{} }()
	}
	<-done
	<-done
	if el := time.Since(start); el > 35*time.Millisecond {
		t.Errorf("groups ran one after another: %v", el)
	}
	if p.Stats != nil || p.Rows != nil {
		t.Errorf("Run set the page: %+v", p)
	}
	for _, d := range ds {
		d.Apply(nil)
	}
	if p.Stats["open"] != 3 || len(p.Rows) != 2 || p.Title != "Orders" {
		t.Errorf("applied: %+v", p)
	}
	s := renderIsland(t, IslandPage(p))
	if got := islandAttr(t, s, "data-vk-props"); got != `{"Q":"","recent":null,"rows":["a","b"],"stats":{"open":3},"title":"Orders"}` {
		t.Errorf("props %s", got)
	}
	if got := islandAttr(t, s, "data-vk-defer"); got != `{"e":{},"p":[]}` {
		t.Errorf("defer %s", got)
	}
}

func TestDeferredError(t *testing.T) {
	p := &ordersPage{fail: true}
	for _, d := range StartDeferred(p) {
		d.Run(context.Background())
		d.Apply(nil)
	}
	s := renderIsland(t, IslandPage(p))
	if got := islandAttr(t, s, "data-vk-defer"); got != `{"e":{"stats":{"message":"stats are down"}},"p":[]}` {
		t.Errorf("defer %s", got)
	}
	if got := islandAttr(t, s, "data-vk-props"); strings.Contains(got, "stats") || !strings.Contains(got, `"rows":["a","b"]`) {
		t.Errorf("props %s", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := &ordersPage{}
	for _, d := range StartDeferred(q) {
		if err := d.Run(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("a done ctx: %v", err)
		}
	}
}

func TestDeferredStateless(t *testing.T) {
	type sl = ordersPage
	p := &sl{Title: "Orders"}
	first, err := PropsJSON(context.Background(), *p)
	if err != nil || string(first) != `{"Q":"","title":"Orders"}` {
		t.Fatalf("first props %s %v", first, err)
	}
	l := LoadsFrom(reflect.TypeFor[sl](), first)
	l.Load(context.Background(), reflect.ValueOf(p), nil)
	after, _ := PropsJSON(context.Background(), *p, WithLoads(l))
	if !strings.Contains(string(after), `"stats":{"open":3}`) || !strings.Contains(string(after), `"recent":null`) {
		t.Errorf("after %s", after)
	}
	l2 := LoadsFrom(reflect.TypeFor[sl](), after)
	if d, _ := json.Marshal(l2.JSON(reflect.TypeFor[sl]())); string(d) != `{"e":{},"p":[]}` {
		t.Errorf("from loaded props %s", d)
	}
	if s := renderIsland(t, IslandPage(p, CallAt("/orders", "sig"), WithLoads(l2))); islandAttr(t, s, "data-vk-props") != string(after) {
		t.Errorf("rendered props differ from signed:\n%s\n%s", islandAttr(t, s, "data-vk-props"), after)
	}
}

type changedPage struct {
	Q    string `bind:"q"`
	Sort string `json:"sort" bind:""`
	got  Changed
	ctx  bool
}

func (p *changedPage) Changed(ctx context.Context, c Changed) error {
	p.got, p.ctx = c, ctx != nil
	return nil
}

type changedOnly struct {
	Q   string `bind:"q"`
	got []string
}

func (p *changedOnly) Changed(c Changed) { p.got = c.Names() }

func TestChangedNames(t *testing.T) {
	p := &changedPage{}
	if err := CheckIsland(reflect.TypeFor[*changedPage]()); err != nil {
		t.Fatal(err)
	}
	if err := SetBound(context.Background(), reflect.ValueOf(p), json.RawMessage(`{"sort":"a","Q":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if !p.ctx || strings.Join(p.got.Names(), ",") != "Q,sort" {
		t.Errorf("names %v", p.got.Names())
	}
	for _, n := range []string{"Q", "q", "sort", "Sort"} {
		if !p.got.Has(n) {
			t.Errorf("Has(%q) false", n)
		}
	}
	if p.got.Has("rows") {
		t.Error("Has(rows)")
	}
	o := &changedOnly{}
	if err := SetBound(context.Background(), reflect.ValueOf(o), json.RawMessage(`{"Q":"x"}`)); err != nil || strings.Join(o.got, ",") != "Q" {
		t.Errorf("Changed(changed): %v %v", o.got, err)
	}
}
