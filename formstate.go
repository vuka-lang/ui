package ui

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vuka-lang/ui/v2/forms"
)

// FormState, embedded in a struct, makes the struct a form: its exported
// fields are the values a visitor edits — the only ones a submit binds — and
// FormState keeps what the form knows about them: their errors, which fields
// the visitor has used, whether the form was submitted.
//
//	type Signup struct {
//		ui.FormState
//		Name  string `label:"Full name" validate:"required,max=80"`
//		Email string `validate:"required,email" help:"We'll send a confirmation link"`
//	}
//
// A field's state is looked up by a pointer to it, so a rename is a compile
// error rather than a silent miss: f.ErrorOf(&f.Email), f.Touched(&f.Email).
// A page holding the form renders it with <ui.Form for={Form}>, and the
// handler its onSubmit names runs only once every rule and Clean method
// passed. Copies of a form share its state.
type FormState struct{ st *formState }

// formState is a form's state, shared by the copies of the form.
type formState struct {
	ft *formType

	errs   []forms.Error            // the rules' and parse errors of the last check, in field order
	pinned map[string]*pinnedErrors // Clean<Field>, Clean and SetError errors, by path ("" for the form's)
	failed []forms.Error            // Fail's, and a Save error's: the form's, until the next submit
	raw    map[string]string        // what was typed in a field that didn't parse, shown back as typed
	parsed map[string]forms.Error   // and the error it got
	used   map[string]bool          // the fields the visitor used (the runtime's markers)
	shown  map[string]bool          // the fields whose error the last render showed
	vis    map[string]bool          // the fields whose error the next render shows

	rendered   map[string]string // the fields the form's last render had, with their model modifiers
	rendering  map[string]string // the render under way's
	controls   map[string]int    // the render under way's controls of each field
	hints      map[string]bool   // its fields with a hint
	errorShown map[string]bool   // its fields showing their error

	base         reflect.Value       // the form's values when it was first rendered or bound, or loaded: Changed's and Dirty's baseline
	pending      reflect.Value       // a record Load was given before the form was attached, loaded when it is
	gen          int                 // bumped by Load and Reset: the browser's fields take the server's values when it changes
	savedUntil   time.Time           // Saved's end: 2s after a successful submit
	bound        map[string]string   // the rendered fields the last bind took, with their modifiers: what Apply writes
	msgs         context.Context     // what the form's messages are resolved with (forms.Detach of its last render's or check's context)
	memo         map[string][]Option // the fields' choices, called once per event
	submitted    bool
	submitting   bool
	summary      bool // the last render showed a <ui.Summary>, so the form doesn't add its own
	inRender     bool // its <ui.Form> is rendering: its controls are counted
	summaryNow   bool
	shownSummary *FormSummary // the summary the render under way showed
	key          string       // the form's key attribute, as text: identity and ids
	idem         string       // the idempotency key of the next submit
	seq          int64        // the highest change sequence the form took
}

// pinnedErrors are errors that stay until what they were about changes: the
// field's value (Clean<Field>, SetError), or any field's (Clean).
type pinnedErrors struct {
	errs  []forms.Error
	value any  // the field's value, or the form's, they were about
	set   bool // SetError's: shown at once
	form  bool // Clean's: kept until any field changes
}

// attachForm is the state of the form ptr points to (a pointer to a struct
// embedding FormState), made when the form has none.
func attachForm(ptr reflect.Value) *formState {
	for ptr.Kind() == reflect.Pointer && ptr.Elem().Kind() == reflect.Pointer {
		ptr = ptr.Elem()
	}
	v := ptr.Elem()
	var ft *formType
	if ptr.Type().Implements(modelFormType) {
		names := v.FieldByName("names")
		list := make([]string, names.Len())
		for i := range list {
			list[i] = names.Index(i).String()
		}
		ft = modelFormTypeOf(v.Type(), list)
	} else {
		ft = formTypeOf(v.Type())
	}
	fs := (*FormState)(v.FieldByIndex(ft.state).Addr().UnsafePointer())
	if fs.st == nil {
		fs.st = &formState{}
	}
	st := fs.st
	if st.ft == nil {
		st.ft = ft
		if st.pending.IsValid() {
			rec := st.pending
			st.pending = reflect.Value{}
			st.load(v, rec)
		} else {
			st.base = cloneValue(v)
		}
	}
	return st
}

// formOfPointer is the form a FormState method's receiver belongs to: its
// state, and the address the form starts at.
func (s *FormState) form() (*formState, uintptr) {
	if s == nil || s.st == nil || s.st.ft == nil {
		return nil, 0
	}
	return s.st, reflect.ValueOf(s).Pointer() - s.st.ft.stateAt
}

// fieldOf is the field of the form field points to.
func (s *FormState) fieldOf(field any) (*formState, *formField) {
	st, base := s.form()
	fv := reflect.ValueOf(field)
	if st == nil || fv.Kind() != reflect.Pointer || fv.IsNil() || fv.Pointer() < base {
		return st, nil
	}
	return st, st.ft.fieldAt(fv.Pointer()-base, fv.Type().Elem())
}

// ErrorOf is the message of the error the field shows, "" when it shows
// none: f.ErrorOf(&f.Email). A field shows its error once the visitor has
// used it, or the form was submitted (see the forms guide's timing table).
func (s *FormState) ErrorOf(field any) string {
	st, f := s.fieldOf(field)
	if f == nil {
		return ""
	}
	if e, ok := st.visible(f.path); ok {
		return forms.Text(st.messages(), e, f.label)
	}
	return ""
}

// SetError gives the field an error that shows at once and stays until the
// field's value changes: f.SetError(&f.Email, "That address is already
// registered"). key is the message, or a message key; params are its
// placeholders' values, name then value ("n", "80").
func (s *FormState) SetError(field any, key string, params ...string) {
	st, f := s.fieldOf(field)
	if f == nil {
		return
	}
	e := forms.Error{Path: f.path, Key: key}
	for i := 0; i+1 < len(params); i += 2 {
		if e.Params == nil {
			e.Params = map[string]string{}
		}
		e.Params[params[i]] = params[i+1]
	}
	if st.pinned == nil {
		st.pinned = map[string]*pinnedErrors{}
	}
	st.pinned[f.path] = &pinnedErrors{errs: []forms.Error{e}, value: pointee(field), set: true}
	st.markVisible(f.path)
}

// Fail sets an error of the form as a whole, shown in the error summary,
// and returns it, for a submit handler to return:
//
//	if err := users.Create(ctx, f.Email); err != nil {
//		return p.Form.Fail("We couldn't create your account. Try again later")
//	}
//
// It stays until the next submit.
func (s *FormState) Fail(format string, args ...any) error {
	e := forms.Errorf(format, args...)
	if s.st != nil {
		s.st.failed = append(s.st.failed, e)
	}
	return e
}

// Touched reports whether the visitor has used the field: left it with
// something in it, or submitted the form.
func (s *FormState) Touched(field any) bool {
	st, f := s.fieldOf(field)
	return f != nil && (st.used[f.path] || st.submitted)
}

// Changed reports whether the field's value differs from the one the form
// was loaded with (Load), or first rendered with; a successful submit
// makes its values the new ones.
func (s *FormState) Changed(field any) bool {
	st, f := s.fieldOf(field)
	if f == nil || !st.base.IsValid() {
		return false
	}
	before, err := st.base.FieldByIndexErr(f.index)
	if err != nil {
		return false
	}
	return !reflect.DeepEqual(before.Interface(), pointee(field))
}

// Valid reports whether the form has no error, shown or not.
func (s *FormState) Valid() bool {
	if s.st == nil {
		return true
	}
	return len(s.st.all()) == 0
}

// Submitting reports whether a submit of the form is being handled: true
// while its submit handler runs.
func (s *FormState) Submitting() bool { return s.st != nil && s.st.submitting }

// Submitted reports whether the visitor submitted the form and it failed:
// every error shows until a submit succeeds.
func (s *FormState) Submitted() bool { return s.st != nil && s.st.submitted }

// Errors are the errors the form shows, the form's own first, then its
// fields' in their order: what its error summary lists.
func (s *FormState) Errors() []forms.Error {
	if s.st == nil {
		return nil
	}
	return s.st.shownErrors()
}

// all is every error the form has, shown or not.
func (st *formState) all() []forms.Error {
	out := slices.Clone(st.failed)
	out = append(out, st.errs...)
	for _, p := range st.pinnedInOrder() {
		out = append(out, p.errs...)
	}
	return out
}

// pinnedInOrder are the pinned errors, the form's first, then by field.
func (st *formState) pinnedInOrder() []*pinnedErrors {
	var out []*pinnedErrors
	if p := st.pinned[""]; p != nil {
		out = append(out, p)
	}
	if st.ft != nil {
		for _, f := range st.ft.fields {
			if p := st.pinned[f.path]; p != nil {
				out = append(out, p)
			}
		}
	}
	return out
}

// errorAt is the field's first error: a pinned one, else a rule's.
func (st *formState) errorAt(path string) (forms.Error, bool) {
	if p := st.pinned[path]; p != nil && len(p.errs) > 0 {
		return p.errs[0], true
	}
	for _, e := range st.errs {
		if e.Path == path {
			return e, true
		}
	}
	for _, p := range st.pinned {
		for _, e := range p.errs {
			if e.Path == path {
				return e, true
			}
		}
	}
	return forms.Error{}, false
}

// visible is the error the field shows, if it shows one.
func (st *formState) visible(path string) (forms.Error, bool) {
	if st == nil || !st.vis[path] {
		return forms.Error{}, false
	}
	return st.errorAt(path)
}

func (st *formState) markVisible(path string) {
	if st.vis == nil {
		st.vis = map[string]bool{}
	}
	st.vis[path] = true
}

// shownErrors are the errors the form shows: its own, then one per field
// in the fields' order.
func (st *formState) shownErrors() []forms.Error {
	out := slices.Clone(st.failed)
	if p := st.pinned[""]; p != nil {
		for _, e := range p.errs {
			if e.Path == "" {
				out = append(out, e)
			}
		}
	}
	if st.ft == nil {
		return out
	}
	for _, f := range st.ft.fields {
		if e, ok := st.visible(f.path); ok {
			out = append(out, e)
		}
	}
	return out
}

// reveal decides which fields show their error: those the visitor used or
// that showed one already, every one once the form was submitted, but an
// empty required field only then; a field whose input can't become valid
// by typing more (letters where a number goes) at once, and a fixed-length
// one once it has its length.
func (st *formState) reveal(form reflect.Value) {
	st.shown, st.vis = st.vis, map[string]bool{}
	for _, f := range st.ft.fields {
		e, ok := st.errorAt(f.path)
		if !ok {
			continue
		}
		if p := st.pinned[f.path]; p != nil && p.set {
			st.vis[f.path] = true
			continue
		}
		switch {
		case st.submitted:
			st.vis[f.path] = true
		case e.Rule == "required":
		case st.used[f.path] || st.shown[f.path]:
			st.vis[f.path] = true
		case impossible(e):
			st.vis[f.path] = true
		case e.Rule == "len" && fullLength(form, f):
			st.vis[f.path] = true
		}
	}
}

// impossible reports whether an error is one more typing can't fix.
func impossible(e forms.Error) bool {
	switch e.Rule {
	case "number", "whole", "digits", "format":
		return true
	}
	return false
}

// fullLength reports whether a fixed-length (len=n) string field has its
// length or more.
func fullLength(form reflect.Value, f *formField) bool {
	r, ok := f.rule("len")
	if !ok {
		return false
	}
	v, err := form.FieldByIndexErr(f.index)
	if err != nil || v.Kind() != reflect.String {
		return false
	}
	return float64(utf8.RuneCountInString(v.String())) >= r.n
}

// succeeded clears what a successful submit leaves behind: errors, the
// used fields, the submitted state; the form's values are its baseline
// now, it is Saved for a while, and the next submit gets another key.
func (st *formState) succeeded(form reflect.Value) {
	st.clear()
	st.base = cloneValue(form)
	st.savedUntil = time.Now().Add(savedFor)
	st.idem = ""
}

// pointee is the value p, a pointer, points to.
func pointee(p any) any { return reflect.ValueOf(p).Elem().Interface() }

// cloneValue is a copy of v, a struct, that later changes to v don't reach:
// its slices and maps are copied too, one level deep.
func cloneValue(v reflect.Value) reflect.Value {
	c := reflect.New(v.Type()).Elem()
	c.Set(v)
	for i := 0; i < c.NumField(); i++ {
		f := c.Field(i)
		if !f.CanSet() {
			continue
		}
		switch f.Kind() {
		case reflect.Slice:
			if !f.IsNil() {
				f.Set(reflect.AppendSlice(reflect.MakeSlice(f.Type(), 0, f.Len()), f))
			}
		case reflect.Struct:
			if f.Type() != formStateType {
				f.Set(cloneValue(f))
			}
		}
	}
	return c
}

// valueOf is the value of the field at f in form, as pinned errors keep it.
func valueOf(form reflect.Value, f *formField) any {
	v, err := form.FieldByIndexErr(f.index)
	if err != nil {
		return nil
	}
	return cloneValue(reflect.ValueOf(struct{ V any }{v.Interface()})).Field(0).Interface()
}

// formValues is the form's values, as Clean's pinned errors keep them: every
// field's, its state left out.
func formValues(form reflect.Value, ft *formType) []any {
	out := make([]any, 0, len(ft.fields))
	for _, f := range ft.fields {
		if !f.group {
			out = append(out, valueOf(form, f))
		}
	}
	return out
}

// asFormErrors is err as form errors, at path when they name none; ok is
// false for an error that isn't one (a failure, not a verdict).
func asFormErrors(err error, path string) ([]forms.Error, bool) {
	var one forms.Error
	var many forms.Errors
	var out []forms.Error
	switch {
	case errors.As(err, &many):
		out = slices.Clone(many)
	case errors.As(err, &one):
		out = []forms.Error{one}
	default:
		return nil, false
	}
	for i := range out {
		if out[i].Path == "" {
			out[i].Path = path
		}
	}
	return out, true
}

// identity is the form's identity: its type's name, and its key — what its
// hidden _vk_form field holds, and its controls' ids start with.
func (st *formState) identity() string {
	if st.key == "" {
		return st.ft.name
	}
	return st.ft.name + "-" + kebabKey(st.key)
}

// kebabKey is a key as part of an id.
func kebabKey(k string) string {
	var b strings.Builder
	for _, c := range k {
		if c < 0x80 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// id is the id of a field's control: the form's identity and the field's
// path, "signup-email", "signup-address-city".
func (st *formState) id(path string) string {
	if path == "" {
		return st.identity()
	}
	return st.identity() + "-" + kebab(path)
}
