package ui

import (
	"context"
	"io"
	"strings"
)

// ActionMenu is one item of a DataTable row's actions menu, what its
// Actions (<slot:actions let={row}>) returns:
//
//	<slot:actions let={b}>
//		<ui.ActionMenu icon="file-text" href={"/books/" + b.Slug}>Open</ui.ActionMenu>
//		<ui.ActionMenu icon="trash" danger on={ui.JS.Push(Delete, ui.Value(b.ID))}>Delete</ui.ActionMenu>
//	</slot:actions>
//
// Choosing an item closes the menu.
type ActionMenu struct {
	// Icon names a kit icon shown before the label.
	Icon string
	// On is what choosing the item does: a live handler or a ui.JS chain.
	On any
	// Href makes the item a link (followed over the live connection).
	Href string
	// Danger marks an item that destroys, in the danger colour.
	Danger   bool
	Disabled bool
	Class    string
	Children Node
}

type actionMenuKey struct{}

func (a ActionMenu) classes() string {
	c := "vk-menu__item"
	if a.Danger {
		c += " vk-menu__item--danger"
	}
	return withClass(c, a.Class)
}

func (a ActionMenu) icon() Node {
	if a.Icon == "" {
		return nil
	}
	return Icon(a.Icon, "vk-menu__icon")
}

// View is the item; inside a row's menu, choosing it hides the menu.
func (a ActionMenu) View() Node {
	return ctxNode(func(ctx context.Context) Node {
		menu, _ := ctx.Value(actionMenuKey{}).(string)
		hide := []Attr(nil)
		if menu != "" {
			hide = []Attr{{"popovertarget", menu}, {"popovertargetaction", "hide"}}
		}
		if a.Href != "" && !a.Disabled {
			return HTML(`<a class={a.classes()} role="menuitem" href={a.Href} data-vk-link>{a.icon()}<span>{a.Children}</span></a>`, a.classes(), a.Href, a.icon(), a.Children)
		}
		return HTML(`<button type="button" class={a.classes()} role="menuitem" disabled={a.Disabled} {hide...} {on("onClick", a.On)...}>{a.icon()}<span>{a.Children}</span></button>`, a.classes(), a.Disabled, hide, on("onClick", a.On), a.icon(), a.Children)
	})
}

func (a ActionMenu) Render(ctx context.Context, w io.Writer) error { return a.View().Render(ctx, w) }

// rowMenu is a row's actions menu: a button opening a popover of items,
// anchored to the button where the browser supports anchor positioning.
func rowMenu(id string, items Node) Node {
	id = menuID(id)
	anchor := "anchor-name:--" + id
	place := "position-anchor:--" + id
	inner := withContext(func(ctx context.Context) context.Context {
		return context.WithValue(ctx, actionMenuKey{}, id)
	}, items)
	return HTML(`<div class="vk-menu"><button type="button" class="vk-menu__trigger" aria-label="Actions" title="Actions" aria-haspopup="menu" popovertarget={id} style={anchor}>{Icon("dots-vertical")}</button><div class="vk-menu__panel" id={id} popover role="menu" style={place}>{inner}</div></div>`,
		id, anchor, Icon("dots-vertical"), place, inner)
}

// menuID is s with what an id and a CSS name can't hold replaced.
func menuID(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, s)
}
