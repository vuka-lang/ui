package ui

import (
	"context"
	"reflect"
	"testing"
)

type eraseRowT struct {
	ID    int
	Title string
}

// TestTableErase: Table's and DataTable's erase carry every field, so a
// field added to either is rendered; and the functions given a row get the
// caller's row.
func TestTableErase(t *testing.T) {
	for _, c := range []struct {
		name   string
		in     any
		erased func(v any) any
	}{
		{"Table", &Table[eraseRowT]{}, func(v any) any { return v.(*Table[eraseRowT]).erase() }},
		{"DataTable", &DataTable[eraseRowT]{}, func(v any) any { return v.(*DataTable[eraseRowT]).erase() }},
	} {
		v := reflect.ValueOf(c.in).Elem()
		for i := range v.NumField() {
			f := v.Field(i)
			f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem() // unexported too
			f.Set(nonZero(f.Type()))
		}
		out := reflect.ValueOf(c.erased(c.in))
		for i := range v.NumField() {
			name := v.Type().Field(i).Name
			if out.FieldByName(name).IsZero() {
				t.Errorf("%s.erase drops %s", c.name, name)
			}
		}
	}

	rows := []eraseRowT{{1, "a"}, {2, "b"}}
	tb := Table[eraseRowT]{Rows: rows, RowKey: func(r eraseRowT) any { return r.ID },
		Columns: []Column[eraseRowT]{{Cell: func(r eraseRowT) Node { return Text(r.Title) }}}}.erase()
	if k := tb.RowKey(tb.Rows[1]); k != 2 {
		t.Errorf("the second row's key: %v", k)
	}
	if s, _ := String(context.Background(), tb.Columns[0].Cell(tb.Rows[0])); s != "a" {
		t.Errorf("the first row's cell: %q", s)
	}
	if e := (Table[any]{Rows: []any{"x"}}).erase(); e.Rows[0] != "x" {
		t.Errorf("Table[any] is erased again: %v", e.Rows)
	}
}

// nonZero is a value of t that isn't its zero value.
func nonZero(t reflect.Type) reflect.Value {
	switch t.Kind() {
	case reflect.Func:
		return reflect.MakeFunc(t, func([]reflect.Value) []reflect.Value {
			out := make([]reflect.Value, t.NumOut())
			for i := range out {
				out[i] = reflect.Zero(t.Out(i))
			}
			return out
		})
	case reflect.Interface:
		return reflect.ValueOf(Text("x"))
	case reflect.Slice:
		return reflect.MakeSlice(t, 1, 1)
	case reflect.Map:
		m := reflect.MakeMap(t)
		m.SetMapIndex(reflect.Zero(t.Key()), nonZero(t.Elem()))
		return m
	case reflect.Struct:
		v := reflect.New(t).Elem()
		for i := range v.NumField() {
			if f := v.Field(i); f.CanSet() {
				f.Set(nonZero(f.Type()))
			}
		}
		return v
	case reflect.String:
		return reflect.ValueOf("x").Convert(t)
	case reflect.Bool:
		return reflect.ValueOf(true)
	case reflect.Int:
		return reflect.ValueOf(1)
	}
	panic("nonZero: " + t.String())
}
