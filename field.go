package ui

import (
	"context"
	"io"
)

// Option is one choice of a Select.
type Option struct{ Value, Label string }

// FieldProps is Field, by its former name.
//
// Deprecated: use Field, or the kind of field: ui.Input{…}, or
// <ui.Input …> in a template.
type FieldProps = Field

type fieldKind int

const (
	kindInput fieldKind = iota
	kindTextarea
	kindSelect
	kindCheckbox
)

// Input is a single-line text field (Type sets the HTML input type):
//
//	<ui.Input name="email" label="Email" type="email" required/>
type Input Field

// View is the Field the input is.
func (f Input) View() Node { return Field(f).View() }

// Render renders the input's View.
func (f Input) Render(ctx context.Context, w io.Writer) error { return f.View().Render(ctx, w) }

// Password is a masked text field.
type Password Field

// View is the Field the password field is.
func (f Password) View() Node {
	f.Type = "password"
	if f.Autocomplete == "" {
		f.Autocomplete = "current-password"
	}
	return Field(f).View()
}

// Render renders the password field's View.
func (f Password) Render(ctx context.Context, w io.Writer) error { return f.View().Render(ctx, w) }

// Textarea is a multi-line text field.
type Textarea Field

// View is the Field the textarea is.
func (f Textarea) View() Node { f.kind = kindTextarea; return Field(f).View() }

// Render renders the textarea's View.
func (f Textarea) Render(ctx context.Context, w io.Writer) error { return f.View().Render(ctx, w) }

// Select is a dropdown over Options.
type Select Field

// View is the Field the select is.
func (f Select) View() Node { f.kind = kindSelect; return Field(f).View() }

// Render renders the select's View.
func (f Select) Render(ctx context.Context, w io.Writer) error { return f.View().Render(ctx, w) }

// Checkbox is a single boolean field; its label sits beside the box.
type Checkbox Field

// View is the Field the checkbox is.
func (f Checkbox) View() Node { f.kind = kindCheckbox; return Field(f).View() }

// Render renders the checkbox's View.
func (f Checkbox) Render(ctx context.Context, w io.Writer) error { return f.View().Render(ctx, w) }
