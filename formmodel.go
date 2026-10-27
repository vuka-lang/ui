package ui

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// ModelForm is a form editing some fields of a model — a database record's
// struct, or any other — without a struct of its own: FormOf makes one.
// Value holds the model; the form is the fields FormOf names, each with
// the model's tags (validate:, label:, …), in the order named:
//
//	type Edit struct {
//		ui.Live
//		ID   int64
//		Form *ui.ModelForm[models.User]
//	}
//
//	@ui.Mount
//	func (e *Edit) load(ctx context.Context, db *DB) error {
//		u, err := models.Users(db).Get(ctx, e.ID)
//		if err != nil {
//			return err
//		}
//		e.Form = ui.FormOf[models.User]("Name", "Email")
//		e.Form.Load(u)
//		return nil
//	}
//
//	func (e *Edit) Save(ctx context.Context, f *ui.ModelForm[models.User], db *DB) error {
//		u, err := models.Users(db).Get(ctx, e.ID)
//		if err != nil {
//			return err
//		}
//		return f.Apply(&u).Save(ctx, db)
//	}
//
// Markup names its fields through Value: model={Form.Value.Email},
// <ui.Field for={Form.Value.Name}/>; their paths and ids are the model's
// field names ("email", "user-email"). A submit binds only the fields
// named, so a model's other columns — its id, its owner — are never the
// visitor's to set. The model's Clean, Clean<Field> and <Field>Choices
// methods, if it has any, are the form's.
type ModelForm[T any] struct {
	FormState
	// Value is the model the form edits; its fields FormOf named are the
	// form's.
	Value T
	names []string
}

// FormOf is a form editing the fields of T named, by their Go names
// ("Name", "Address.City"): there is no "every field", so a column added
// to the model later is never a submit's to set until the form names it.
// A name T has no field of is an error the form's render shows (and that
// vuka's compiler reports at the call, with the field it most likely
// meant).
func FormOf[T any](fields ...string) *ModelForm[T] {
	return &ModelForm[T]{names: fields}
}

func (f *ModelForm[T]) modelFields() []string { return f.names }

// Load sets the form's fields from v's, and makes them the values Dirty
// and Changed compare with: the record the form edits. (See FormState.Load
// for any other record type.)
func (f *ModelForm[T]) Load(v T) {
	rv := reflect.ValueOf(f)
	st := attachForm(rv)
	st.load(rv.Elem(), reflect.ValueOf(v))
}

// Apply sets the fields of dst the form edits — the ones it named, as the
// visitor sent them — and returns dst, for the call saving it:
// f.Apply(&u).Save(ctx, db).
func (f *ModelForm[T]) Apply(dst *T) *T {
	rv := reflect.ValueOf(f)
	attachForm(rv).apply(rv.Elem(), reflect.ValueOf(dst))
	return dst
}

var modelFormType = reflect.TypeFor[interface{ modelFields() []string }]()

// modelTypes are the form types of model forms, by type and field list.
var modelTypes sync.Map // modelKey → *formType

type modelKey struct {
	t     reflect.Type
	names string
}

// modelFormTypeOf is the form a ModelForm of type t naming names is: the
// fields of its model's type, those named, in their order.
func modelFormTypeOf(t reflect.Type, names []string) *formType {
	key := modelKey{t, strings.Join(names, ",")}
	if v, ok := modelTypes.Load(key); ok {
		return v.(*formType)
	}
	all := formTypeOf(t)
	ft := &formType{t: t, name: all.name, state: all.state, stateAt: all.stateAt, recv: all.recv,
		byPath: map[string]*formField{}, cleans: map[string]*reflect.Method{}, clean: all.clean, err: all.err}
	model := t.Field(all.recv[0]).Type
	if len(names) == 0 && ft.err == nil {
		ft.err = fmt.Errorf("ui: ui.FormOf[%s] names no field: name the fields the form edits, ui.FormOf[%[1]s](\"Name\", \"Email\")", model.Name())
	}
	for _, name := range names {
		var picked []*formField
		for _, f := range all.fields {
			if f.goPath == name || strings.HasPrefix(f.goPath, name+".") {
				picked = append(picked, f)
			}
		}
		if len(picked) == 0 {
			if ft.err == nil {
				ft.err = unknownModelField(model, name, all)
			}
			continue
		}
		for _, f := range picked {
			if ft.byPath[f.path] != nil {
				continue
			}
			ft.fields = append(ft.fields, f)
			ft.byPath[f.path] = f
			if m := all.cleans[f.path]; m != nil {
				ft.cleans[f.path] = m
			}
		}
		// a group's own path, when only its fields were named
		if parent, _, ok := cutLast(name); ok {
			for _, f := range all.fields {
				if f.goPath == parent && f.group && ft.byPath[f.path] == nil {
					ft.byPath[f.path] = f
				}
			}
		}
	}
	for _, f := range ft.fields {
		for _, d := range f.depends {
			if ft.byPath[d] == nil && ft.err == nil {
				ft.err = fmt.Errorf("ui: ui.FormOf[%s]: %s's choices depend on %s, which the form doesn't name", model.Name(), f.goPath, d)
			}
		}
	}
	v, _ := modelTypes.LoadOrStore(key, ft)
	return v.(*formType)
}

func cutLast(path string) (string, string, bool) {
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return "", path, false
	}
	return path[:i], path[i+1:], true
}

// unknownModelField is the error of a FormOf naming no field of the model
// (vuka's compiler reports it at the call: E0161).
func unknownModelField(model reflect.Type, name string, all *formType) error {
	var names []string
	for _, f := range all.fields {
		names = append(names, f.goPath)
	}
	if s := closestName(name, names); s != "" {
		return fmt.Errorf("ui: ui.FormOf[%s]: %[1]s has no field %s; did you mean %s?", model.Name(), name, s)
	}
	return fmt.Errorf("ui: ui.FormOf[%s]: %[1]s has no field %s; its fields are %s", model.Name(), name, strings.Join(names, ", "))
}

// closestName is the name of names a mistyped one most likely meant: the
// same ignoring case, else one edit or two away.
func closestName(name string, names []string) string {
	best, bestD := "", 3
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return n
		}
		if d := editDistance(strings.ToLower(n), strings.ToLower(name)); d < bestD {
			best, bestD = n, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
