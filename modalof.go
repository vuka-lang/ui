package ui

import (
	"context"
	"io"
	"time"
)

// ModalOf is a Modal driven by one state: shown while For holds a value,
// closed while it is nil, the value its content's. The page keeps one
// ui.State[*T]; any write of a value opens it, a write of nil closes it:
//
//	sync ui.State[*SyncTarget]
//
//	<tr onClick={sync = &SyncTarget{ID: r.ID, Name: r.Name}}>…</tr>
//
//	<ui.ModalOf for={sync} let={t} title={ui.Text("Sync")}>
//	  <p>Sync {t.Name} now?</p>
//	  <ui.Button on={RunSync(t.ID)}>Sync now</ui.Button>
//	</ui.ModalOf>
//
// let names the value for the content (and <slot:footer let={t}>): it is
// never nil there. Each open is a new write of the state, so the fields in
// the content take the new value's (what one-state's versions give a
// field), with no remount or key of the modal's own.
//
// The close button, a click on the backdrop and Escape write nil into For
// in the browser, at once, and give the focus back to what had it when the
// modal opened. The modal's markup stays as it was while it fades out:
// the server is told once the fade is over (OnClose, when set, is called
// then), and its render is what removes the content. Opening shows the
// modal at once and asks the server for the content (a ui.Lazy region
// rendering again each time it shows): the write rides that request, so a
// browser-only write (an on… attribute's) opens it as a handler's does.
// The focus goes to the dialog, and on to its content's first field when
// the content is there.
//
// It renders in a Teleport at the end of <body>, labelled by its title,
// its ids its tag's (ui.AutoID). Modal, with Open and OnClose or
// OpenModal, stays for a modal whose open state isn't a value.
type ModalOf[T any] struct {
	// For is the state the modal follows: shown while it holds a value.
	For State[*T] `vuka:"required"`
	// ID is the dialog's id; its parts' ids are suffixes of it.
	ID AutoID
	// Title is the header content, and what labels the dialog; omitted when
	// nil.
	Title Node
	// Icon is an optional icon shown in a tile before the title.
	Icon Node
	// Subtitle is optional muted text under the title.
	Subtitle string
	// Footer is the footer content for the value shown (buttons): a slot,
	// <slot:footer let={t}>; omitted when nil or when it renders nothing.
	Footer func(*T) Node
	// Closed hides the close button, and a click on the backdrop and Escape
	// don't close it: only its own buttons, or the server, write nil.
	Closed bool
	// OnClose, when set, is the handler called once a close by the modal's
	// own affordances has faded out (a func, as an on… attribute takes).
	OnClose any
	// Size is "sm", "" / "md" (default), "lg" or "xl".
	Size  string
	Class string
	// Children is the content for the value shown: the tag's content, its
	// let naming the value.
	Children func(*T) Node
}

// modalTime is how long ModalOf fades in and out.
const modalTime = 150 * time.Millisecond

func (m ModalOf[T]) classes() string {
	class := "vk-modal"
	switch m.Size {
	case "sm", "lg", "xl":
		class += " vk-modal--" + m.Size
	}
	return withClass(class, m.Class)
}

// open is the browser's expression of the modal showing: For isn't nil.
func (m ModalOf[T]) open() Binding {
	return BindExpr(m.For.Get() != nil, `["!=",{"$":0},null]`, m.For)
}

// close is what the close button, the backdrop and Escape do: nil into For
// and the focus back, then, once the fade is over, the server told (the
// region the content is in rendered again, or OnClose called).
func (m ModalOf[T]) close(c Commands) Commands {
	c = c.Set(m.For, nil).PopFocus()
	switch {
	case m.OnClose != nil:
		return c.Debounce(modalTime).Push(m.OnClose)
	case m.ID != "":
		return c.Debounce(modalTime).Reload("#" + m.ID.String())
	}
	return c
}

// dismiss is the backdrop's click handler: nil for a Closed modal.
func (m ModalOf[T]) dismiss() any { return when(!m.Closed, m.close(JS)) }

// escape is the dialog's keydown handler: Escape closes it.
func (m ModalOf[T]) escape() any { return when(!m.Closed, m.close(JS.Key("Escape"))) }

// labelled is the dialog's aria-labelledby: its title's id.
func (m ModalOf[T]) labelled() any {
	return when(m.Title != nil && m.ID != "", m.ID.Suffix("title").String())
}

// opened is what the browser does each time the modal shows, as its
// content region asks for the content: the focus remembered, then moved to
// the dialog and on to the content's first field when it has one.
func (m ModalOf[T]) opened() []Attr {
	return []Attr{{"onVkInit", On(JS.PushFocus().Focus(Closest("[role=dialog]")).FocusFirst(Closest("vk-lazy")))}}
}

// content is the body and footer for the value shown; an empty body, busy,
// while the modal is closed or its content is on its way.
func (m ModalOf[T]) content() Node {
	v := m.For.Get()
	if v == nil {
		return El("div", []Attr{{"class", "vk-modal__body"}, {"aria-busy", "true"}})
	}
	var body, footer Node
	if m.Children != nil {
		body = m.Children(v)
	}
	if m.Footer != nil {
		if f := m.Footer(v); Has(f) {
			footer = El("div", []Attr{{"class", "vk-modal__footer"}}, f)
		}
	}
	return Fragment(El("div", []Attr{{"class", "vk-modal__body"}}, body), footer)
}

func (m ModalOf[T]) header() Node {
	if m.Icon == nil && m.Title == nil && m.Subtitle == "" && m.Closed {
		return nil
	}
	var kids []Node
	if m.Icon != nil {
		kids = append(kids, El("span", []Attr{{"class", "vk-modal__icon"}}, m.Icon))
	}
	if m.Title != nil || m.Subtitle != "" {
		var h []Node
		if m.Title != nil {
			h = append(h, El("h2", []Attr{{"class", "vk-modal__title"}, {"id", m.labelled()}}, m.Title))
		}
		if m.Subtitle != "" {
			h = append(h, El("p", []Attr{{"class", "vk-modal__subtitle"}}, Text(m.Subtitle)))
		}
		kids = append(kids, El("div", []Attr{{"class", "vk-modal__heading"}}, h...))
	}
	if !m.Closed {
		kids = append(kids, El("button", []Attr{{"type", "button"}, {"class", "vk-modal__close"}, {"aria-label", "Close"}, {"onClick", On(m.close(JS))}}, Icon("x")))
	}
	return El("div", []Attr{{"class", "vk-modal__header"}}, kids...)
}

func (m ModalOf[T]) View() Node {
	open := m.open()
	region := lazy(LazyProps{When: open, Bare: true, Each: true, Children: m.content()}, m.opened())
	dialog := El("div", append([]Attr{{"id", nonEmpty(m.ID.String())}, {"class", m.classes()}, {"role", "dialog"}, {"aria-modal", "true"},
		{"aria-labelledby", m.labelled()}, {"tabindex", "-1"}}, on("onKeyDown", m.escape())...), m.header(), region)
	root := El("div", []Attr{{"class", "vk-modal-root"}, {"show", open}, {"transition", Fade(modalTime)}},
		El("div", append([]Attr{{"class", "vk-modal-backdrop"}}, on("onClick", m.dismiss())...)), dialog)
	var id string
	if m.ID != "" {
		id = m.ID.Suffix("layer").String()
	}
	return Teleport{ID: id, Children: root}
}

func (m ModalOf[T]) Render(ctx context.Context, w io.Writer) error { return m.View().Render(ctx, w) }
