package ui_test

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/a-h/templ"
	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/uigen"
)

type order struct {
	Ref   string
	Total int
	Late  bool
}

func orderRowOf(o order) ui.Node {
	return ui.HTML(`
<tr class="row" class:late={o.Late}>
  <td>{o.Ref}</td>
  <td class="tabular-nums">{o.Total}</td>
</tr>`, o.Late, o.Ref, o.Total)
}

func TestHTMLRender(t *testing.T) {
	got := render(t, ui.Each([]order{{"A-1", 12, true}, {"B-2", 7, false}}, orderRowOf))
	want := `<tr class="late row"><td>A-1</td><td class="tabular-nums">12</td></tr>` +
		`<tr class="row"><td>B-2</td><td class="tabular-nums">7</td></tr>`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// TestHTMLMatchesF renders templates and the F lowering of the same JSX:
// the same HTML.
func TestHTMLMatchesF(t *testing.T) {
	user := "Ann <admin> & co"
	for _, c := range []struct {
		name string
		tmpl ui.Node
		f    ui.Node
	}{
		{"text",
			ui.HTML(`
				<p>
					Hello, {user}!
					Welcome &amp; enjoy {{braces}.
				</p>`, user),
			ui.F(1, "E(tht)", ui.El("p", nil, ui.Text("Hello, "), ui.Child(user), ui.Text("! Welcome & enjoy {braces}.")))},
		{"attributes",
			ui.HTML(`<a href={href} class="link" disabled={yes} hidden="{no}" title='{title}' data-n={n}>x</a>`, "javascript:alert(1)", true, false, `"q"`, 3),
			ui.F(2, "Edsdddd(t)", ui.El("a", []ui.Attr{{Name: "href", Value: "javascript:alert(1)"}, {Name: "class", Value: "link"}, {Name: "disabled", Value: true}, {Name: "hidden", Value: false}, {Name: "title", Value: `"q"`}, {Name: "data-n", Value: 3}}, ui.Text("x")))},
		{"parts",
			ui.HTML(`<a href="/users/{u.ID}?tab={tab}" class="btn {cls}">u</a>`, "a b", 2, ui.Class{"on": true}),
			ui.F(3, "Edd(t)", ui.El("a", []ui.Attr{{Name: "href", Value: "/users/a b?tab=2"}, {Name: "class", Value: "btn on"}}, ui.Text("u")))},
		{"void, and braces in script",
			ui.HTML(`<div><br><input name=q /><script>let x = {a: 1};</script></div>`),
			ui.F(4, "E(E()Es()E(t))", ui.El("div", nil, ui.El("br", nil), ui.El("input", []ui.Attr{{Name: "name", Value: "q"}}), ui.El("script", nil, ui.Safe("let x = {a: 1};"))))},
		{"pre and textarea keep whitespace",
			ui.HTML("<pre>\n  a\n  b</pre><textarea>\n x {y}</textarea>", "y"),
			ui.F(5, "E(t)E(th)", ui.El("pre", nil, ui.Text("\n  a\n  b")), ui.El("textarea", nil, ui.Text("\n x "), ui.Child("y")))},
		{"children",
			ui.HTML(`<ul>{items}</ul>{err}{none}`, []ui.Node{ui.Text("a"), ui.Text("b")}, errors.New("e<"), nil),
			ui.F(6, "E(h)hh", ui.El("ul", nil, ui.Text("a"), ui.Text("b")), ui.Text("e<"))},
		{"attribute sets",
			ui.HTML(`<input {attrs...} type="text"><i {more...}></i><b {none...}></b>`, []ui.Attr{{Name: "name", Value: "q"}, {Name: "required", Value: true}}, templ.Attributes{"id": "x", "class": "c"}, nil),
			ui.F(7, "Eds()Ed()Ed()", ui.El("input", []ui.Attr{{Name: "name", Value: "q"}, {Name: "required", Value: true}, {Name: "type", Value: "text"}}), ui.El("i", []ui.Attr{{Name: "class", Value: "c"}, {Name: "id", Value: "x"}}), ui.El("b", nil))},
		{"comments and doctype",
			ui.HTML(`<!doctype html><html><!-- note -->{/* a {comment} */}<body>b</body></html>`),
			ui.F(8, "E(E(t))", ui.El("html", nil, ui.El("body", nil, ui.Text("b"))))},
		{"one label, one value",
			ui.HTML(`<p title={name}>{name} and {name}</p>`, user),
			ui.F(9, "Ed(hth)", ui.El("p", []ui.Attr{{Name: "title", Value: user}}, ui.Child(user), ui.Text(" and "), ui.Child(user)))},
		{"toggled classes",
			ui.HTML(`<i class:a={yes} class="b" class:c={no}></i>`, true, false),
			ui.F(10, "Ed()", ui.El("i", []ui.Attr{{Name: "class", Value: "a b"}}))},
	} {
		if got, want := render(t, c.tmpl), render(t, c.f); got != want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.name, got, want)
		}
	}
}

func TestHTMLHoleKinds(t *testing.T) {
	for _, c := range []struct {
		src  string
		args []any
		err  string
	}{
		{`<p>{x}</p>`, []any{ui.Attr{Name: "id", Value: "x"}}, `{x} is a child of <p>; it got ui.Attr, an attribute set`},
		{`<p>{x}</p>`, []any{ui.On(func() {})}, `{x} is a child of <p>; it got ui.EventHandler, an event handler`},
		{`<p>{x}</p>`, []any{func() {}}, `{x} is a child of <p>; it got func(), a func`},
		{`<p>{x}</p>`, []any{ui.Class{"a": true}}, `it got ui.Class, an attribute's value`},
		{`<p class={x}></p>`, []any{[]ui.Attr{}}, `{x} is class on <p>; it got []ui.Attr, an attribute set`},
		{`<p id={x}></p>`, []any{ui.Class{}}, `{x} is id on <p>; it got ui.Class, a class: only class takes one`},
		{`<p id={x}></p>`, []any{ui.On(func() {})}, `an event handler: only an on… attribute takes one`},
		{`<p id={x}></p>`, []any{func() {}}, `a func: only an on… attribute takes one`},
		{`<p onClick={x}></p>`, []any{"alert(1)"}, `{x} is onClick on <p>; it got string, not a handler`},
		{`<p id="a{x}"></p>`, []any{true}, `{x} is part of id on <p>; it got bool, not text`},
		{`<p id="a{x}"></p>`, []any{ui.Safe("x")}, `it got ui.RawHTML, not text`},
		{`<p {x...}></p>`, []any{"id"}, `{x} is an attribute set of <p>; it got string, not an attribute set`},
		{`<p {x...}></p>`, []any{ui.Attr{Name: "key", Value: 1}}, `holding key`},
		{`<p class:on={x}></p>`, []any{"yes"}, `{x} is class on <p>; it got string, not a bool or a ui.State[bool]`},
		{`<title>{x}</title>`, []any{ui.Text("x")}, ``},
		{`<title>{x}</title>`, []any{ui.Safe("<b>")}, `{x} is text in <title>; it got ui.RawHTML, not text`},
		{`<p>{a}{b}</p>`, []any{"x"}, `has 2 values ({a}, {b}), it got 1`},
	} {
		_, err := ui.String(context.Background(), ui.HTML(c.src, c.args...))
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

func TestHTMLParseErrors(t *testing.T) {
	for _, c := range []struct{ src, err string }{
		{"<div>\n<p></div>", "</div> closes <p>"},
		{"<div>", "<div> isn't closed"},
		{"</p>", "</p> closes nothing"},
		{"<p ={x}>", "has an attribute with no name"},
		{"<p a{x}=1>", "a hole can't name an attribute"},
		{"<p {x}>", "attribute set: <p {x...}>"},
		{"<p>{x...}</p>", "{x...} is an attribute set"},
		{"<p>{ }</p>", "a hole needs a label"},
		{"<p>{x</p>", "a hole isn't closed"},
		{"<p onClick=\"a{x}\"></p>", "takes a handler"},
		{"<p class=a{x}></p>", "quote a value"},
		{"<p class:on=\"x\"></p>", "takes a hole"},
		{"<Card/>", "a component is a hole, {Card(…)}"},
		{"<p", "<p isn't closed"},
	} {
		_, err := ui.String(context.Background(), ui.HTML(c.src))
		if err == nil || !strings.Contains(err.Error(), c.err) || !strings.HasPrefix(err.Error(), "ui: template ") || !strings.Contains(err.Error(), "/template_test.go:") {
			t.Errorf("%q: got %v, want …%s…", c.src, err, c.err)
		}
	}
}

// TestHTMLErrorLine: an error names the hole's own line.
func TestHTMLErrorLine(t *testing.T) {
	_, _, line, _ := runtime.Caller(0)
	_, err := ui.String(context.Background(), ui.HTML(`
<p>
  {x}
</p>`, ui.Attr{}))
	if err == nil || !strings.Contains(err.Error(), "template_test.go:"+strconv.Itoa(line+3)+":") {
		t.Fatalf("got %v", err)
	}
}

// TestHTMLBuiltSource: a source built at run time is parsed once for each
// text, and two texts sharing memory are told apart.
func TestHTMLBuiltSource(t *testing.T) {
	for i, want := range []string{"<b>1</b>", "<i>1</i>", "<b>1</b>"} {
		tag := "b"
		if i == 1 {
			tag = "i"
		}
		src := strings.Join([]string{"<", tag, ">{v}</", tag, ">"}, "")
		if got := render(t, ui.HTML(src, 1)); got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
}

type tCount struct {
	ui.Live
	Label string
	n     int
}

func (c *tCount) Inc()                        { c.n++ }
func (c *tCount) LiveSchema(l *ui.LiveSchema) { l.Mount(c.mount) }
func (c *tCount) mount(context.Context) error { c.n = len(c.Label); return nil }
func (c *tCount) Render() ui.Node {
	return ui.HTML(`<span>{c.Label}={c.n}<button onClick={c.Inc}>+</button></span>`, c.Label, c.n, c.Inc)
}

func TestHTMLComponents(t *testing.T) {
	got := render(t, ui.HTML(`<div>{a}{b}</div>`, &tCount{Label: "ab"}, ui.Key(1, &tCount{Label: "c"})))
	if want := `<div><span>ab=2<button>+</button></span><span>c=1<button>+</button></span></div>`; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestHTMLState(t *testing.T) {
	open := uigen.UseState(false)
	got := render(t, ui.HTML(`<button aria-expanded={open} onClick={open.Toggle()} class:on={open}>{open}</button>`, open, uigen.Toggle(ui.JS, open)))
	for _, w := range []string{`aria-expanded="false" data-vk-b-aria-expanded=`, `data-vk-js-click=`, `data-vk-b-class=`, `<vk-t data-vk-b=`} {
		if !strings.Contains(got, w) {
			t.Errorf("no %s in %s", w, got)
		}
	}
}

func TestUseStateSites(t *testing.T) {
	var keys []string
	for range 2 {
		a, b := uigen.UseState(1), uigen.UseState(2)
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

// TestHTMLConcurrent renders one node from many goroutines, under a live
// host and without.
func TestHTMLConcurrent(t *testing.T) {
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
			if _, err := ui.String(context.Background(), orderRowOf(order{"B", 2, false})); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

func TestHTMLRootAttrs(t *testing.T) {
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

func TestHTMLIslands(t *testing.T) {
	got := render(t, ui.HTML(`<div>{chart}{lazy}</div>`, tChart{Points: []int{1}}, ui.Mount(tChart{}, ui.Idle(), ui.Fallback(ui.Text("…")))))
	for _, w := range []string{
		`<vk-island data-vk-s="github.com/vuka-lang/ui/v2_test/template_test.go:` /* the hole's site, by package */, `#0" data-vk-island="tChart" data-vk-client="visible" data-vk-props="{&#34;Points&#34;:[1]}"`,
		`#1" data-vk-island="tChart" data-vk-client="idle"`, `>…</vk-island>`,
	} {
		if !strings.Contains(got, w) {
			t.Errorf("no %s in %s", w, got)
		}
	}
}

// TestHTMLListSites: components in two lists of one template are apart,
// though their keys are the same.
func TestHTMLListSites(t *testing.T) {
	items := []string{"a", "b"}
	row := func(s string) ui.Node { return ui.Key(s, &tCount{Label: s}) }
	n := ui.HTML(`<div>{a}</div><div>{b}</div>`, ui.Each(items, row), ui.Each(items, row))
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return n })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLAttrSetWalk(t *testing.T) {
	var got []ui.Attr
	err := ui.Walk(context.Background(), ui.HTML(`<a {attrs...} id="x"></a>`, ui.Attr{Name: "href", Value: "/"}), attrsOf{&got})
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

func TestHTMLSitesOnOneLine(t *testing.T) {
	a, b := ui.HTML(`<p>{x}</p>`, ui.Attr{}), ui.HTML(`<i>{x}</i>`, ui.Attr{})
	_, errA := ui.String(context.Background(), a)
	_, errB := ui.String(context.Background(), b)
	site := func(err error) string { return strings.SplitN(err.Error(), ": ", 3)[1] }
	if errA == nil || errB == nil || site(errA) == site(errB) {
		t.Fatalf("two templates on a line are two sites: %v / %v", errA, errB)
	}
}

// TestHTMLRawText checks script and style: braces are their text, kept as
// written.
func TestHTMLRawText(t *testing.T) {
	got := render(t, ui.HTML(`<style>a{color:red}</style><script>if (x) { y({z}) }</script>`))
	if want := `<style>a{color:red}</style><script>if (x) { y({z}) }</script>`; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
