// Package widget is a form field's control as a component: the built-in
// widgets, and the contract an app's own widget follows. A widget is an
// ordinary component that embeds Props (or has a Model ui.FieldRef prop):
// the form fills the props it declares, by name and type, from the field
// its model names, and markup or code can set any of them itself:
//
//	<widget.Textarea model={Form.Notes} rows={6}/>
//	<components.ProductPicker model={Form.Product}/>
//
// A field takes its widget from, first to last: the markup naming one; a
// @ui.Spec{Widget: widget.Radios{}} on the form type's ui.Meta member or
// the model's field; Use[T], the app's widget for every field of type T;
// else the control the field's type gives. <ui.Field for={…}> and a
// childless <ui.Form for={…}> take the same order. A widget's options are
// its fields: widget.Textarea{Rows: 4}, widget.RelatedSearch{Limit: 20}.
//
// What the form fills, when the widget declares it and it is still zero:
//
//	Model       ui.FieldRef                        the field: <input model={Model}/> gives a control its name, id, value and ARIA
//	Label, Help, Placeholder string                the field's, in the visitor's language
//	Error       string                             the field's error, when it shows one
//	Required, Disabled bool
//	Value       T                                  the field's value (an Option's value when it holds one)
//	Choices     []widget.Choice[T]                 the field's choices: its <Field>Choices, its attributes' choices, an orm.FK's rows
//	Search      func(ctx, q string) []Choice[T]    the field's rows whose label holds q, up to the widget's Limit
//
// A Value or a Choices' Row of a type the field's isn't fails the render,
// naming both.
//
// A widget of several inputs (Django's MultiWidget) has Format and Parse
// methods: Format turns the field's value into what each input shows,
// Parse what they send into the value, its error the field's. Each input
// is named Model.Part(name) and shows Model.PartValue(name):
//
//	type Money struct{ widget.Props }
//
//	func (Money) Format(v Amount) map[string]string          { … }
//	func (Money) Parse(in map[string]string) (Amount, error) { … }
//
//	<input name={Model.Part("amount")} value={Model.PartValue("amount")}/>
//	<select name={Model.Part("currency")}>…</select>
package widget

import (
	"context"
	"reflect"
	"strconv"

	"github.com/vuka-lang/ui/v2"
)

// Props is what every widget embeds: the props the form fills from its
// field. (Field is the field group component; Props is the contract a
// widget's struct embeds.)
type Props struct {
	// Model is the field the widget edits: model={Form.Price}.
	Model ui.FieldRef
	// Label, Help and Placeholder are the field's, translated.
	Label, Help, Placeholder string
	// Error is the field's error message, when it shows one.
	Error string
	// Required and Disabled are the field's.
	Required, Disabled bool
}

// Choice is one of a field's choices, as a widget's Choices hold them.
type Choice[T any] struct {
	// Value is what the control sends for it: a row's key.
	Value string
	// Label is what the visitor reads: a row's String().
	Label string
	// Selected reports whether the field is (or holds) it.
	Selected bool
	// Row is the whole row (an orm.FK's model, a <Field>Choices query's).
	Row T
	// Group puts the choice under a heading (an <optgroup>).
	Group string
}

// Use makes w the widget of every form field of type T (an Option or a
// pointer of T too) that names no control of its own: the app's widget
// for the type, under the markup's and a @ui.Spec's.
//
//	widget.Use[time.Time](widget.DateTime{})
//	widget.Use[orm.FK[Product]](widget.RelatedSearch{Limit: 20})
func Use[T any](w any) { ui.UseWidget(reflect.TypeFor[T](), w) }

// view is w's markup: w filled from its field as it renders, then what
// build makes of it. A widget is a component as a .vuka.html file's is:
// its View the markup, its Render the View's.
func view[W any](w W, build func(w W, ctx context.Context) ui.Node) ui.Node {
	return ui.Ctx(func(ctx context.Context) ui.Node {
		if err := ui.FillWidget(ctx, &w); err != nil {
			return ui.Try(nil, err)
		}
		return build(w, ctx)
	})
}

// control is a control's attributes: its class (marked when the field
// shows an error), what the field gives it (ui.Model), and the props set.
func (p Props) control(tag, typ, class string, extra ...ui.Attr) []ui.Attr {
	if p.Error != "" {
		class += " " + class + "--invalid"
	}
	own := "class"
	attrs := []ui.Attr{{Name: "class", Value: class}}
	if p.Placeholder != "" && tag != "select" {
		own += " placeholder"
		attrs = append(attrs, ui.Attr{Name: "placeholder", Value: p.Placeholder})
	}
	if p.Disabled {
		own += " disabled"
		attrs = append(attrs, ui.Attr{Name: "disabled", Value: true})
	}
	for _, a := range extra {
		own += " " + a.Name
	}
	attrs = append(attrs, extra...)
	return append(attrs, ui.Model(ui.ModelSpec{Tag: tag, Type: typ, Own: own}, p.Model)...)
}

// options are a select's options: an empty first one when the field may
// stay empty or has no value yet, then the choices.
func options[T any](ctx context.Context, p Props, choices []Choice[T], multiple bool) []ui.Node {
	var out []ui.Node
	any := false
	for _, c := range choices {
		any = any || c.Selected
	}
	if !multiple && (!p.Required || !any) {
		out = append(out, ui.El("option", []ui.Attr{{Name: "value", Value: ""}}, ui.Text(ui.Tr(ctx, cmpOr(p.Placeholder, "form.choose")))))
	}
	label := ""
	var group []ui.Node
	flush := func() {
		if label != "" {
			out = append(out, ui.El("optgroup", []ui.Attr{{Name: "label", Value: label}}, group...))
		}
		group, label = nil, ""
	}
	for _, c := range choices {
		opt := ui.El("option", []ui.Attr{{Name: "value", Value: c.Value}, {Name: "selected", Value: c.Selected}}, ui.Text(c.Label))
		if c.Group != label {
			flush()
			label = c.Group
		}
		if label == "" {
			out = append(out, opt)
			continue
		}
		group = append(group, opt)
	}
	flush()
	return out
}

// items are a radio or a checkbox per choice, each with its label.
func items[T any](p Props, choices []Choice[T], typ string) ui.Node {
	var out []ui.Node
	for i, c := range choices {
		id := p.Model.ID()
		if i > 0 {
			id += "-" + strconv.Itoa(i+1)
		}
		attrs := []ui.Attr{{Name: "class", Value: "vk-check__box"}, {Name: "id", Value: id}, {Name: "value", Value: c.Value}}
		own := "class id value"
		if p.Disabled {
			attrs = append(attrs, ui.Attr{Name: "disabled", Value: true})
			own += " disabled"
		}
		attrs = append(attrs, ui.Model(ui.ModelSpec{Tag: "input", Type: typ, Value: c.Value, HasValue: true, Own: own}, p.Model)...)
		out = append(out, ui.El("div", []ui.Attr{{Name: "class", Value: "vk-check"}}, ui.El("input", attrs),
			ui.El("label", []ui.Attr{{Name: "class", Value: "vk-check__label"}, {Name: "for", Value: id}}, ui.Text(c.Label))))
	}
	return ui.El("div", []ui.Attr{{Name: "class", Value: "vk-choices"}}, out...)
}

// cls is base with the app's classes after it.
func cls(base, extra string) string {
	if extra == "" {
		return base
	}
	return base + " " + extra
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// nonEmpty is s, or nil (no attribute) when it is empty.
func nonEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
