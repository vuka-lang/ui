package live

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/vuka-lang/ui"
)

var (
	ctxType   = reflect.TypeFor[context.Context]()
	errType   = reflect.TypeFor[error]()
	eventType = reflect.TypeFor[ui.Event]()
	formType  = reflect.TypeFor[url.Values]()
)

// callHandler calls an event handler, its parameters filled by type
// (ui.Injector): a context.Context the session's, and the event's payload
// the one parameter that asks for it:
//
//	func()                    nothing
//	func(string)              the value: the key of a keyboard event, else the element's value
//	func(bool), func(int), …  the value, parsed (a checkbox's value is "true" or "false"; "on" is true)
//	func(ui.Event)            the whole event
//	func(url.Values)          the form's fields, as sent (also map[string][]string)
//	func(T), func(*T)         T a struct: the form's fields bound to its fields
//
// Its other parameters are what the session's context's Injector fills:
// the request, the signed-in user, a service. A struct breaking its
// validate: tags is a ui.Invalid, the handler not called. The handler may
// return an error.
func callHandler(ctx context.Context, fn any, ev ui.Event) (err error) {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() {
		return fmt.Errorf("live: handler %s is %T, not a function", ev.Target, fn)
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("live: handler %s panicked: %v", ev.Target, r)
		}
	}()
	return ui.CallHandler(ctx, fn, func(t reflect.Type) (reflect.Value, error) { return payload(t, ev) })
}

func result(out []reflect.Value) error {
	if len(out) == 1 && out[0].Type() == errType && !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}

func payload(t reflect.Type, ev ui.Event) (reflect.Value, error) {
	switch {
	case t == eventType:
		return reflect.ValueOf(ev), nil
	case formType.ConvertibleTo(t) && t.Kind() == reflect.Map:
		return reflect.ValueOf(ev.Form).Convert(t), nil
	case t.Kind() == reflect.Struct:
		v := reflect.New(t)
		return v.Elem(), structPayload(v, ev)
	case t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct:
		v := reflect.New(t.Elem())
		return v, structPayload(v, ev)
	}
	s := ev.Value
	if ev.Key != "" && strings.HasPrefix(ev.Type, "key") {
		s = ev.Key
	}
	if len(ev.Data) > 0 && s == "" { // an island's function prop called with a scalar
		var err error
		if s, err = scalarData(ev.Data); err != nil {
			return reflect.New(t).Elem(), publicErr{fmt.Errorf("live: %s event payload: %w", ev.Type, err)}
		}
	}
	v := reflect.New(t).Elem()
	if u, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
		if err := u.UnmarshalText([]byte(s)); err != nil {
			return v, publicErr{fmt.Errorf("live: %s event: %w", ev.Type, err)}
		}
		return v, nil
	}
	if err := setScalar(v, s); err != nil {
		return v, publicErr{fmt.Errorf("live: %s event: %w", ev.Type, err)}
	}
	return v, nil
}

// structPayload binds the event's payload into ptr (a *T): an island
// emit's JSON argument, else the form's fields; then checks T's validate:
// tags, as a route checks what it binds, naming each field as it was sent.
func structPayload(ptr reflect.Value, ev ui.Event) error {
	if len(ev.Data) > 0 {
		if err := jsonPayload(ptr.Interface(), ev); err != nil {
			return err
		}
		return ui.Validate(ptr.Interface())
	}
	if err := bindForm(ptr.Elem(), ev.Form); err != nil {
		return err
	}
	return ui.ValidateTags(ptr.Interface(), "form", "json")
}

// jsonPayload binds an island emit's JSON argument (ev.Data) into ptr (a *T).
func jsonPayload(ptr any, ev ui.Event) error {
	if err := json.Unmarshal(ev.Data, ptr); err != nil {
		return publicErr{fmt.Errorf("live: %s event payload: %w", ev.Type, err)}
	}
	return nil
}

// scalarData is a JSON scalar (an island function's argument) as the text an
// event's value would be.
func scalarData(data []byte) (string, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var x any
	if err := d.Decode(&x); err != nil {
		return "", err
	}
	switch x := x.(type) {
	case nil:
		return "", nil
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	case bool:
		return strconv.FormatBool(x), nil
	}
	return "", fmt.Errorf("an object or a list isn't a %s; take a struct", "scalar")
}

func setScalar(v reflect.Value, s string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Bool:
		b := s == "on"
		if !b && s != "" {
			var err error
			if b, err = strconv.ParseBool(s); err != nil {
				return fmt.Errorf("%q isn't true or false", s)
			}
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q isn't a whole number", s)
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(strings.TrimSpace(s), 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q isn't a whole number", s)
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(strings.TrimSpace(s), v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q isn't a number", s)
		}
		v.SetFloat(n)
	default:
		return fmt.Errorf("can't take a %s", v.Type())
	}
	return nil
}

// bindForm sets v's fields from form: each field is named by its form tag, else
// its json tag, else its name, matched ignoring case; embedded structs' fields
// are v's own. A slice takes every value, anything else the first.
func bindForm(v reflect.Value, form url.Values) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := bindForm(fv, form); err != nil {
				return err
			}
			continue
		}
		if !f.IsExported() {
			continue
		}
		name := fieldName(f)
		if name == "-" {
			continue
		}
		vals := lookup(form, name)
		if vals == nil {
			continue
		}
		if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() != reflect.Uint8 {
			s := reflect.MakeSlice(fv.Type(), len(vals), len(vals))
			for j, x := range vals {
				if err := setScalar(s.Index(j), x); err != nil {
					return publicErr{fmt.Errorf("live: field %s: %w", name, err)}
				}
			}
			fv.Set(s)
			continue
		}
		if err := setScalar(fv, vals[0]); err != nil {
			return publicErr{fmt.Errorf("live: field %s: %w", name, err)}
		}
	}
	return nil
}

func fieldName(f reflect.StructField) string {
	for _, tag := range []string{"form", "json"} {
		if n, _, _ := strings.Cut(f.Tag.Get(tag), ","); n != "" {
			return n
		}
	}
	return f.Name
}

func lookup(form url.Values, name string) []string {
	if vs, ok := form[name]; ok {
		return vs
	}
	for k, vs := range form {
		if strings.EqualFold(k, name) {
			return vs
		}
	}
	return nil
}

// callUpdate delivers msg to c's Update method taking its type: Update itself,
// or one of its overloads (UpdateT, ui.IsUpdate), each taking an optional
// context.Context and one message, returning nothing or an error. A method
// taking msg's type exactly wins over one it is only assignable to; of those,
// one taking a non-empty interface wins over one taking any.
func callUpdate(ctx context.Context, c ui.Stateful, msg any) (called bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			called, err = true, fmt.Errorf("live: %T's Update panicked: %v", c, r)
		}
	}()
	m, err := updateMethod(c, msg)
	if !m.IsValid() || err != nil {
		return false, err
	}
	arg := reflect.New(m.Type().In(m.Type().NumIn() - 1)).Elem()
	if msg != nil {
		arg.Set(reflect.ValueOf(msg))
	}
	args := []reflect.Value{arg}
	if m.Type().NumIn() == 2 {
		args = []reflect.Value{reflect.ValueOf(ctx), arg}
	}
	return true, result(m.Call(args))
}

// takesUpdate reports whether c has one Update method msg reaches.
func takesUpdate(c ui.Stateful, msg any) bool {
	m, err := updateMethod(c, msg)
	return m.IsValid() && err == nil
}

// updateMethod is the Update method of c that msg reaches (callUpdate);
// invalid when there is none.
func updateMethod(c ui.Stateful, msg any) (reflect.Value, error) {
	v := reflect.ValueOf(c)
	mt := reflect.TypeOf(msg)
	var exact, assignable, catchAll []reflect.Value
	for i := 0; i < v.NumMethod(); i++ {
		m := v.Type().Method(i)
		if !ui.IsUpdate(m) {
			continue
		}
		ft := m.Type
		n := ft.NumIn() - 1
		first := 1
		if n == 2 && ft.In(1) == ctxType {
			n, first = 1, 2
		}
		if n != 1 || ft.NumOut() > 1 || ft.NumOut() == 1 && ft.Out(0) != errType {
			continue
		}
		pt := ft.In(first)
		switch {
		case mt == nil && (pt.Kind() == reflect.Interface || pt.Kind() == reflect.Pointer):
			catchAll = append(catchAll, v.Method(i))
		case mt == nil:
		case pt == mt:
			exact = append(exact, v.Method(i))
		case pt.Kind() == reflect.Interface && pt.NumMethod() == 0:
			catchAll = append(catchAll, v.Method(i))
		case mt.AssignableTo(pt):
			assignable = append(assignable, v.Method(i))
		}
	}
	for _, set := range [][]reflect.Value{exact, assignable, catchAll} {
		switch len(set) {
		case 0:
			continue
		case 1:
			return set[0], nil
		default:
			return reflect.Value{}, fmt.Errorf("live: %s has %d Update methods taking %v", v.Type().Elem().Name(), len(set), mt)
		}
	}
	return reflect.Value{}, nil
}

// mount runs c's mount hook (ui.MountLive); a panic is its error.
func mount(ctx context.Context, c ui.Stateful) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("live: %T's Mount panicked: %v", c, r)
		}
	}()
	return ui.MountLive(ctx, c)
}
