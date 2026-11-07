package ui

import (
	"context"
	"fmt"
	"io"
)

// FormElement is the form a <form for={Form}> renders: the same form
// <ui.Form for={Form}> is — bound, checked and submitted by ui, with its
// busy state, error summary and hooks — on the page's own <form> element,
// its class only the page's:
//
//	<form for={Form} onSubmit={save} class="space-y-4">
//		<label for={Form.Name}/>
//		<input model={Form.Name} class="input"/>
//		<ui.Error for={Form.Name}/>
//		<button>Save</button>
//	</form>
//
// The compiler writes it: <ui.Form>'s options are the tag's attributes
// (key, validate, submit, keep, confirmLeave, noSummary, eager, error,
// onSubmit, onChange), and every other attribute is the element's (Attrs).
// <ui.Form for> is FormElement with the token-styled vk-form class.
//
// The element's id is the form's identity — its type's name in kebab case,
// then -key when it has a key — unless an id is written, which wins; its
// controls' ids start with the identity either way, and a control outside
// it naming it (<button form={Form}>) gets the element's id.
type FormElement Form

// View is the form.
func (f FormElement) View() Node {
	if f.For == nil {
		return Try(nil, fmt.Errorf("ui: <form for={…}> is missing its form: write for={Form}"))
	}
	return Form(f).formView(true)
}

// Render renders the form's View.
func (f FormElement) Render(ctx context.Context, w io.Writer) error { return f.View().Render(ctx, w) }

// ModelLabel is a field's label as an empty <label for={Form.Email}/> or
// <legend/> shows it: the field's label (@ui.Label, label:, else its name
// in words), then what the label says of the field being optional or
// required (Config.MarkRequired).
func ModelLabel(field any, outer ...any) Node {
	return Ctx(func(ctx context.Context) Node {
		b, err := For(field, outer...).resolve()
		if err != nil {
			return Try(nil, err)
		}
		if b.f == nil {
			return Try(nil, fmt.Errorf("ui: a label takes a field of a form, not the form"))
		}
		return Fragment(Text(tr(ctx, b.f.label)), b.marker(ctx))
	})
}

// LabelText is the label of the field a FieldRef names (a component's
// Model), in the visitor's language, without the optional or required
// marker: for a component laying out its own label.
//
//	<label for={ui.ModelID(Model)} class="label">{ui.LabelText(Model)}</label>
func LabelText(field any, outer ...any) Node {
	return fieldText(field, outer, func(ctx context.Context, b boundField) Node { return Text(tr(ctx, b.f.label)) })
}

// HelpText is the hint of the field a FieldRef names (@ui.Help, help:),
// nothing when it has none.
func HelpText(field any, outer ...any) Node {
	return fieldText(field, outer, func(ctx context.Context, b boundField) Node {
		if b.f.help == "" {
			return nil
		}
		return Text(tr(ctx, b.f.help))
	})
}

// ErrorText is the message of the error the field a FieldRef names shows,
// nothing while it shows none: for a component laying out its own error
// (a <ui.Error for={Model}/> renders the whole element, linked from the
// control).
func ErrorText(field any, outer ...any) Node {
	return fieldText(field, outer, func(ctx context.Context, b boundField) Node {
		e, shows := b.st.visible(b.f.path)
		if !shows {
			return nil
		}
		return Text(b.st.message(ctx, e))
	})
}

func fieldText(field any, outer []any, text func(ctx context.Context, b boundField) Node) Node {
	return Ctx(func(ctx context.Context) Node {
		b, err := For(field, outer...).resolve()
		if err != nil {
			return Try(nil, err)
		}
		if b.f == nil {
			return Try(nil, fmt.Errorf("ui: takes a field of a form, not the form"))
		}
		return text(ctx, b)
	})
}

// FieldsetAttrs are the attributes of a <fieldset for={Form.Plan}>: for a
// field with choices (a radio group, checkboxes) its error state and the
// aria-describedby linking its hint and error, as <ui.Field>'s group has;
// for a struct field (for={Form.Address}, its children naming its fields)
// none.
func FieldsetAttrs(field any, outer ...any) []Attr {
	b, err := For(field, outer...).resolve()
	if err != nil {
		return []Attr{{Name: "data-vk-error", Value: Try(nil, err)}}
	}
	if b.f == nil {
		return []Attr{{Name: "data-vk-error", Value: Try(nil, fmt.Errorf("ui: <fieldset for={…}> takes a field of a form, not the form: <form for={…}> renders a form"))}}
	}
	if b.f.group {
		return nil
	}
	f, st := b.f, b.st
	return []Attr{
		{Name: "data-invalid", Value: lazyValue{func(context.Context) any {
			_, shows := st.visible(f.path)
			return shows
		}}},
		{Name: "aria-describedby", Value: lazyValue{func(context.Context) any { return st.describedBy(f) }}},
		{Name: "data-vk-name", Value: f.path},
		{Name: "data-vk-check", Value: lazyValue{func(ctx context.Context) any { return b.checkSpec(ctx, f.raw) }}},
	}
}
