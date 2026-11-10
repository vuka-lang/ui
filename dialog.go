package ui

import (
	"context"
	"io"
)

// Dialog is a native <dialog>: a header with its title and a close button,
// its children, and a footer. As the root of a component opened with
// JS.Call (or ui.Call) it is the component's presentation: shown modal for
// as long as the opened instance lives, Escape and its close button
// closing it (and destroying the instance), focus going back to what had
// it when it opened.
//
//	<ui.Dialog title={ui.Text("Edit product")}>
//	  <form onSubmit={Save}>…</form>
//	</ui.Dialog>
//
// Anywhere else it is a <dialog> the page shows itself: Open renders it
// open (not modal), and JS.Close in it closes it.
type Dialog struct {
	// Title is the header's heading; omitted when nil.
	Title Node
	// Footer is the footer's content (buttons); omitted when nil.
	Footer Node
	// Open renders a dialog outside a Call open; a Call's shows on its own.
	Open bool
	// Size is "sm", "" / "md" (default) or "lg".
	Size     string
	Class    string
	Children Node
}

func (d Dialog) View() Node {
	return ctxNode(func(ctx context.Context) Node {
		call, ctx := callRoot(ctx, "modal")
		class := "vk-dialog"
		switch d.Size {
		case "sm", "lg":
			class += " vk-dialog--" + d.Size
		}
		if d.Open && call == nil {
			call = []Attr{{"open", true}}
		}
		return presentation("dialog", withClass(class, d.Class), call, ctx, d.parts(), true)
	})
}

func (d Dialog) Render(ctx context.Context, w io.Writer) error { return d.View().Render(ctx, w) }
