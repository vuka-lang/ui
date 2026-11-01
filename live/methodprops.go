package live

import (
	"reflect"
	"sync/atomic"
	"unsafe"

	"github.com/vuka-lang/ui/v2"
)

// A func prop set to a method value (ui.MethodProps) compares by its receiver
// and method, so a parent's render passing the same one leaves a tracked child
// skipped. That is only right while the child doesn't call it to render:
//
//	<td>{Format(order.Total)}</td>
//
// shows what the receiver holds now, and a skipped child would keep showing
// what it held. So the instance holds a stand-in in the field, calling the
// method and noting a call made while the session renders; a child whose last
// render called one renders whenever its parent does, as with any func prop. It
// is a run-time note, so it covers what no reading of the source could: a
// helper in plain Go, a component further down given the func, a call only some
// renders make.

// methodProp is one such prop of an instance.
type methodProp struct {
	name string                        // the field's
	fn   atomic.Pointer[reflect.Value] // the method value the tag last set
	wrap reflect.Value                 // the stand-in, the field's value
}

// standIn is a stand-in func's code pointer: one a handler has is checked
// against the instances' (boundMethod), any other isn't.
var standIn = reflect.ValueOf(reflect.MakeFunc(reflect.TypeFor[func()](), nil).Interface()).Pointer()

// trackMethods puts stand-ins in the fields of in's component which the tag
// props sets to method values, those values being in the fields now.
func (in *instance) trackMethods(props ui.Stateful) {
	ms := ui.MethodPropsOf(props)
	if len(ms) == 0 && len(in.methods) == 0 {
		return
	}
	last := ui.MethodPropsOf(in.comp)
	ui.SetMethodProps(in.comp, ms)
	v := reflect.ValueOf(in.comp).Elem()
	prev := in.methods
	in.methods = nil
	for _, m := range ms {
		var t *methodProp
		for _, o := range prev {
			if o.name == m.Field {
				t = o
			}
		}
		f := v.FieldByName(m.Field)
		if !f.IsValid() || f.Kind() != reflect.Func || !f.CanSet() || f.IsNil() {
			continue
		}
		if t == nil {
			t = in.newStandIn(m.Field, f)
		}
		if t.fn.Load() == nil || !sameMethod(last, ms, m.Field) {
			val := reflect.ValueOf(f.Interface())
			t.fn.Store(&val)
		}
		f.Set(t.wrap)
		in.methods = append(in.methods, t)
	}
}

func (in *instance) newStandIn(name string, f reflect.Value) *methodProp {
	t := &methodProp{name: name}
	variadic := f.Type().IsVariadic()
	s := in.s
	t.wrap = reflect.MakeFunc(f.Type(), func(args []reflect.Value) []reflect.Value {
		if s.inPass.Load() {
			in.called.Store(true)
		}
		fn := *t.fn.Load()
		if variadic {
			return fn.CallSlice(args)
		}
		return fn.Call(args)
	})
	return t
}

// boundMethod is the method value a stand-in calls when fn is one, what a
// transport names a handler by and reads its decorators from; else fn.
func (s *Session) boundMethod(fn any) any {
	if fn == nil {
		return nil
	}
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() || v.Pointer() != standIn {
		return fn
	}
	p := funcData(fn)
	for _, in := range s.byID {
		for _, t := range in.methods {
			if funcData(t.wrap.Interface()) == p {
				return t.fn.Load().Interface()
			}
		}
	}
	return fn
}

// funcData is the closure a func in an interface is: what tells stand-ins
// apart, a Value's Pointer being their shared code.
func funcData(fn any) unsafe.Pointer { return (*[2]unsafe.Pointer)(unsafe.Pointer(&fn))[1] }

// sameMethod reports whether the prop name is set to the same method of the
// same receiver in both.
func sameMethod(a, b []ui.MethodProp, name string) bool {
	var x, y *ui.MethodProp
	for i := range a {
		if a[i].Field == name {
			x = &a[i]
		}
	}
	for i := range b {
		if b[i].Field == name {
			y = &b[i]
		}
	}
	return x != nil && y != nil && x.Method == y.Method && sameReceiver(x.Recv, y.Recv)
}

// sameReceiver reports whether a method bound to a and one bound to b do the
// same: the same pointer of the same type, where it reads through at the call,
// or equal values by sameValue, whose pointers inside (what may have changed
// since being bound) count as different.
func sameReceiver(a, b any) bool {
	x, y := reflect.ValueOf(a), reflect.ValueOf(b)
	if !x.IsValid() || !y.IsValid() || x.Type() != y.Type() {
		return false
	}
	if x.Kind() == reflect.Pointer {
		return x.Pointer() == y.Pointer()
	}
	return sameValue(x, y)
}
