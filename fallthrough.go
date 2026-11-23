package ui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Fallthrough is what a caller writes on a component's tag that no prop
// claims (RFC components-seamless §5): its class, the HTML, ARIA and data
// attributes, the DOM events, a bind:value and a ref, in the order
// written. vuka gives a .vuka.html component that declares no Attrs a
// field of this type and passes it on to the component's forward element
// — its one top-level element, or the one marked forward:
//
//	<div class="rounded-xl border">{Children}</div>
//	<design.Card class="border-red-200 mt-4" id="billing">…</design.Card>
//
// renders <div class="rounded-xl border border-red-200 mt-4" id="billing">.
// The element's own class merges with the caller's (MergeClasses: the
// caller's utilities win), and an attribute the caller sets replaces the
// element's own of that name.
//
// From Go, a component's Fallthrough is set like any field:
// design.Card{Fallthrough: ui.Fallthrough{{Name: "id", Value: "billing"}}}.
type Fallthrough []Attr

// Class is the caller's classes, merged in the order they were written.
func (f Fallthrough) Class() string {
	var cs []string
	for _, a := range f {
		if isClassName(a.Name) {
			if s := classText(a.Value); s != "" {
				cs = append(cs, s)
			}
		}
	}
	switch len(cs) {
	case 0:
		return ""
	case 1:
		return cs[0]
	}
	return MergeClasses(cs...)
}

// Merge is the forward element's own class, written in pieces (the text
// and holes of class="card {x}"), merged with the caller's: the class
// attribute vuka writes on the forward element. A ui.Class or ui.Classes
// own class keeps its toggles, the caller's classes added on.
func (f Fallthrough) Merge(own ...any) any {
	caller := f.Class()
	if len(own) == 1 {
		switch v := own[0].(type) {
		case Classes:
			if caller == "" {
				return v
			}
			cp := maps.Clone(v)
			if cp == nil {
				cp = Classes{}
			}
			for c := range strings.FieldsSeq(caller) {
				cp[c] = true
			}
			return cp
		case Class:
			if caller == "" {
				return v
			}
		}
	}
	var b strings.Builder
	for _, p := range own {
		b.WriteString(classText(p))
	}
	if caller == "" {
		return b.String()
	}
	return MergeClasses(b.String(), caller)
}

// Attrs are the caller's attributes but its class: a bind:value's set in
// its place, and, of two of one name, the later.
func (f Fallthrough) Attrs() []Attr {
	if len(f) == 0 {
		return nil
	}
	out := make([]Attr, 0, len(f))
	add := func(a Attr) {
		if i := slices.IndexFunc(out, func(b Attr) bool { return strings.EqualFold(a.Name, b.Name) }); i >= 0 {
			out = slices.Delete(out, i, i+1)
		}
		out = append(out, a)
	}
	for _, a := range f {
		switch v := a.Value.(type) {
		case []Attr:
			for _, b := range v {
				add(b)
			}
			continue
		}
		if !isClassName(a.Name) {
			add(a)
		}
	}
	return out
}

// All is Attrs and, when the caller wrote one, its class: the attributes
// of a forward element with no class of its own.
func (f Fallthrough) All() []Attr {
	as := f.Attrs()
	if c := f.Class(); c != "" {
		as = append(as, Attr{Name: "class", Value: c})
	}
	return as
}

// Or is v, the forward element's own value of the attribute name, or nil —
// leaving it out — when the caller sets that attribute: the caller's wins.
func (f Fallthrough) Or(name string, v any) any {
	if f.Has(name) {
		return nil
	}
	return v
}

// Has reports whether the caller sets the attribute name (class, a
// bind:value's value among them) to something: an attribute it leaves out
// (name?={v} with v zero) isn't set.
func (f Fallthrough) Has(name string) bool {
	for _, a := range f {
		if a.Value == nil {
			continue // left out: name?={v} with v zero
		}
		if strings.EqualFold(a.Name, name) {
			return true
		}
		if set, ok := a.Value.([]Attr); ok && slices.ContainsFunc(set, func(b Attr) bool { return strings.EqualFold(b.Name, name) }) {
			return true
		}
	}
	return false
}

func isClassName(name string) bool { return name == "class" || name == "className" }

// classText is a class value as text: a string, a ui.Class's names that
// are on, or what it prints as.
func classText(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case Class:
		var on []string
		for c, ok := range v {
			if ok {
				on = append(on, c)
			}
		}
		slices.Sort(on)
		return strings.Join(on, " ")
	case fmt.Stringer:
		return v.String()
	}
	return fmt.Sprint(v)
}
