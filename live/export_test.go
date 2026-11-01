package live

import (
	"reflect"
	"runtime"
)

// SetWholeTrees makes new sessions render every instance without its
// frames, as components without render tracking render.
func SetWholeTrees(on bool) { wholeTrees = on }

// SameProps is sameProps, for its tests.
var SameProps = sameProps

// FuncName is the name of a func's code.
func FuncName(fn any) string { return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name() }
