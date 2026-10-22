// Package templx bridges templ: children for templ components, and the
// components of .templ files a vuka.File names. ui.Node is templ.Component,
// so components need no adapter either way; children do, because templ
// passes them on the context.
package templx

import "github.com/vuka-lang/ui/v2"

// WithChildren renders c with children as its templ children: ui.WithChildren.
func WithChildren(c ui.Node, children ui.Node) ui.Node { return ui.WithChildren(c, children) }
