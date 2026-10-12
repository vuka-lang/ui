package ui

// AlertVariant is an Alert's tone.
type AlertVariant string

const (
	AlertInfo    AlertVariant = "info"
	AlertSuccess AlertVariant = "success"
	AlertWarning AlertVariant = "warning"
	AlertError   AlertVariant = "error"
)

// AlertProps configures an Alert; Children is the message body.
type AlertProps struct {
	// Variant is the tone: AlertInfo (default), AlertSuccess, AlertWarning or
	// AlertError.
	Variant AlertVariant
	// Title is an optional bold heading above the body.
	Title string
	// Dismissible adds a close button that hides the alert in the browser, no
	// round trip.
	Dismissible bool
	Class       string
	Children    Node
}

// Alert is an inline message box in one of four tones:
//
//	ui.Alert(ui.AlertProps{Variant: ui.AlertSuccess, Title: "Saved"}, ui.Text("Your changes are live."))
//
// A dismissible alert hides itself with a browser command; a server that
// wants it gone for good drops it from its own render instead.
func Alert(p AlertProps) Node {
	variant := p.Variant
	if variant == "" {
		variant = AlertInfo
	}
	class := "vk-alert vk-alert--" + string(variant)
	if p.Class != "" {
		class += " " + p.Class
	}

	var body []Node
	if p.Title != "" {
		body = append(body, El("div", []Attr{{"class", "vk-alert__title"}}, Text(p.Title)))
	}
	body = append(body, Child(p.Children))

	kids := []Node{El("div", []Attr{{"class", "vk-alert__body"}}, body...)}
	if p.Dismissible {
		kids = append(kids, El("button", []Attr{
			{"type", "button"}, {"class", "vk-alert__close"}, {"aria-label", "Dismiss"},
			{"onClick", On(JS.Hide(Closest(".vk-alert")))},
		}, Text("×")))
	}
	return El("div", []Attr{{"class", class}, {"role", "alert"}}, kids...)
}
