package ui_test

import (
	"context"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

func TestFallthroughClass(t *testing.T) {
	for _, c := range []struct {
		name string
		f    ui.Fallthrough
		own  []any
		want any
	}{
		{"none", nil, []any{"rounded-xl border"}, "rounded-xl border"},
		{"caller's utilities win", ui.Fallthrough{{Name: "class", Value: "border-red-200 bg-muted"}}, []any{"rounded-xl border bg-card"}, "rounded-xl border border-red-200 bg-muted"},
		{"pieces join", ui.Fallthrough{{Name: "class", Value: "p-2"}}, []any{"card p-", 4}, "card p-2"},
		{"two classes merge in order", ui.Fallthrough{{Name: "class", Value: "px-2"}, {Name: "className", Value: "px-6"}}, []any{"px-4 py-2"}, "py-2 px-6"},
		{"a left-out class", ui.Fallthrough{{Name: "class", Value: nil}}, []any{"p-4"}, "p-4"},
		{"a ui.Class own", ui.Fallthrough{{Name: "class", Value: "p-2"}}, []any{ui.Class{"p-4": true, "hidden": false}}, "p-2"},
	} {
		if got := c.f.Merge(c.own...); got != c.want {
			t.Errorf("%s: Merge = %#v, want %#v", c.name, got, c.want)
		}
	}
	got := ui.Fallthrough{{Name: "class", Value: "mt-4"}}.Merge(ui.Classes{"p-4": true})
	if cs, ok := got.(ui.Classes); !ok || cs["mt-4"] != true || cs["p-4"] != true {
		t.Errorf("Classes: got %#v", got)
	}
}

func TestFallthroughAttrs(t *testing.T) {
	f := ui.Fallthrough{
		{Name: "id", Value: "a"},
		{Name: "class", Value: "mt-4"},
		{Name: "bind:value", Value: []ui.Attr{{Name: "value", Value: "go"}, {Name: "data-vk-bind", Value: "q"}}},
		{Name: "id", Value: "b"},
		{Name: "title", Value: nil},
	}
	got := f.Attrs()
	want := []ui.Attr{{Name: "value", Value: "go"}, {Name: "data-vk-bind", Value: "q"}, {Name: "id", Value: "b"}, {Name: "title", Value: nil}}
	if len(got) != len(want) {
		t.Fatalf("Attrs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Attrs[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if all := f.All(); all[len(all)-1] != (ui.Attr{Name: "class", Value: "mt-4"}) {
		t.Errorf("All ends %v", all[len(all)-1])
	}
	for name, has := range map[string]bool{"id": true, "ID": true, "value": true, "class": true, "title": false, "role": false} {
		if f.Has(name) != has {
			t.Errorf("Has(%q) = %v", name, !has)
		}
	}
	if f.Or("id", "own") != nil || f.Or("role", "own") != "own" {
		t.Errorf("Or: %v %v", f.Or("id", "own"), f.Or("role", "own"))
	}
}

// The forward element as vuka writes it: its class merged, its own
// attributes the caller sets left out, the caller's spread.
func TestFallthroughRender(t *testing.T) {
	render := func(f ui.Fallthrough) string {
		t.Helper()
		n := ui.HTML(`<div class={f.Merge("rounded-xl border")} role={f.Or("role", "region")} {f.Attrs()...}>x</div>`,
			f.Merge("rounded-xl border"), f.Or("role", "region"), f.Attrs())
		s, err := ui.String(context.Background(), n)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got, want := render(nil), `<div class="rounded-xl border" role="region">x</div>`; got != want {
		t.Errorf("no fallthrough:\n got %s\nwant %s", got, want)
	}
	f := ui.Fallthrough{{Name: "class", Value: "border-red-200"}, {Name: "role", Value: "alert"}, {Name: "aria-busy", Value: "true"}, {Name: "disabled", Value: false}}
	if got, want := render(f), `<div class="rounded-xl border border-red-200" role="alert" aria-busy="true">x</div>`; got != want {
		t.Errorf("fallthrough:\n got %s\nwant %s", got, want)
	}
}
