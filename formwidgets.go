package ui

import (
	"fmt"
	"reflect"
	"sync/atomic"
)

// The app's own renderers of a form's error messages and error summary
// (widget.UseError, widget.UseSummary): components filled, by name and
// type, with what the form's own would show.
var appErrorWidget, appSummaryWidget atomic.Value // reflect.Value

// UseErrorWidget makes c, a component, what renders every form field's
// error message: what widget.UseError does. The form sets, by name, its
// Message string, its Attrs []ui.Attr (the class, id and data-vk-error
// its root element carries, for the control's aria-describedby and the
// runtime) and its Model ui.FieldRef, the field, those it declares. A nil
// c puts the form's own back.
func UseErrorWidget(c any) { usePartWidget(&appErrorWidget, c, "Message", "Attrs") }

// UseSummaryWidget makes c, a component, what renders every form's error
// summary: what widget.UseSummary does. The form sets, by name, its Title
// string, its Items []ui.SummaryItem (each error and the link to its
// field) and its Attrs []ui.Attr (the id, tabindex, autofocus and
// data-vk-summary its root element carries, which the runtime moves focus
// to), those it declares. A nil c puts the form's own back.
func UseSummaryWidget(c any) { usePartWidget(&appSummaryWidget, c, "Items", "Attrs") }

func usePartWidget(slot *atomic.Value, c any, need ...string) {
	if c == nil {
		slot.Store(reflect.Value{})
		return
	}
	v := reflect.ValueOf(c)
	if _, ok := c.(Node); !ok || v.Kind() != reflect.Struct {
		panic(fmt.Sprintf("ui: %T isn't a component (a struct with a Render method)", c))
	}
	for _, n := range need {
		if _, ok := v.Type().FieldByName(n); !ok {
			panic(fmt.Sprintf("ui: %s has no %s field for the form to set", v.Type(), n))
		}
	}
	slot.Store(v)
}

// renderPartWidget is a copy of w with the fields of props it declares
// set, as a Node.
func renderPartWidget(w reflect.Value, props map[string]any) Node {
	cp := reflect.New(w.Type()).Elem()
	cp.Set(w)
	for name, v := range props {
		f := cp.FieldByName(name)
		if !f.IsValid() || !f.CanSet() {
			continue
		}
		rv := reflect.ValueOf(v)
		if !rv.Type().AssignableTo(f.Type()) {
			return Try(nil, fmt.Errorf("ui: %s.%s is a %s; the form sets it to a %s", w.Type(), name, f.Type(), rv.Type()))
		}
		f.Set(rv)
	}
	return cp.Interface().(Node)
}
