package ui

import (
	"context"
	"fmt"
	"reflect"

	"github.com/vuka-lang/ui/v2/forms"
)

// FormError is one error CheckForm finds: the error, and its message as a
// visitor reads it, the field's label in it.
type FormError struct {
	forms.Error
	Message string
}

// CheckForm runs a form's checks once, as a submit does: each field's
// validate: rules, then the Clean<Field> method of each field passing them,
// then the form's Clean method, the methods' dependencies from ctx's
// Injector (WithInjector). form points to a struct: one embedding
// FormState, or any struct with the same tags and methods, such as the
// input of a GraphQL mutation, which shares the form an HTML page submits.
// It returns every error with its message — the rules' in field order, then
// the Clean methods' — none when the form passes. err is a
// check that failed to run: a Clean method returning an error that isn't a
// forms.Error.
func CheckForm(ctx context.Context, form any) ([]FormError, error) {
	v := reflect.ValueOf(form)
	for v.Kind() == reflect.Pointer && !v.IsNil() && v.Elem().Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("ui: CheckForm of %T, not a pointer to a struct", form)
	}
	st := &formState{ft: formTypeOf(v.Elem().Type())}
	if err := st.check(ctx, v.Elem(), false); err != nil {
		return nil, err
	}
	var out []FormError
	for _, e := range st.all() {
		out = append(out, FormError{Error: e, Message: st.message(ctx, e)})
	}
	return out, nil
}
