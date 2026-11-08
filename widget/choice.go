package widget

import (
	"context"
	"io"

	"github.com/vuka-lang/ui/v2"
)

// Select is a dropdown of the field's choices.
type Select struct {
	Props
	Choices []Choice[any]
	Class   string
}

// MultiSelect is a list of the field's choices, several of which may be
// chosen: a slice field's control.
type MultiSelect struct {
	Props
	Choices []Choice[any]
	// Size is how many choices show at once; the browser's default when 0.
	Size  int
	Class string
}

// Radios is a radio button per choice, under the field's label as a
// legend.
type Radios struct {
	Props
	Choices []Choice[any]
	Class   string
}

// Checkboxes is a checkbox per choice: a slice field's.
type Checkboxes struct {
	Props
	Choices []Choice[any]
	Class   string
}

func (w Select) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, func(ctx context.Context) ui.Node {
		return ui.El("select", w.control("select", "", cls("vk-select", w.Class)), options(ctx, w.Props, w.Choices, false)...)
	})
}

func (w MultiSelect) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, func(ctx context.Context) ui.Node {
		extra := []ui.Attr{{Name: "multiple", Value: true}}
		if w.Size > 0 {
			extra = append(extra, ui.Attr{Name: "size", Value: w.Size})
		}
		return ui.El("select", w.control("select", "", cls("vk-select", w.Class), extra...), options(ctx, w.Props, w.Choices, true)...)
	})
}

func (w Radios) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, func(context.Context) ui.Node { return items(w.Props, w.Choices, "radio") })
}

// FieldLayout: a field group shows the radios as a fieldset.
func (Radios) FieldLayout() string { return "fieldset" }

func (w Checkboxes) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, func(context.Context) ui.Node { return items(w.Props, w.Choices, "checkbox") })
}

// FieldLayout: a field group shows the checkboxes as a fieldset.
func (Checkboxes) FieldLayout() string { return "fieldset" }
