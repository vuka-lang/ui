package ui_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/a-h/templ"
	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
)

type order struct {
	Ref   string
	Total int
	Late  bool
}

var orderRow = ui.Template(`
<tr class="{{0}}">
  <td>{{1}}</td>
  <td class="tabular-nums">{{2}}</td>
</tr>`)

func orderRowOf(o order) ui.Node {
	return orderRow.Render(ui.Class{"row": true, "late": o.Late}, o.Ref, o.Total)
}

func TestTemplateRender(t *testing.T) {
	got := render(t, ui.Each([]order{{"A-1", 12, true}, {"B-2", 7, false}}, orderRowOf))
	want := `<tr class="late row"><td>A-1</td><td class="tabular-nums">12</td></tr>` +
		`<tr class="row"><td>B-2</td><td class="tabular-nums">7</td></tr>`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// TestTemplateMatchesF renders templates and the F lowering of the same JSX:
// the same HTML.
func TestTemplateMatchesF(t *testing.T) {
	user := "Ann <admin> & co"
	for _, c := range []struct {
		name string
		tmpl ui.Node
		f    ui.Node
	}{
		{"text",
			ui.Template(`
				<p>
					Hello, {{0}}!
					Welcome &amp; enjoy.
				</p>`).Render(user),
			ui.F(1, "E(tht)", ui.El("p", nil, ui.Text("Hello, "), ui.Child(user), ui.Text("! Welcome & enjoy.")))},
		{"attributes",
			ui.Template(`<a href="{{0}}" className="link" disabled="{{1}}" hidden="{{2}}" title='{{3}}' data-n={{4}}>x</a>`).Render("javascript:alert(1)", true, false, `"q"`, 3),
			ui.F(2, "Edsdddd(t)", ui.El("a", []ui.Attr{{Name: "href", Value: "javascript:alert(1)"}, {Name: "className", Value: "link"}, {Name: "disabled", Value: true}, {Name: "hidden", Value: false}, {Name: "title", Value: `"q"`}, {Name: "data-n", Value: 3}}, ui.Text("x")))},
		{"parts",
			ui.Template(`<a href="/users/{{0}}?tab={{1}}" class="btn {{2}}">u</a>`).Render("a b", 2, ui.Class{"on": true}),
			ui.F(3, "Edd(t)", ui.El("a", []ui.Attr{{Name: "href", Value: "/users/a b?tab=2"}, {Name: "class", Value: "btn on"}}, ui.Text("u")))},
		{"void and script",
			ui.Template(`<div><br><input name=q /><script>let x = {{0}};</script></div>`).Render(`</script><b>`),
			ui.F(4, "E(E()Es()E(tht))", ui.El("div", nil, ui.El("br", nil), ui.El("input", []ui.Attr{{Name: "name", Value: "q"}}), ui.El("script", nil, ui.Safe("let x = "), ui.Child(`</script><b>`), ui.Safe(";"))))},
		{"pre and textarea keep whitespace",
			ui.Template("<pre>\n  a\n  b</pre><textarea>\n x {{0}}</textarea>").Render("y"),
			ui.F(5, "E(t)E(th)", ui.El("pre", nil, ui.Text("\n  a\n  b")), ui.El("textarea", nil, ui.Text("\n x "), ui.Child("y")))},
		{"children",
			ui.Template(`<ul>{{0}}</ul>{{1}}{{2}}`).Render([]ui.Node{ui.Text("a"), ui.Text("b")}, errors.New("e<"), nil),
			ui.F(6, "E(h)hh", ui.El("ul", nil, ui.Text("a"), ui.Text("b")), ui.Text("e<"))},
		{"attribute set",
			ui.Template(`<input {{0}} type="text"><i {{1}}></i><b {{2}}></b>`).Render([]ui.Attr{{Name: "name", Value: "q"}, {Name: "required", Value: true}}, templ.Attributes{"id": "x", "class": "c"}, nil),
			ui.F(7, "Eds()Ed()Ed()", ui.El("input", []ui.Attr{{Name: "name", Value: "q"}, {Name: "required", Value: true}, {Name: "type", Value: "text"}}), ui.El("i", []ui.Attr{{Name: "class", Value: "c"}, {Name: "id", Value: "x"}}), ui.El("b", nil))},
		{"comments and doctype",
			ui.Template(`<!doctype html><html><!-- note --><body>b</body></html>`).Render(),
			ui.F(8, "E(E(t))", ui.El("html", nil, ui.El("body", nil, ui.Text("b"))))},
		{"if",
			ui.Template(`<p>{{0}}{{1}}</p>`).Render(ui.If(true, ui.Text("yes")), ui.If(false, ui.Text("no"))),
			ui.F(9, "E(t)", ui.El("p", nil, ui.Text("yes")))},
	} {
		if got, want := render(t, c.tmpl), render(t, c.f); got != want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.name, got, want)
		}
	}
}

func TestTemplateHoleKinds(t *testing.T) {
	for _, c := range []struct {
		src  string
		args []any
		err  string
	}{
		{`<p>{{0}}</p>`, []any{ui.Attr{Name: "id", Value: "x"}}, `{{0}} is a child of <p>; it got ui.Attr, an attribute set`},
		{`<p>{{0}}</p>`, []any{ui.On(func() {})}, `{{0}} is a child of <p>; it got ui.EventHandler, an event handler`},
		{`<p>{{0}}</p>`, []any{func() {}}, `{{0}} is a child of <p>; it got func(), a func`},
		{`<p>{{0}}</p>`, []any{ui.Class{"a": true}}, `it got ui.Class, an attribute's value`},
		{`<p class="{{0}}"></p>`, []any{[]ui.Attr{}}, `{{0}} is class on <p>; it got []ui.Attr, an attribute set`},
		{`<p id="{{0}}"></p>`, []any{ui.Class{}}, `{{0}} is id on <p>; it got ui.Class, a class: only class takes one`},
		{`<p id="{{0}}"></p>`, []any{ui.On(func() {})}, `an event handler: only an on… attribute takes one`},
		{`<p id="{{0}}"></p>`, []any{func() {}}, `a func: only an on… attribute takes one`},
		{`<p onClick="{{0}}"></p>`, []any{"alert(1)"}, `{{0}} is onClick on <p>; it got string, not a handler`},
		{`<p id="a{{0}}"></p>`, []any{true}, `{{0}} is part of id on <p>; it got bool, not text`},
		{`<p id="a{{0}}"></p>`, []any{ui.Safe("x")}, `it got ui.RawHTML, not text`},
		{`<p {{0}}></p>`, []any{"id"}, `{{0}} is an attribute set of <p>; it got string, not an attribute set`},
		{`<p {{0}}></p>`, []any{ui.Attr{Name: "key", Value: 1}}, `holding key`},
		{`<title>{{0}}</title>`, []any{ui.Text("x")}, ``},
		{`<title>{{0}}</title>`, []any{ui.Safe("<b>")}, `{{0}} is text in <title>; it got ui.RawHTML, not text`},
		{`<p>{{0}}{{1}}</p>`, []any{"x"}, `has 2 holes, Render got 1 values`},
	} {
		_, err := ui.String(context.Background(), ui.Template(c.src).Render(c.args...))
		switch {
		case c.err == "" && err != nil:
			t.Errorf("%s %v: %v", c.src, c.args, err)
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s %v: got %v, want …%s…", c.src, c.args, err, c.err)
		case err != nil && !strings.Contains(err.Error(), "/template_test.go:"):
			t.Errorf("%s: the error doesn't say where the template is: %v", c.src, err)
		}
	}
}

func TestTemplateParseErrors(t *testing.T) {
	for _, c := range []struct{ src, err string }{
		{"<div>\n<p></div>", "</div> closes <p>"},
		{"<div>", "<div> isn't closed"},
		{"</p>", "</p> closes nothing"},
		{"<p ={{0}}>", "has an attribute with no name"},
		{"<p a{{0}}=1>", "a hole can't name an attribute"},
		{"<p>{{1}}</p>", "{{0}} is missing"},
		{"<!-- {{0}} -->", "a hole can't be in a comment"},
		{"<p onClick=\"a{{0}}\"></p>", "takes a handler"},
		{"<p class=a{{0}}></p>", "quote a value"},
		{"<p", "<p isn't closed"},
	} {
		func() {
			defer func() {
				r := recover()
				msg, _ := r.(string)
				if !strings.Contains(msg, c.err) || !strings.HasPrefix(msg, "ui: template ") || !strings.Contains(msg, "/template_test.go:") {
					t.Errorf("%q: got %v, want …%s…", c.src, r, c.err)
				}
			}()
			ui.Template(c.src)
		}()
	}
}

type tCount struct {
	ui.Live
	Label string
	n     int
}

func (c *tCount) Inc()            { c.n++ }
func (c *tCount) Mount()          { c.n = len(c.Label) }
func (c *tCount) Render() ui.Node { return tCountT.Render(c.Label, c.n, c.Inc) }

var tCountT = ui.Template(`<span>{{0}}={{1}}<button onClick="{{2}}">+</button></span>`)

func TestTemplateComponents(t *testing.T) {
	got := render(t, ui.Template(`<div>{{0}}{{1}}</div>`).Render(&tCount{Label: "ab"}, ui.Key(1, &tCount{Label: "c"})))
	if want := `<div><span>ab=2<button>+</button></span><span>c=1<button>+</button></span></div>`; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestTemplateState(t *testing.T) {
	open := ui.UseState(false)
	got := render(t, ui.Template(`<button aria-expanded="{{0}}" onClick="{{1}}">{{2}}</button>`).Render(open, open.Toggle(), open))
	for _, w := range []string{`aria-expanded="false" data-vk-b-aria-expanded=`, `data-vk-js-click=`, `<vk-t data-vk-b=`} {
		if !strings.Contains(got, w) {
			t.Errorf("no %s in %s", w, got)
		}
	}
}

func TestUseStateSites(t *testing.T) {
	var keys []string
	for range 2 {
		a, b := ui.UseState(1), ui.UseState(2)
		for _, s := range []ui.State[int]{a, b} {
			j, err := ui.EncodeJS(context.Background(), ui.JS.Set(s, 3))
			if err != nil {
				t.Fatal(err)
			}
			keys = append(keys, string(j))
		}
	}
	site := func(s string) string { return strings.SplitN(strings.SplitN(s, `"s":"`, 2)[1], ":", 2)[0] }
	if site(keys[0]) == site(keys[1]) || site(keys[0]) != site(keys[2]) || site(keys[1]) != site(keys[3]) {
		t.Fatalf("each call is a site of its own, the same each time: %q", keys)
	}
}

// TestTemplateConcurrent renders one node from many goroutines, under a live
// host and without.
func TestTemplateConcurrent(t *testing.T) {
	n := orderRowOf(order{"A", 1, true})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := ui.BuildTree(context.Background(), n, false); err != nil {
				t.Error(err)
			}
			if _, err := ui.String(context.Background(), n); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

func TestTemplateRootAttrs(t *testing.T) {
	n, ok := ui.RootAttrs(orderRowOf(order{"A", 1, false}), ui.Attr{Name: "id", Value: "r"})
	if !ok {
		t.Fatal("a template of one element takes root attributes")
	}
	if got := render(t, n); !strings.HasPrefix(got, `<tr class="row" id="r">`) {
		t.Fatalf("got %s", got)
	}
}

type tChart struct{ Points []int }

func init() { ui.RegisterIsland[tChart]("islands/Chart.vue") }

func TestTemplateIslands(t *testing.T) {
	got := render(t, ui.Template(`<div>{{0}}{{1}}</div>`).Render(tChart{Points: []int{1}}, ui.Mount(tChart{}, ui.Idle(), ui.Fallback(ui.Text("…")))))
	for _, w := range []string{
		`<vk-island data-vk-s="` /* the hole's site */, `/template_test.go:`, `#0" data-vk-island="tChart" data-vk-client="visible" data-vk-props="{&#34;Points&#34;:[1]}"`,
		`#1" data-vk-island="tChart" data-vk-client="idle"`, `>…</vk-island>`,
	} {
		if !strings.Contains(got, w) {
			t.Errorf("no %s in %s", w, got)
		}
	}
}

// TestTemplateListSites: components in two lists of one template are apart,
// though their keys are the same.
func TestTemplateListSites(t *testing.T) {
	items := []string{"a", "b"}
	row := func(s string) ui.Node { return ui.Key(s, &tCount{Label: s}) }
	n := ui.Template(`<div>{{0}}</div><div>{{1}}</div>`).Render(ui.Each(items, row), ui.Each(items, row))
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return n })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateAttrSetWalk(t *testing.T) {
	var got []ui.Attr
	err := ui.Walk(context.Background(), ui.Template(`<a {{0}} id="x"></a>`).Render(ui.Attr{Name: "href", Value: "/"}), attrsOf{&got})
	if err != nil || len(got) != 2 || got[0].Name != "href" || got[1].Name != "id" {
		t.Fatalf("a Renderer sees the set's attributes in its place: %v %v", got, err)
	}
}

type attrsOf struct{ attrs *[]ui.Attr }

func (a attrsOf) Open(e *ui.Element) error            { *a.attrs = append(*a.attrs, e.Attrs...); return nil }
func (attrsOf) Close(*ui.Element) error               { return nil }
func (attrsOf) Text(string) error                     { return nil }
func (attrsOf) Raw(string) error                      { return nil }
func (attrsOf) Opaque(context.Context, ui.Node) error { return nil }

func TestTemplateSitesOnOneLine(t *testing.T) {
	a, b := ui.Template(`<p>{{0}}</p>`), ui.Template(`<p>{{0}}</p>`)
	_, errA := ui.String(context.Background(), a.Render(ui.Attr{}))
	_, errB := ui.String(context.Background(), b.Render(ui.Attr{}))
	site := func(err error) string { return strings.SplitN(err.Error(), ": ", 3)[1] }
	if errA == nil || errB == nil || site(errA) == site(errB) {
		t.Fatalf("two templates on a line are two sites: %v / %v", errA, errB)
	}
}

// TestTemplateRawHoles checks holes in script and style: written as the
// element's content, as JSX's {…} there is.
func TestTemplateRawHoles(t *testing.T) {
	tm := ui.Template(`<style>{{0}}</style><script>{{1}}</script>`)
	got, err := ui.String(context.Background(), tm.Render("a{}</style>", ui.RawHTML("var x = 1")))
	if err != nil {
		t.Fatal(err)
	}
	if want := `<style>a{}<\/style></style><script>var x = 1</script>`; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
