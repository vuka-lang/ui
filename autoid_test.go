package ui_test

import (
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// TestOmitZero: name?={v} leaves the attribute out when v is zero.
func TestOmitZero(t *testing.T) {
	type point struct{ X, Y int }
	var node ui.Node
	for _, c := range []struct {
		v    any
		zero bool
	}{
		{ui.OmitZero(""), true}, {ui.OmitZero("x"), false},
		{ui.OmitZero(0), true}, {ui.OmitZero(3), false},
		{ui.OmitZero(0.0), true}, {ui.OmitZero(ui.AutoID("")), true},
		{ui.OmitZero(point{}), true}, {ui.OmitZero(point{X: 1}), false},
		{ui.OmitZero([]string(nil)), true}, {ui.OmitZero(node), true},
	} {
		if (c.v == nil) != c.zero {
			t.Errorf("OmitZero gave %#v", c.v)
		}
	}
	html, err := ui.String(t.Context(), ui.El("input", []ui.Attr{{Name: "id", Value: ui.OmitZero("")}, {Name: "placeholder", Value: ui.OmitZero("Search")}}))
	if err != nil || html != `<input placeholder="Search">` {
		t.Errorf("%s %v", html, err)
	}
}
