package ui

import (
	"context"
	"fmt"
	"reflect"

	"github.com/vuka-lang/ui/v2/forms"
)

// check runs every check of the form: each field's validate: rules, then,
// for each field passing them, its Clean<Field> method, then the form's
// Clean method, the methods' dependencies injected (ctx's Injector). live
// is a check run while the visitor types (forms.Live is true). It records
// the errors and decides which show; it returns an error only when a check
// failed to run (a Clean method returning an error that isn't a form
// error).
func (st *formState) check(ctx context.Context, form reflect.Value, live bool) error {
	ft := st.ft
	if ft.err != nil {
		return ft.err
	}
	st.keepMessages(ctx)
	st.errs = st.errs[:0]
	for _, f := range ft.fields {
		if e, ok := st.parsed[f.path]; ok {
			st.errs = append(st.errs, e)
			continue
		}
		if f.needsInput && st.blank[f.path] {
			st.errs = append(st.errs, f.errorFor(rule{name: "required"}, ""))
			continue
		}
		if e, ok := f.verdict(form); ok {
			st.errs = append(st.errs, e)
			continue
		}
		e, ok, err := st.checkChoice(ctx, form, f, live)
		if err != nil {
			return err
		}
		if ok {
			st.errs = append(st.errs, e)
		}
	}
	// SetError's errors stay until their field changes.
	for path, p := range st.pinned {
		if p.set && !p.form {
			if f := ft.byPath[path]; f == nil || !sameValue(p.value, valueOf(form, f)) {
				delete(st.pinned, path)
			}
		}
	}
	if !live {
		if err := st.checkFields(ctx, form); err != nil {
			return err
		}
	}
	cctx := formCtx(ctx, live)
	for _, f := range ft.fields {
		m := ft.cleans[f.path]
		if m == nil {
			continue
		}
		p := st.pinned[f.path]
		if p != nil && p.set {
			continue
		}
		if st.hasRuleError(f.path) {
			delete(st.pinned, f.path)
			continue
		}
		err := callForm(cctx, m, ft.recvOf(m, form))
		if err == nil {
			if !live || p != nil && !sameValue(p.value, valueOf(form, f)) {
				delete(st.pinned, f.path)
			}
			continue
		}
		errs, ok := ft.asFormErrors(err, f.path)
		if !ok {
			return fmt.Errorf("%s.%s: %w", ft.t.Name(), m.Name, err)
		}
		st.pin(f.path, &pinnedErrors{errs: errs, value: valueOf(form, f)})
	}
	if ft.clean != nil {
		values := formValues(form, ft)
		if p := st.pinned[""]; p != nil && !sameValue(p.value, values) {
			delete(st.pinned, "")
		}
		err := callForm(cctx, ft.clean, ft.recvOf(ft.clean, form))
		switch errs, ok := ft.asFormErrors(err, ""); {
		case err == nil:
			if !live {
				delete(st.pinned, "")
			}
		case !ok:
			return fmt.Errorf("%s.Clean: %w", ft.t.Name(), err)
		default:
			st.pin("", &pinnedErrors{errs: errs, value: values, form: true})
		}
	}
	st.reveal(form)
	return nil
}

func (st *formState) pin(path string, p *pinnedErrors) {
	if st.pinned == nil {
		st.pinned = map[string]*pinnedErrors{}
	}
	st.pinned[path] = p
}

func (st *formState) hasRuleError(path string) bool {
	for _, e := range st.errs {
		if e.Path == path {
			return true
		}
	}
	return false
}

func sameValue(a, b any) bool { return reflect.DeepEqual(a, b) }

// verdict is the error of the first rule the field's value breaks.
func (f *formField) verdict(form reflect.Value) (forms.Error, bool) {
	v, err := form.FieldByIndexErr(f.index)
	if err != nil {
		return forms.Error{}, false
	}
	if f.input == "numeric" && v.Kind() == reflect.String && !digitsOnly(v.String()) {
		return f.errorFor(rule{name: "digits"}, ""), true
	}
	for _, r := range f.rules {
		if r.name == "omitempty" {
			if v.IsZero() {
				return forms.Error{}, false
			}
			continue
		}
		if r.check(v) != "" {
			return f.errorFor(r, f.kind(r)), true
		}
	}
	for _, check := range f.validators {
		if err := check(v.Interface()); err != nil {
			return forms.Error{Path: f.path, Rule: "invalid", Key: err.Error()}, true
		}
	}
	return forms.Error{}, false
}

// kind is the kind of value a rule's message speaks of: characters
// (string), a number, a list's items, a choice, a checkbox, an email
// address.
func (f *formField) kind(r rule) string {
	base := valueType(f.typ)
	switch r.name {
	case "required":
		switch {
		case f.input == "file":
			return "file"
		case f.input == "email":
			return "email"
		case base.Kind() == reflect.Bool:
			return "check"
		case base.Kind() == reflect.Slice:
			return "list"
		case f.hasChoices() || f.input == "radio" || f.input == "select":
			return "choice"
		}
	case "min", "max", "len":
		switch {
		case base.Kind() == reflect.String:
			return "string"
		case base.Kind() == reflect.Slice, base.Kind() == reflect.Map:
			return "list"
		default:
			return "number"
		}
	case "oneof":
		if f.hasChoices() || f.input == "radio" || f.input == "select" {
			return "choice"
		}
	}
	return ""
}

// callForm calls a form's method m on recv, its parameters after the
// receiver filled as a hook's are: the context, and what ctx's Injector
// provides.
func callForm(ctx context.Context, m *reflect.Method, recv reflect.Value) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s panicked: %v", m.Name, r)
		}
	}()
	return call(ctx, m.Func, []reflect.Value{recv}, hookCall, Changed{}, nil)
}

var optionsType = reflect.TypeFor[[]Option]()

// choices are the options of a field with a <Field>Choices method, called
// as a Clean method is; it returns []ui.Option, or a query of the rows the
// field may take (an orm query: its rows the options, and the allow-list
// a submitted key is checked against), and maybe an error.
func choices(ctx context.Context, m *reflect.Method, recv reflect.Value) (opts []Option, src rowSource, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s panicked: %v", m.Name, r)
		}
	}()
	ft := m.Func.Type()
	if !choicesShape(ft) {
		return nil, nil, fmt.Errorf("%s is %s: a <Field>Choices method returns []ui.Option or a query of rows (an orm query), and maybe an error", m.Name, ft)
	}
	slots, issues := plan(ft, 1, hookCall, InjectorOf(ctx))
	if len(issues) > 0 {
		is := issues[0]
		return nil, nil, fmt.Errorf("%s's %s parameter, %s, %s", m.Name, ordinal(is.index+1), is.t, is.why)
	}
	args := []reflect.Value{recv}
	for _, s := range slots {
		var v reflect.Value
		switch {
		case s.ctx:
			v = reflect.ValueOf(&ctx).Elem()
		case s.inject != nil:
			if v, err = s.inject(ctx); err != nil {
				return nil, nil, err
			}
		}
		if !v.IsValid() {
			v = reflect.Zero(s.t)
		}
		args = append(args, v)
	}
	out := m.Func.Call(args)
	if len(out) == 2 && !out[1].IsNil() {
		return nil, nil, out[1].Interface().(error)
	}
	if out[0].Type() == optionsType {
		return out[0].Interface().([]Option), nil, nil
	}
	return nil, out[0].Interface().(rowSource), nil
}

// choicesShape reports whether a <Field>Choices method's type (its receiver
// first) returns []ui.Option or a rowSource, and maybe an error.
func choicesShape(ft reflect.Type) bool {
	if ft.NumOut() == 0 || ft.NumOut() > 2 || ft.NumOut() == 2 && ft.Out(1) != errType || ft.IsVariadic() {
		return false
	}
	return ft.Out(0) == optionsType || ft.Out(0).Implements(rowSourceType)
}

// checkChoice checks that a field with choices (<Field>Choices) holds one
// of them. While the visitor types, a value no longer among them — a City
// once its Country changed — is cleared, its error (an empty required
// field's) showing on submit; on submit it is an error.
func (st *formState) checkChoice(ctx context.Context, form reflect.Value, f *formField, live bool) (forms.Error, bool, error) {
	if !f.hasChoices() {
		return forms.Error{}, false, nil
	}
	v, err := form.FieldByIndexErr(f.index)
	if err != nil || v.IsZero() {
		return forms.Error{}, false, nil
	}
	allowed, err := st.allowList(ctx, form, f, v)
	if err != nil {
		return forms.Error{}, false, err
	}
	var stray bool
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
		for i := 0; i < v.Len(); i++ {
			stray = stray || !allowed[valueText(v.Index(i))]
		}
	} else {
		stray = !allowed[valueText(v)]
	}
	if !stray {
		return forms.Error{}, false, nil
	}
	if live && v.CanSet() {
		if v.Kind() == reflect.Slice {
			kept := reflect.MakeSlice(v.Type(), 0, v.Len())
			for i := 0; i < v.Len(); i++ {
				if allowed[valueText(v.Index(i))] {
					kept = reflect.Append(kept, v.Index(i))
				}
			}
			if kept.Len() == 0 {
				kept = reflect.Zero(v.Type())
			}
			v.Set(kept)
		} else {
			v.SetZero()
		}
		if e, ok := f.verdict(form); ok {
			return e, true, nil
		}
		return forms.Error{}, false, nil
	}
	return f.errorFor(rule{name: "choice"}, ""), true, nil
}

// allowList is which of the values v holds are among the field's
// choices: looked up one by one in a query's rows (an orm query, a large
// table never listed), else the choices listed.
func (st *formState) allowList(ctx context.Context, form reflect.Value, f *formField, v reflect.Value) (map[string]bool, error) {
	src, err := st.sourceOf(ctx, form, f)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	if src != nil {
		var vals []string
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
			for i := 0; i < v.Len(); i++ {
				vals = append(vals, valueText(v.Index(i)))
			}
		} else {
			vals = []string{valueText(v)}
		}
		for _, k := range vals {
			if _, _, ok, err := src.FormChoice(ctx, k); err != nil {
				return nil, fmt.Errorf("ui: checking %s: %w", f.goPath, err)
			} else if ok {
				allowed[k] = true
			}
		}
		return allowed, nil
	}
	opts, err := st.choicesOf(ctx, form, f)
	if err != nil {
		return nil, err
	}
	for _, o := range opts {
		allowed[o.Value] = true
	}
	return allowed, nil
}

// choicesOf are a field's choices, its <Field>Choices called once per
// event (bind) or render. Every field with a <Field>Choices method
// reports what its render showed: one whose choices follow another
// field's value (reading f.Value.Country) gets them as that field
// changes, whether or not a depends: tag says so.
func (st *formState) choicesOf(ctx context.Context, form reflect.Value, f *formField) ([]Option, error) {
	rows, err := st.choiceRows(ctx, form, f)
	if err != nil {
		return nil, err
	}
	opts := make([]Option, len(rows))
	for i, r := range rows {
		opts[i] = r.Option
	}
	if st.inRender && (f.choices != nil || len(f.depends) > 0) {
		if st.shownChoices == nil {
			st.shownChoices = map[string][]Option{}
		}
		st.shownChoices[f.path] = opts
	}
	return opts, nil
}
