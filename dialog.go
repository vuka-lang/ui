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
		call, ctx := callRootAttrs(ctx)
		class := "vk-dialog"
		switch d.Size {
		case "sm", "lg":
			class += " vk-dialog--" + d.Size
		}
		attrs := append([]Attr{{"class", withClass(class, d.Class)}}, call...)
		if d.Open && call == nil {
			attrs = append(attrs, Attr{"open", true})
		}
		header := []Node{}
		if d.Title != nil {
			header = append(header, El("h2", []Attr{{"class", "vk-dialog__title"}}, d.Title))
		}
		header = append(header, callClose(ctx, "vk-dialog__close"))
		kids := []Node{El("div", []Attr{{"class", "vk-dialog__header"}}, header...), El("div", []Attr{{"class", "vk-dialog__body"}}, d.Children)}
		if d.Footer != nil {
			kids = append(kids, El("div", []Attr{{"class", "vk-dialog__footer"}}, d.Footer))
		}
		return withContext(func(c context.Context) context.Context { return context.WithValue(c, callRootKey{}, []Attr(nil)) }, El("dialog", attrs, kids...))
	})
}

func (d Dialog) Render(ctx context.Context, w io.Writer) error { return d.View().Render(ctx, w) }
