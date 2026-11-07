package ui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/vuka-lang/ui/v2/forms"
	"github.com/vuka-lang/vuka/v2"
	vfield "github.com/vuka-lang/vuka/v2/field"
)

// FormFor is a form editing a T: a model's row, or any struct. Its fields
// are T's that the page's markup binds — <ui.Field for={Form.Name}/>,
// model={Form.Price} — which the compiler records, so a field the page
// doesn't show is never a visitor's to set; @ui.Fields on the form's field
// names them instead, for markup the compiler can't see into. T's field
// attributes describe them (labels, limits, choices: ui's and a model's
// own), and its type decides what is required: a plain field is, an
// Option isn't (empty is None), a bool may stay unticked.
//
//	type Products struct {
//		ui.Live
//		Form ui.FormFor[Product]
//	}
//
//	func (p *Products) edit(ctx context.Context, id int64) error {
//		return p.Form.Load(ctx, Product.Objects.Get(ctx, Product.ID.Eq(id))?)
//	}
//
//	func (p *Products) save(ctx context.Context) error {
//		return p.Form.Save(ctx)
//	}
//
// Markup names a field through the form, Form.Name, which is Form.Value.Name
// in Go. A model's form saves itself (Save); any other form's submit
// handler takes the checked T as a parameter, and on success the form is
// emptied again unless <ui.Form keep>. T's Clean, Clean<Field> and
// <Field>Choices methods are the form's, as a ModelForm's are.
//
// A struct embedding a FormFor is a form type: a form of T with code of
// its own. Its Init hook runs in Load, after the record is copied in; its
// Clean hook once every other check passed, before a submit's handler or
// Save's write — a FieldError shows under its field, any other error as
// the form's, and either stops the save. The fields it declares itself
// are not the form's.
//
//	type ProductForm struct {
//		ui.FormFor[Product]
//	}
//
//	func (f *ProductForm) Init(ctx context.Context) error {
//		if f.IsNew() {
//			f.Value.Stock = 1
//		}
//		return nil
//	}
//
//	func (f *ProductForm) Clean(ctx context.Context) error {
//		if f.Value.Sale > f.Value.Price {
//			return ui.FieldError(Product.Sale, "Sale price must be below the price.")
//		}
//		return nil
//	}
type FormFor[T any] struct {
	FormState
	// Value is the form's values.
	Value T
	names []string
	owned bool // names are its owner's @ui.Fields
}

func (f *FormFor[T]) modelFields() []string { return f.names }
func (*FormFor[T]) formFor()                {}

// namedForm is a FormFor, whose fields its owner or its markup names.
type namedForm interface {
	setNames(names []string, owned bool)
	ownNames() bool
}

// vukaFormFor has the compiler resolve Form.X, X a field of T, as
// Form.Value.X.
func (FormFor[T]) vukaFormFor() {}

var formForType = reflect.TypeFor[interface{ formFor() }]()

// Load makes v the record the form edits: its values the form's, and the
// ones Dirty and Changed compare with. A model's row read from the
// database is saved by an update of the form's fields; a new one, T{}, by
// an insert. A form type's Init hook runs then, with ctx, and Load returns
// its error.
func (f *FormFor[T]) Load(ctx context.Context, v T) error {
	st, form := f.attached()
	if st.ft.err != nil {
		return st.ft.err
	}
	rec := reflect.New(reflect.TypeFor[T]())
	rec.Elem().Set(reflect.ValueOf(v))
	f.Value = v
	st.record = rec
	st.load(form, rec)
	if st.ft.init != nil {
		if err := callForm(ctx, st.ft.init, form.Addr()); err != nil {
			return err
		}
		st.base = cloneValue(form) // what Init set is the record's, not a change
	}
	return nil
}

// attached is f's state and the form it is: f, or the form type f is
// embedded in, once it was attached as that type (Attach, or its page).
func (f *FormFor[T]) attached() (*formState, reflect.Value) {
	ptr := ownerForm(reflect.ValueOf(f))
	return attachForm(ptr), ptr.Elem()
}

// Attach makes form, a pointer to a struct embedding a FormFor (a form
// type), the form its FormFor's methods are of, and returns it: Load and
// Save then run the type's Init and Clean hooks. A page attaches its
// forms once Mount returned, and the compiler writes Attach at each Load
// and Save of a form type in a .vuka file; Go code loading one before its
// page has it calls it: ui.Attach(&p.Form).Load(ctx, row).
func Attach[F any](form *F) *F {
	if form != nil && isForm(reflect.TypeFor[F]()) {
		attachForm(reflect.ValueOf(form))
	}
	return form
}

// IsNew reports whether the record the form edits is new: none was loaded,
// or its primary key is unset.
func (f *FormFor[T]) IsNew() bool {
	st, _ := f.attached()
	return st.isNew()
}

// Save saves the record the form edits, a model's row: T has Save and
// SaveFields methods, as every orm model does (the compiler reports a
// FormFor whose T hasn't). It runs the form's checks when no submit did —
// the field rules, then T's Clean methods, then a form type's Clean hook —
// inserts a new record, the form's value, or copies the form's fields onto
// a loaded one and updates only those — a change made meanwhile to another
// column stays — then reads the row again, so a new one has its key and
// what the database computed, and the form is clean. A check that fails
// shows at its field, and Save returns ErrFormInvalid.
func (f *FormFor[T]) Save(ctx context.Context) error {
	st, form := f.attached()
	if !st.submitting {
		if err := st.check(ctx, form, false); err != nil {
			return err
		}
		if len(st.all()) > 0 {
			st.submitted = true
			st.reveal(form)
			return ErrFormInvalid
		}
	}
	rec := st.record
	if !rec.IsValid() {
		rec = reflect.New(form.FieldByIndex(st.ft.recv).Type())
	}
	saver, ok := rec.Interface().(interface {
		Save(context.Context) error
		SaveFields(context.Context, ...any) error
	})
	if !ok {
		return fmt.Errorf("ui: FormFor[%s].Save: %[1]s has no Save and SaveFields methods to save it with", rec.Type().Elem())
	}
	var err error
	if st.isNew() {
		// a new record is the form's value, what the server set in it too
		rec.Elem().Set(form.FieldByIndex(st.ft.recv))
		err = saver.Save(ctx)
	} else {
		names := st.apply(form, rec)
		fields := make([]any, len(names))
		for i, n := range names {
			fields[i] = n
		}
		err = saver.SaveFields(ctx, fields...)
	}
	if err != nil {
		return err
	}
	if r, ok := saver.(interface{ Refresh(context.Context) error }); ok {
		if err := r.Refresh(ctx); err != nil {
			return err
		}
	}
	st.record, st.saved = rec, true
	st.load(form, rec)
	return nil
}

// isNew reports whether the form's record is new: none loaded, or its key
// unset.
func (st *formState) isNew() bool {
	if !st.record.IsValid() {
		return true
	}
	if st.ft == nil || st.ft.key == nil {
		return false
	}
	k, err := st.record.Elem().FieldByIndexErr(st.ft.key)
	return err != nil || k.IsZero()
}

// emptied empties a FormFor after a successful submit that didn't save it:
// its values are T's zero ones again, no record loaded.
func (st *formState) emptied(form reflect.Value) {
	v := form.FieldByIndex(st.ft.recv)
	v.SetZero()
	st.record = reflect.Value{}
	st.load(form, reflect.New(v.Type()))
}

// checkRecord is the record a field's Checker sees: a FormFor's loaded
// one, a new one of T when it loaded none; nil for another form.
func (st *formState) checkRecord(form reflect.Value) any {
	if !st.ft.typed {
		return nil
	}
	if st.record.IsValid() {
		return st.record.Interface()
	}
	return reflect.New(form.FieldByIndex(st.ft.recv).Type()).Interface()
}

// FieldError is an error of one field of a FormFor's T, for its submit
// handler or Clean method to return: field is a field reference
// (Signup.Email), so a misspelling is a compile error, and message the
// field's message, or a message key, its args filled in as fmt does.
//
//	if users.Exists(ctx, in.Email) {
//		return ui.FieldError(Signup.Email, "That email is already registered")
//	}
func FieldError(field any, message string, args ...any) error {
	path := ""
	switch f := field.(type) {
	case string:
		path = f
	case interface{ FieldPath() vuka.FieldPath }:
		path = f.FieldPath().String()
	case interface{ Name() string }:
		path = f.Name()
	}
	return &fieldErr{goPath: path, e: forms.Errorf(message, args...)}
}

// fieldErr is FieldError's: the form error of the field at a Go path,
// which the form gives its path.
type fieldErr struct {
	goPath string
	e      forms.Error
}

func (e *fieldErr) Error() string { return e.goPath + ": " + e.e.Error() }

// byRef is the field a Go path or a typed field's name names: the same
// path, else the same ignoring case and underscores ("unit_price").
func (ft *formType) byRef(name string) *formField {
	norm := func(s string) string { return strings.ToLower(strings.NewReplacer("_", "", ".", "").Replace(s)) }
	for _, f := range ft.fields {
		if f.goPath == name {
			return f
		}
	}
	for _, f := range ft.fields {
		if !f.group && norm(f.goPath) == norm(name) {
			return f
		}
	}
	return nil
}

// checkFields runs the fields' checks that need the server (vfield.Checker,
// a unique column's): on submit, for the fields passing their rules. A
// refused value is the field's error until it changes.
func (st *formState) checkFields(ctx context.Context, form reflect.Value) error {
	var rec any
	for _, f := range st.ft.fields {
		if len(f.checkers) == 0 || st.hasRuleError(f.path) {
			continue
		}
		if p := st.pinned[f.path]; p != nil && p.set {
			continue
		}
		v, err := form.FieldByIndexErr(f.index)
		if err != nil {
			continue
		}
		if rec == nil {
			if rec = st.checkRecord(form); rec == nil {
				return nil
			}
		}
		for _, c := range f.checkers {
			err := c.Check(ctx, vfield.Value{Name: f.goPath, Value: v.Interface(), Record: rec})
			if err == nil {
				continue
			}
			var inv *vfield.Invalid
			if !errors.As(err, &inv) {
				return fmt.Errorf("ui: checking %s: %w", f.goPath, err)
			}
			e := f.errorFor(rule{name: inv.Rule}, "")
			if inv.Message != "" {
				e.Key = inv.Message
			}
			st.pin(f.path, &pinnedErrors{errs: []forms.Error{e}, value: valueOf(form, f), set: true})
			st.markVisible(f.path)
			break
		}
	}
	return nil
}

// setNames sets the fields the form edits, by Go path: owned, its owner's
// @ui.Fields.
func (f *FormFor[T]) setNames(names []string, owned bool) { f.names, f.owned = names, owned }

func (f *FormFor[T]) ownNames() bool { return f.owned }

// ownerFields are the fields @ui.Fields names on the field of owner (a
// pointer to a struct) holding the form at form; nil when it names none.
func ownerFields(owner, form reflect.Value) []string {
	for owner.Kind() == reflect.Pointer && !owner.IsNil() && owner.Elem().Kind() == reflect.Pointer {
		owner = owner.Elem()
	}
	if owner.Kind() != reflect.Pointer || owner.IsNil() || owner.Elem().Kind() != reflect.Struct || form.Pointer() < owner.Pointer() {
		return nil
	}
	t := owner.Elem().Type()
	off := form.Pointer() - owner.Pointer()
	for i := range t.NumField() {
		sf := t.Field(i)
		if sf.Offset != off || sf.Type != form.Type().Elem() {
			continue
		}
		if o := FieldOptionsOf(t, sf); o.FormFields != nil {
			return o.FormFields
		}
		for _, a := range vuka.FieldAttrsOf(t)[sf.Name] {
			if fa, ok := a.(FieldsAttr); ok {
				return fa
			}
		}
	}
	return nil
}
