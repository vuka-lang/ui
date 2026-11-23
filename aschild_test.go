package ui_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// button is a component as vuka writes one: its forward element takes its
// Fallthrough.
type button struct {
	Fallthrough ui.Fallthrough `json:"-"`
	Children    ui.Node
}

func (b button) View() ui.Node {
	return ui.HTML(`<div class="wrap"><button {b.Fallthrough.Attrs()...} class="btn px-4 text-sm{b.Fallthrough.Classes()}" type={b.Fallthrough.Or("type", "button")} onClick={ui.JS.Show(ui.Ref("x"))}>{b.Children}</button></div>`,
		b.Fallthrough.Attrs(), b.Fallthrough.Classes(), b.Fallthrough.Or("type", "button"), ui.JS.Show(ui.Ref("x")), b.Children)
}

func (b button) Render(ctx context.Context, w io.Writer) error { return b.View().Render(ctx, w) }

func TestAsChild(t *testing.T) {
	child := ui.HTML(`<a href="/reports/42" class="px-2" onClick={ui.JS.Hide(ui.Ref("y"))}>Report</a>`, ui.JS.Hide(ui.Ref("y")))
	n := ui.AsChild(button{Fallthrough: ui.Fallthrough{{Name: "class", Value: "mt-4"}, {Name: "id", Value: "r"}, ui.AsChildAttr(child)}})
	got, err := ui.String(context.Background(), n)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<div class="wrap"><a href="/reports/42" class="btn text-sm mt-4 px-2"`,
		`id="r"`, `type="button"`, `>Report</a></div>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in %s", want, got)
		}
	}
	if strings.Contains(got, "<button") || strings.Contains(got, "aschild") {
		t.Errorf("the forward element or the slot rendered: %s", got)
	}
	if i, j := strings.Index(got, `hide`), strings.Index(got, `show`); i < 0 || j < 0 || i > j || strings.Count(got, "data-vk-js-click") != 1 {
		t.Errorf("handlers not composed, the child's first: %s", got)
	}
	// without asChild the slot writes nothing, and nothing is replaced
	plain, err := ui.String(context.Background(), button{Children: ui.Text("Go")})
	if err != nil || !strings.HasPrefix(plain, `<div class="wrap"><button class="btn px-4 text-sm" type="button"`) {
		t.Errorf("plain: %s %v", plain, err)
	}
	if _, err := ui.String(context.Background(), ui.AsChild(button{})); err == nil {
		t.Error("asChild with no child rendered")
	}
}
