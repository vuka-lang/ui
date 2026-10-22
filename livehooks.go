package ui

import (
	"context"
	"reflect"

	"github.com/vuka-lang/vuka/v2/diag"
)

// A page's hooks are declared where they are written, with attributes on
// its methods — @ui.Mount, @ui.OnChange, @ui.Loads("lists") — which the
// compiler writes into the page's LiveSchema. A method is never a hook by
// its name: one named as earlier releases found hooks by (Mount, Changed)
// is refused unless it is declared one, so a 1.x hook can't become an
// event the browser calls.

// namedHooks are the methods earlier releases found a hook by, and the
// attribute that declares each.
var namedHooks = [...]struct{ method, attr string }{{"Mount", "@ui.Mount"}, {"Changed", "@ui.OnChange"}}

// namedHookErr is what is wrong with t's (a struct, or a pointer to one)
// methods named as earlier releases found hooks by: one its LiveSchema
// doesn't declare a hook.
func namedHookErr(t reflect.Type) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	pt := reflect.PointerTo(t)
	for _, h := range namedHooks {
		if _, ok := pt.MethodByName(h.method); ok && !isHook(pt, h.method) {
			return diag.New("E0337", "%s.%s isn't a hook: a hook is declared with %s, never found by its name", t.Name(), h.method, h.attr).
				WithHelp("mark the method %s (vuka fix hooks), or rename it", h.attr)
		}
	}
	return nil
}

// CheckHooks reports what is wrong with the hooks of t (a struct, or a
// pointer to one): its LiveSchema's mistakes (E0335, E0336), and a method
// named as earlier releases found a hook by that isn't declared one
// (E0337). Package web checks a stateless page with it at boot.
func CheckHooks(t reflect.Type) error {
	if err := schemaErr(t); err != nil {
		return err
	}
	return namedHookErr(t)
}

// MountPage checks p's hooks (CheckHooks), then runs the mount hook its
// LiveSchema declares, if any: what package web runs for a stateless page
// as MountLive does for a stateful component.
func MountPage(ctx context.Context, p any) error {
	t := reflect.TypeOf(p)
	if err := CheckHooks(t); err != nil {
		return err
	}
	if hasMount(t) {
		return liveHooks(reflect.ValueOf(p)).mount(ctx)
	}
	return nil
}
