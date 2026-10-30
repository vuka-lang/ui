package ui

import (
	"context"
	"html"
	"reflect"
	"strings"

	"github.com/a-h/templ"
)

// TemplAttrs is the templ.Attributes a component whose props have an
// Attributes field (templUI's, shadcn-templ's) takes from its tag: base, the
// tag's attributes={…}, with attrs, the tag's attributes that aren't fields
// (aria-label, data-x, role, onClick). Each renders as it would on an
// element in ctx: an on… handler as the live event it binds
// (data-vk-on-click), JS commands as data-vk-js-<event>, a class list as a
// string. An on… entry of base holding a handler binds the same way. The
// generated code of a tag calls it; base is not modified.
func TemplAttrs(ctx context.Context, base templ.Attributes, attrs ...Attr) templ.Attributes {
	out := make(templ.Attributes, len(base)+len(attrs))
	for k, v := range base {
		if isEventName(k) && handlerValue(v) {
			attrs = append(attrs, Attr{Name: k, Value: v})
			continue
		}
		out[k] = v
	}
	if len(attrs) == 0 {
		return out
	}
	var b strings.Builder
	h := HTMLRenderer{ctx: ctx, w: &b}
	for _, a := range attrs {
		if err := h.attrNamed("", nil, a); err != nil {
			panic(mustFailure{err}) // the render fails with it, as with Must
		}
	}
	parseAttrs(b.String(), out)
	return out
}

// handlerValue reports whether v is what an on… attribute binds: a
// function, an EventHandler or JS commands.
func handlerValue(v any) bool {
	switch v.(type) {
	case nil, templ.ComponentScript:
		return false
	case EventHandler, JSChain:
		return true
	}
	return reflect.TypeOf(v).Kind() == reflect.Func
}

// parseAttrs reads attributes as HTMLRenderer writes them — ` name` or
// ` name="escaped"` — into out.
func parseAttrs(s string, out templ.Attributes) {
	for len(s) > 0 {
		s = strings.TrimLeft(s, " ")
		i := strings.IndexAny(s, "= ")
		if i < 0 {
			if s != "" {
				out[s] = true
			}
			return
		}
		name := s[:i]
		if s[i] == ' ' {
			out[name] = true
			s = s[i:]
			continue
		}
		rest := s[i+2:] // past ="
		j := strings.IndexByte(rest, '"')
		if j < 0 {
			return
		}
		out[name] = html.UnescapeString(rest[:j])
		s = rest[j+1:]
	}
}
