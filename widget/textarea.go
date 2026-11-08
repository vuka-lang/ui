package widget

import (
	"context"
	"io"

	"github.com/vuka-lang/ui/v2"
)

// Textarea is a multi-line text input, Rows high (3 when 0).
type Textarea struct {
	Props
	Rows  int
	Class string
}

func (w Textarea) View() ui.Node {
	return view(w, func(w Textarea, _ context.Context) ui.Node {
		rows := w.Rows
		if rows == 0 {
			rows = 3
		}
		return ui.El("textarea", w.control("textarea", "", cls("vk-textarea", w.Class), ui.Attr{Name: "rows", Value: rows}), ui.Text(ui.ModelText(w.Model)))
	})
}

func (w Textarea) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }
