package widget

import (
	"context"
	"io"

	"github.com/vuka-lang/ui/v2"
)

// Text is a single-line text input.
type Text struct {
	Props
	Class string
}

// Email is an email address input.
type Email struct {
	Props
	Class string
}

// Password is a masked input.
type Password struct {
	Props
	Class string
}

// Number is a number input; Min, Max and Step bound it when set.
type Number struct {
	Props
	Min, Max, Step string
	Class          string
}

// Tel is a telephone number input.
type Tel struct {
	Props
	Class string
}

// URL is a web address input.
type URL struct {
	Props
	Class string
}

// Search is a search box.
type Search struct {
	Props
	Class string
}

func input(p Props, typ, class string, extra ...ui.Attr) ui.Node {
	return ui.El("input", p.control("input", typ, cls("vk-input", class), extra...))
}

func (w Text) View() ui.Node {
	return view(w, func(w Text, _ context.Context) ui.Node { return input(w.Props, "text", w.Class) })
}

func (w Email) View() ui.Node {
	return view(w, func(w Email, _ context.Context) ui.Node { return input(w.Props, "email", w.Class) })
}

func (w Email) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }

func (w Password) View() ui.Node {
	return view(w, func(w Password, _ context.Context) ui.Node { return input(w.Props, "password", w.Class) })
}

func (w Password) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }

func (w Number) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }

func (w Number) View() ui.Node {
	return view(w, func(w Number, _ context.Context) ui.Node {
		var extra []ui.Attr
		for _, a := range []ui.Attr{{Name: "min", Value: w.Min}, {Name: "max", Value: w.Max}, {Name: "step", Value: w.Step}} {
			if a.Value != "" {
				extra = append(extra, a)
			}
		}
		return input(w.Props, "number", w.Class, extra...)
	})
}

func (w Text) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }

func (w Tel) View() ui.Node {
	return view(w, func(w Tel, _ context.Context) ui.Node { return input(w.Props, "tel", w.Class) })
}

func (w Tel) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }

func (w URL) View() ui.Node {
	return view(w, func(w URL, _ context.Context) ui.Node { return input(w.Props, "url", w.Class) })
}

func (w URL) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }

func (w Search) View() ui.Node {
	return view(w, func(w Search, _ context.Context) ui.Node { return input(w.Props, "search", w.Class) })
}

func (w Search) Render(ctx context.Context, out io.Writer) error { return w.View().Render(ctx, out) }
