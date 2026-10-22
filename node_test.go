package ui_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/a-h/templ"
	"github.com/vuka-lang/ui/v2"
)

type stringer struct{}

func (stringer) String() string { return "<s>" }

var errBoom = errors.New("boom")

func failing() ui.Node {
	return ui.NodeFunc(func(context.Context, io.Writer) error { return errBoom })
}

func render(t *testing.T, n ui.Node) string {
	t.Helper()
	s, err := ui.String(context.Background(), n)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return s
}

func el(tag string, attrs ...ui.Attr) ui.Node { return ui.El(tag, attrs) }

func TestRender(t *testing.T) {
	a := func(name string, v any) ui.Attr { return ui.Attr{Name: name, Value: v} }
	tests := []struct {
		name string
		n    ui.Node
		want string
	}{
		{"text escaped", ui.Text(`<a href="x">Tom & 'Jerry'</a>`), `&lt;a href=&#34;x&#34;&gt;Tom &amp; &#39;Jerry&#39;&lt;/a&gt;`},
		{"text number", ui.Text(42), "42"},
		{"text nil", ui.Text(nil), ""},
		{"element", ui.El("div", nil, ui.Text("hi")), "<div>hi</div>"},
		{"nested", ui.El("ul", nil, ui.El("li", nil, ui.Text("a")), ui.El("li", nil, ui.Text("b"))), "<ul><li>a</li><li>b</li></ul>"},
		{"attr escaped", el("div", a("title", `"><script>x</script>`)), `<div title="&#34;&gt;&lt;script&gt;x&lt;/script&gt;"></div>`},
		{"attr quotes", el("div", a("title", `it's "q"`)), `<div title="it&#39;s &#34;q&#34;"></div>`},
		{"attr true", el("input", a("disabled", true)), `<input disabled>`},
		{"attr false", el("input", a("disabled", false)), `<input>`},
		{"attr nil", el("input", a("value", nil)), `<input>`},
		{"attr number", el("td", a("colspan", 2)), `<td colspan="2"></td>`},
		{"attr float", el("meter", a("value", 0.5)), `<meter value="0.5"></meter>`},
		{"attr stringer", el("div", a("title", stringer{})), `<div title="&lt;s&gt;"></div>`},
		{"attr node", el("div", a("title", ui.El("b", nil, ui.Text("x")))), `<div title="&lt;b&gt;x&lt;/b&gt;"></div>`},
		{"className", el("div", a("className", "card")), `<div class="card"></div>`},
		{"htmlFor", el("label", a("htmlFor", "name")), `<label for="name"></label>`},
		{"attr order kept", el("a", a("id", "x"), a("className", "y")), `<a id="x" class="y"></a>`},
		{"class list", el("div", a("className", []string{"a", "b"})), `<div class="a b"></div>`},
		{"class map", el("div", a("class", map[string]bool{"on": true, "off": false})), `<div class="on"></div>`},
		{"class KV", el("div", a("class", templ.KV("active", true))), `<div class="active"></div>`},
		{"style sorted", el("p", a("style", ui.Style{"margin": "0", "color": "red"})), `<p style="color:red;margin:0;"></p>`},
		{"style unsafe value", el("p", a("style", ui.Style{"background": "url(javascript:x)"})), `<p style="background:zTemplUnsafeCSSPropertyValue;"></p>`},
		{"style string", el("p", a("style", "color: red")), `<p style="color: red;"></p>`},
		{"href relative", el("a", a("href", "/users/1?x=a:b")), `<a href="/users/1?x=a:b"></a>`},
		{"href https", el("a", a("href", "HTTPS://x.dev")), `<a href="HTTPS://x.dev"></a>`},
		{"href mailto", el("a", a("href", "mailto:a@b.c")), `<a href="mailto:a@b.c"></a>`},
		{"href tel", el("a", a("href", "tel:+255")), `<a href="tel:+255"></a>`},
		{"href javascript", el("a", a("href", "javascript:alert(1)")), `<a href="about:invalid#TemplFailedSanitizationURL"></a>`},
		{"href JAVASCRIPT", el("a", a("href", "JAVASCRIPT:alert(1)")), `<a href="about:invalid#TemplFailedSanitizationURL"></a>`},
		{"href spaced javascript", el("a", a("href", " javascript:alert(1)")), `<a href="about:invalid#TemplFailedSanitizationURL"></a>`},
		{"href data", el("a", a("href", "data:text/html,x")), `<a href="about:invalid#TemplFailedSanitizationURL"></a>`},
		{"href SafeURL", el("a", a("href", templ.SafeURL("javascript:ok()"))), `<a href="javascript:ok()"></a>`},
		{"form action", el("form", a("action", "vbscript:x")), `<form action="about:invalid#TemplFailedSanitizationURL"></form>`},
		{"link href", el("link", a("href", "javascript:x")), `<link href="about:invalid#TemplFailedSanitizationURL">`},
		{"img src data image", el("img", a("src", "data:image/png;base64,AAAA")), `<img src="data:image/png;base64,AAAA">`},
		{"script attr", el("button", a("onclick", templ.JSUnsafeFuncCall("go()"))), `<button onclick="go()"></button>`},
		{"void", el("br"), "<br>"},
		{"doctype", ui.El("html", nil, ui.El("body", nil)), "<!DOCTYPE html><html><body></body></html>"},
		{"script text", ui.El("script", nil, ui.Text("</script><b>")), `<script>"\u003c/script\u003e\u003cb\u003e"</script>`},
		{"script value", ui.El("script", nil, ui.Safe("const u = "), ui.Child("a\"b")), `<script>const u = "a\"b"</script>`},
		{"style text", ui.El("style", nil, ui.Text("a > b {} </style><b>")), `<style>a > b {} <\/style><b></style>`},
		{"text after script", ui.Fragment(ui.El("script", nil), ui.Text("<")), `<script></script>&lt;`},
		{"safe", ui.Safe("<b>raw</b>"), "<b>raw</b>"},
		{"child node", ui.Child(ui.Text("x")), "x"},
		{"child nil", ui.Child(nil), ""},
		{"child string", ui.Child("<x>"), "&lt;x&gt;"},
		{"child bool", ui.Child(true), "true"},
		{"child int", ui.Child(7), "7"},
		{"child float", ui.Child(1.5), "1.5"},
		{"child stringer", ui.Child(stringer{}), "&lt;s&gt;"},
		{"child error", ui.Child(errors.New("<e>")), "&lt;e&gt;"},
		{"child nodes", ui.Child([]ui.Node{ui.Text("a"), nil, ui.Text("b")}), "ab"},
		{"child anys", ui.Child([]any{"a", 1, nil, ui.El("i", nil)}), "a1<i></i>"},
		{"child other", ui.Child(struct{ A int }{1}), "{1}"},
		{"fragment", ui.Fragment(ui.Text("a"), nil, ui.Text("b")), "ab"},
		{"nodes", ui.Nodes(func(add func(ui.Node)) {
			for i := range 3 {
				add(ui.El("li", nil, ui.Child(i)))
			}
		}), "<li>0</li><li>1</li><li>2</li>"},
		{"try ok", ui.Try(ui.Text("ok"), nil), "ok"},
		{"try nil", ui.Try(nil, nil), ""},
		{"boundary ok", ui.ErrorBoundary(func(error) ui.Node { return ui.Text("fb") }, ui.Text("a"), ui.Text("b")), "ab"},
		{"boundary fallback", ui.El("div", nil, ui.ErrorBoundary(
			func(err error) ui.Node { return ui.El("p", nil, ui.Child(err)) },
			ui.El("span", nil, ui.Text("partial")), failing(),
		)), "<div><p>boom</p></div>"},
		{"boundary try", ui.ErrorBoundary(func(err error) ui.Node { return ui.Child(err) }, ui.Try(nil, errBoom)), "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(t, tt.n); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestRenderErrors(t *testing.T) {
	tests := []struct {
		name string
		n    ui.Node
		want string
	}{
		{"void children", ui.El("img", nil, ui.Text("x")), "void element"},
		{"attr name space", el("div", ui.Attr{Name: "x onload", Value: "y"}), "invalid attribute name"},
		{"attr name quote", el("div", ui.Attr{Name: `x"`, Value: "y"}), "invalid attribute name"},
		{"attr name empty", el("div", ui.Attr{Name: "", Value: "y"}), "invalid attribute name"},
		{"tag name", ui.El("di v", nil), "invalid tag name"},
		{"try", ui.Try(ui.Text("x"), errBoom), "boom"},
		{"child", ui.El("div", nil, failing()), "boom"},
		{"nodes stop", ui.Nodes(func(add func(ui.Node)) { add(failing()); add(ui.Text("x")) }), "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ui.String(context.Background(), tt.n)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNodesBuildsPerRender(t *testing.T) {
	calls := 0
	n := ui.Nodes(func(add func(ui.Node)) { calls++; add(ui.Text(calls)) })
	if a, b := render(t, n), render(t, n); a != "1" || b != "2" {
		t.Fatalf("renders %q %q", a, b)
	}
}

type errWriter struct{ n int }

func (w *errWriter) Write(p []byte) (int, error) {
	w.n++
	return 0, errors.New("write failed")
}

func TestWriteError(t *testing.T) {
	w := &errWriter{}
	big := ui.Text(strings.Repeat("x", 10000))
	err := ui.El("div", nil, big, big, big).Render(context.Background(), w)
	if err == nil {
		t.Fatal("no error")
	}
	if w.n != 1 {
		t.Fatalf("%d writes after the first error", w.n-1)
	}
}

func TestTemplInterop(t *testing.T) {
	var c templ.Component = ui.El("b", nil, ui.Text("v")) // a Node is a templ component
	var n ui.Node = templ.Raw("<i>t</i>")                 // and vice versa
	got := render(t, ui.El("p", nil, c, n))
	if got != "<p><b>v</b><i>t</i></p>" {
		t.Fatal(got)
	}
}

func TestHandler(t *testing.T) {
	h := ui.Handler(func(r *http.Request) (ui.Node, error) {
		switch r.URL.Path {
		case "/err":
			return nil, errBoom
		case "/fail":
			return ui.El("div", nil, ui.Text("partial"), failing()), nil
		}
		return ui.El("h1", nil, ui.Text(r.URL.Query().Get("name"))), nil
	})
	tests := []struct {
		path, body, ctype string
		status            int
	}{
		{"/?name=<x>", "<h1>&lt;x&gt;</h1>", "text/html; charset=utf-8", 200},
		{"/err", "Internal Server Error\n", "text/plain; charset=utf-8", 500},
		{"/fail", "Internal Server Error\n", "text/plain; charset=utf-8", 500},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", tt.path, nil))
		if rec.Code != tt.status || rec.Body.String() != tt.body || rec.Header().Get("Content-Type") != tt.ctype {
			t.Errorf("%s: %d %q %q", tt.path, rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
		}
	}
}

func TestWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	if err := ui.Write(rec, r, ui.El("div", nil, ui.Text("x"), failing())); err == nil || rec.Body.Len() != 0 {
		t.Fatalf("err %v, body %q", err, rec.Body.String())
	}
	if err := ui.Write(rec, r, ui.El("p", nil)); err != nil || rec.Body.String() != "<p></p>" ||
		rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("err %v, body %q", err, rec.Body.String())
	}
}

func table(rows int) ui.Node {
	return ui.El("table", []ui.Attr{{Name: "className", Value: "users"}},
		ui.El("tbody", nil, ui.Nodes(func(add func(ui.Node)) {
			for i := range rows {
				add(ui.El("tr", []ui.Attr{{Name: "id", Value: i}, {Name: "className", Value: []string{"row", "even"}}},
					ui.El("td", nil, ui.Child(i)),
					ui.El("td", nil, ui.Text(fmt.Sprintf("user <%d>", i))),
					ui.El("td", nil, ui.El("a", []ui.Attr{{Name: "href", Value: fmt.Sprintf("/users/%d", i)}}, ui.Text("open"))),
				))
			}
		})))
}

func TestConcurrent(t *testing.T) {
	n := table(50)
	want := render(t, n)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				var b strings.Builder
				if err := n.Render(context.Background(), &b); err != nil || b.String() != want {
					t.Error("concurrent render differs")
					return
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkTable1000(b *testing.B) {
	n := table(1000)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := n.Render(ctx, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

// TestElCopies checks El and F at every size their one-allocation boxes
// cover and beyond: the element holds copies of its attributes and children.
func TestElCopies(t *testing.T) {
	for na := 0; na <= 9; na++ {
		for nk := 0; nk <= 20; nk++ {
			attrs := make([]ui.Attr, na)
			for i := range attrs {
				attrs[i] = ui.Attr{Name: fmt.Sprintf("a%d", i), Value: i}
			}
			kids := make([]ui.Node, nk)
			for i := range kids {
				kids[i] = ui.Text(i)
			}
			e := ui.El("p", attrs, kids...).(*ui.Element)
			if !reflect.DeepEqual(e.Attrs, attrs) && na > 0 || na == 0 && e.Attrs != nil ||
				!reflect.DeepEqual(e.Children, kids) && nk > 0 || nk == 0 && e.Children != nil {
				t.Fatalf("El with %d attrs, %d children: %+v", na, nk, e)
			}
			if na > 0 {
				attrs[0].Value = "changed"
				if e.Attrs[0].Value != 0 {
					t.Fatalf("El shares the caller's attributes")
				}
			}
			f := ui.F(1, "", kids...).(*ui.Frame)
			if !reflect.DeepEqual(f.Kids, kids) && nk > 0 || nk == 0 && f.Kids != nil {
				t.Fatalf("F with %d kids: %+v", nk, f)
			}
		}
	}
}

func TestChildText(t *testing.T) {
	s := "a<b"
	for _, n := range []ui.Node{ui.Child(s), ui.Text(s), ui.Child(s[:1])} {
		tn, ok := n.(ui.TextNode)
		if !ok || !strings.HasPrefix(s, string(tn)) {
			t.Fatalf("%#v", n)
		}
	}
	if got, _ := ui.String(context.Background(), ui.El("p", nil, ui.Child(s), ui.Text(""), ui.Child(""), ui.Text(7))); got != "<p>a&lt;b7</p>" {
		t.Fatal(got)
	}
}
