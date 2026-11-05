package ui

import (
	"context"
	"net/url"
	"reflect"
	"strings"

	"github.com/vuka-lang/ui/v2/forms"
	"github.com/vuka-lang/vuka/v2/diag"
)

// Forms in stream rows. A <ui.Form for> inside a stream row can't keep
// state on the server (the server forgets the row) or a handler for the
// row: its submit is a Push of a method of the stream's owner taking the
// form, as any row handler is, and the form keeps nothing between events.
// The submit carries the form's identity and the row's key (FormRowField);
// the session binds the values into a fresh form and runs its checks —
// rules, Clean methods — as a submit of any struct form does. When they
// fail, or the method answers a form error (Fail, a forms.Error, a public
// error), the reply's result is the form's report, {"report":{…}} (what
// FormReport says, as a form on a page that isn't live gets it over HTTP),
// which the browser puts in place on that row's form, touching none of its
// fields; nothing else is rendered for it. When the method succeeds it
// typically inserts the row again, which morphs in place, and the result is
// a report with no errors, clearing the ones the form showed.
//
// A row's form checks as it is submitted: there is no state to check a
// field against as the visitor types, so onChange is an error (E0363).

// FormRowField is the hidden field of a form in a stream row: the row's key.
const FormRowField = "_vk_row"

// rowForm checks a <ui.Form for> rendered in a stream row: its submit
// pushes a method taking the form, and it has no onChange.
func rowForm(f Form, b boundField) error {
	name := b.st.ft.t.Name()
	if f.OnChange != nil {
		return diag.New("E0363", "the <ui.Form> of %s in a stream row has onChange: a row's form checks as it is submitted", name).
			WithHelp("leave onChange out: the submit runs the form's checks and puts its errors in place")
	}
	if f.OnSubmit == nil {
		return diag.New("E0362", "the <ui.Form> of %s in a stream row has no onSubmit", name).
			WithHelp("push a method of the stream's owner taking the form, with the row's key: onSubmit={ui.JS.Push(Rename, t.ID)}")
	}
	if reflect.ValueOf(f.OnSubmit).Kind() == reflect.Func {
		return rowHandlerErr("onSubmit")
	}
	fn := submitPush(f.OnSubmit)
	if fn == nil {
		return diag.New("E0362", "the onSubmit of the <ui.Form> of %s in a stream row pushes no method", name).
			WithHelp("push a method of the stream's owner taking the form: onSubmit={ui.JS.Push(Rename, t.ID)}")
	}
	pm, err := handlerShape(fn)
	if err != nil {
		return nil // the push's own encoding says why
	}
	if pm.form < 0 || derefType(pm.args[pm.form]) != b.form.Type().Elem() {
		return diag.New("E0362", "the onSubmit of the <ui.Form> of %s in a stream row pushes %s, which doesn't take the form", name, pm.name).
			WithHelp("push a method taking %s as its last parameter: func (p *Page) Rename(ctx context.Context, id int64, f %[1]s) error", name)
	}
	return nil
}

// submitPush is the handler of the first Push of a row form's onSubmit.
func submitPush(v any) any {
	ch, ok := v.(JSChain)
	if !ok {
		return nil
	}
	for _, op := range ch.jsChain().ops {
		if op.name != "push" {
			continue
		}
		for _, a := range op.args {
			if p, ok := a.val.(*push); ok {
				return p.fn
			}
		}
	}
	return nil
}

// rowSubmit is a row form's submit under way: the form its values went
// into, and its state.
type rowSubmit struct {
	st   *formState
	form reflect.Value
}

// bind binds the submitted values into v, a fresh form of the method's
// parameter, and runs every check, as a struct form's submit does; ok is
// false when they failed.
func (rs *rowSubmit) bind(ctx context.Context, v reflect.Value, vals url.Values) (ok bool, err error) {
	st := attachForm(v.Addr())
	if key, found := strings.CutPrefix(vals.Get(FormIDField), st.ft.name+"-"); found {
		st.key = key // the row's: its controls' ids
	}
	st.usedFrom(vals)
	st.bind(ctx, v, vals, st.renderedFrom(ctx, vals))
	st.submitted = true
	if err := st.check(ctx, v, false); err != nil {
		return false, err
	}
	rs.st, rs.form = st, v
	return len(st.all()) == 0, nil
}

// answer is the reply to a row form's submit, given the method's: its
// report, unless the method returned a result of its own or an error that
// isn't the form's.
func (rs *rowSubmit) answer(ctx context.Context, res any, err error, none bool) (any, error) {
	switch {
	case err == nil && !none:
		return res, nil
	case err == nil:
		rs.st.clear()
		return rs.report(ctx), nil
	}
	if _, ok := RedirectOf(err); ok {
		return nil, err
	}
	if rs.st.takeError(rs.form, err) {
		return rs.report(ctx), nil
	}
	return nil, err
}

// report is the row form's report as the push's result.
func (rs *rowSubmit) report(ctx context.Context) map[string]FormReport {
	st := rs.st
	r := FormReport{Form: st.identity(), Errors: []FormFieldError{},
		Words: map[string]string{"problem": tr(ctx, "form.problem"), "error": tr(ctx, "form.error")}}
	for _, f := range st.ft.fields {
		if f.group {
			continue
		}
		e, ok := st.visible(f.path)
		if !ok {
			continue
		}
		key := e.Key
		if key == "" {
			key = forms.DefaultKey(e)
		}
		r.Errors = append(r.Errors, FormFieldError{Path: f.path, ID: st.id(f.path), Key: key, Text: st.message(ctx, e)})
	}
	if errs := st.shownErrors(); len(errs) > 0 {
		r.Summary = st.summaryOf(ctx, errs, "")
	}
	return map[string]FormReport{"report": r}
}
