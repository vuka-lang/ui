package widget

import (
	"context"
	"io"

	"github.com/vuka-lang/ui/v2"
)

// Date is a date picker (a time.Time's date).
type Date struct {
	Props
	Min, Max string
	Class    string
}

// DateTime is a date and time picker, the visitor's local time.
type DateTime struct {
	Props
	Min, Max string
	Class    string
}

// Time is a time-of-day picker.
type Time struct {
	Props
	Min, Max, Step string
	Class          string
}

func bounds(attrs ...ui.Attr) []ui.Attr {
	var out []ui.Attr
	for _, a := range attrs {
		if a.Value != "" {
			out = append(out, a)
		}
	}
	return out
}

func (w Date) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, func(context.Context) ui.Node {
		return input(w.Props, "date", w.Class, bounds(ui.Attr{Name: "min", Value: w.Min}, ui.Attr{Name: "max", Value: w.Max})...)
	})
}

func (w DateTime) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, func(context.Context) ui.Node {
		return input(w.Props, "datetime-local", w.Class, bounds(ui.Attr{Name: "min", Value: w.Min}, ui.Attr{Name: "max", Value: w.Max})...)
	})
}

func (w Time) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, func(context.Context) ui.Node {
		return input(w.Props, "time", w.Class, bounds(ui.Attr{Name: "min", Value: w.Min}, ui.Attr{Name: "max", Value: w.Max}, ui.Attr{Name: "step", Value: w.Step})...)
	})
}
