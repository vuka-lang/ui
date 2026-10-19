package ui

// Option is one choice of a Select.
type Option struct{ Value, Label string }

// Field is a form field: its control, label, help and error. It renders the
// control its Type names, an HTML input type ("text" by default); Input,
// Password, Textarea, Select and Checkbox are the fields of each kind:
//
//	<ui.Field name="age" label="Age" type="number" min="18"/>
//	<ui.Input name="email" label="Email" type="email" required/>
type Field struct {
	// Name is the control's name, matching the form struct's form: tag.
	Name string
	// Label is shown above the control; omitted when empty.
	Label string
	// Type is the HTML input type ("text" default, "email", "number", "date",
	// "time", "url", "tel"); the dedicated kinds set it for password,
	// textarea, select and checkbox.
	Type string
	// Value is the control's value; for a checkbox, "true" ticks it.
	Value string
	// Options are a select's choices.
	Options []Option
	// Error is the field's message; when set the control is marked invalid.
	// The host supplies it (its own validation), since ui does not import the
	// runtime that tracks errors.
	Error string
	// Help is a hint shown under the control.
	Help        string
	Placeholder string
	Required    bool
	Disabled    bool
	// Rows is a textarea's height; 3 when 0.
	Rows int
	// Min, Max and Step bound a number or date control.
	Min, Max, Step string
	Autocomplete   string
	Class          string
	// Attrs are extra attributes on the control.
	Attrs []Attr
	kind  fieldKind
}

// FieldProps is Field, by its former name.
//
// Deprecated: use Field, or the kind of field: ui.Input{…}.Render(), or
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

// Render is the Field the input is.
func (f Input) Render() Node { return Field(f).Render() }

// Password is a masked text field.
type Password Field

// Render is the Field the password field is.
func (f Password) Render() Node {
	f.Type = "password"
	if f.Autocomplete == "" {
		f.Autocomplete = "current-password"
	}
	return Field(f).Render()
}

// Textarea is a multi-line text field.
type Textarea Field

// Render is the Field the textarea is.
func (f Textarea) Render() Node { f.kind = kindTextarea; return Field(f).Render() }

// Select is a dropdown over Options.
type Select Field

// Render is the Field the select is.
func (f Select) Render() Node { f.kind = kindSelect; return Field(f).Render() }

// Checkbox is a single boolean field; its label sits beside the box.
type Checkbox Field

// Render is the Field the checkbox is.
func (f Checkbox) Render() Node { f.kind = kindCheckbox; return Field(f).Render() }

func (f Field) id() string    { return "f-" + f.Name }
func (f Field) invalid() bool { return f.Error != "" }

func (f Field) classes() string {
	c := "vk-field"
	if f.kind == kindCheckbox {
		c += " vk-field--check"
	}
	if f.invalid() {
		c += " vk-field--invalid"
	}
	return withClass(c, f.Class)
}

// control is the class of a control: base, marked when the field is invalid.
func (f Field) control(base string) string {
	if f.invalid() {
		return base + " " + base + "--invalid"
	}
	return base
}

// common are the attributes every control but the checkbox has, after its
// own class (and type or rows).
func (f Field) common() []Attr {
	a := []Attr{{"id", f.id()}, {"name", f.Name}}
	if f.Required {
		a = append(a, Attr{"required", true})
	}
	if f.Disabled {
		a = append(a, Attr{"disabled", true})
	}
	if f.invalid() {
		a = append(a, Attr{"aria-invalid", "true"}, Attr{"aria-describedby", f.id() + "-error"})
	}
	if f.Autocomplete != "" {
		a = append(a, Attr{"autocomplete", f.Autocomplete})
	}
	return append(a, f.Attrs...)
}

func (f Field) inputType() string {
	if f.Type == "" {
		return "text"
	}
	return f.Type
}

func (f Field) rows() int {
	if f.Rows == 0 {
		return 3
	}
	return f.Rows
}
