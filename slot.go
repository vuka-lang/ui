package ui

import (
	"reflect"
	"strings"
)

// Has reports whether a slot was filled: n is a Node with something in it.
// It is false for nil (a slot the caller left out), a nil pointer or func,
// and text, raw HTML or a fragment holding only white space; anything else —
// an element, a template, a component — counts as content without rendering
// it.
//
//	if ui.Has(Actions) {
//	  <footer>{Actions}</footer>
//	}
func Has(n Node) bool {
	switch v := n.(type) {
	case nil:
		return false
	case TextNode:
		return strings.TrimSpace(string(v)) != ""
	case RawHTML:
		return strings.TrimSpace(string(v)) != ""
	case Group:
		return anyHas(v)
	case *list:
		return v != nil && (v.items != nil || anyHas(v.nodes))
	case keyedNode:
		return Has(v.n)
	case TryNode:
		return v.Err != nil || Has(v.Node)
	}
	switch rv := reflect.ValueOf(n); rv.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Interface:
		return !rv.IsNil()
	}
	return true
}

func anyHas(ns []Node) bool {
	for _, n := range ns {
		if Has(n) {
			return true
		}
	}
	return false
}

// SlotOr is a slot with fallback content: n when it was filled (Has), else
// fallback. A component's markup writes it as the slot's tag holding the
// fallback, <slot:actions><button>Close</button></slot:actions>.
func SlotOr(n, fallback Node) Node {
	if Has(n) {
		return n
	}
	return fallback
}
