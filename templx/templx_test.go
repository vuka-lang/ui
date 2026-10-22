package templx_test

import (
	"context"
	"io"
	"testing"

	"github.com/a-h/templ"
	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/templx"
)

// card is what templ generates for <div class="card">{ children... }</div>.
var card = templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
	io.WriteString(w, `<div class="card">`)
	if err := templ.GetChildren(ctx).Render(templ.ClearChildren(ctx), w); err != nil {
		return err
	}
	_, err := io.WriteString(w, `</div>`)
	return err
})

func TestWithChildren(t *testing.T) {
	tests := []struct {
		name string
		n    ui.Node
		want string
	}{
		{"children", templx.WithChildren(card, ui.Fragment(ui.El("b", nil, ui.Text("<hi>")), ui.Text("!"))), `<div class="card"><b>&lt;hi&gt;</b>!</div>`},
		{"no children", templx.WithChildren(card, nil), `<div class="card"></div>`},
		{"nested", templx.WithChildren(card, templx.WithChildren(card, ui.Text("in"))), `<div class="card"><div class="card">in</div></div>`},
		{"inside vuka", ui.El("main", nil, templx.WithChildren(card, ui.Text("x"))), `<main><div class="card">x</div></main>`},
	}
	for _, tt := range tests {
		got, err := ui.String(context.Background(), tt.n)
		if err != nil || got != tt.want {
			t.Errorf("%s: %q, %v; want %q", tt.name, got, err, tt.want)
		}
	}
}

func TestInterchangeable(t *testing.T) {
	var c templ.Component = ui.El("p", nil)
	var n ui.Node = card
	_, _ = c, n
}
