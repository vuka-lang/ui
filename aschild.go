package ui

import (
	"context"
	"fmt"
	"io"
	"maps"
	"reflect"
	"strings"
)

// asChild (RFC components-seamless §5) runs fallthrough in reverse: a bare
// asChild on a component's tag makes the tag's one element child the
// component's root. The component renders as written; its forward element
// — the one its Fallthrough spreads on — is replaced by the child, which
// takes the forward element's attributes:
//
//	<design.Button asChild variant="outline"><a href="/reports/42">Report</a></design.Button>
//
// renders the button's classes, type and handlers on the <a>. vuka writes
// the tag as AsChild(Button{…, Fallthrough: Fallthrough{…, AsChildAttr(<a>)}}).
//
// The child wins: its class merges last (MergeClasses), an attribute it
// sets is its own, and of two handlers for one event both run, the
// child's first. The forward element's content is the child's.

// asChildName is the attribute the child rides in a Fallthrough: never
// written, AsChild takes it out.
const asChildName = "data-vk-aschild"

// asChildSlot is the child a Fallthrough carries.
type asChildSlot struct{ child Node }

// AsChildAttr is the entry of a component's Fallthrough carrying the
// element its forward element is to be: what vuka writes for a tag's
// asChild.
func AsChildAttr(child Node) Attr { return Attr{Name: asChildName, Value: asChildSlot{child}} }

// AsChild is the component c with its forward element replaced by the
// element child its Fallthrough carries (AsChildAttr). It fails to render
// when c renders no element carrying it.
func AsChild(c Node) Node { return asChildNode{c} }

type asChildNode struct{ n Node }

func (n asChildNode) Render(ctx context.Context, w io.Writer) error { return renderHTML(ctx, w, n) }

// resolve is the component's tree with the child in its forward element's
// place.
func (n asChildNode) resolve(ctx context.Context) (Node, error) {
	out, found, err := donateIn(ctx, unview(n.n))
	switch {
	case err != nil:
		return nil, err
	case !found:
		return nil, fmt.Errorf("ui: asChild: %T renders no forward element to give its attributes to the child", n.n)
	}
	return out, nil
}

// donateIn finds the element carrying an asChild slot in n and replaces it
// with the slot's child, which takes its attributes. The frames on the way
// are flattened: their shape is the forward element's.
func donateIn(ctx context.Context, n Node) (Node, bool, error) {
	switch t := n.(type) {
	case *Element:
		if t == nil {
			return n, false, nil
		}
		if slot, ok := asChildOf(t.Attrs); ok {
			el, err := donate(ctx, t, slot.child)
			return el, err == nil, err
		}
		kids, found, err := donateKids(ctx, t.Children)
		if !found || err != nil {
			return n, found, err
		}
		cp := *t
		cp.Children = kids
		return &cp, true, nil
	case *Frame:
		if t == nil {
			return n, false, nil
		}
		kids, found, err := donateKids(ctx, t.Kids)
		if !found || err != nil {
			return n, found, err
		}
		return Group(kids), true, nil
	case *lazyFrame:
		if t == nil {
			return n, false, nil
		}
		return donateIn(ctx, t.frame())
	case Group:
		kids, found, err := donateKids(ctx, t)
		if !found || err != nil {
			return n, found, err
		}
		return Group(kids), true, nil
	case *list:
		if t == nil || t.items != nil {
			return n, false, nil
		}
		kids, found, err := donateKids(ctx, t.nodes)
		if !found || err != nil {
			return n, found, err
		}
		cp := *t
		cp.nodes = kids
		return &cp, true, nil
	case ctxNode:
		if t == nil {
			return n, false, nil
		}
		return donateIn(ctx, ResolveContext(ctx, t))
	case keyedNode:
		inner, found, err := donateIn(t.in(ctx), t.n)
		if !found || err != nil {
			return n, found, err
		}
		t.n = inner
		return t, true, nil
	case TryNode:
		if t.Err != nil {
			return n, false, t.Err
		}
		return donateIn(ctx, t.Node)
	}
	return n, false, nil
}

func donateKids(ctx context.Context, kids []Node) ([]Node, bool, error) {
	for i, k := range kids {
		d, found, err := donateIn(ctx, k)
		if err != nil {
			return nil, false, err
		}
		if found {
			out := append([]Node(nil), kids...)
			out[i] = d
			return out, true, nil
		}
	}
	return kids, false, nil
}

// asChildOf is the asChild slot among attrs, a spread's included.
func asChildOf(attrs []Attr) (asChildSlot, bool) {
	for _, a := range attrs {
		switch v := a.Value.(type) {
		case asChildSlot:
			return v, true
		case attrSet:
			if s, ok := asChildOf(v); ok {
				return s, true
			}
		case []Attr:
			if s, ok := asChildOf(v); ok {
				return s, true
			}
		}
	}
	return asChildSlot{}, false
}

// childElement is the one element child renders.
func childElement(ctx context.Context, n Node) (*Element, bool) {
	switch t := unview(n).(type) {
	case *Element:
		return t, t != nil
	case *lazyFrame:
		if t == nil {
			return nil, false
		}
		return childElement(ctx, t.frame())
	case *Frame:
		if t == nil {
			return nil, false
		}
		return childElement(ctx, Group(t.Kids))
	case Group:
		var el *Element
		for _, k := range t {
			if s, ok := k.(TextNode); ok && strings.TrimSpace(string(s)) == "" {
				continue
			}
			e, ok := childElement(ctx, k)
			if !ok || el != nil {
				return nil, false
			}
			el = e
		}
		return el, el != nil
	case ctxNode:
		if t == nil {
			return nil, false
		}
		return childElement(ctx, ResolveContext(ctx, t))
	}
	return nil, false
}

// donate is child's element taking the forward element fwd's attributes:
// its own class merged last, its own attributes kept, two handlers of one
// event composed, the child's first.
func donate(ctx context.Context, fwd *Element, child Node) (*Element, error) {
	el, ok := childElement(ctx, child)
	if !ok {
		return nil, fmt.Errorf("ui: asChild: the child is %T, not one element", child)
	}
	own := flatAttrs(el.Attrs)
	out := make([]Attr, 0, len(own)+len(fwd.Attrs))
	out = append(out, own...)
	for _, d := range flatAttrs(fwd.Attrs) {
		if _, slot := d.Value.(asChildSlot); slot || d.Value == nil {
			continue
		}
		i := -1
		for j, a := range out {
			if strings.EqualFold(a.Name, d.Name) || isClassName(a.Name) && isClassName(d.Name) {
				i = j
				break
			}
		}
		switch {
		case i < 0:
			out = append(out, d)
		case isClassName(d.Name):
			out[i].Value = mergeClassValues(d.Value, out[i].Value)
		case isEventName(d.Name):
			out[i].Value = composeHandlers(out[i].Value, d.Value)
		}
	}
	return &Element{Tag: el.Tag, Attrs: out, Children: el.Children}, nil
}

// flatAttrs is attrs with its spreads' attributes in their place.
func flatAttrs(attrs []Attr) []Attr {
	out := make([]Attr, 0, len(attrs))
	for _, a := range attrs {
		switch v := a.Value.(type) {
		case attrSet:
			out = append(out, flatAttrs(v)...)
		case []Attr:
			out = append(out, flatAttrs(v)...)
		default:
			out = append(out, a)
		}
	}
	return out
}

// mergeClassValues is the forward element's class and the child's, the
// child's utilities winning: text merged, toggles kept.
func mergeClassValues(fwd, child any) any {
	_, fs := fwd.(Classes)
	_, cs := child.(Classes)
	if !fs && !cs {
		return MergeClasses(classText(fwd), classText(child))
	}
	out := Classes{}
	for _, v := range []any{fwd, child} {
		switch v := v.(type) {
		case Classes:
			maps.Copy(out, v)
		case Class:
			for k, on := range v {
				out[k] = on
			}
		default:
			for c := range strings.FieldsSeq(classText(v)) {
				out[c] = true
			}
		}
	}
	return out
}

// composeHandlers is a handler running first, then second: browser
// commands chained, a server handler pushed.
func composeHandlers(first, second any) any {
	chain := func(h any) (Commands, bool) {
		if e, ok := h.(EventHandler); ok {
			h = e.Fn
		}
		switch v := h.(type) {
		case Commands:
			return v, true
		case JSChain:
			return v.jsChain(), true
		}
		if h != nil && reflect.TypeOf(h).Kind() == reflect.Func {
			return JS.Push(h), true
		}
		return Commands{}, false
	}
	a, okA := chain(first)
	b, okB := chain(second)
	if !okA || !okB {
		return first // a script handler can't be chained: the child's stands
	}
	return a.Then(b)
}
