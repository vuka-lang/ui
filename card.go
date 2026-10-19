package ui

// Card is a bordered surface with an optional header (a title on the left,
// actions on the right) and a body:
//
//	<ui.Card title={ui.Text("Members")}>
//		<slot:header><ui.Button variant={ui.Primary}>Invite</ui.Button></slot:header>
//		{table}
//	</ui.Card>
//
// or in Go, ui.Card{Title: ui.Text("Members"), Children: table}.
type Card struct {
	// Title is the header's left content; a string becomes a heading, any
	// other Node is placed as is.
	Title Node
	// Header is the header's right slot, for actions (ui.Actions) or a badge.
	Header Node
	// NoPad drops the body's padding, e.g. for a flush DataTable.
	NoPad    bool
	Class    string
	Children Node
}

// CardProps is Card, by its former name.
//
// Deprecated: use Card: ui.Card{…}, or <ui.Card …> in a template.
type CardProps = Card
