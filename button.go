package ui

import (
	"context"
	"io"
)

// Variant is a component's tone, shared by Button and Badge. It maps to the
// --ui-* colour tokens, not to any framework's class names.
type Variant string

const (
	// Default is the neutral, subtle look (Button's resting state).
	Default Variant = ""
	Primary Variant = "primary"
	Success Variant = "success"
	Danger  Variant = "danger"
	Warning Variant = "warning"
	Info    Variant = "info"
	// Ghost is a transparent button that only shows its hover state.
	Ghost Variant = "ghost"
)

// Size is a Button's size.
type Size string

const (
	SizeMd Size = ""   // the default
	SizeSm Size = "sm" // compact
	SizeLg Size = "lg" // large
)

// ButtonProps is Button, by its former name.
//
// Deprecated: use Button: ui.Button{…}, or <ui.Button …> in a template.
type ButtonProps = Button

// Spinner is the busy indicator Button and the form controls use: a spinning
// ring drawn in CSS, so it needs no script.
func Spinner() Node { return HTML(`<span class="vk-spinner" aria-hidden="true"/>`) }

// Submit is a form's submit button: the Button in its primary variant, type
// submit. It shows its busy state while the live form waits for the submit's
// reply. Set Form to submit a form it sits outside (a dialog footer):
//
//	<ui.Submit>Save</ui.Submit>
type Submit Button

// View is the Button the submit button is.
func (s Submit) View() Node {
	b := Button(s)
	b.Type = "submit"
	if b.Variant == "" {
		b.Variant = Primary
	}
	return b.View()
}

// Render renders the submit button's View.
func (s Submit) Render(ctx context.Context, w io.Writer) error { return s.View().Render(ctx, w) }
