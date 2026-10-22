package ui_test

import (
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/uigen"
)

type rng struct{ Lo, Hi int }

type query struct {
	Text  string
	Range rng
	Tags  []string
}

func exprAttr(v any) ui.Node { return ui.HTML(`<p data-x={v}>{v}</p>`, v) }

// TestDerive checks State's expression methods against the bindings a .vuka
// file's compiler wrote for the same expressions: the same HTML, the same
// expression for the browser.
func TestDerive(t *testing.T) {
	open := uigen.State("o", false)
	tab := uigen.State("t", "logs")
	n := uigen.State("n", 2)
	x := uigen.State("x", 0.5)
	q := uigen.State("q", query{Text: "a", Range: rng{Hi: 5}, Tags: []string{"x"}})
	min := 1
	tests := []struct {
		name     string
		got, old any
	}{
		{"not", uigen.Not(open), ui.BindExpr(!open.Get(), `["!",{"$":0}]`, open)},
		{"and", uigen.And(uigen.Not(open), uigen.Eq(tab, "env")), ui.BindExpr(!open.Get() && tab.Get() == "env", `["&&",["!",{"$":0}],["==",{"$":1},"env"]]`, open, tab)},
		{"or value", uigen.Or(open, true), ui.BindExpr(true, `["||",{"$":0},true]`, open)},
		{"ne", uigen.Ne(tab, "logs"), ui.BindExpr(false, `["!=",{"$":0},"logs"]`, tab)},
		{"order", uigen.Or(uigen.And(uigen.Le(n, min), uigen.Ge(n, 0)), uigen.Gt(n, uigen.At[int](q, "Range.Hi"))),
			ui.BindExpr(n.Get() <= min && n.Get() >= 0 || n.Get() > q.Get().Range.Hi, `["||",["&&",["<=",{"$":0},{"$":1}],[">=",{"$":0},0]],[">",{"$":0},{"$":2,"p":["Range","Hi"]}]]`, n, min, q)},
		{"state operand", uigen.And(uigen.Ne(n, uigen.Minus(n, 1)), uigen.Gt(x, 0.25)), ui.BindExpr(true, `["&&",["!=",{"$":0},["-",{"$":0},1]],[">",{"$":1},0.25]]`, n, x)},
		{"plus", uigen.Plus(n, 1), ui.BindExpr(3, `["+",{"$":0},1]`, n)},
		{"float plus int", uigen.Plus(x, 1), ui.BindExpr(1.5, `["+",{"$":0},1]`, x)},
		{"neg", uigen.Neg(n), ui.BindExpr(-2, `["neg",{"$":0}]`, n)},
		{"concat", uigen.Plus(uigen.Plus(uigen.At[string](q, "Text"), " "), "items"), ui.BindExpr("a items", `["+",["+",{"$":0,"p":["Text"]}," "],"items"]`, q)},
		{"len", uigen.Gt(uigen.Len(uigen.At[[]string](q, "Tags")), min), ui.BindExpr(false, `[">",["len",{"$":0,"p":["Tags"]}],{"$":1}]`, q, min)},
		{"constant left", uigen.Plus(uigen.Derived("x"), tab), ui.BindExpr("xlogs", `["+","x",{"$":0}]`, tab)},
		{"field", uigen.Eq(uigen.At[int](q, "Range.Lo"), 0), ui.BindExpr(true, `["==",{"$":0,"p":["Range","Lo"]},0]`, q)},
	}
	for _, tt := range tests {
		got := pageHTML(t, exprAttr(tt.got))
		want := pageHTML(t, exprAttr(tt.old))
		if got != want {
			t.Errorf("%s:\ngot  %s\nwant %s", tt.name, got, want)
		}
	}

	cls := func(v any) ui.Node { return ui.HTML(`<div class={v}></div>`, v) }
	got := pageHTML(t, cls(ui.Classes{"menu": true, "is-open": open, "env": uigen.Eq(tab, "env")}))
	want := pageHTML(t, cls(ui.BindExpr(ui.Class{"menu": true, "is-open": false, "env": false}, `["class",{"env":["==",{"$":1},"env"],"is-open":{"$":0},"menu":true}]`, open, tab)))
	if got != want {
		t.Errorf("classes:\ngot  %s\nwant %s", got, want)
	}

	f := uigen.At[string](q, "Text")
	if got, want := encode(t, f.Set("b")), encode(t, ui.JS.Set(f, "b")); got != want {
		t.Errorf("At's Set: %s, want %s", got, want)
	}
}

func pageHTML(t *testing.T, n ui.Node) string {
	t.Helper()
	s, _ := page(t, n)
	return s
}

func TestDeriveRefuses(t *testing.T) {
	tab := uigen.State("t", "logs")
	q := uigen.State("q", query{})
	for name, f := range map[string]func(){
		"not a string":     func() { uigen.Not(tab) },
		"eq of a number":   func() { uigen.Eq(tab, 3) },
		"neg of a string":  func() { uigen.Neg(tab) },
		"no such field":    func() { uigen.At[string](q, "Nope") },
		"field of a type":  func() { uigen.At[int](q, "Text") },
		"classes of a str": func() { pageHTML(t, ui.HTML(`<i class={c}></i>`, ui.Classes{"a": "b"})) },
	} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.HasPrefix(r.(string), "ui: ") {
					t.Errorf("%s: recovered %v, want a ui: panic", name, r)
				}
			}()
			f()
		}()
	}
}
