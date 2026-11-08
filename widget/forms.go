package widget

import "github.com/vuka-lang/ui/v2"

// The form's components beside the controls, so a form's markup takes
// everything from this package:
//
//	<form for={Form} onSubmit={save}>
//		<widget.Summary/>
//		<widget.Field for={Form.Name}/>
//		<label for={Form.Email}>Email</label>
//		<widget.Error for={Form.Email}/>
//		<widget.Email model={Form.Email}/>
//		<widget.Fieldset for={Form.Address}/>
//	</form>
//
// They are the components package ui declared under the same names
// (ui.Field, ui.Error, …), which remain as deprecated names of these.
// Field is the field group component — label, hint, error and the control
// the resolution order picks — and Props the contract a widget embeds.
type (
	// Field is a form field's group: its label, hint, error and control,
	// the control its widget (see the package's resolution order).
	Field = ui.Field
	// Error is a field's error message, where the field's markup puts
	// it; it renders nothing when the field shows none.
	Error = ui.Error
	// Summary is a form's error summary, which a failed submit shows.
	Summary = ui.Summary
	// Fieldset groups fields under a legend: a struct field's, or its
	// children.
	Fieldset = ui.Fieldset
	// Choices are the controls of a field with choices, a radio or a
	// checkbox each, for markup laying the field out itself.
	Choices = ui.Choices
)

// ErrorProps is what an app's error message component embeds: the field,
// its message, and the attributes its root element carries (the class,
// the id the control's aria-describedby names, the data-vk-error the
// runtime reads).
//
//	type FieldError struct{ widget.ErrorProps }
//
//	<p {Attrs...} class="text-red-600">{Message}</p>
type ErrorProps struct {
	Model   ui.FieldRef
	Message string
	Attrs   []ui.Attr
}

// SummaryProps is what an app's error summary component embeds: its
// title, each error with the link to its field, and the attributes its
// root element carries (the id, tabindex, autofocus and data-vk-summary
// the runtime moves focus to). Its root should hold a role="alert".
type SummaryProps struct {
	Title string
	Items []ui.SummaryItem
	Attrs []ui.Attr
}

// UseError makes c, a component embedding ErrorProps, what renders every
// form field's error message, in every form (<widget.Error>, the field
// groups); nil puts the form's own back.
//
//	widget.UseError(components.FieldError{})
func UseError(c any) { ui.UseErrorWidget(c) }

// UseSummary makes c, a component embedding SummaryProps, what renders
// every form's error summary; nil puts the form's own back. A form
// checked over HTTP without a connection shows the runtime's own.
func UseSummary(c any) { ui.UseSummaryWidget(c) }
