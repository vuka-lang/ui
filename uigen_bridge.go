package ui

import (
	"fmt"
	"hash/fnv"
	"reflect"
	"strconv"
	"sync"

	"github.com/vuka-lang/ui/v2/internal/bridge"
)

// stateOps is what package uigen calls on a State through the bridge: its
// unexported operations, for any T.
type stateOps interface {
	stateOf() stateView
	declare(site string, page bool, init any) any
	derivedOf(v any) any
	fieldOf(sv stateView, path string) any
	op(name string, o any) any
	write(name string, c Commands) Commands
}

func (State[T]) declare(site string, page bool, init any) any {
	v, _ := init.(T)
	if page {
		return pageState(site, v)
	}
	return newState(site, v)
}

func (State[T]) derivedOf(v any) any {
	x, _ := v.(T)
	return derived(x)
}

func (s State[T]) op(name string, o any) any {
	switch name {
	case "Not":
		return s.not()
	case "And":
		return s.and(o)
	case "Or":
		return s.or(o)
	case "Eq":
		return s.eq(o)
	case "Ne":
		return s.ne(o)
	case "Lt":
		return s.lt(o)
	case "Le":
		return s.le(o)
	case "Gt":
		return s.gt(o)
	case "Ge":
		return s.ge(o)
	case "Plus":
		return s.plus(o)
	case "Minus":
		return s.minus(o)
	case "Neg":
		return s.neg()
	case "Len":
		return s.length()
	}
	panic("ui: no state operation " + name)
}

func (s State[T]) write(name string, c Commands) Commands {
	switch name {
	case "Toggle":
		return s.toggle(c)
	case "Inc":
		return s.incBy(c, "Inc", 1)
	case "Dec":
		return s.incBy(c, "Dec", -1)
	}
	panic("ui: no state write " + name)
}

func init() {
	bridge.Declare = func(zero any, site string, page bool, init any) any {
		return zero.(stateOps).declare(site, page, init)
	}
	bridge.Derived = func(zero, v any) any { return zero.(stateOps).derivedOf(v) }
	bridge.At = func(s, out any, path string) any { return out.(stateOps).fieldOf(s.(stateOps).stateOf(), path) }
	bridge.Op = func(name string, s, o any) any { return s.(stateOps).op(name, o) }
	bridge.Write = func(name string, c, s any) any { return s.(stateOps).write(name, c.(Commands)) }
	bridge.States = func(p any) { initStates(reflect.ValueOf(p)) }
}

// fieldStates is the indexes of a struct type's ui.State fields.
var fieldStates sync.Map // reflect.Type → []int

// initStates gives each zero ui.State field of the struct p points to a
// state of its own, its initial value its type's zero: a .vuka.html file
// declares a state as a field (open ui.State[bool]). Its site names the
// type and the field, so the browser tells the fields of one instance
// apart, and an instance's from another's (StateHost keys it).
func initStates(p reflect.Value) {
	if p.Kind() != reflect.Pointer || p.IsNil() || p.Elem().Kind() != reflect.Struct {
		return
	}
	v := p.Elem()
	t := v.Type()
	idx, ok := fieldStates.Load(t)
	if !ok {
		var out []int
		ops := reflect.TypeFor[stateOps]()
		for i := 0; i < t.NumField(); i++ {
			if f := t.Field(i); !f.Anonymous && f.Type.Implements(ops) {
				out = append(out, i)
			}
		}
		idx, _ = fieldStates.LoadOrStore(t, out)
	}
	for _, i := range idx.([]int) {
		f := v.Field(i)
		s := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
		if s.Interface().(stateOps).stateOf().c != nil {
			continue
		}
		h := fnv.New64a()
		fmt.Fprintf(h, "%s.%s.%s", t.PkgPath(), t.Name(), t.Field(i).Name)
		site := "f" + strconv.FormatUint(h.Sum64()&0xffffffffff, 36)
		s.Set(reflect.ValueOf(reflect.Zero(f.Type()).Interface().(stateOps).declare(site, false, nil)))
	}
}
