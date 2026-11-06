package ui

import (
	"maps"
	"slices"
	"strings"

	"github.com/vuka-lang/vuka/v2/diag"
)

// Variants are a component's classes by its props, as cva writes them:
// Base always, and for each variant the classes of the value it is given,
// or its default.
//
//	var alert = ui.Variants{
//		Base: "flex items-start gap-2 rounded-lg border px-4 py-3 text-sm",
//		Variants: map[string]map[string]string{
//			"tone": {"error": "border-red-200 bg-red-50 text-red-800", "info": "border-border bg-gray-50"},
//		},
//		Defaults: map[string]string{"tone": "info"},
//	}
//
//	<div class={alert.Class("tone", Tone, "class", Class)}>
//
// Class merges them (MergeClasses): a variant's utility wins over Base's
// in its group, and the classes given as "class", the caller's, over both.
type Variants struct {
	Base     string
	Variants map[string]map[string]string
	Defaults map[string]string
}

// Class is the classes for the values given, name then value — "tone",
// Tone — with "class" naming classes added last, the caller's. A variant
// given no value, or "", takes its default, else adds nothing. A name
// that isn't a variant, or a value it doesn't have, fails the render
// (E0371).
func (v Variants) Class(pairs ...string) string {
	if len(pairs)%2 != 0 {
		panic("ui: Variants.Class takes names and values in pairs")
	}
	parts := make([]string, 1, 2+len(pairs)/2+len(v.Defaults))
	parts[0] = v.Base
	var extra []string
	for i := 0; i < len(pairs); i += 2 {
		name, value := pairs[i], pairs[i+1]
		if name == "class" {
			extra = append(extra, value)
			continue
		}
		if _, ok := v.Variants[name]; !ok {
			panic(mustFailure{v.unknownName(name)})
		}
		if value == "" {
			value = v.Defaults[name]
		}
		if value != "" {
			parts = append(parts, v.value(name, value))
		}
	}
	var defaults []string
	for name := range v.Defaults {
		if !named(pairs, name) {
			defaults = append(defaults, name)
		}
	}
	slices.Sort(defaults)
	for _, name := range defaults {
		parts = append(parts, v.value(name, v.Defaults[name]))
	}
	return MergeClasses(append(parts, extra...)...)
}

// named reports whether pairs give the variant name a value.
func named(pairs []string, name string) bool {
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i] == name {
			return true
		}
	}
	return false
}

// value is the classes of a variant's value.
func (v Variants) value(name, value string) string {
	classes, ok := v.Variants[name][value]
	if !ok {
		panic(mustFailure{v.unknownValue(name, value)})
	}
	return classes
}

// names are the variants' names, in order.
func (v Variants) names() []string { return slices.Sorted(maps.Keys(v.Variants)) }

func (v Variants) unknownName(name string) error {
	d := diag.New("E0371", "the variants have no %q: they are %s", name, quoteAll(v.names()))
	if s := closestName(name, v.names()); s != "" {
		return d.WithSuggest(`"` + s + `"`)
	}
	return d.WithHelp(`name one of them, or "class" for the caller's classes`)
}

func (v Variants) unknownValue(name, value string) error {
	values := slices.Sorted(maps.Keys(v.Variants[name]))
	d := diag.New("E0371", "the variant %s has no value %q: it is %s", name, value, quoteAll(values))
	if s := closestName(value, values); s != "" {
		return d.WithSuggest(`"` + s + `"`)
	}
	return d.WithHelp("give it one of its values, or add %q to Variants[%q]", value, name)
}

func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = `"` + s + `"`
	}
	if len(q) > 1 {
		return strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
	}
	return strings.Join(q, "")
}
