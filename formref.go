package ui

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/vuka-lang/ui/v2/forms"
)

// FieldRef names a form, or one field of a form, for a component to render:
// what a for={…} attribute of <ui.Form>, <ui.Field>, <ui.Error> and
// <ui.Fieldset> holds. The compiler writes for={Form.Email} as
// ui.For(&p.Form.Email, &p.Form): a pointer to the field, then pointers to
// the values holding it, innermost first, one of which is the form (a
// struct embedding FormState). Go code writes the same:
//
//	ui.Field{For: ui.For(&p.Form.Email, &p.Form)}
//	ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save}
type FieldRef struct {
	field reflect.Value
	outer []reflect.Value
}

// For is a reference to the field field points to, outer pointing to the
// values holding it (see FieldRef).
func For(field any, outer ...any) FieldRef {
	r := FieldRef{field: reflect.ValueOf(field)}
	for _, o := range outer {
		r.outer = append(r.outer, reflect.ValueOf(o))
	}
	return r
}

// boundField is a resolved FieldRef: the form (a pointer to its struct), its
// state, and the field (nil for the whole form).
type boundField struct {
	form reflect.Value
	st   *formState
	f    *formField
}

// refOf is the FieldRef a for attribute's value is: a FieldRef, or a pointer
// to a form or a field (a pointer to a form standing for the whole form).
func refOf(v any) (FieldRef, bool) {
	switch x := v.(type) {
	case FieldRef:
		return x, true
	case *FieldRef:
		return *x, x != nil
	case nil:
		return FieldRef{}, false
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		return FieldRef{field: rv}, true
	}
	return FieldRef{}, false
}

// resolve finds the form the field is in, and the field.
func (r FieldRef) resolve() (boundField, error) {
	if !r.field.IsValid() || r.field.Kind() != reflect.Pointer || r.field.IsNil() {
		return boundField{}, fmt.Errorf("ui: for={…} takes a form or a field of one, not %v", r.field)
	}
	for i, c := range append([]reflect.Value{r.field}, r.outer...) {
		for c.Kind() == reflect.Pointer && !c.IsNil() && c.Elem().Kind() == reflect.Pointer {
			c = c.Elem()
		}
		if c.Kind() != reflect.Pointer || c.IsNil() || !isForm(c.Elem().Type()) {
			continue
		}
		st := attachForm(c)
		if i == 0 {
			return boundField{form: c, st: st}, nil
		}
		at, field := c.Pointer(), r.field.Pointer()
		if field >= at {
			if f := st.ft.fieldAt(field-at, r.field.Type().Elem()); f != nil {
				return boundField{form: c, st: st, f: f}, nil
			}
		}
		return boundField{}, fmt.Errorf("ui: for={…}: the field isn't one of %s's (a field form:\"-\" leaves out, or an unexported one)", c.Elem().Type().Name())
	}
	return boundField{}, fmt.Errorf("ui: for={…}: %s isn't in a form: a form is a struct embedding ui.FormState", r.field.Type().Elem())
}

// value is the form's struct.
func (b boundField) value() reflect.Value { return b.form.Elem() }

// id is the field's control's id.
func (b boundField) id() string { return b.st.id(b.f.path) }

// lazyValue is an attribute's value computed as the attribute is written,
// with the render's context and once the elements before it rendered: the
// ids a form's key gives its fields, what shows of its state.
type lazyValue struct{ fn func(ctx context.Context) any }

// ModelSpec is what the compiler knows of an element carrying model={…}:
// its tag, its own type and value attributes (a radio's, a checkbox's in a
// group), the model's modifiers (model.lazy, model.raw, model.number) and
// the attributes it sets itself, which Model leaves to it.
type ModelSpec struct {
	Tag      string
	Type     string
	Value    any
	HasValue bool
	Mods     string // comma-separated
	Own      string // space-separated attribute names
}

func (s ModelSpec) owns(name string) bool {
	for _, a := range strings.Fields(s.Own) {
		if a == name {
			return true
		}
	}
	return false
}

// Model is the attributes of an <input>, <textarea> or <select> carrying
// model={Form.Email}, which the compiler writes as ui.Model(spec,
// &p.Form.Email, &p.Form): its name, id and value, the type, inputmode and
// autocomplete its Go type, name and tags give, aria-describedby linking
// the field's hint and error, aria-invalid once its error shows. A radio or
// a checkbox of a group is checked when the field is (or holds) its value.
// The element's own attributes win: Model writes none it sets (spec.Own).
//
// model on a field of a page that isn't in a form binds the field as
// bind:value does.
func Model(spec ModelSpec, field any, outer ...any) []Attr {
	b, err := For(field, outer...).resolve()
	if err != nil {
		if fv := reflect.ValueOf(field); fv.Kind() == reflect.Pointer && len(outer) > 0 {
			if a, ok := pageModel(fv, outer); ok {
				return a
			}
		}
		return []Attr{{Name: "name", Value: Try(nil, err)}}
	}
	if b.f == nil || b.f.group {
		return []Attr{{Name: "name", Value: Try(nil, fmt.Errorf("ui: model={…} takes a field of a form, not the form itself"))}}
	}
	return b.modelAttrs(spec)
}

// pageModel is model on a page's own field, outside any form: bind:value's
// attributes, the page being the outermost value.
func pageModel(field reflect.Value, outer []any) ([]Attr, bool) {
	owner := reflect.ValueOf(outer[len(outer)-1])
	for owner.Kind() == reflect.Pointer && !owner.IsNil() && owner.Elem().Kind() == reflect.Pointer {
		owner = owner.Elem()
	}
	if owner.Kind() != reflect.Pointer || owner.IsNil() || owner.Elem().Kind() != reflect.Struct {
		return nil, false
	}
	return BindValue(owner.Interface(), field.Interface()), true
}

func (b boundField) modelAttrs(spec ModelSpec) []Attr {
	f, st, form := b.f, b.st, b.value()
	tag := cmpOr(spec.Tag, "input")
	typ := spec.Type
	if typ == "" && tag == "input" {
		typ = f.htmlType()
		if spec.HasValue && (f.input == "checkboxes" || f.input == "checkbox") {
			typ = "checkbox"
		}
	}
	choice := tag == "input" && (typ == "radio" || typ == "checkbox") && spec.HasValue
	var out []Attr
	add := func(name string, v any) {
		if !spec.owns(name) {
			out = append(out, Attr{Name: name, Value: v})
		}
	}
	add("name", lazyValue{func(context.Context) any {
		st.renderField(f.path, spec.Mods)
		return f.path
	}})
	add("id", lazyValue{func(context.Context) any { return st.controlID(f.path) }})
	if tag == "input" {
		add("type", typ)
	}
	switch {
	case choice:
		checked := selected(form, f, optionText(spec.Value))
		out = append(out, Attr{Name: "checked", Value: checked})
	case tag == "input" && typ == "checkbox":
		add("value", "true")
		out = append(out, Attr{Name: "checked", Value: selected(form, f, "")})
	case tag == "input" && typ != "password" && f.input != "one-time-code":
		add("value", st.fieldText(form, f))
	}
	if tag == "input" && !choice {
		if f.inputmode != "" {
			add("inputmode", f.inputmode)
		}
		if f.autocomplete != "" {
			add("autocomplete", f.autocomplete)
		}
		if f.placeholder != "" {
			add("placeholder", f.placeholder)
		}
	}
	if tag == "textarea" && f.placeholder != "" {
		add("placeholder", f.placeholder)
	}
	if f.required() && !(choice && typ == "checkbox") {
		add("required", lazyValue{func(ctx context.Context) any {
			if configOf(ctx).RequiredAttr {
				return true
			}
			return nil
		}})
	}
	if tag == "input" && !choice {
		for _, a := range constraintAttrs(f, typ) {
			add(a.Name, a.Value)
		}
		raw := f.raw || hasMod(spec.Mods, "raw")
		if !hasMod(spec.Mods, "number") {
			add("data-vk-check", lazyValue{func(ctx context.Context) any { return b.checkSpec(ctx, raw) }})
		}
	}
	if tag == "textarea" || tag == "select" {
		raw := f.raw || hasMod(spec.Mods, "raw")
		add("data-vk-check", lazyValue{func(ctx context.Context) any { return b.checkSpec(ctx, raw) }})
	}
	if hasMod(spec.Mods, "lazy") {
		out = append(out, Attr{Name: "data-vk-lazy", Value: true})
	}
	if !choice {
		add("aria-describedby", lazyValue{func(context.Context) any { return st.describedBy(f) }})
		add("aria-invalid", lazyValue{func(context.Context) any {
			if _, ok := st.visible(f.path); ok {
				return "true"
			}
			return nil
		}})
	}
	return out
}

// ModelText is the text of a <textarea model={…}>: the field's value.
func ModelText(field any, outer ...any) string {
	b, err := For(field, outer...).resolve()
	if err != nil || b.f == nil {
		return ""
	}
	return b.st.fieldText(b.value(), b.f)
}

// ModelOptions are the options of a <select model={…}> written without
// any: the field's <Field>Choices, an empty first option when the field
// has no value yet or may stay empty.
func ModelOptions(field any, outer ...any) Node {
	return Ctx(func(ctx context.Context) Node {
		b, err := For(field, outer...).resolve()
		if err != nil {
			return Try(nil, err)
		}
		if b.f == nil {
			return nil
		}
		return b.options(ctx)
	})
}

// options are a select's options.
func (b boundField) options(ctx context.Context) Node {
	opts, err := b.choices(ctx)
	if err != nil {
		return Try(nil, err)
	}
	form, f := b.value(), b.f
	var out []Node
	if !f.required() || optionText(form.FieldByIndex(f.index).Interface()) == "" {
		out = append(out, El("option", []Attr{{Name: "value", Value: ""}}, Text(f.placeholder)))
	}
	for _, o := range opts {
		out = append(out, El("option", []Attr{{Name: "value", Value: o.Value}, {Name: "selected", Value: selected(form, f, o.Value)}}, Text(o.Label)))
	}
	return Fragment(out...)
}

// choices are the field's options: its <Field>Choices.
func (b boundField) choices(ctx context.Context) ([]Option, error) {
	if b.f.choices == nil {
		return nil, nil
	}
	return b.st.choicesOf(ctx, b.value(), b.f)
}

// ModelSelected reports whether an <option value={v}> of a <select
// model={…}> is selected.
func ModelSelected(v any, field any, outer ...any) bool {
	b, err := For(field, outer...).resolve()
	if err != nil || b.f == nil {
		return false
	}
	return selected(b.value(), b.f, optionText(v))
}

// ModelID is the id of the field's first control: <label for={Form.Email}>.
func ModelID(field any, outer ...any) any {
	b, err := For(field, outer...).resolve()
	if err != nil {
		if fv := reflect.ValueOf(field); fv.Kind() == reflect.Pointer && !fv.IsNil() {
			return fmt.Sprint(fv.Elem().Interface()) // a plain value: <label for={p.id}>
		}
		return Try(nil, err)
	}
	if b.f == nil {
		return lazyValue{func(context.Context) any { return b.st.identity() }}
	}
	return lazyValue{func(context.Context) any { return b.st.id(b.f.path) }}
}

// renderField notes a field the form's render under way has, with its
// model modifiers.
func (st *formState) renderField(path, mods string) {
	if st.rendering == nil {
		st.rendering = map[string]string{}
	}
	if old, ok := st.rendering[path]; !ok || old == "" {
		st.rendering[path] = mods
	}
}

// controlID is the id of a control of the field: the field's id for its
// first control in the render, then the id with -2, -3, … (the radios of a
// group): an error summary's link goes to the first.
func (st *formState) controlID(path string) string {
	if !st.inRender {
		return st.id(path)
	}
	if st.controls == nil {
		st.controls = map[string]int{}
	}
	st.controls[path]++
	if n := st.controls[path]; n > 1 {
		return st.id(path) + "-" + strconv.Itoa(n)
	}
	return st.id(path)
}

// describedBy is a control's aria-describedby: its field's hint and error,
// when the render shows them.
func (st *formState) describedBy(f *formField) any {
	var ids []string
	if st.hints[f.path] {
		ids = append(ids, st.id(f.path)+"-hint")
	}
	if _, ok := st.visible(f.path); ok && st.errorShown[f.path] {
		ids = append(ids, st.id(f.path)+"-error")
	}
	if len(ids) == 0 {
		return nil
	}
	return strings.Join(ids, " ")
}

// message is the text of the field's error, as a visitor reads it.
func (st *formState) message(ctx context.Context, e forms.Error) string {
	label := ""
	if f := st.ft.byPath[e.Path]; f != nil {
		label = f.label
	}
	return forms.Text(messageCtx(ctx), e, label)
}

// keepMessages notes what the form's messages are resolved with, for
// ErrorOf and Errors outside a render.
func (st *formState) keepMessages(ctx context.Context) {
	if ctx != nil {
		st.msgs = forms.Detach(messageCtx(ctx))
	}
}

// messages is what the form's messages are resolved with outside a render.
func (st *formState) messages() context.Context {
	if st.msgs != nil {
		return st.msgs
	}
	return messageCtx(context.Background())
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// FieldPath is the path of the field a pointer names, field pointing into
// one of the values outer point to (see For): "email", "address.city".
// Tests name the fields they fill by it.
func FieldPath(field any, outer ...any) (string, error) {
	b, err := For(field, outer...).resolve()
	if err != nil {
		// outer may be a page holding the form: find the form among its fields
		for _, o := range outer {
			if form := formHolding(reflect.ValueOf(o), reflect.ValueOf(field)); form.IsValid() {
				b, err = For(field, form.Interface()).resolve()
				break
			}
		}
	}
	if err != nil {
		return "", err
	}
	if b.f == nil {
		return "", fmt.Errorf("ui: FieldPath: %s is the form, not a field of it", b.st.ft.t.Name())
	}
	return b.f.path, nil
}

// formHolding is a pointer to the form among v's fields (v a pointer to a
// struct) that holds the field p points to; invalid when none does.
func formHolding(v, p reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer && !v.IsNil() && v.Elem().Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct || p.Kind() != reflect.Pointer {
		return reflect.Value{}
	}
	st := v.Elem()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if f.Kind() == reflect.Pointer && !f.IsNil() {
			f = f.Elem()
		}
		if f.Kind() != reflect.Struct || !f.CanAddr() {
			continue
		}
		at := f.Addr().Pointer()
		if p.Pointer() >= at && p.Pointer() < at+f.Type().Size() && isForm(f.Type()) {
			return f.Addr()
		}
	}
	return reflect.Value{}
}
