package ui_test

import (
	"context"
	"html"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/uigen"
)

type filters struct {
	Q    string `json:"q"`
	Sort string
	Page int
}

// page renders n as RenderPage does a page, its states keyed as the page's.
func page(t *testing.T, n ui.Node) (string, ui.PageInfo) {
	t.Helper()
	var b strings.Builder
	info, err := ui.RenderPage(context.Background(), n, &b)
	if err != nil {
		t.Fatal(err)
	}
	return b.String(), info
}

func TestStateCommands(t *testing.T) {
	open := uigen.State("s1", false)
	tab := uigen.State("s2", "logs")
	n := uigen.State("s3", 2)
	f := uigen.State("s4", filters{Sort: "name"})
	q := uigen.At[string](f, "Q")
	g := uigen.PageState("s5", 1.5)
	save := func() {}
	tests := []struct {
		ch   ui.JSChain
		want string
	}{
		{uigen.Toggle(ui.JS, open), `[["state_toggle",{"s":"s1:1","i":false}]]`},
		{tab.Set("env"), `[["state_set",{"s":"s2:1","i":"logs","v":"env"}]]`},
		{uigen.Inc(ui.JS, n), `[["state_inc",{"s":"s3:1","i":2,"by":1}]]`},
		{uigen.Dec(ui.JS, n), `[["state_inc",{"s":"s3:1","i":2,"by":-1}]]`},
		{ui.JS.Set(n, n.Get()), `[["state_set",{"s":"s3:1","i":2,"v":2}]]`},
		{tab.Set("env").Set(open, false).Push(save), `[["state_set",{"s":"s2:1","i":"logs","v":"env"}],["state_set",{"s":"s1:1","i":false,"v":false}],["push",{}]]`},
		{f.Set(filters{Q: "x", Sort: "age"}), `[["state_set",{"s":"s4:1","i":{"q":"","Sort":"name","Page":0},"v":{"q":"x","Sort":"age","Page":0}}]]`},
		{q.Set("dog"), `[["state_set",{"s":"s4:1","i":{"q":"","Sort":"name","Page":0},"p":["q"],"v":"dog"}]]`},
		{ui.JS.Set(f, f.Get()), `[["state_set",{"s":"s4:1","i":{"q":"","Sort":"name","Page":0},"v":{"q":"","Sort":"name","Page":0}}]]`},
		{ui.JS.Set(g, 3), `[["state_set",{"s":"g:s5","i":1.5,"v":3}]]`}, // a number converts
		{ui.JS.Show(ui.Ref("m")).Set(n, 7).On(ui.This).Set(open, true).AddClass("x"),
			`[["show",{"to":{"ref":"m"}}],["state_set",{"s":"s3:1","i":2,"v":7}],["state_set",{"s":"s1:1","i":false,"v":true}],["add_class",{"names":["x"]}]]`},
	}
	for _, tt := range tests {
		got := renumber(encode(t, tt.ch))
		if got != tt.want {
			t.Errorf("got  %s\nwant %s", got, tt.want)
		}
	}
}

// renumber writes the static keys s<n>:<counter> as s<n>:1: the counter is
// the process's.
func renumber(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, `"s":"s`)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := i + len(`"s":"s`)
		b.WriteString(s[:j])
		s = s[j:]
		k := strings.IndexByte(s, ':')
		e := strings.IndexByte(s, '"')
		b.WriteString(s[:k] + ":1")
		s = s[e:]
	}
}

func TestStateCommandPanics(t *testing.T) {
	tab := uigen.State("s", "a")
	d := uigen.Derived(true)
	for name, f := range map[string]func(){
		"set another type": func() { ui.JS.Set(tab, 3) },
		"set a derived":    func() { ui.JS.Set(d, false) },
		"zero state":       func() { var z ui.State[bool]; uigen.Toggle(ui.JS, z) },
		"nil for a string": func() { ui.JS.Set(tab, nil) },
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
}

func TestBindings(t *testing.T) {
	open := uigen.State("open", false)
	tab := uigen.State("tab", "env")
	n := uigen.State("n", 3)
	f := uigen.State("f", filters{Q: "go"})
	ready := uigen.And(uigen.Not(open), uigen.Eq(tab, "env"))
	out, info := page(t, ui.El("div", nil,
		ui.El("button", []ui.Attr{
			{Name: "aria-expanded", Value: ui.BindExpr(open.Get(), `{"$":0}`, open)},
			{Name: "disabled", Value: ui.BindExpr(f.Get().Q == "" || tab.Get() != "logs", `["||",["==",{"$":0,"p":["Q"]},""],["!=",{"$":1},"logs"]]`, f, tab)},
			{Name: "onClick", Value: ui.On(uigen.Toggle(ui.JS, open))},
		}, ui.Text("Menu")),
		ui.El("ul", []ui.Attr{{Name: "hidden", Value: ui.BindExpr(!open.Get(), `["!",{"$":0}]`, open)}}),
		ui.El("p", []ui.Attr{{Name: "class", Value: ui.BindExpr(ui.Class{"tab": true, "bg-brand": tab.Get() == "env"}, `["class",{"tab":true,"bg-brand":["==",{"$":0},"env"]}]`, tab)}},
			ui.Child(ui.BindExpr(n.Get(), `{"$":0}`, n)), ui.Text(" of "), ui.Child(ui.BindExpr(len(tab.Get()), `["len",{"$":0}]`, tab))),
		ui.El("i", []ui.Attr{{Name: "data-ready", Value: ui.BindExpr(ready.Get(), `{"$":0}`, ready)}}),
		ui.El("b", []ui.Attr{{Name: "title", Value: ui.BindExpr(tab.Get()+"!", `["+",{"$":0},{"$":1}]`, tab, map[string]int{"x": 1})}}),
	))
	if !info.JS {
		t.Error("a page with bindings isn't marked JS")
	}
	for _, want := range []string{
		`aria-expanded="false" data-vk-b-aria-expanded="{&#34;i&#34;:false,&#34;s&#34;:&#34;c0:open:1&#34;}"`,
		`disabled data-vk-b-disabled="[&#34;||&#34;,[&#34;==&#34;,{&#34;i&#34;:{&#34;q&#34;:&#34;go&#34;,&#34;Sort&#34;:&#34;&#34;,&#34;Page&#34;:0},&#34;p&#34;:[&#34;q&#34;],&#34;s&#34;:&#34;c0:f:1&#34;},&#34;&#34;]`,
		`data-vk-js-click="[[&#34;state_toggle&#34;,{&#34;s&#34;:&#34;c0:open:1&#34;,&#34;i&#34;:false}]]"`,
		`<ul hidden data-vk-b-hidden=`,
		`<p class="bg-brand tab" data-vk-b-class="[&#34;class&#34;,{&#34;bg-brand&#34;:[&#34;==&#34;,{&#34;i&#34;:&#34;env&#34;,&#34;s&#34;:&#34;c0:tab:1&#34;},&#34;env&#34;],&#34;tab&#34;:true}]">`,
		`<vk-t data-vk-b="{&#34;i&#34;:3,&#34;s&#34;:&#34;c0:n:1&#34;}">3</vk-t> of <vk-t data-vk-b="[&#34;len&#34;,{&#34;i&#34;:&#34;env&#34;,&#34;s&#34;:&#34;c0:tab:1&#34;}]">3</vk-t>`,
		// a derived state is its expression
		`data-ready data-vk-b-data-ready="[&#34;&amp;&amp;&#34;,[&#34;!&#34;,{&#34;i&#34;:false,&#34;s&#34;:&#34;c0:open:1&#34;}],[&#34;==&#34;,{&#34;i&#34;:&#34;env&#34;,&#34;s&#34;:&#34;c0:tab:1&#34;},&#34;env&#34;]]"`,
		// a server's value that isn't a scalar is {"v": …}
		`title="env!" data-vk-b-title="[&#34;+&#34;,{&#34;i&#34;:&#34;env&#34;,&#34;s&#34;:&#34;c0:tab:1&#34;},{&#34;v&#34;:{&#34;x&#34;:1}}]"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", html.UnescapeString(want), html.UnescapeString(out))
		}
	}
}

func TestBindAttr(t *testing.T) {
	open := uigen.State("open", false)
	tab := uigen.State("tab", "env")
	busy := uigen.And(uigen.Not(open), uigen.Eq(tab, "env"))
	out, _ := page(t, ui.El("button", []ui.Attr{
		ui.BindAttr("aria-expanded", open),
		ui.BindAttr("hidden", open),
		ui.BindAttr("title", tab),
		ui.BindAttr("data-busy", busy),
	}))
	for _, want := range []string{
		`aria-expanded="false" data-vk-b-aria-expanded="{&#34;i&#34;:false,&#34;s&#34;:&#34;c0:open:1&#34;}"`,
		`data-vk-b-hidden="{&#34;i&#34;:false,&#34;s&#34;:&#34;c0:open:1&#34;}"`,
		`title="env" data-vk-b-title="{&#34;i&#34;:&#34;env&#34;,&#34;s&#34;:&#34;c0:tab:1&#34;}"`,
		`data-busy data-vk-b-data-busy="[&#34;&amp;&amp;&#34;,[&#34;!&#34;,{&#34;i&#34;:false,&#34;s&#34;:&#34;c0:open:1&#34;}],[&#34;==&#34;,{&#34;i&#34;:&#34;env&#34;,&#34;s&#34;:&#34;c0:tab:1&#34;},&#34;env&#34;]]"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", html.UnescapeString(want), html.UnescapeString(out))
		}
	}
	if strings.Contains(out, " hidden ") || strings.Contains(out, "<button hidden") {
		t.Errorf("a false state rendered hidden: %s", out)
	}
}

func TestBindingRefusals(t *testing.T) {
	open := uigen.State("o", false)
	for _, name := range []string{"onclick", "srcdoc", "ref", "key"} {
		if _, err := ui.String(context.Background(), ui.El("div", []ui.Attr{{Name: name, Value: ui.BindExpr(true, `{"$":0}`, open)}})); err == nil {
			t.Errorf("%s bound", name)
		}
	}
	var zero ui.State[bool]
	if _, err := ui.String(context.Background(), ui.El("div", []ui.Attr{{Name: "hidden", Value: ui.BindExpr(true, `{"$":0}`, zero)}})); err == nil {
		t.Error("a zero state bound")
	}
}

func TestStateKeysPerInstance(t *testing.T) {
	g := uigen.PageState("shared", "a")
	menu := func() ui.Node {
		open := uigen.State("menu.open", false)
		return ui.El("ul", []ui.Attr{{Name: "hidden", Value: ui.BindExpr(!open.Get(), `["!",{"$":0}]`, open)}, {Name: "title", Value: ui.BindExpr(g.Get(), `{"$":0}`, g)}})
	}
	out, _ := page(t, ui.Fragment(menu(), menu()))
	for _, want := range []string{"c0:menu.open:1", "c0:menu.open:2", "g:shared"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in %s", want, html.UnescapeString(out))
		}
	}
	if strings.Count(out, "g:shared") != 2 {
		t.Errorf("the page's state isn't one: %s", html.UnescapeString(out))
	}
}

func TestShow(t *testing.T) {
	open := uigen.State("open", false)
	kid := ui.El("div", []ui.Attr{{Name: "class", Value: "transition data-closed:opacity-0"}}, ui.Text("hi"))
	for _, tt := range []struct {
		when any
		want string
	}{
		{true, `<vk-show style="display: contents;"><div class="transition data-closed:opacity-0">hi</div></vk-show>`},
		{false, `<vk-show hidden><div`},
		{open, `<vk-show data-vk-show="{&#34;i&#34;:false,&#34;s&#34;:&#34;c0:open:1&#34;}" hidden><div`},
		{ui.BindExpr(!open.Get(), `["!",{"$":0}]`, open), `<vk-show data-vk-show="[&#34;!&#34;,{&#34;i&#34;:false,&#34;s&#34;:&#34;c0:open:1&#34;}]" style="display: contents;"><div`},
	} {
		out, _ := page(t, ui.Show(ui.ShowProps{When: tt.when, Children: kid}))
		out = strings.ReplaceAll(out, "c0:open:2", "c0:open:1")
		if !strings.Contains(out, tt.want) {
			t.Errorf("when %v: %s\nwant %s", tt.when, out, tt.want)
		}
	}
	if _, err := ui.String(context.Background(), ui.Show(ui.ShowProps{When: uigen.State("s", "x")})); err == nil {
		t.Error("a ui.State[string] shown")
	}
}
