package ui_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// groups are a component's deferred groups as Await reads them: one held
// with a nil error loads, with an error failed; one not held has loaded.
type groups map[string]error

func (g groups) Loading(group string) bool {
	err, ok := g[group]
	return ok && err == nil
}

func (g groups) LoadErr(group string) error { return g[group] }

func TestAwaitSlots(t *testing.T) {
	loaded := func() ui.Node { return ui.Text("rows") }
	failed := func(err error) ui.Node { return ui.Textf("failed: %v", err) }
	cases := []struct {
		name string
		of   groups
		a    ui.Await
		want string
	}{
		{"loading", groups{"orders": nil}, ui.Await{Pending: ui.Text("…"), Children: loaded}, "…"},
		{"failed", groups{"orders": errors.New("down")}, ui.Await{Error: failed, Children: loaded}, "failed: down"},
		{"loaded", groups{}, ui.Await{Pending: ui.Text("…"), Error: failed, Children: loaded}, "rows"},
		{"loaded, no children", groups{}, ui.Await{}, ""},
		{"default pending", groups{"orders": nil}, ui.Await{Children: loaded}, `<span aria-busy="true"></span>`},
		{"default error", groups{"orders": errors.New("down")}, ui.Await{Children: loaded}, `<span role="alert">This part couldn&#39;t load.</span>`},
	}
	for _, c := range cases {
		c.a.Of, c.a.Key = c.of, "orders"
		if got := mustString(t, c.a); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// The app's defaults stand in for the slots a markup leaves out.
func TestAwaitDefaults(t *testing.T) {
	ui.SetAwaitDefaults(ui.AwaitDefaults{
		Pending: ui.Text("please wait"),
		Error:   func(err error) ui.Node { return ui.Textf("oops: %v", err) },
	})
	t.Cleanup(func() { ui.SetAwaitDefaults(ui.AwaitDefaults{}) })
	if got := mustString(t, ui.Await{Of: groups{"orders": nil}, Key: "orders"}); got != "please wait" {
		t.Errorf("pending: %s", got)
	}
	if got := mustString(t, ui.Await{Of: groups{"orders": errors.New("down")}, Key: "orders"}); got != "oops: down" {
		t.Errorf("error: %s", got)
	}
	if got := mustString(t, ui.Await{Of: groups{"orders": nil}, Key: "orders", Pending: ui.Text("own")}); got != "own" {
		t.Errorf("a slot of its own: %s", got)
	}
}

func TestAwaitNeedsOf(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	ui.Await{Key: "orders"}.View()
}

func mustString(t *testing.T, n ui.Node) string {
	t.Helper()
	s, err := ui.String(context.Background(), n)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
