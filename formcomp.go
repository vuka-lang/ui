package ui

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/vuka-lang/ui/v2/forms"
)

// The form components: what <ui.Form for>, <ui.Field for>, <ui.Error for>,
// <ui.Fieldset> and <ui.Summary> render. Each renders with its context
// (ui.Ctx), when the form's state is final: the ids its key gives, the
// errors that show.

// forView is a Form with For set: a struct form, bound, checked and
// submitted by the form itself (see FormState).
func (f Form) forView() Node {
	return Ctx(func(ctx context.Context) Node {
		ref, _ := refOf(f.For)
		b, err := ref.resolve()
		if err != nil {
			return Try(nil, err)
		}
		if b.f != nil {
			return Try(nil, fmt.Errorf("ui: <ui.Form for={…}> takes the form, not one of its fields (%s)", b.f.goPath))
		}
		st := b.st
		st.key = keyString(f.Key)
		st.beginRender()
		if st.idem == "" {
			st.idem = newIdemKey()
		}
		ctl := &formCtl{form: b.form, st: st, onSubmit: f.OnSubmit, onChange: f.OnChange, validate: f.Validate}
		identity := st.identity()
		if reg, ok := liveHost(ctx).(FormRegistrar); ok {
			reg.Form(identity, ctl.submit)
		}
		attrs := []Attr{
			{"class", withClass("vk-form", f.Class)},
			{"method", "post"},
			{"action", nonEmpty(f.Action)},
			{"id", cmpOr(f.ID, identity)},
			{"novalidate", true},
			{"data-vk-form", identity},
			{"data-vk-submit", cmpOr(f.Submit, "prevent")},
			{"onSubmit", On(ctl.submit)},
		}
		if f.Validate == "" || f.OnChange != nil {
			attrs = append(attrs,
				Attr{"data-vk-debounce", "250"},
				Attr{"onInput", On(ctl.change)},
				Attr{"onChange", On(ctl.change)},
				Attr{"onFocusout", On(ctl.change)})
		}
		used := make([]string, 0, len(st.used))
		for _, fl := range st.ft.fields {
			if st.used[fl.path] {
				used = append(used, fl.path)
			}
		}
		hidden := func(name string, v any) Node {
			return El("input", []Attr{{"type", "hidden"}, {"name", name}, {"value", v}})
		}
		kids := []Node{
			hidden(FormIDField, identity),
			hidden(FormIdemField, st.idem),
			hidden(FormUsedField, strings.Join(used, ",")),
		}
		if h := formHostOf(ctx); h != nil {
			if tok := h.FormToken(ctx); tok != "" {
				kids = append(kids, hidden(FormCSRFField, tok))
			}
		}
		if f.Error != "" {
			kids = append(kids, Alert{Variant: AlertError, Children: Text(f.Error)})
		}
		kids = append(kids, Ctx(func(ctx context.Context) Node {
			if st.summary {
				return nil // the form renders its <ui.Summary> elsewhere
			}
			return summaryOf(ctx, b, "")
		}))
		body := f.Children
		if !Has(body) {
			body = Fragment(allFields(b), Submit{Children: Text(cmpOr(f.SubmitLabel, "Save"))})
		}
		kids = append(kids, withContext(func(ctx context.Context) context.Context {
			return context.WithValue(ctx, currentKey{}, &current{b: b})
		}, body))
		kids = append(kids, El("input", []Attr{{"type", "hidden"}, {"name", FormFieldsField}, {"value", lazyValue{func(ctx context.Context) any {
			st.rendered, st.summary = st.rendering, st.summaryNow
			return signFields(formKeyOf(ctx), identity, fieldList(st.rendering, st.ft))
		}}}}))
		return El("form", attrs, kids...)
	})
}

// beginRender starts the form's render: what it notes of its fields (the
// controls, hints and errors it has) starts afresh.
func (st *formState) beginRender() {
	st.rendering = map[string]string{}
	st.controls, st.hints, st.errorShown = map[string]int{}, map[string]bool{}, map[string]bool{}
	st.summaryNow = false
}

func keyString(k any) string {
	if k == nil {
		return ""
	}
	return fmt.Sprint(k)
}

// allFields is every field of the form, in order: a childless <ui.Form
// for>'s body. A struct field is a fieldset of its own fields.
func allFields(b boundField) Node {
	var out []Node
	for _, f := range b.st.ft.fields {
		if strings.Contains(f.path, ".") {
			continue // a group's, rendered by its fieldset
		}
		out = append(out, fieldNode(b, f, Field{}))
	}
	return Fragment(out...)
}

// fieldNode is the field group of field f, opts overriding what f's tags
// say.
func fieldNode(b boundField, f *formField, opts Field) Node {
	fb := boundField{form: b.form, st: b.st, f: f}
	if f.group {
		return Fieldset{Legend: f.label, For: fb}
	}
	return Ctx(func(ctx context.Context) Node { return fb.group(ctx, opts) })
}

// forView is a Field with For set: the field group of a form's field.
func (f Field) forView() Node {
	return Ctx(func(ctx context.Context) Node {
		b, err := resolveFor(ctx, f.For)
		if err != nil {
			return Try(nil, err)
		}
		if b.f == nil {
			return Try(nil, fmt.Errorf("ui: <ui.Field for={…}> takes a field of a form, not the form"))
		}
		if b.f.group {
			return Fieldset{Legend: cmpOr(f.Label, b.f.label), For: b, Class: f.Class}
		}
		return b.group(ctx, f)
	})
}

// resolveFor resolves a for attribute's value; a path string names a field
// of the form being rendered.
func resolveFor(ctx context.Context, v any) (boundField, error) {
	if b, ok := v.(boundField); ok {
		return b, nil
	}
	if path, ok := v.(string); ok {
		c := currentForm(ctx)
		if c == nil {
			return boundField{}, fmt.Errorf("ui: for=%q names a field outside a <ui.Form for>", path)
		}
		f := c.b.st.ft.byPath[path]
		if f == nil {
			return boundField{}, fmt.Errorf("ui: for=%q: %s has no field %[1]q", path, c.b.st.ft.t.Name())
		}
		return boundField{form: c.b.form, st: c.b.st, f: f}, nil
	}
	if v == nil {
		if c := currentForm(ctx); c != nil {
			return c.b, nil
		}
		return boundField{}, fmt.Errorf("ui: for is missing: write for={Form.Field}")
	}
	ref, ok := refOf(v)
	if !ok {
		return boundField{}, fmt.Errorf("ui: for takes a field of a form, for={Form.Email}, not %T", v)
	}
	return ref.resolve()
}

// group is a field's group: its label, hint, error and control, in that
// order, the control linked to the hint and error by aria-describedby.
func (b boundField) group(ctx context.Context, opts Field) Node {
	f, st := b.f, b.st
	input := f.input
	switch {
	case opts.kind == kindTextarea:
		input = "textarea"
	case opts.kind == kindSelect:
		input = "select"
	case opts.kind == kindCheckbox:
		input = "checkbox"
	case opts.Type != "":
		input = opts.Type
	}
	if (input == "radio" || input == "checkboxes") && (f.choices != nil || len(opts.Options) > 0) {
		return b.choiceGroup(ctx, opts, input)
	}
	label := cmpOr(opts.Label, f.label)
	help := cmpOr(opts.Help, f.help)
	id := st.id(f.path)
	e, shows := st.visible(f.path)
	classes := "vk-field"
	if input == "checkbox" {
		classes += " vk-field--check"
	}
	if shows {
		classes += " vk-field--invalid"
	}
	var kids []Node
	hint := b.hint(help)
	errNode := b.errorNode(ctx, e, shows)
	ctl := b.control(ctx, opts, input, shows)
	if input == "checkbox" {
		kids = append(kids, hint, errNode, El("div", []Attr{{"class", "vk-check"}}, ctl,
			El("label", []Attr{{"class", "vk-check__label"}, {"for", id}}, Text(label), b.marker(ctx))))
	} else {
		kids = append(kids, El("label", []Attr{{"class", "vk-field__label"}, {"for", id}}, Text(label), b.marker(ctx)), hint, errNode, ctl)
		if isPassword(input) {
			kids = append(kids, passwordToggle(id))
		}
	}
	return El("div", []Attr{{"class", withClass(classes, opts.Class)}, {"data-invalid", shows}}, kids...)
}

func isPassword(input string) bool {
	switch input {
	case "password", "new-password", "current-password":
		return true
	}
	return false
}

// passwordToggle shows and hides a password (WCAG 3.3.8): a toggle button,
// its state aria-pressed.
func passwordToggle(id string) Node {
	toggle := JS.ToggleAttr(Sel("#"+id), "type", "text", "password").ToggleAttr(This, "aria-pressed", "true", "false")
	return El("button", []Attr{{"type", "button"}, {"class", "vk-field__reveal"}, {"aria-controls", id}, {"aria-pressed", "false"}, {"onClick", toggle}},
		Text("Show password"))
}

// marker is what a label says of the field being required: "(optional)"
// on optional fields, or "(required)" on required ones with
// Config.MarkRequired.
func (b boundField) marker(ctx context.Context) Node {
	req := b.f.required()
	switch c := configOf(ctx); {
	case c.MarkRequired && req:
		return El("span", []Attr{{"class", "vk-field__marker"}}, Text(" (required)"))
	case !c.MarkRequired && !req && b.f.typ.Kind() != reflect.Bool:
		return El("span", []Attr{{"class", "vk-field__marker"}}, Text(" (optional)"))
	}
	return nil
}

// hint is the field's hint, noted for its control's aria-describedby.
func (b boundField) hint(help string) Node {
	if help == "" {
		return nil
	}
	b.st.hints[b.f.path] = true
	return El("div", []Attr{{"class", "vk-field__help"}, {"id", b.st.id(b.f.path) + "-hint"}}, Text(help))
}

// errorNode is the field's error when it shows one: not a live region, its
// "Error:" prefix for screen readers only.
func (b boundField) errorNode(ctx context.Context, e forms.Error, shows bool, class ...string) Node {
	if !shows {
		return nil
	}
	if b.st.errorShown == nil {
		b.st.errorShown = map[string]bool{}
	}
	b.st.errorShown[b.f.path] = true
	cls := "vk-field__error"
	if len(class) > 0 {
		cls = withClass(cls, class[0])
	}
	return El("p", []Attr{{"class", cls}, {"id", b.st.id(b.f.path) + "-error"}},
		El("span", []Attr{{"class", "vk-sr-only"}}, Text("Error:")), Text(" "+b.st.message(ctx, e)))
}

// control is the field's control.
func (b boundField) control(ctx context.Context, opts Field, input string, invalid bool) Node {
	f := b.f
	mark := func(base string) string {
		if invalid {
			base += " " + base + "--invalid"
		}
		return base
	}
	extra := func(a []Attr) []Attr {
		if opts.Disabled {
			a = append(a, Attr{"disabled", true})
		}
		if opts.Placeholder != "" {
			a = append(a, Attr{"placeholder", opts.Placeholder})
		}
		if opts.Autocomplete != "" {
			a = append(a, Attr{"autocomplete", opts.Autocomplete})
		}
		return append(a, opts.Attrs...)
	}
	own := ""
	if opts.Placeholder != "" {
		own += " placeholder"
	}
	if opts.Autocomplete != "" {
		own += " autocomplete"
	}
	switch input {
	case "textarea":
		rows := opts.Rows
		if rows == 0 {
			rows = 3
		}
		attrs := append([]Attr{{"class", mark("vk-textarea")}, {"rows", rows}}, b.modelAttrs(ModelSpec{Tag: "textarea", Own: own})...)
		return El("textarea", extra(attrs), Text(b.st.fieldText(b.value(), f)))
	case "select":
		attrs := append([]Attr{{"class", mark("vk-select")}}, b.modelAttrs(ModelSpec{Tag: "select", Own: own})...)
		var options Node
		if len(opts.Options) > 0 {
			var os []Node
			if !f.required() || b.st.fieldText(b.value(), f) == "" {
				os = append(os, El("option", []Attr{{"value", ""}}, Text(f.placeholder)))
			}
			for _, o := range opts.Options {
				os = append(os, El("option", []Attr{{"value", o.Value}, {"selected", selected(b.value(), f, o.Value)}}, Text(o.Label)))
			}
			options = Fragment(os...)
		} else {
			options = b.options(ctx)
		}
		return El("select", extra(attrs), options)
	case "checkbox":
		attrs := append([]Attr{{"class", "vk-check__box"}}, b.modelAttrs(ModelSpec{Tag: "input", Type: "checkbox", Own: own})...)
		return El("input", extra(attrs))
	}
	spec := ModelSpec{Tag: "input", Own: own}
	switch input {
	case "password", "new-password", "current-password":
		spec.Type = "password"
	case "email", "tel", "url", "date", "time", "datetime-local", "month", "week", "search", "number", "hidden", "color":
		spec.Type = input
	case "one-time-code", "numeric", "text":
		spec.Type = "text"
	}
	attrs := append([]Attr{{"class", mark("vk-input")}}, b.modelAttrs(spec)...)
	return El("input", extra(attrs))
}

// choiceGroup is a radio group or a group of checkboxes: a fieldset whose
// legend is the field's label, its hint and error linked from the fieldset,
// a control per choice.
func (b boundField) choiceGroup(ctx context.Context, opts Field, input string) Node {
	f, st := b.f, b.st
	choices := opts.Options
	if len(choices) == 0 {
		var err error
		if choices, err = b.choices(ctx); err != nil {
			return Try(nil, err)
		}
	}
	e, shows := st.visible(f.path)
	hint := b.hint(cmpOr(opts.Help, f.help))
	errNode := b.errorNode(ctx, e, shows)
	typ := "radio"
	if input == "checkboxes" {
		typ = "checkbox"
	}
	var items []Node
	for i, o := range choices {
		id := st.id(f.path)
		if i > 0 {
			id += "-" + fmt.Sprint(i+1)
		}
		attrs := append([]Attr{{"class", "vk-check__box"}, {"id", id}, {"value", o.Value}},
			b.modelAttrs(ModelSpec{Tag: "input", Type: typ, Value: o.Value, HasValue: true, Own: "id value"})...)
		if opts.Disabled {
			attrs = append(attrs, Attr{"disabled", true})
		}
		items = append(items, El("div", []Attr{{"class", "vk-check"}}, El("input", attrs),
			El("label", []Attr{{"class", "vk-check__label"}, {"for", id}}, Text(o.Label))))
	}
	classes := "vk-fieldset vk-field"
	if shows {
		classes += " vk-field--invalid"
	}
	return El("fieldset", []Attr{{"class", withClass(classes, opts.Class)}, {"data-invalid", shows},
		{"aria-describedby", lazyValue{func(context.Context) any { return st.describedBy(f) }}}},
		El("legend", []Attr{{"class", "vk-fieldset__legend"}}, Text(cmpOr(opts.Label, f.label)), b.marker(ctx)),
		hint, errNode, El("div", []Attr{{"class", "vk-choices"}}, items...))
}

// Error is a form field's error message, where the field's markup puts it
// (label, hint, error, control), when the field shows one:
//
//	<label for={Form.Email}>Email</label>
//	<ui.Error for={Form.Email}/>
//	<input model={Form.Email}/>
//
// It renders nothing when the field shows no error; the control's
// aria-describedby links it when it does.
type Error struct {
	// For is the field: for={Form.Email}.
	For   any
	Class string
}

// View is the field's error message.
func (e Error) View() Node {
	return Ctx(func(ctx context.Context) Node {
		b, err := resolveFor(ctx, e.For)
		if err != nil {
			return Try(nil, err)
		}
		if b.f == nil {
			return Try(nil, fmt.Errorf("ui: <ui.Error for={…}> takes a field of a form, not the form"))
		}
		fe, shows := b.st.visible(b.f.path)
		return b.errorNode(ctx, fe, shows, e.Class)
	})
}

// Render renders the error's View.
func (e Error) Render(ctx context.Context, w io.Writer) error { return e.View().Render(ctx, w) }

// Fieldset groups fields under a legend: the fields of a struct field of
// the form (for={Form.Address}, its fields rendered when it has no
// children), or the children:
//
//	<ui.Fieldset legend="Delivery address">
//		<ui.Field for={Form.Street}/>
//		<ui.Field for={Form.City}/>
//	</ui.Fieldset>
type Fieldset struct {
	Legend   string
	For      any
	Class    string
	Children Node
}

// View is the fieldset.
func (s Fieldset) View() Node {
	return Ctx(func(ctx context.Context) Node {
		body := s.Children
		legend := s.Legend
		if s.For != nil && !Has(body) {
			b, err := resolveFor(ctx, s.For)
			if err != nil {
				return Try(nil, err)
			}
			if b.f == nil || !b.f.group {
				return Try(nil, fmt.Errorf("ui: <ui.Fieldset for={…}> takes a struct field of a form"))
			}
			legend = cmpOr(legend, b.f.label)
			var kids []Node
			for _, f := range b.st.ft.fields {
				if strings.HasPrefix(f.path, b.f.path+".") && !strings.Contains(f.path[len(b.f.path)+1:], ".") {
					kids = append(kids, fieldNode(b, f, Field{}))
				}
			}
			body = Fragment(kids...)
		}
		var lg Node
		if legend != "" {
			lg = El("legend", []Attr{{"class", "vk-fieldset__legend"}}, Text(legend))
		}
		return El("fieldset", []Attr{{"class", withClass("vk-fieldset", s.Class)}}, lg, body)
	})
}

// Render renders the fieldset's View.
func (s Fieldset) Render(ctx context.Context, w io.Writer) error { return s.View().Render(ctx, w) }

// Summary is a form's error summary, which a failed submit shows: "There is
// a problem", then each error, linking to its field's control, in the words
// the field shows. <ui.Form for> puts one at the top of the form; a
// <ui.Summary/> inside it puts it there instead. The browser moves focus to
// it, and the page's title starts "Error: ".
type Summary struct {
	// For is the form, when the summary is outside its <ui.Form>.
	For any
	// Title is the heading; "There is a problem" by default.
	Title string
	Class string
}

// View is the summary, when the form has errors to list.
func (s Summary) View() Node {
	return Ctx(func(ctx context.Context) Node {
		b, err := resolveFor(ctx, s.For)
		if err != nil {
			return Try(nil, err)
		}
		b.st.summaryNow = true
		return summaryOf(ctx, b, s.Title, s.Class)
	})
}

// Render renders the summary's View.
func (s Summary) Render(ctx context.Context, w io.Writer) error { return s.View().Render(ctx, w) }

// summaryOf is the error summary of the form b holds, empty until a submit
// fails.
func summaryOf(ctx context.Context, b boundField, title string, class ...string) Node {
	st := b.st
	errs := st.shownErrors()
	if len(errs) == 0 || !st.submitted && len(st.failed) == 0 {
		return nil
	}
	var items []Node
	for _, e := range errs {
		text := Text(st.message(ctx, e))
		if e.Path == "" || st.ft.byPath[e.Path] == nil {
			items = append(items, El("li", nil, text))
			continue
		}
		items = append(items, El("li", nil, El("a", []Attr{{"href", "#" + st.id(e.Path)}}, text)))
	}
	cls := "vk-summary"
	if len(class) > 0 {
		cls = withClass(cls, class[0])
	}
	return El("div", []Attr{{"class", cls}, {"id", st.identity() + "-summary"}, {"tabindex", "-1"}, {"autofocus", true}, {"data-vk-summary", true}},
		El("div", []Attr{{"role", "alert"}},
			El("h2", []Attr{{"class", "vk-summary__title"}}, Text(cmpOr(title, "There is a problem"))),
			El("ul", []Attr{{"class", "vk-summary__list"}}, items...)))
}
