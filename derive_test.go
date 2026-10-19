package ui_test

import (
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
)

type rng struct{ Lo, Hi int }

type query struct {
	Text  string
	Range rng
	Tags  []string
}

var exprAttr = ui.Template(`<p data-x="{{0}}">{{1}}</p>`)

// TestDerive checks State's expression methods against the bindings a .vuka
// file's compiler wrote for the same expressions: the same HTML, the same
// expression for the browser.
func TestDerive(t *testing.T) {
	open := ui.NewState("o", false)
	tab := ui.NewState("t", "logs")
	n := ui.NewState("n", 2)
	x := ui.NewState("x", 0.5)
	q := ui.NewState("q", query{Text: "a", Range: rng{Hi: 5}, Tags: []string{"x"}})
	min := 1
	tests := []struct {
		name     string
		got, old any
	}{
		{"not", open.Not(), ui.BindExpr(!open.Get(), `["!",{"$":0}]`, open)},
		{"and", open.Not().And(tab.Eq("env")), ui.BindExpr(!open.Get() && tab.Get() == "env", `["&&",["!",{"$":0}],["==",{"$":1},"env"]]`, open, tab)},
		{"or value", open.Or(true), ui.BindExpr(true, `["||",{"$":0},true]`, open)},
		{"ne", tab.Ne("logs"), ui.BindExpr(false, `["!=",{"$":0},"logs"]`, tab)},
		{"order", n.Le(min).And(n.Ge(0)).Or(n.Gt(ui.At[int](q, "Range.Hi"))),
			ui.BindExpr(n.Get() <= min && n.Get() >= 0 || n.Get() > q.Get().Range.Hi, `["||",["&&",["<=",{"$":0},{"$":1}],[">=",{"$":0},0]],[">",{"$":0},{"$":2,"p":["Range","Hi"]}]]`, n, min, q)},
		{"state operand", n.Ne(n.Minus(1)).And(x.Gt(0.25)), ui.BindExpr(true, `["&&",["!=",{"$":0},["-",{"$":0},1]],[">",{"$":1},0.25]]`, n, x)},
		{"plus", n.Plus(1), ui.BindExpr(3, `["+",{"$":0},1]`, n)},
		{"float plus int", x.Plus(1), ui.BindExpr(1.5, `["+",{"$":0},1]`, x)},
		{"neg", n.Neg(), ui.BindExpr(-2, `["neg",{"$":0}]`, n)},
		{"concat", ui.At[string](q, "Text").Plus(" ").Plus("items"), ui.BindExpr("a items", `["+",["+",{"$":0,"p":["Text"]}," "],"items"]`, q)},
		{"len", ui.At[[]string](q, "Tags").Len().Gt(min), ui.BindExpr(false, `[">",["len",{"$":0,"p":["Tags"]}],{"$":1}]`, q, min)},
		{"constant left", ui.Derived("x").Plus(tab), ui.BindExpr("xlogs", `["+","x",{"$":0}]`, tab)},
		{"field", ui.At[int](q, "Range.Lo").Eq(0), ui.BindExpr(true, `["==",{"$":0,"p":["Range","Lo"]},0]`, q)},
	}
	for _, tt := range tests {
		got := pageHTML(t, exprAttr.Render(tt.got, tt.got))
		want := pageHTML(t, exprAttr.Render(tt.old, tt.old))
		if got != want {
			t.Errorf("%s:\ngot  %s\nwant %s", tt.name, got, want)
		}
	}

	cls := ui.Template(`<div class="{{0}}"></div>`)
	got := pageHTML(t, cls.Render(ui.Classes{"menu": true, "is-open": open, "env": tab.Eq("env")}))
	want := pageHTML(t, cls.Render(ui.BindExpr(ui.Class{"menu": true, "is-open": false, "env": false}, `["class",{"env":["==",{"$":1},"env"],"is-open":{"$":0},"menu":true}]`, open, tab)))
	if got != want {
		t.Errorf("classes:\ngot  %s\nwant %s", got, want)
	}

	f := ui.At[string](q, "Text")
	if got, want := encode(t, f.Set("b")), encode(t, ui.StateField(q, "Text", q.Get().Text).Set("b")); got != want {
		t.Errorf("At's Set: %s, want %s", got, want)
	}
}

func pageHTML(t *testing.T, n ui.Node) string {
	t.Helper()
	s, _ := page(t, n)
	return s
}

func TestDeriveRefuses(t *testing.T) {
	tab := ui.NewState("t", "logs")
	q := ui.NewState("q", query{})
	for name, f := range map[string]func(){
		"not a string":     func() { tab.Not() },
		"eq of a number":   func() { tab.Eq(3) },
		"neg of a string":  func() { tab.Neg() },
		"no such field":    func() { ui.At[string](q, "Nope") },
		"field of a type":  func() { ui.At[int](q, "Text") },
		"classes of a str": func() { pageHTML(t, ui.Template(`<i class="{{0}}"></i>`).Render(ui.Classes{"a": "b"})) },
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
