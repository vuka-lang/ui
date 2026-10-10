package templx

import (
	"sync"

	"github.com/vuka-lang/vuka"
)

// Component is a component of a .templ file: its name, the generated function
// (func(params…) templ.Component) and its parameters' names.
type Component struct {
	Name   string
	Func   any
	Params []string
}

var registry sync.Map // vuka.File → []Component

// Register records the components compiled from a .templ file. Code the vuka
// command generates for a vuka.File naming one calls it.
func Register(key string, components ...Component) { registry.Store(vuka.File(key), components) }

// Components are the components of an embedded .templ file, in source order:
// all of them for a file of the referencing package, the exported ones for a
// file of a subdirectory's package.
func Components(f vuka.File) ([]Component, bool) {
	c, ok := registry.Load(f)
	if !ok {
		return nil, false
	}
	return c.([]Component), true
}
