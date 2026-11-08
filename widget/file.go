package widget

import (
	"context"
	"io"

	"github.com/vuka-lang/ui/v2"
)

// File is a file input: a ui.File field's. Accept narrows what the
// browser offers (the field's accept: tag when empty).
type File struct {
	Props
	Accept string
	Class  string
}

func (w File) View() ui.Node {
	return view(w, func(w File, _ context.Context) ui.Node {
		var extra []ui.Attr
		if w.Accept != "" {
			extra = append(extra, ui.Attr{Name: "accept", Value: w.Accept})
		}
		return ui.El("input", w.control("input", "file", cls("vk-input", w.Class), extra...))
	})
}

func (w File) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }
