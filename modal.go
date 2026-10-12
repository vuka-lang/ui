package ui

// ModalProps configures a Modal; Children is its body.
type ModalProps struct {
	// Ref names the modal's root for the open/close commands (OpenModal,
	// CloseModal) and for a server variant's Show/Hide. It must be unique on
	// the page; the default is "modal".
	Ref string
	// Title is the header content; omitted when nil.
	Title Node
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
	// Size is "sm", "" (default) or "lg".
	Size     string
	Class    string
	Children Node
}

// Modal is a dialog centred over a backdrop. A browser-only modal is hidden
// until OpenModal shows it and keeps its open state in the DOM:
//
//	<button onClick={ui.OpenModal("edit")}>Edit</button>
//	ui.Modal(ui.ModalProps{Ref: "edit", Title: ui.Text("Edit")}, body…)
//
// A server-owned modal sets Open from a live field and OnClose to the handler
// that clears it, so the server decides when it shows:
//
//	ui.Modal(ui.ModalProps{Ref: "edit", Open: m.open, OnClose: m.Close}, body…)
//
// Opening traps focus (PushFocus then FocusFirst); closing restores it
// (PopFocus). Escape and a backdrop click close a dismissible modal.
func Modal(p ModalProps) Node {
	ref := p.Ref
	if ref == "" {
		ref = "modal"
	}
	dismissible := !p.Closed

	class := "vk-modal"
	switch p.Size {
	case "sm":
		class += " vk-modal--sm"
	case "lg":
		class += " vk-modal--lg"
	}
	if p.Class != "" {
		class += " " + p.Class
	}

	// close is what the close affordances do: the server handler (plus a
	// browser PopFocus) when given, else a browser-only close.
	var close any
	if p.OnClose != nil {
		close = On(JS.PopFocus().Push(p.OnClose))
	} else {
		close = On(CloseModal(ref))
	}

	var head []Node
	if p.Title != nil {
		head = append(head, El("h2", []Attr{{"class", "vk-modal__title"}}, p.Title))
	}
	if dismissible {
		head = append(head, El("button", []Attr{
			{"type", "button"}, {"class", "vk-modal__close"}, {"aria-label", "Close"}, {"onClick", close},
		}, Text("×")))
	}

	dialogKids := []Node{}
	if len(head) > 0 {
		dialogKids = append(dialogKids, El("div", []Attr{{"class", "vk-modal__header"}}, head...))
	}
	dialogKids = append(dialogKids, El("div", []Attr{{"class", "vk-modal__body"}}, Child(p.Children)))
	if p.Footer != nil {
		dialogKids = append(dialogKids, El("div", []Attr{{"class", "vk-modal__footer"}}, p.Footer))
	}

	dialogAttrs := []Attr{{"class", class}, {"role", "dialog"}, {"aria-modal", "true"}, {"tabindex", "-1"}}
	if dismissible {
		esc := JS.Key("Escape").Hide(Ref(ref)).PopFocus()
		if p.OnClose != nil {
			esc = esc.Push(p.OnClose)
		}
		dialogAttrs = append(dialogAttrs, Attr{"onKeyDown", On(esc)})
	}

	backdropAttrs := []Attr{{"class", "vk-modal-backdrop"}}
	if dismissible {
		backdropAttrs = append(backdropAttrs, Attr{"onClick", close})
	}

	rootAttrs := []Attr{{"ref", ref}, {"class", "vk-modal-root"}}
	if !p.Open {
		rootAttrs = append(rootAttrs, Attr{"style", Style{"display": "none"}})
	}
	return El("div", rootAttrs,
		El("div", backdropAttrs),
		El("div", dialogAttrs, dialogKids...),
	)
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
