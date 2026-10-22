package ui

// AlertVariant is an Alert's tone.
type AlertVariant string

const (
	AlertInfo    AlertVariant = "info"
	AlertSuccess AlertVariant = "success"
	AlertWarning AlertVariant = "warning"
	AlertError   AlertVariant = "error"
)

// AlertProps is Alert, by its former name.
//
// Deprecated: use Alert: ui.Alert{…}, or <ui.Alert …> in a template.
type AlertProps = Alert
