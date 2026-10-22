package ui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/vuka/v2"
)

type layoutPage struct {
	Q string `json:"q"`
}

type strategyPage struct{}

type badLayoutPage struct{}

type ctxKey struct{}

func wrap(tag string) func(Node) Node {
	return func(page Node) Node { return El(tag, nil, page) }
}

func TestIslandLayout(t *testing.T) {
	who := func(ctx context.Context, page Node) Node {
		u, _ := ctx.Value(ctxKey{}).(string)
		return El("section", []Attr{{"data-user", u}, {"data-title", PageTitle(ctx)}}, page)
	}
	Island("islands/L.vue", Layout(wrap("main"), who), Layout(wrap("div")))(vuka.TypeOf[layoutPage]("app.layoutPage"))
	l := IslandLayout(reflect.TypeFor[*layoutPage]())
	if l == nil {
		t.Fatal("no layout")
	}
	ctx := WithPageTitle(context.WithValue(context.Background(), ctxKey{}, "ann"), "Orders")
	s := renderIsland(t, l(ctx, Text("page")))
	if want := `<main><section data-user="ann" data-title="Orders"><div>page</div></section></main>`; s != want {
		t.Errorf("got %s\nwant %s", s, want)
	}
	if info := lookupIsland(reflect.TypeFor[layoutPage]()); info.Path != "islands/L.vue" || info.Framework != "vue" {
		t.Errorf("path %q framework %q", info.Path, info.Framework)
	}
	if IslandLayout(reflect.TypeFor[Chart]()) != nil {
		t.Error("an island without Layout has a layout")
	}
}

func TestIslandStrategyOption(t *testing.T) {
	Island(Strategy("load"))(vuka.TypeOf[strategyPage]("app.strategyPage"))
	info := lookupIsland(reflect.TypeFor[strategyPage]())
	if info.Strategy != "load" || info.Path != "" {
		t.Errorf("strategy %q path %q", info.Strategy, info.Path)
	}
	s := renderIsland(t, MountIsland("s", strategyPage{}))
	if !strings.Contains(s, `data-vk-client="load"`) {
		t.Errorf("not loaded: %s", s)
	}
}

func TestIslandBadOptions(t *testing.T) {
	var nilLayout func(Node) Node
	for _, args := range [][]any{
		{Layout("x")},
		{Layout(nilLayout)},
		{"a.vue", "load", "third"},
		{42},
	} {
		Island(args...)(vuka.TypeOf[badLayoutPage]("app.badLayoutPage"))
		if err := CheckIsland(reflect.TypeFor[badLayoutPage]()); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
}

type regBare struct {
	N int `json:"n"`
}

type regOpts struct{}

type regOld struct{}

// RegisterIsland takes what Island takes; its older forms keep working.
func TestRegisterIsland(t *testing.T) {
	RegisterIsland[regBare]()
	RegisterIsland[regOpts]("islands/Opts.tsx", Strategy("idle"), Layout(wrap("main")))
	RegisterIsland[regOld]("./islands/Old.vue", "load")
	for _, c := range []struct {
		t                               reflect.Type
		name, path, framework, strategy string
		layout                          bool
	}{
		{reflect.TypeFor[regBare](), "regBare", "", "", "", false},
		{reflect.TypeFor[regOpts](), "regOpts", "islands/Opts.tsx", "react", "idle", true},
		{reflect.TypeFor[regOld](), "regOld", "./islands/Old.vue", "vue", "load", false},
	} {
		info := lookupIsland(c.t)
		if info == nil {
			t.Fatalf("%s not registered", c.t)
		}
		if info.Name != c.name || info.Path != c.path || info.Framework != c.framework || info.Strategy != c.strategy || (IslandLayout(c.t) != nil) != c.layout {
			t.Errorf("%s: %+v, layout %v", c.t, info, IslandLayout(c.t) != nil)
		}
	}
	s := renderIsland(t, MountIsland("b", regBare{N: 2}))
	if !strings.Contains(s, `data-vk-island="regBare"`) || !strings.Contains(s, "&#34;n&#34;:2") {
		t.Errorf("bare island host: %s", s)
	}
}
