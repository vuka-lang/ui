// Package uigen is what the vuka compiler's output calls for client state
// and tracked server state: the operations a .vuka.html file's reads and
// writes of states lower to. It is not an API to write by hand — a person
// writes the .vuka.html form, and vukavet reports a hand-written use — but
// it is public so generated code, ejected code included, builds with plain
// go build. Its names follow the compiler, not the compatibility promise's
// documented API.
package uigen

import (
	"fmt"
	"hash/fnv"
	"runtime"
	"strconv"
	"sync"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/internal/bridge"
	"github.com/vuka-lang/ui/internal/track"
)

// Assign is an unexported field of a .vuka.html file's struct: server state
// a live session tracks. The compiler writes reads as Get and writes as Set
// or Update.
type Assign[T any] = track.Assign[T]

// UseState is a state declared where it is called: a component instance's
// own, one per call site and instance.
func UseState[T any](initial T) ui.State[T] { return declare(callSite(), false, initial) }

// UsePageState is a state the whole page shares, declared where it is
// called: a package-level state.
func UsePageState[T any](initial T) ui.State[T] { return declare(callSite(), true, initial) }

// State is a state declared at site, a name unique in the program: a
// component instance's own, one per site and instance.
func State[T any](site string, initial T) ui.State[T] { return declare(site, false, initial) }

// PageState is a state the whole page shares, declared at site.
func PageState[T any](site string, initial T) ui.State[T] { return declare(site, true, initial) }

func declare[T any](site string, page bool, initial T) ui.State[T] {
	var zero ui.State[T]
	return bridge.Declare(zero, site, page, initial).(ui.State[T])
}

// States gives each zero ui.State field of the struct p points to a state
// of its own: a static component's, before its View renders (a live one's
// get theirs before it mounts).
func States(p any) { bridge.States(p) }

// Derived is a state that never changes: the first operand of an
// expression starting with a value, "Hello, " + name.
func Derived[T any](v T) ui.State[T] {
	var zero ui.State[T]
	return bridge.Derived(zero, v).(ui.State[T])
}

// At is the field at path (Go field names, dotted) of a struct state: f.Q.
func At[T, S any](s ui.State[S], path string) ui.State[T] {
	var out ui.State[T]
	return bridge.At(s, out, path).(ui.State[T])
}

// Not is !s.
func Not[T any](s ui.State[T]) ui.State[bool] { return op[bool]("Not", s, nil) }

// And is s && o.
func And[T any](s ui.State[T], o any) ui.State[bool] { return op[bool]("And", s, o) }

// Or is s || o.
func Or[T any](s ui.State[T], o any) ui.State[bool] { return op[bool]("Or", s, o) }

// Eq is s == o.
func Eq[T any](s ui.State[T], o any) ui.State[bool] { return op[bool]("Eq", s, o) }

// Ne is s != o.
func Ne[T any](s ui.State[T], o any) ui.State[bool] { return op[bool]("Ne", s, o) }

// Lt is s < o.
func Lt[T any](s ui.State[T], o any) ui.State[bool] { return op[bool]("Lt", s, o) }

// Le is s <= o.
func Le[T any](s ui.State[T], o any) ui.State[bool] { return op[bool]("Le", s, o) }

// Gt is s > o.
func Gt[T any](s ui.State[T], o any) ui.State[bool] { return op[bool]("Gt", s, o) }

// Ge is s >= o.
func Ge[T any](s ui.State[T], o any) ui.State[bool] { return op[bool]("Ge", s, o) }

// Plus is s + o: numbers added, strings joined.
func Plus[T any](s ui.State[T], o any) ui.State[T] { return op[T]("Plus", s, o) }

// Minus is s - o.
func Minus[T any](s ui.State[T], o any) ui.State[T] { return op[T]("Minus", s, o) }

// Neg is -s.
func Neg[T any](s ui.State[T]) ui.State[T] { return op[T]("Neg", s, nil) }

// Len is len(s).
func Len[T any](s ui.State[T]) ui.State[int] { return op[int]("Len", s, nil) }

func op[R, T any](name string, s ui.State[T], o any) ui.State[R] {
	return bridge.Op(name, s, o).(ui.State[R])
}

// Toggle is c, then the command flipping s: an on… attribute's open = !open.
func Toggle(c ui.Commands, s ui.State[bool]) ui.Commands { return write("Toggle", c, s) }

// Inc is c, then the command adding one to s: n++.
func Inc[T number](c ui.Commands, s ui.State[T]) ui.Commands { return write("Inc", c, s) }

// Dec is c, then the command subtracting one from s: n--.
func Dec[T number](c ui.Commands, s ui.State[T]) ui.Commands { return write("Dec", c, s) }

type number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr | ~float32 | ~float64
}

func write(name string, c ui.Commands, s any) ui.Commands {
	return bridge.Write(name, c, s).(ui.Commands)
}

var callSites sync.Map // pc → site

// callSite is a name for the place UseState's caller calls it from.
func callSite() string {
	var pc [1]uintptr
	runtime.Callers(3, pc[:])
	if s, ok := callSites.Load(pc[0]); ok {
		return s.(string)
	}
	f, _ := runtime.CallersFrames(pc[:]).Next()
	h := fnv.New64a()
	fmt.Fprintf(h, "%s:%d:%d", f.Function, f.Line, f.PC-f.Entry)
	s, _ := callSites.LoadOrStore(pc[0], "s"+strconv.FormatUint(h.Sum64()&0xffffffffff, 36))
	return s.(string)
}
