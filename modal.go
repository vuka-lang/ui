package ui

// Modal is a dialog centred over a backdrop. A browser-only modal is hidden
// until OpenModal shows it and keeps its open state in the DOM:
//
//	<button onClick={ui.OpenModal("edit")}>Edit</button>
//	<ui.Modal ref="edit" title={ui.Text("Edit")}>…</ui.Modal>
//
// A server-owned modal sets Open from a live field and OnClose to the handler
// that clears it, so the server decides when it shows:
//
//	<ui.Modal ref="edit" open={m.open} onClose={m.Close}>…</ui.Modal>
//
// Opening traps focus (PushFocus then FocusFirst); closing restores it
// (PopFocus). Escape and a backdrop click close a dismissible modal.
type Modal struct {
	// Ref names the modal's root for the open/close commands (OpenModal,
	// CloseModal) and for a server variant's Show/Hide. It must be unique on
	// the page; the default is "modal".
	Ref string
	// Title is the header content; omitted when nil.
	Title Node
	// Icon is an optional icon shown in a tile before the title, e.g.
	// ui.Icon("check-circle").
	Icon Node
	// Subtitle is optional muted text under the title.
	Subtitle string
	// Footer is the footer content (buttons); omitted when nil.
	Footer Node
	// Open renders the modal already shown, for a server-owned modal whose
	// open state is a live field. A browser-only modal leaves it false and
	// opens with OpenModal.
	Open bool
	// Dismissible shows a close button and closes on backdrop click and
	// Escape. Default true; set Closed to make a modal that only its own
	// buttons or the server can close.
	Closed bool
	// OnClose, when set, is the server handler the close affordances call
	// (a func, as an on… attribute takes); the close also pops focus in the
	// browser. Without it the close is browser-only (CloseModal).
	OnClose any
	// Size is "sm", "" / "md" (default), "lg" or "xl".
	Size     string
	Class    string
	Children Node
}

// ModalProps is Modal, by its former name.
//
// Deprecated: use Modal: ui.Modal{…}, or <ui.Modal …> in a template.
type ModalProps = Modal

func (m Modal) ref() string {
	if m.Ref == "" {
		return "modal"
	}
	return m.Ref
}

func (m Modal) classes() string {
	class := "vk-modal"
	switch m.Size {
	case "sm", "lg", "xl":
		class += " vk-modal--" + m.Size
	}
	return withClass(class, m.Class)
}

// hidden is the root's style until the modal opens.
func (m Modal) hidden() any {
	if m.Open {
		return nil
	}
	return Style{"display": "none"}
}

// close is what the close affordances do: the server handler (after a
// browser PopFocus) when there is one, else a browser-only close.
func (m Modal) close() Commands {
	if m.OnClose != nil {
		return JS.PopFocus().Push(m.OnClose)
	}
	return CloseModal(m.ref())
}

// dismiss is the close affordances' handler: close, or nil for a modal
// only its own buttons or the server close.
func (m Modal) dismiss() any { return when(!m.Closed, m.close()) }

// escape is the dialog's keydown handler: Escape closes a dismissible modal.
func (m Modal) escape() any {
	if m.Closed {
		return nil
	}
	esc := JS.Key("Escape").Hide(Ref(m.ref())).PopFocus()
	if m.OnClose != nil {
		esc = esc.Push(m.OnClose)
	}
	return esc
}

// OpenModal shows the modal named ref, trapping focus inside it: it remembers
// what had focus, shows the root and focuses the dialog's first field.
func OpenModal(ref string) Commands {
	return JS.PushFocus().Show(Ref(ref)).FocusFirst(Ref(ref))
}

// CloseModal hides the modal named ref and restores the focus OpenModal
// remembered.
func CloseModal(ref string) Commands {
	return JS.Hide(Ref(ref)).PopFocus()
}
