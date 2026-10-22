package ui

import (
	"context"
	"github.com/vuka-lang/ui/v2/internal/track"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/vuka/v2"
)

type Chart struct {
	Series   []int  `json:"series"`
	Title    string `json:"title"`
	Children Node   // the slot — never serialized into data-vk-props
	Hidden   int    `json:"-"`
}

func registerChart(t *testing.T, strategy ...string) {
	t.Helper()
	args := []any{"islands/Chart.vue"}
	for _, s := range strategy {
		args = append(args, s)
	}
	Island(args...)(vuka.TypeOf[Chart]("dashboard.Chart"))
}

func renderIsland(t *testing.T, n Node) string {
	t.Helper()
	s, err := String(context.Background(), n)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return s
}

func TestIslandHostAttributes(t *testing.T) {
	registerChart(t)
	html := renderIsland(t, MountIsland("p#1", Chart{Series: []int{1, 2}, Title: "Sales"}, Visible()))

	for _, want := range []string{
		"<vk-island",
		`data-vk-island="Chart"`,
		`data-vk-client="visible"`,
		`data-vk-s="p#1"`, // the stable site, not a per-render counter
		`data-vk-props="`,
		"&#34;series&#34;:[1,2]", // JSON, HTML-escaped in the attribute
		"&#34;title&#34;:&#34;Sales&#34;",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("island host missing %q:\n%s", want, html)
		}
	}
	// The slot (Children, a Node) and json:"-" fields never reach data-vk-props.
	for _, no := range []string{"Children", "Hidden"} {
		if strings.Contains(html, no) {
			t.Errorf("data-vk-props leaked %q:\n%s", no, html)
		}
	}
}

func TestIslandFrameworkInferred(t *testing.T) {
	cases := map[string]string{
		"a.vue": "vue", "a.tsx": "react", "a.jsx": "react",
		"a.svelte": "svelte", "a.ts": "vanilla", "a.js": "vanilla",
	}
	for path, want := range cases {
		if got := frameworkOf(path); got != want {
			t.Errorf("frameworkOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestIslandStrategyDefaultAndOverride(t *testing.T) {
	// decorator default = idle
	Island("islands/Chart.vue", "idle")(vuka.TypeOf[Chart]("dashboard.Chart"))

	def := renderIsland(t, MountIsland("p#1", Chart{}))
	if !strings.Contains(def, `data-vk-client="idle"`) {
		t.Errorf("decorator default strategy not used:\n%s", def)
	}

	over := renderIsland(t, MountIsland("p#1", Chart{}, Load()))
	if !strings.Contains(over, `data-vk-client="load"`) {
		t.Errorf("tag strategy did not override default:\n%s", over)
	}
}

func TestIslandMediaAndRootMargin(t *testing.T) {
	registerChart(t)

	m := renderIsland(t, MountIsland("p#1", Chart{}, Media("(max-width:600px)")))
	if !strings.Contains(m, `data-vk-client="media"`) || !strings.Contains(m, `data-vk-media="(max-width:600px)"`) {
		t.Errorf("media strategy not emitted:\n%s", m)
	}

	v := renderIsland(t, MountIsland("p#1", Chart{}, Visible("200px")))
	if !strings.Contains(v, `data-vk-rootmargin="200px"`) {
		t.Errorf("visible rootMargin not emitted:\n%s", v)
	}
}

func TestIslandSlotAndFallback(t *testing.T) {
	registerChart(t)
	html := renderIsland(t, MountIsland("p#1", Chart{},
		Visible(),
		Fallback(El("div", []Attr{{"class", "skeleton"}})),
		Slot(Text("clicks")),
	))

	if !strings.Contains(html, `<div class="skeleton">`) {
		t.Errorf("fallback not rendered as host children:\n%s", html)
	}
	if !strings.Contains(html, `<template data-vk-slot>clicks</template>`) {
		t.Errorf("slot not wrapped in <template data-vk-slot>:\n%s", html)
	}
}

func TestIslandLivePropUnwrapped(t *testing.T) {
	type Live struct {
		Rows *track.Assign[[]int] `json:"rows"`
	}
	Island("islands/Live.ts")(vuka.TypeOf[Live]("dashboard.Live"))
	a := &track.Assign[[]int]{}
	a.Set([]int{1, 2, 3})
	html := renderIsland(t, MountIsland("p#1", Live{Rows: a}))
	if !strings.Contains(html, "&#34;rows&#34;:[1,2,3]") {
		t.Errorf("live *Assign prop should serialize its value, not the struct:\n%s", html)
	}
}

func TestIslandUnregistered(t *testing.T) {
	type orphan struct{ X int }
	html := renderIsland(t, MountIsland("p#1", orphan{}))
	if !strings.Contains(html, "data-vk-error") {
		t.Errorf("unregistered island should render an error host:\n%s", html)
	}
}

type Bare struct {
	N int `json:"n"`
}

// A bare @ui.Island registers the type by name (the vuka command infers the
// file); "" with a strategy keeps the inference and sets the default.
func TestIslandBare(t *testing.T) {
	Island()(vuka.TypeOf[Bare]("dashboard.Bare"))
	if s := renderIsland(t, MountIsland("s", Bare{N: 1})); !strings.Contains(s, `data-vk-island="Bare"`) || !strings.Contains(s, `data-vk-client="visible"`) {
		t.Fatalf("bare: %s", s)
	}
	Island("", "load")(vuka.TypeOf[Bare]("dashboard.Bare"))
	if s := renderIsland(t, MountIsland("s", Bare{N: 1})); !strings.Contains(s, `data-vk-client="load"`) {
		t.Fatalf("strategy with inference: %s", s)
	}
	if info := lookupIsland(reflect.TypeFor[Bare]()); info.Path != "" {
		t.Fatalf("path: %q", info.Path)
	}
}
