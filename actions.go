package ui

// Cancel is a neutral button that dismisses a modal or form. On is what it
// does: a ui.JS chain (ui.CloseModal) or a live handler. The label is
// "Cancel" unless the first label is given.
func Cancel(on any, label ...string) Node {
	l := "Cancel"
	if len(label) > 0 && label[0] != "" {
		l = label[0]
	}
	return Button{On: on, Children: Text(l)}.Render()
}
