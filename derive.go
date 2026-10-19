package ui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Expressions of states are methods of State, each a derived state the
// browser evaluates again when a state it reads changes, as a .vuka file's
// !open && tab == "env" is:
//
//	ready := open.Not().And(tab.Eq("env"))
//	<button disabled={empty}>       // empty := ui.At[string](f, "Q").Eq("").Or(busy)
//	<span>{next}</span>             // next := n.Plus(1)
//
// An operand is a value of the state's type or a state of it; a number
// converts to a number state's type. The server renders the value the
// expression has with the states' initial values. Not, And and Or take bool
// states, Neg, Minus and the ordering ones numbers (Lt… strings too), Plus
// numbers and strings, Len strings, slices and maps; another state panics,
// as a .vuka file's compiler refuses it.

// Not is !s.
func (s State[T]) Not() State[bool] {
	return NewDerived(!s.kind("Not", isBool).Bool(), exprNot, s)
}

// And is s && o.
func (s State[T]) And(o any) State[bool] {
	a, b := s.kind("And", isBool), operand("And", reflect.TypeFor[bool](), o)
	return NewDerived(a.Bool() && b.Bool(), exprAnd, s, o)
}

// Or is s || o.
func (s State[T]) Or(o any) State[bool] {
	a, b := s.kind("Or", isBool), operand("Or", reflect.TypeFor[bool](), o)
	return NewDerived(a.Bool() || b.Bool(), exprOr, s, o)
}

// Eq is s == o.
func (s State[T]) Eq(o any) State[bool] { return s.equal("Eq", o, exprEq, false) }

// Ne is s != o.
func (s State[T]) Ne(o any) State[bool] { return s.equal("Ne", o, exprNe, true) }

func (s State[T]) equal(op string, o any, expr string, not bool) State[bool] {
	a, b := s.value(), operand(op, reflect.TypeFor[T](), o)
	return NewDerived((a.Interface() == b.Interface()) != not, expr, s, o)
}

// Lt is s < o.
func (s State[T]) Lt(o any) State[bool] {
	return s.order("Lt", o, exprLt, func(c int) bool { return c < 0 })
}

// Le is s <= o.
func (s State[T]) Le(o any) State[bool] {
	return s.order("Le", o, exprLe, func(c int) bool { return c <= 0 })
}

// Gt is s > o.
func (s State[T]) Gt(o any) State[bool] {
	return s.order("Gt", o, exprGt, func(c int) bool { return c > 0 })
}

// Ge is s >= o.
func (s State[T]) Ge(o any) State[bool] {
	return s.order("Ge", o, exprGe, func(c int) bool { return c >= 0 })
}

func (s State[T]) order(op string, o any, expr string, holds func(int) bool) State[bool] {
	a, b := s.kind(op, isOrdered), operand(op, reflect.TypeFor[T](), o)
	return NewDerived(holds(compareValues(a, b)), expr, s, o)
}

// Plus is s + o: a sum, or strings joined.
func (s State[T]) Plus(o any) State[T] {
	a, b := s.kind("Plus", isAddable), operand("Plus", reflect.TypeFor[T](), o)
	return NewDerived(arith(a, b, 1).Interface().(T), exprPlus, s, o)
}

// Minus is s - o.
func (s State[T]) Minus(o any) State[T] {
	a, b := s.kind("Minus", isNumber), operand("Minus", reflect.TypeFor[T](), o)
	return NewDerived(arith(a, b, -1).Interface().(T), exprMinus, s, o)
}

// Neg is -s.
func (s State[T]) Neg() State[T] {
	a := s.kind("Neg", isNumber)
	return NewDerived(arith(reflect.Zero(a.Type()), a, -1).Interface().(T), exprNeg, s)
}

// Len is len(s).
func (s State[T]) Len() State[int] {
	return NewDerived(s.kind("Len", isLenable).Len(), exprLen, s)
}

// At is the field at path (Go field names, dotted) of a struct state:
// a state of its own, which Set writes alone and what reads it follows
// alone, as f.Q is in a .vuka file.
//
//	q := ui.At[string](f, "Q")
//	hi := ui.At[int](f, "Range.Hi")
func At[T, S any](s State[S], path string) State[T] {
	if s.c != nil && s.c.derived != nil {
		panic("ui: a field of a ui.Derived isn't a state")
	}
	names := strings.Split(path, ".")
	t, v := reflect.TypeFor[S](), reflect.ValueOf(&s.v).Elem()
	for _, name := range names {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
			if v.IsValid() && !v.IsNil() {
				v = v.Elem()
			} else {
				v = reflect.Value{}
			}
		}
		f, ok := t.FieldByName(name)
		if t.Kind() != reflect.Struct || !ok {
			panic(fmt.Sprintf("ui: At: %s has no field %s", t, name))
		}
		t = f.Type
		if v.IsValid() {
			v = v.FieldByIndex(f.Index)
		}
	}
	if want := reflect.TypeFor[T](); t != want {
		panic(fmt.Sprintf("ui: At: %s of a ui.State[%s] is a %s, not a %s", path, reflect.TypeFor[S](), t, want))
	}
	var x T
	if v.IsValid() {
		x = v.Interface().(T)
	}
	return State[T]{c: s.c, path: append(s.path[:len(s.path):len(s.path)], names...), v: x}
}

// Classes is a class attribute whose names may be switched by states: each
// value a bool or a bool state (ui.Class holds bools only).
//
//	class={cls}   // cls := ui.Classes{"menu": true, "is-open": open}
type Classes map[string]any

// binding is the class attribute: its names as rendered, and the browser's
// expression of them.
func (c Classes) binding() Binding {
	names := make([]string, 0, len(c))
	for k := range c {
		names = append(names, k)
	}
	sort.Strings(names)
	init := make(Class, len(c))
	var b strings.Builder
	var deps []any
	b.WriteString(`["class",{`)
	for i, k := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		j, _ := json.Marshal(k)
		b.Write(j)
		b.WriteByte(':')
		switch v := c[k].(type) {
		case bool:
			init[k] = v
			b.WriteString(fmt.Sprint(v))
		case AnyState:
			init[k] = operand("Classes", reflect.TypeFor[bool](), v).Bool()
			fmt.Fprintf(&b, `{"$":%d}`, len(deps))
			deps = append(deps, v)
		default:
			panic(fmt.Sprintf("ui: Classes: %s is %T; it takes a bool or a ui.State[bool]", k, v))
		}
	}
	b.WriteString(`}]`)
	return BindExpr(init, b.String(), deps...)
}

const (
	exprNot   = `["!",{"$":0}]`
	exprNeg   = `["neg",{"$":0}]`
	exprLen   = `["len",{"$":0}]`
	exprAnd   = `["&&",{"$":0},{"$":1}]`
	exprOr    = `["||",{"$":0},{"$":1}]`
	exprEq    = `["==",{"$":0},{"$":1}]`
	exprNe    = `["!=",{"$":0},{"$":1}]`
	exprLt    = `["<",{"$":0},{"$":1}]`
	exprLe    = `["<=",{"$":0},{"$":1}]`
	exprGt    = `[">",{"$":0},{"$":1}]`
	exprGe    = `[">=",{"$":0},{"$":1}]`
	exprPlus  = `["+",{"$":0},{"$":1}]`
	exprMinus = `["-",{"$":0},{"$":1}]`
)

func isBool(k reflect.Kind) bool    { return k == reflect.Bool }
func isNumber(k reflect.Kind) bool  { return numeric(k) }
func isOrdered(k reflect.Kind) bool { return numeric(k) || k == reflect.String }
func isAddable(k reflect.Kind) bool { return numeric(k) || k == reflect.String }
func isLenable(k reflect.Kind) bool {
	return k == reflect.String || k == reflect.Slice || k == reflect.Map || k == reflect.Array
}

// value is the state's value, its type's zero when it holds nil.
func (s State[T]) value() reflect.Value { return reflect.ValueOf(&s.v).Elem() }

// kind is the state's value, which op takes only of the kinds ok allows.
func (s State[T]) kind(op string, ok func(reflect.Kind) bool) reflect.Value {
	if v := s.value(); ok(v.Kind()) {
		return v
	}
	panic(fmt.Sprintf("ui: %s doesn't take a ui.State[%s]", op, reflect.TypeFor[T]()))
}

// operand is o, a value or a state, as a value of type t.
func operand(op string, t reflect.Type, o any) reflect.Value {
	v := reflect.ValueOf(o)
	if s, ok := o.(AnyState); ok {
		sv := s.stateOf()
		v = reflect.Zero(sv.t)
		if sv.v != nil {
			v = reflect.ValueOf(sv.v)
		}
	}
	switch {
	case !v.IsValid():
		if k := t.Kind(); k == reflect.Pointer || k == reflect.Slice || k == reflect.Map || k == reflect.Interface {
			return reflect.Zero(t)
		}
	case v.Type() == t:
		return v
	case v.Kind() == t.Kind(), numeric(v.Kind()) && numeric(t.Kind()):
		if v.CanConvert(t) {
			return v.Convert(t)
		}
	}
	panic(fmt.Sprintf("ui: %s: a ui.State[%s] takes no %T", op, t, o))
}

// compareValues orders a and b, numbers or strings of one type.
func compareValues(a, b reflect.Value) int {
	switch {
	case a.CanInt():
		return cmp3(a.Int() < b.Int(), a.Int() > b.Int())
	case a.CanUint():
		return cmp3(a.Uint() < b.Uint(), a.Uint() > b.Uint())
	case a.CanFloat():
		return cmp3(a.Float() < b.Float(), a.Float() > b.Float())
	}
	return strings.Compare(a.String(), b.String())
}

func cmp3(lt, gt bool) int {
	switch {
	case lt:
		return -1
	case gt:
		return 1
	}
	return 0
}

// arith is a + sign*b, numbers or strings (sign 1) of one type.
func arith(a, b reflect.Value, sign int64) reflect.Value {
	r := reflect.New(a.Type()).Elem()
	switch {
	case a.CanInt():
		r.SetInt(a.Int() + sign*b.Int())
	case a.CanUint():
		if sign < 0 {
			r.SetUint(a.Uint() - b.Uint())
		} else {
			r.SetUint(a.Uint() + b.Uint())
		}
	case a.CanFloat():
		r.SetFloat(a.Float() + float64(sign)*b.Float())
	default:
		r.SetString(a.String() + b.String())
	}
	return r
}
