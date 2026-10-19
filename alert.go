package ui

// AlertVariant is an Alert's tone.
type AlertVariant string

const (
	AlertInfo    AlertVariant = "info"
	AlertSuccess AlertVariant = "success"
	AlertWarning AlertVariant = "warning"
	AlertError   AlertVariant = "error"
)

// Alert is an inline message box in one of four tones:
//
//	<ui.Alert variant={ui.AlertSuccess} title="Saved">Your changes are live.</ui.Alert>
//
// A dismissible alert hides itself with a browser command; a server that
// wants it gone for good drops it from its own render instead.
type Alert struct {
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

// AlertProps is Alert, by its former name.
//
// Deprecated: use Alert: ui.Alert{…}, or <ui.Alert …> in a template.
type AlertProps = Alert

func (a Alert) classes() string {
	variant := a.Variant
	if variant == "" {
		variant = AlertInfo
	}
	return withClass("vk-alert vk-alert--"+string(variant), a.Class)
}
