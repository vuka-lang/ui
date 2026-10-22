package ui

// ModalProps is Modal, by its former name.
//
// Deprecated: use Modal: ui.Modal{…}, or <ui.Modal …> in a template.
type ModalProps = Modal

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
