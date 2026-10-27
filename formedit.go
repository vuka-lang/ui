package ui

import (
	"reflect"
	"strings"
	"time"
	"unsafe"
)

// Editing a record: Load copies a record's values into the form and makes
// them its baseline, Apply writes the form's back; Dirty, Changed and Reset
// compare with, and go back to, the baseline; Saved follows a successful
// submit.

// savedFor is how long Saved stays true after a successful submit.
const savedFor = 2 * time.Second

// value is the form s is the state of, and its state; invalid until the
// form was first rendered, bound or loaded.
func (s *FormState) value() (reflect.Value, *formState) {
	if s == nil || s.st == nil || s.st.ft == nil {
		return reflect.Value{}, nil
	}
	st := s.st
	return reflect.NewAt(st.ft.t, unsafe.Add(unsafe.Pointer(s), -int(st.ft.stateAt))).Elem(), st
}

// Load sets the form's fields from record's — a struct, or a pointer to
// one, such as the database record the form edits — and makes them the
// values Dirty and Changed compare with and Reset goes back to. A field
// takes the record's field of the same Go name (Email, Address.City) when
// its type is the same kind of value: a string's, a number's, a bool's,
// a slice of them, a time; a pointer stands for its value. The form's
// errors and what the visitor used of it start afresh, and the browser's
// fields take the new values.
//
// Called in Mount, before the form first renders, it takes effect as the
// form is attached to its page, once Mount returned.
func (s *FormState) Load(record any) {
	if s.st == nil {
		s.st = &formState{}
	}
	form, st := s.value()
	if st == nil {
		s.st.pending = reflect.ValueOf(record)
		return
	}
	st.load(form, reflect.ValueOf(record))
}

// Apply sets dst's fields — dst a pointer to a struct, the record the form
// edits — from the form's fields of the same Go names: only the fields the
// form declares, and of those, the ones its last submit rendered. It
// returns the Go names of the fields it set, the columns to save.
func (s *FormState) Apply(dst any) []string {
	form, st := s.value()
	if st == nil {
		return nil
	}
	return st.apply(form, reflect.ValueOf(dst))
}

// Dirty reports whether any field differs from the values the form was
// loaded with (Load), or first rendered with; a successful submit makes
// its values the new ones.
func (s *FormState) Dirty() bool {
	form, st := s.value()
	if st == nil {
		return false
	}
	return st.dirty(form)
}

// Reset puts back the values the form was loaded with (Load), or first
// rendered with, and clears its errors; the browser's fields take them,
// a focused one too.
func (s *FormState) Reset() {
	form, st := s.value()
	if st == nil {
		return
	}
	base := cloneValue(st.base)
	for _, f := range st.ft.fields {
		if f.group {
			continue
		}
		if dst, err := form.FieldByIndexErr(f.index); err == nil && dst.CanSet() {
			if src, err := base.FieldByIndexErr(f.index); err == nil {
				dst.Set(src)
			}
		}
	}
	st.clear()
	st.gen++
}

// Saved reports whether the form's last submit succeeded in the last 2
// seconds: for a "Saved" note beside its button. A live page shows the
// form again when the time is up.
func (s *FormState) Saved() bool {
	return s.st != nil && time.Now().Before(s.st.savedUntil)
}

// load sets the form's fields from rec's, rec's values its baseline.
func (st *formState) load(form, rec reflect.Value) {
	for rec.Kind() == reflect.Pointer || rec.Kind() == reflect.Interface {
		if rec.IsNil() {
			rec = reflect.Value{}
			break
		}
		rec = rec.Elem()
	}
	if rec.IsValid() && rec.Kind() == reflect.Struct {
		for _, f := range st.ft.fields {
			if f.group {
				continue
			}
			src, ok := fieldByGoPath(rec, f.goPath, false)
			dst, err := form.FieldByIndexErr(f.index)
			if ok && err == nil && dst.CanSet() && src.CanInterface() {
				copyValue(dst, src, false)
			}
		}
	}
	st.clear()
	st.base = cloneValue(form)
	st.gen++
}

// apply sets dst's fields from the form's, those its last bind took; the
// Go names set.
func (st *formState) apply(form, dst reflect.Value) []string {
	for dst.Kind() == reflect.Pointer && !dst.IsNil() && dst.Elem().Kind() == reflect.Pointer {
		dst = dst.Elem()
	}
	if dst.Kind() != reflect.Pointer || dst.IsNil() || dst.Elem().Kind() != reflect.Struct {
		return nil
	}
	rec := dst.Elem()
	var out []string
	for _, f := range st.ft.fields {
		if f.group {
			continue
		}
		if st.bound != nil {
			if _, ok := st.bound[f.path]; !ok {
				continue
			}
		}
		src, err := form.FieldByIndexErr(f.index)
		to, ok := fieldByGoPath(rec, f.goPath, true)
		if err != nil || !ok || !to.CanSet() {
			continue
		}
		if copyValue(to, src, true) {
			out = append(out, f.goPath)
		}
	}
	return out
}

func (st *formState) dirty(form reflect.Value) bool {
	if !st.base.IsValid() {
		return false
	}
	for _, f := range st.ft.fields {
		if !f.group && !sameValue(valueOf(form, f), valueOf(st.base, f)) {
			return true
		}
	}
	return false
}

// clear drops what the form knows of the visitor's use of it: errors, the
// used fields, the submitted state.
func (st *formState) clear() {
	st.errs, st.pinned, st.failed, st.raw, st.parsed = nil, nil, nil, nil, nil
	st.used, st.shown, st.vis = nil, nil, nil
	st.submitted = false
}

// fieldByGoPath is the field of struct v at a Go path ("Address.City");
// alloc allocates the nil pointers on the way.
func fieldByGoPath(v reflect.Value, path string, alloc bool) (reflect.Value, bool) {
	for _, name := range strings.Split(path, ".") {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				if !alloc || !v.CanSet() {
					return reflect.Value{}, false
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		sf, ok := v.Type().FieldByName(name)
		if !ok || !sf.IsExported() {
			return reflect.Value{}, false
		}
		f, err := v.FieldByIndexErr(sf.Index)
		if err != nil {
			return reflect.Value{}, false // through a nil embedded pointer
		}
		v = f
	}
	return v, true
}

// copyValue sets dst from src when they hold the same kind of value (see
// Load); a pointer stands for its value. nilZero sets a pointer dst to nil
// for a zero src: an empty field is no value.
func copyValue(dst, src reflect.Value, nilZero bool) bool {
	if src.Type().AssignableTo(dst.Type()) && sameClass(src.Type(), dst.Type()) {
		if src.Kind() == reflect.Pointer && nilZero && !src.IsNil() && src.Elem().IsZero() {
			dst.SetZero()
			return true
		}
		dst.Set(cloneOne(src))
		return true
	}
	if src.Kind() == reflect.Pointer {
		if src.IsNil() {
			if !sameClass(src.Type().Elem(), derefType(dst.Type())) {
				return false
			}
			dst.SetZero()
			return true
		}
		return copyValue(dst, src.Elem(), nilZero)
	}
	if dst.Kind() == reflect.Pointer {
		if !sameClass(src.Type(), derefType(dst.Type())) {
			return false
		}
		if nilZero && src.IsZero() {
			dst.SetZero()
			return true
		}
		p := reflect.New(dst.Type().Elem())
		if !copyValue(p.Elem(), src, nilZero) {
			return false
		}
		dst.Set(p)
		return true
	}
	if sameClass(src.Type(), dst.Type()) && src.Type().ConvertibleTo(dst.Type()) {
		dst.Set(cloneOne(src).Convert(dst.Type()))
		return true
	}
	return false
}

// cloneOne is v, a slice copied so the copy and v don't share elements.
func cloneOne(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Slice && !v.IsNil() {
		return reflect.AppendSlice(reflect.MakeSlice(v.Type(), 0, v.Len()), v)
	}
	return v
}

// sameClass reports whether a and b hold the same kind of value: text,
// whole numbers, fractions, truth values, slices of one class, the same
// struct. An int is never copied into a string, nor a string into bytes.
func sameClass(a, b reflect.Type) bool {
	a, b = derefType(a), derefType(b)
	ca, cb := valueClass(a), valueClass(b)
	if ca != cb {
		return false
	}
	switch ca {
	case "slice":
		return sameClass(a.Elem(), b.Elem())
	case "other":
		return a == b
	}
	return true
}

func valueClass(t reflect.Type) string {
	switch {
	case t == timeType:
		return "time"
	case t.Kind() == reflect.String:
		return "string"
	case isInt(t):
		return "int"
	case isFloat(t):
		return "float"
	case t.Kind() == reflect.Bool:
		return "bool"
	case t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Uint8:
		return "slice"
	}
	return "other"
}

// attachForms attaches the forms among p's fields (p a pointer to a
// page's struct): a record Load was given in Mount is loaded.
func attachForms(p reflect.Value) {
	if p.Kind() != reflect.Pointer || p.IsNil() || p.Elem().Kind() != reflect.Struct {
		return
	}
	v := p.Elem()
	for i := 0; i < v.NumField(); i++ {
		sf := v.Type().Field(i)
		if !sf.IsExported() || sf.Anonymous {
			continue
		}
		f := v.Field(i)
		switch {
		case f.Kind() == reflect.Pointer && !f.IsNil() && isForm(f.Type().Elem()):
			attachForm(f)
		case f.Kind() == reflect.Struct && isForm(f.Type()) && f.CanAddr():
			attachForm(f.Addr())
		}
	}
}
