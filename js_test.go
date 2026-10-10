package ui_test

import (
	"context"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui"
)

func encode(t *testing.T, ch ui.JSChain) string {
	t.Helper()
	b, err := ui.EncodeJS(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestJSEncoding(t *testing.T) {
	menu := ui.Ref("menu")
	noop := func() {}
	tests := []struct {
		ch   ui.JSChain
		want string
	}{
		{ui.JS, `[]`},
		{ui.JS.AddClass(menu, "a", "b"), `[["add_class",{"to":{"ref":"menu"},"names":["a","b"]}]]`},
		{ui.JS.RemoveClass(ui.This, "a"), `[["remove_class",{"names":["a"]}]]`},
		{ui.JS.ToggleClass("#x", "on"), `[["toggle_class",{"to":{"sel":"#x"},"names":["on"]}]]`},
		{ui.JS.SetAttr(ui.Closest(".card"), "aria-expanded", "true"), `[["set_attr",{"to":{"closest":".card"},"name":"aria-expanded","value":"true"}]]`},
		{ui.JS.SetAttr(ui.This, "title", ""), `[["set_attr",{"name":"title","value":""}]]`},
		{ui.JS.RemoveAttr(ui.Inner("li"), "hidden"), `[["remove_attr",{"to":{"inner":"li"},"name":"hidden"}]]`},
		{ui.JS.ToggleAttr(ui.Sel("#m"), "aria-hidden", "true", "false"), `[["toggle_attr",{"to":{"sel":"#m"},"name":"aria-hidden","values":["true","false"]}]]`},
		{ui.JS.Show(menu), `[["show",{"to":{"ref":"menu"}}]]`},
		{ui.JS.Show(menu, ui.Display("flex"), ui.Transition("ease-out duration-200", "opacity-0", "opacity-100"), ui.Time(300*time.Millisecond)),
			`[["show",{"to":{"ref":"menu"},"display":"flex","transition":["ease-out duration-200","opacity-0","opacity-100"],"time":300}]]`},
		{ui.JS.Hide(menu).Toggle(ui.This), `[["hide",{"to":{"ref":"menu"}}],["toggle",{}]]`},
		{ui.JS.Transition(menu, "a", "b", "c"), `[["transition",{"to":{"ref":"menu"},"transition":["a","b","c"]}]]`},
		{ui.JS.Focus(menu).FocusFirst(menu).PushFocus().PushFocus(menu).PopFocus(),
			`[["focus",{"to":{"ref":"menu"}}],["focus_first",{"to":{"ref":"menu"}}],["push_focus",{}],["push_focus",{"to":{"ref":"menu"}}],["pop_focus",{}]]`},
		{ui.JS.Dispatch(menu, "opened"), `[["dispatch",{"to":{"ref":"menu"},"event":"opened","bubbles":true}]]`},
		{ui.JS.Dispatch(ui.This, "x", ui.Detail(map[string]int{"n": 1}), ui.NoBubble()), `[["dispatch",{"event":"x","bubbles":false,"detail":{"n":1}}]]`},
		{ui.JS.Exec(menu, "data-cancel"), `[["exec",{"to":{"ref":"menu"},"attr":"data-cancel"}]]`},
		{ui.JS.Push(noop), `[["push",{}]]`}, // no session: no handler id
		{ui.JS.Patch("/books?page=2").On(ui.This).Navigate("/books/7"), `[["patch",{"href":"/books?page=2"}],["navigate",{"href":"/books/7"}]]`},
		{ui.JS.Debounce(300 * time.Millisecond).Throttle(time.Second), `[["debounce",{"ms":300}],["throttle",{"ms":1000}]]`},
		{ui.JS.Key("Escape", "Enter").Confirm(`Delete "x" <now>?`), `[["key",{"keys":["Escape","Enter"]}],["confirm",{"msg":"Delete \"x\" <now>?"}]]`},
		{ui.JS.If(menu.Checked()).Hide(menu).ElseIf(menu.Value().Eq("a")).Show(menu).Else().Toggle(menu).End(),
			`[["if",{"cond":{"to":{"ref":"menu"},"checked":true}}],["hide",{"to":{"ref":"menu"}}],["else_if",{"cond":{"to":{"ref":"menu"},"value_eq":"a"}}],["show",{"to":{"ref":"menu"}}],["else",{}],["toggle",{"to":{"ref":"menu"}}],["end",{}]]`},
		{ui.JS.If(menu.Value()).If(ui.This.Is(".open")).End().ElseIf(ui.Sel("#x").Exists()).ElseIf(menu.Attr("open")).End(),
			`[["if",{"cond":{"to":{"ref":"menu"},"value":true}}],["if",{"cond":{"is":".open"}}],["end",{}],["else_if",{"cond":{"to":{"sel":"#x"},"exists":true}}],["else_if",{"cond":{"to":{"ref":"menu"},"attr":"open"}}],["end",{}]]`},
		{ui.JS.Copy(menu).CopyText("hi").ScrollTo(menu).SetValue(ui.Ref("q"), ""),
			`[["copy",{"to":{"ref":"menu"}}],["copy_text",{"text":"hi"}],["scroll_to",{"to":{"ref":"menu"}}],["set_value",{"to":{"ref":"q"},"value":""}]]`},
		{ui.JS.On(menu).RemoveClass("hidden").AddClass("opacity-100").Focus().On("#x").Hide(),
			`[["remove_class",{"to":{"ref":"menu"},"names":["hidden"]}],["add_class",{"to":{"ref":"menu"},"names":["opacity-100"]}],["focus",{"to":{"ref":"menu"}}],["hide",{"to":{"sel":"#x"}}]]`},
		{ui.JS.On(menu).SetValue("v").Copy().ScrollTo().PushFocus().Show(ui.Display("grid")).Commands().PopFocus(),
			`[["set_value",{"to":{"ref":"menu"},"value":"v"}],["copy",{"to":{"ref":"menu"}}],["scroll_to",{"to":{"ref":"menu"}}],["push_focus",{"to":{"ref":"menu"}}],["show",{"to":{"ref":"menu"},"display":"grid"}],["pop_focus",{}]]`},
		{ui.JS.SetText(ui.Ref("likes"), "8").On(menu).SetText(`<b>"x"</b>`),
			`[["set_text",{"to":{"ref":"likes"},"text":"8"}],["set_text",{"to":{"ref":"menu"},"text":"<b>\"x\"</b>"}]]`},
		{ui.JS.Call("confetti"), `[["call",{"fn":"confetti"}]]`},
		{ui.JS.Call("Picker.refresh", 3, "a", map[string]bool{"x": true}, nil, menu.Value(), menu.Checked(), menu.Attr("data-id"), menu.Value().Eq("b")),
			`[["call",{"fn":"Picker.refresh","args":[3,"a",{"x":true},null,{"$read":{"to":{"ref":"menu"},"value":true}},{"$read":{"to":{"ref":"menu"},"checked":true}},{"$read":{"to":{"ref":"menu"},"attr":"data-id"}},{"$read":{"to":{"ref":"menu"},"value_eq":"b"}}]}]]`},
		{ui.JS.On(menu).Call("$.fx_1", ui.This.Value()), `[["call",{"fn":"$.fx_1","to":{"ref":"menu"},"args":[{"$read":{"value":true}}]}]]`},
		// Without a session a push has no handler id, nor hooks to settle.
		{ui.JS.Hide(menu).Push(noop, ui.Timeout(time.Second), ui.KeepOnError()).OnError(ui.JS.Show(menu)).OnSuccess(ui.JS.Focus(menu)),
			`[["hide",{"to":{"ref":"menu"}}],["push",{}]]`},
	}
	for _, tt := range tests {
		if got := encode(t, tt.ch); got != tt.want {
			t.Errorf("got  %s\nwant %s", got, tt.want)
		}
	}
}

func TestJSImmutable(t *testing.T) {
	base := ui.JS.AddClass("#a", "x")
	one := base.Hide("#b")
	two := base.Show("#c")
	if a, b, c := encode(t, base), encode(t, one), encode(t, two); a != `[["add_class",{"to":{"sel":"#a"},"names":["x"]}]]` ||
		!strings.HasSuffix(b, `["hide",{"to":{"sel":"#b"}}]]`) || !strings.HasSuffix(c, `["show",{"to":{"sel":"#c"}}]]`) {
		t.Fatalf("a chain changed by extending it:\n%s\n%s\n%s", a, b, c)
	}
	on := ui.JS.On("#m")
	on.Hide()
	if encode(t, on) != `[]` {
		t.Fatal("On changed")
	}
}

func TestJSBranchErrors(t *testing.T) {
	for name, f := range map[string]func(){
		"else without if": func() { ui.JS.Else() },
		"end without if":  func() { ui.JS.If(ui.This.Checked()).End().End() },
		"elseif after else": func() {
			ui.JS.If(ui.This.Checked()).Else().ElseIf(ui.This.Checked())
		},
		"two elses": func() { ui.JS.If(ui.This.Checked()).Else().Else() },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
	ui.JS.If(ui.This.Checked()).Else().If(ui.This.Checked()).Else().End().End() // nested: fine
}

func TestJSMisplaced(t *testing.T) {
	for name, f := range map[string]func(){
		"OnError without push":   func() { ui.JS.Hide("#x").OnError(ui.JS) },
		"OnSuccess without push": func() { ui.JS.OnSuccess(ui.JS) },
		"Timeout without push":   func() { ui.JS.On("#x").Hide().Timeout(time.Second) },
		"call of code":           func() { ui.JS.Call("alert(1)") },
		"call of nothing":        func() { ui.JS.Call("") },
		"call, trailing dot":     func() { ui.JS.On("#x").Call("a.") },
		"call, leading digit":    func() { ui.JS.Call("a.1b") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
	ui.JS.Push(func() {}).Hide("#x").OnError(ui.JS).Timeout(time.Second) // the last push, steps after it or not
}

func TestJSRender(t *testing.T) {
	a := func(name string, v any) ui.Attr { return ui.Attr{Name: name, Value: v} }
	n := ui.El("div", nil,
		ui.El("button", []ui.Attr{a("onClick", ui.On(ui.JS.RemoveClass(ui.Ref("menu"), "hidden").Push(func() {})))}, ui.Text("Open")),
		ui.El("ul", []ui.Attr{a("ref", "menu"), a("class", "hidden"), a("onKeyDown", ui.On(ui.JS.On(ui.This).Key("Escape").Hide()))}),
	)
	got := render(t, n)
	want := `<div><button data-vk-js-click="` + html.EscapeString(`[["remove_class",{"to":{"ref":"menu"},"names":["hidden"]}],["push",{}]]`) +
		`">Open</button><ul data-vk-ref="menu" class="hidden" data-vk-js-keydown="` + html.EscapeString(`[["key",{"keys":["Escape"]}],["hide",{}]]`) + `"></ul></div>`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	bad := ui.El("b", []ui.Attr{a("onClick", ui.On(ui.JS.Push(42)))})
	if _, err := ui.String(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "onClick: ui: JS Push takes a func()") {
		t.Fatalf("a bad Push handler: %v", err)
	}
}
