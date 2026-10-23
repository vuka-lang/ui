package ui

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestPageContext(t *testing.T) {
	ctx := WithPageCrumbs(WithPageTitle(context.Background(), "Users"), []Crumb{{Label: "Admin", Href: "/admin"}, {Label: "Users"}})
	v, err := PageContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := PageHeader{Title: "Users", Crumbs: []Crumb{{Label: "Admin", Href: "/admin"}, {Label: "Users"}}}
	if !reflect.DeepEqual(v.Page, want) {
		t.Errorf("PageContext = %+v, want %+v", v.Page, want)
	}
	if v, _ := PageContext(context.Background()); v.Page.Title != "" || v.Page.Crumbs != nil {
		t.Errorf("no page: %+v", v)
	}
}

// TestIslandBrowserVars: an island carries the variables BrowserVars gives
// it, as JSON, read when it renders.
func TestIslandBrowserVars(t *testing.T) {
	registerChart(t)
	defer func(f func(context.Context) any) { BrowserVars = f }(BrowserVars)
	BrowserVars = func(ctx context.Context) any { return map[string]any{"currency": "TZS"} }
	html := renderIsland(t, MountIsland("p#1", Chart{Title: "Sales"}))
	if !strings.Contains(html, `data-vk-vars="{&#34;currency&#34;:&#34;TZS&#34;}"`) {
		t.Errorf("no data-vk-vars:\n%s", html)
	}
	BrowserVars = nil
	if html := renderIsland(t, MountIsland("p#1", Chart{Title: "Sales"})); strings.Contains(html, "data-vk-vars") {
		t.Errorf("data-vk-vars without BrowserVars:\n%s", html)
	}
}

func TestReadVar(t *testing.T) {
	ReadVar(context.Background(), "user", 1) // no host: nothing
	ReadContext(context.Background())
	src := VarSourceOf(context.Background())
	if src != nil {
		t.Fatal("a source from nothing")
	}
}
