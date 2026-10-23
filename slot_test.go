package ui_test

import (
	"context"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

func TestHas(t *testing.T) {
	var nilElem *ui.Element
	cases := []struct {
		name string
		n    ui.Node
		want bool
	}{
		{"nil", nil, false},
		{"nil pointer", nilElem, false},
		{"nil func", ui.NodeFunc(nil), false},
		{"empty text", ui.Text(""), false},
		{"space", ui.Text("  \n\t"), false},
		{"text", ui.Text("x"), true},
		{"empty fragment", ui.Fragment(), false},
		{"blank fragment", ui.Fragment(ui.Text(" "), nil, ui.Fragment()), false},
		{"fragment", ui.Fragment(ui.Text(" "), ui.Text("x")), true},
		{"raw space", ui.RawHTML(" "), false},
		{"element", ui.El("b", nil), true},
		{"template", ui.HTML(`<b>{x}</b>`, 1), true},
		{"static template", ui.HTML(`<i>x</i>`), true},
		{"empty nodes", ui.Child([]ui.Node{}), false},
		{"keyed blank", ui.Keyed(1, ui.Text("")), false},
	}
	for _, c := range cases {
		if got := ui.Has(c.n); got != c.want {
			t.Errorf("%s: Has = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSlotOr(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		n    ui.Node
		want string
	}{
		{nil, "<i>none</i>"},
		{ui.Text(" "), "<i>none</i>"},
		{ui.HTML(`<b>set</b>`), "<b>set</b>"},
	} {
		got, err := ui.String(ctx, ui.SlotOr(c.n, ui.HTML(`<i>none</i>`)))
		if err != nil || got != c.want {
			t.Errorf("SlotOr(%v) = %q, %v; want %q", c.n, got, err, c.want)
		}
	}
	if got, err := ui.String(ctx, ui.SlotOr(nil, nil)); got != "" || err != nil {
		t.Errorf("SlotOr(nil, nil) = %q, %v", got, err)
	}
}
