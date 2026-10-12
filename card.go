package ui

// CardProps configures a Card; Children is its body.
type CardProps struct {
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

// Card is a bordered surface with an optional header (a title on the left,
// actions on the right) and a body:
//
//	ui.Card(ui.CardProps{
//		Title:  ui.Text("Members"),
//		Header: ui.Actions(ui.Button(ui.ButtonProps{Variant: ui.Primary, Children: ui.Text("Invite")})),
//		Children: table,
//	})
func Card(p CardProps) Node {
	var kids []Node
	if p.Title != nil || p.Header != nil {
		head := []Node{El("div", []Attr{{"class", "vk-card__title"}}, Child(p.Title))}
		if p.Header != nil {
			head = append(head, El("div", []Attr{{"class", "vk-card__header-actions"}}, p.Header))
		}
		kids = append(kids, El("div", []Attr{{"class", "vk-card__header"}}, head...))
	}
	bodyClass := "vk-card__body"
	if p.NoPad {
		bodyClass += " vk-card__body--flush"
	}
	kids = append(kids, El("div", []Attr{{"class", bodyClass}}, Child(p.Children)))

	class := "vk-card"
	if p.Class != "" {
		class += " " + p.Class
	}
	return El("div", []Attr{{"class", class}}, kids...)
}
