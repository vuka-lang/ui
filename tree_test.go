package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
)

func TestBuildTree(t *testing.T) {
	ctx := context.Background()
	rows := func(n int) ui.Node {
		return ui.Nodes(func(add func(ui.Node)) {
			for i := range n {
				add(ui.F(2, "Ed(h)", ui.El("li", []ui.Attr{{Name: "key", Value: i}}, ui.Child(i))))
			}
		})
	}
	n := ui.F(1, "Esd(tbE(h))tE()", ui.El("ul", []ui.Attr{{Name: "className", Value: "x"}, {Name: "id", Value: 7}},
		ui.Text("a<"), rows(3), ui.El("script", nil, ui.Child("</script>"))), ui.Text(" "), ui.El("br", nil))
	tr, err := ui.BuildTree(ctx, n, true)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	tr.HTML(&b, nil)
	want, _ := ui.String(ctx, n)
	if b.String() != want {
		t.Fatalf("tree html\n%s\nwant\n%s", b.String(), want)
	}
	if len(tr.Statics) != len(tr.Dyn)+1 || len(tr.Dyn) != 3 {
		t.Fatalf("statics %q dyn %#v", tr.Statics, tr.Dyn)
	}
	if l := tr.Dyn[1].(*ui.TreeList); len(l.Items) != 3 || l.Keys[2] != "int=2" {
		t.Fatalf("list %#v", l)
	}
	bad := ui.F(3, "E(h)", ui.Text("not an element"))
	tr, err = ui.BuildTree(ctx, bad, true)
	if err != nil || len(tr.Dyn) != 1 || tr.Dyn[0] != "not an element" {
		t.Fatalf("a frame its shape doesn't fit is opaque: %#v %v", tr, err)
	}
}
