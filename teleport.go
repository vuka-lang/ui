package ui

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Teleport renders its children where they are written and has the live
// runtime move them to the end of <body>, or of the element To names, as
// Vue's <Teleport> and React's createPortal do. A fixed overlay — a modal's
// backdrop, a toast, a menu — written inside an ancestor that clips it
// (mask-image, transform, filter, contain, overflow with backdrop-filter)
// escapes it, and its z-index is no longer bound by the ancestor's stacking
// context:
//
//	<ui.Teleport>
//		<div class="fixed inset-0 z-50">…</div>
//	</ui.Teleport>
//
//	<ui.Teleport to="#overlays">…</ui.Teleport>
//
// The children are the page's markup as any other: the same scope, state,
// handlers and form (a model= in them binds to the enclosing for={Form}).
// The runtime keeps treating them as written in place: live patches apply
// to them where they were moved, a re-render never moves them back, and
// they go when the server stops rendering the Teleport. An event in them
// reaches vuka's handlers on the Teleport's ancestors (an onKeyDown on the
// page), a command's ref finds them in their component, and their fields
// stay in the form around the Teleport (the runtime sets their form
// attribute). The DOM's own bubbling — a script's listener — follows where
// they are, at the end of the target. A focused field keeps its focus and
// what it shows when moved, and across patches.
//
// Without JavaScript, and until the runtime starts, the children show in
// place. The element is a <vk-teleport> with display: contents, so in place
// it doesn't affect the layout. A component can teleport itself: a
// Teleport as the root of its markup moves all of it.
type Teleport struct {
	// To is where the children go: "body" (the default) or "#id", an element
	// the layout owns. Until an element with that id is in the page the
	// children stay in place.
	To string
	// ID is the element's id, a key that keeps a teleport the same element
	// when the teleports around it come and go. Optional.
	ID       string
	Children Node
}

func (t Teleport) View() Node {
	to := t.To
	if to == "" {
		to = "body"
	}
	if to != "body" && (len(to) < 2 || to[0] != '#' || strings.ContainsAny(to, " \t\n\"'")) {
		return Try(nil, fmt.Errorf("ui: Teleport's to is %q: body or #id", t.To))
	}
	attrs := make([]Attr, 0, 3)
	if t.ID != "" {
		attrs = append(attrs, Attr{Name: "id", Value: t.ID})
	}
	attrs = append(attrs, Attr{Name: "data-vk-teleport", Value: to}, Attr{Name: "style", Value: "display: contents"})
	return El("vk-teleport", attrs, t.Children)
}

func (t Teleport) Render(ctx context.Context, w io.Writer) error { return t.View().Render(ctx, w) }
