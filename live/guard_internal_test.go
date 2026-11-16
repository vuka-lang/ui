package live

import (
	"testing"

	"github.com/vuka-lang/ui/v2"
)

func TestKeepable(t *testing.T) {
	row := ui.OpaqueTree("a", "b")
	for _, c := range []struct {
		v    any
		want bool
	}{
		{"text", true},
		{row, true},
		{&ui.TreeList{Items: []*ui.Tree{row}}, true},
		{&ui.TreeList{Stream: true}, false},
		{&ui.TreeList{Temp: "x"}, false},
		{ui.OpaqueTree(ui.TreeRef("c2")), false},
		{ui.OpaqueTree("<p id=\"id-ABCDEFGHIJKLMNOPQRSTUVWXYZ\">"), false},
		{ui.OpaqueTree(marker("c2")), false},
		{&ui.TreeList{Items: []*ui.Tree{ui.OpaqueTree(&ui.TreeList{Stream: true})}}, false},
	} {
		if got := keepable(c.v); got != c.want {
			t.Errorf("keepable(%#v) = %v", c.v, got)
		}
	}
}
