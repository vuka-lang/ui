package widget

import (
	"context"
	"io"

	"github.com/vuka-lang/ui/v2"
)

// Checkbox is a single box, ticked when the field is true.
type Checkbox struct {
	Props
	Class string
}

// Switch is a checkbox drawn as an on/off switch (role="switch").
type Switch struct {
	Props
	Class string
}

func (w Checkbox) View() ui.Node {
	return view(w, func(w Checkbox, _ context.Context) ui.Node {
		return ui.El("input", w.control("input", "checkbox", cls("vk-check__box", w.Class)))
	})
}

func (w Checkbox) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }

// FieldLayout: a field group puts the label beside the box.
func (Checkbox) FieldLayout() string { return "check" }

func (w Switch) View() ui.Node {
	return view(w, func(w Switch, _ context.Context) ui.Node {
		return ui.El("input", w.control("input", "checkbox", cls("vk-check__box vk-switch", w.Class), ui.Attr{Name: "role", Value: "switch"}))
	})
}

func (w Switch) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }

// FieldLayout: a field group puts the label beside the switch.
func (Switch) FieldLayout() string { return "check" }
