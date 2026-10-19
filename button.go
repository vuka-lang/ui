package ui

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

// Button is a token-styled button, or a link when Href is set:
//
//	<ui.Button variant={ui.Primary} icon={ui.Icon("plus")}>New</ui.Button>
//	<ui.Button on={c.Save}>Save</ui.Button>       // spins while saving
//	<ui.Button href="/books" nav>Back</ui.Button>
//
// A click handler (On) goes busy until its reply, so the spinner shows itself;
// Loading forces it for a server-controlled state.
type Button struct {
	// ID names the element, e.g. for a JS command to target it.
	ID string
	// Variant is the tone: Default, Primary, Success, Danger, Warning, Info or
	// Ghost.
	Variant Variant
	// Size is SizeMd (default), SizeSm or SizeLg.
	Size Size
	// Icon is an optional icon shown before the label, e.g. ui.Icon("plus").
	Icon Node
	// IconOnly drops the label's spacing for a square icon button; give it a
	// Title for its accessible name.
	IconOnly bool
	// Loading forces the busy state: a spinner in place of the icon, and the
	// button disabled. A live button left to its own busy state (see On) needs
	// nothing here — it goes busy while its event is in flight.
	Loading bool
	// Disabled renders the button disabled.
	Disabled bool
	// Type is "button" (default), "submit" or "reset".
	Type string
	// Form is the id of the form a submit button submits when it sits outside
	// it.
	Form string
	// On is what a click does: a ui.JS chain or a live event handler (a func,
	// as an on… attribute takes). A live click goes busy until its reply, so a
	// click button shows the spinner on its own.
	On any
	// Href makes the button a link (an a element). Nav follows it over a live
	// connection (ui.Link's navigation) instead of a full load.
	Href string
	Nav  bool
	// Title is the element's title/tooltip and, for an icon-only button, its
	// accessible name.
	Title string
	Class string
	// Attrs are any extra attributes.
	Attrs    []Attr
	Children Node
}

// ButtonProps is Button, by its former name.
//
// Deprecated: use Button: ui.Button{…}.Render(), or <ui.Button …> in a template.
type ButtonProps = Button

func (b Button) classes() string {
	c := "vk-btn"
	if b.Variant != "" {
		c += " vk-btn--" + string(b.Variant)
	}
	if b.Size != "" {
		c += " vk-btn--" + string(b.Size)
	}
	if b.IconOnly {
		c += " vk-btn--icon"
	}
	if b.Type == "submit" {
		c += " vk-btn--submit"
	}
	return withClass(c, b.Class)
}

func (b Button) typ() string {
	if b.Type == "" {
		return "button"
	}
	return b.Type
}

// lead is what goes before the label: the spinner while loading, else the
// icon.
func (b Button) lead() Node {
	if b.Loading {
		return Spinner()
	}
	return b.Icon
}

// Submit is a form's submit button: the Button in its primary variant, type
// submit. It shows its busy state while the live form waits for the submit's
// reply. Set Form to submit a form it sits outside (a dialog footer):
//
//	<ui.Submit>Save</ui.Submit>
type Submit Button

// Render is the Button the submit button is.
func (s Submit) Render() Node {
	b := Button(s)
	b.Type = "submit"
	if b.Variant == "" {
		b.Variant = Primary
	}
	return b.Render()
}
