package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// refLayoutF and refPageF are the F lowering the compiler emitted before FT:
// the reference the FT lowering (benchPage, in page_bench_test.go) must match
// byte for byte, in HTML and in the live render tree.
func refLayoutF(title string, children ui.Node) ui.Node {
	return ui.F(0x4b44aa8876d5918c, "Es(E(Es()Ess()E(ht))Es(Es(E(Es(t)tEs(t)))Es(h)Es(t)))", ui.El("html", []ui.Attr{{Name: "lang", Value: "en"}}, ui.El("head", nil, ui.El("meta", []ui.Attr{{Name: "charset", Value: "utf-8"}}), ui.El("meta", []ui.Attr{{Name: "name", Value: "viewport"}, {Name: "content", Value: "width=device-width, initial-scale=1"}}), ui.El("title", nil, ui.Child(title), ui.Text(" · Bench"))), ui.El("body", []ui.Attr{{Name: "class", Value: "page"}}, ui.El("header", []ui.Attr{{Name: "class", Value: "top"}}, ui.El("nav", nil, ui.El("a", []ui.Attr{{Name: "href", Value: "/"}}, ui.Text("Home")), ui.Text(" "), ui.El("a", []ui.Attr{{Name: "href", Value: "/page"}}, ui.Text("Users")))), ui.El("main", []ui.Attr{{Name: "class", Value: "content"}}, ui.Child(children)), ui.El("footer", []ui.Attr{{Name: "class", Value: "foot"}}, ui.Text("Benchmark page")))))
}

func refPageF() ui.Node {
	rs := benchRows()
	return ui.F(0xaf63de4c8601eff2, "c", refLayoutF("Users", ui.F(0xd553e1cee178f424, "Es(t)Es(E(E(E(t)E(t)E(t)E(t)E(t)))E(b))", ui.El("h1", []ui.Attr{{Name: "class", Value: "title"}}, ui.Text("Users")), ui.El("table", []ui.Attr{{Name: "class", Value: "table"}}, ui.El("thead", nil, ui.El("tr", nil, ui.El("th", nil, ui.Text("ID")), ui.El("th", nil, ui.Text("Name")), ui.El("th", nil, ui.Text("Email")), ui.El("th", nil, ui.Text("Role")), ui.El("th", nil, ui.Text("Status")))), ui.El("tbody", nil, ui.Nodes(func(__add func(ui.Node)) {
		for _, r := range rs {
			__add(ui.F(0x8b2b40ae54e58b1f, "Es(Es(h)Es(h)Es(Ed(h))Es(h)Ed(h))", ui.El("tr", []ui.Attr{{Name: "class", Value: "row"}}, ui.El("td", []ui.Attr{{Name: "class", Value: "num"}}, ui.Child(strconv.Itoa(r.ID))), ui.El("td", []ui.Attr{{Name: "class", Value: "name"}}, ui.Child(r.Name)), ui.El("td", []ui.Attr{{Name: "class", Value: "email"}}, ui.El("a", []ui.Attr{{Name: "href", Value: "mailto:" + r.Email}}, ui.Child(r.Email))), ui.El("td", []ui.Attr{{Name: "class", Value: "role"}}, ui.Child(r.Role)), ui.El("td", []ui.Attr{{Name: "class", Value: "status status-" + r.Status}}, ui.Child(r.Status)))))
		}
	}))))))
}

// TestLazyFrameHTMLMatchesF renders the FT page and the F page and requires
// the same HTML, byte for byte.
func TestLazyFrameHTMLMatchesF(t *testing.T) {
	ctx := context.Background()
	want, err := ui.String(ctx, refPageF())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ui.String(ctx, benchPage())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FT HTML differs from F:\n got %q\nwant %q", got, want)
	}
}

// treeString serializes a render tree so two trees that would emit the same
// live protocol (statics by fingerprint, dynamics, loop item keys) compare
// equal as text.
func treeString(sb *strings.Builder, d int, v any) {
	pad := strings.Repeat("  ", d)
	switch v := v.(type) {
	case *ui.Tree:
		fmt.Fprintf(sb, "%sTree key=%#x statics=%q\n", pad, v.Key, v.Statics)
		for _, dy := range v.Dyn {
			treeString(sb, d+1, dy)
		}
	case *ui.TreeList:
		fmt.Fprintf(sb, "%sList keys=%q\n", pad, v.Keys)
		for _, it := range v.Items {
			treeString(sb, d+1, it)
		}
	default:
		fmt.Fprintf(sb, "%s%T %v\n", pad, v, v)
	}
}

// TestLazyFrameTreeMatchesF builds the live render tree of the FT page and the
// F page and requires the same statics, dynamics and keys: the live protocol a
// session emits is unchanged.
func TestLazyFrameTreeMatchesF(t *testing.T) {
	ctx := context.Background()
	wantT, err := ui.BuildTree(ctx, refPageF(), false)
	if err != nil {
		t.Fatal(err)
	}
	gotT, err := ui.BuildTree(ctx, benchPage(), false)
	if err != nil {
		t.Fatal(err)
	}
	var want, got strings.Builder
	treeString(&want, 0, wantT)
	treeString(&got, 0, gotT)
	if got.String() != want.String() {
		t.Fatalf("FT render tree differs from F:\n=== got ===\n%s\n=== want ===\n%s", got.String(), want.String())
	}
}

// BenchmarkPageF builds and renders the F lowering into a reused buffer, the
// before to BenchmarkPage's after.
func BenchmarkPageF(b *testing.B) {
	ctx := context.Background()
	var buf bytes.Buffer
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := refPageF().Render(ctx, &buf); err != nil {
			b.Fatal(err)
		}
	}
}

// TestLazyFrameDynamicKinds checks the fast HTML path against the F reference
// (the build closure's own tree) for dynamic attribute and child values that
// aren't plain strings: a bare/omitted bool, a nil, a sanitized URL and a
// non-string child.
func TestLazyFrameDynamicKinds(t *testing.T) {
	ctx := context.Background()
	check := func(name string, fp uint64, shape string, build func([]any) ui.Node, dyns ...any) {
		t.Helper()
		want, err := ui.String(ctx, build(dyns))
		if err != nil {
			t.Fatalf("%s: F: %v", name, err)
		}
		got, err := ui.String(ctx, ui.FT(fp, shape, build, dyns...))
		if err != nil {
			t.Fatalf("%s: FT: %v", name, err)
		}
		if got != want {
			t.Fatalf("%s: FT %q != F %q", name, got, want)
		}
	}
	check("bool+nil attrs", 0x1001, "Edd()", func(d []any) ui.Node {
		return ui.F(0x1001, "Edd()", ui.El("input", []ui.Attr{{Name: "disabled", Value: d[0]}, {Name: "value", Value: d[1]}}))
	}, true, nil)
	check("bool false omits", 0x1002, "Ed()", func(d []any) ui.Node {
		return ui.F(0x1002, "Ed()", ui.El("input", []ui.Attr{{Name: "checked", Value: d[0]}}))
	}, false)
	check("url sanitized + int child", 0x1003, "Ed(h)", func(d []any) ui.Node {
		return ui.F(0x1003, "Ed(h)", ui.El("a", []ui.Attr{{Name: "href", Value: d[0]}}, ui.Child(d[1])))
	}, "javascript:alert(1)", 42)
	check("class list value", 0x1004, "Ed(h)", func(d []any) ui.Node {
		return ui.F(0x1004, "Ed(h)", ui.El("div", []ui.Attr{{Name: "class", Value: d[0]}}, ui.Child(d[1])))
	}, []string{"a", "b"}, "x<y")
}
