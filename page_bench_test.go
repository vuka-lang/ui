package ui_test

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
)

// benchRow and benchPage are a 12 KB page as the compiler lowers its JSX: a
// layout and a 50-row table, each row's cells with a link and a dynamic class.
type benchRow struct {
	ID                        int
	Name, Email, Role, Status string
}

func benchRows() []benchRow {
	out := make([]benchRow, 50)
	roles := []string{"admin", "editor", "viewer"}
	for i := range out {
		s := strconv.Itoa(i + 1)
		st := "active"
		if i%4 == 0 {
			st = "invited"
		}
		out[i] = benchRow{ID: i + 1, Name: "User number " + s, Email: "user" + s + "@example.com", Role: roles[i%3], Status: st}
	}
	return out
}

func benchLayout(title string, children ui.Node) ui.Node {
	return ui.F(0x4b44aa8876d5918c, "Es(E(Es()Ess()E(ht))Es(Es(E(Es(t)tEs(t)))Es(h)Es(t)))", ui.El("html", []ui.Attr{{Name: "lang", Value: "en"}}, ui.El("head", nil, ui.El("meta", []ui.Attr{{Name: "charset", Value: "utf-8"}}), ui.El("meta", []ui.Attr{{Name: "name", Value: "viewport"}, {Name: "content", Value: "width=device-width, initial-scale=1"}}), ui.El("title", nil, ui.Child(title), ui.Text(" · Bench"))), ui.El("body", []ui.Attr{{Name: "class", Value: "page"}}, ui.El("header", []ui.Attr{{Name: "class", Value: "top"}}, ui.El("nav", nil, ui.El("a", []ui.Attr{{Name: "href", Value: "/"}}, ui.Text("Home")), ui.Text(" "), ui.El("a", []ui.Attr{{Name: "href", Value: "/page"}}, ui.Text("Users")))), ui.El("main", []ui.Attr{{Name: "class", Value: "content"}}, ui.Child(children)), ui.El("footer", []ui.Attr{{Name: "class", Value: "foot"}}, ui.Text("Benchmark page")))))
}

func benchPage() ui.Node {
	rs := benchRows()
	return ui.F(0xaf63de4c8601eff2, "c", benchLayout("Users", ui.F(0xd553e1cee178f424, "Es(t)Es(E(E(E(t)E(t)E(t)E(t)E(t)))E(b))", ui.El("h1", []ui.Attr{{Name: "class", Value: "title"}}, ui.Text("Users")), ui.El("table", []ui.Attr{{Name: "class", Value: "table"}}, ui.El("thead", nil, ui.El("tr", nil, ui.El("th", nil, ui.Text("ID")), ui.El("th", nil, ui.Text("Name")), ui.El("th", nil, ui.Text("Email")), ui.El("th", nil, ui.Text("Role")), ui.El("th", nil, ui.Text("Status")))), ui.El("tbody", nil, ui.Nodes(func(__add func(ui.Node)) {
		for _, r := range rs {
			__add(ui.F(0x8b2b40ae54e58b1f, "Es(Es(h)Es(h)Es(Ed(h))Es(h)Ed(h))", ui.El("tr", []ui.Attr{{Name: "class", Value: "row"}}, ui.El("td", []ui.Attr{{Name: "class", Value: "num"}}, ui.Child(strconv.Itoa(r.ID))), ui.El("td", []ui.Attr{{Name: "class", Value: "name"}}, ui.Child(r.Name)), ui.El("td", []ui.Attr{{Name: "class", Value: "email"}}, ui.El("a", []ui.Attr{{Name: "href", Value: "mailto:" + r.Email}}, ui.Child(r.Email))), ui.El("td", []ui.Attr{{Name: "class", Value: "role"}}, ui.Child(r.Role)), ui.El("td", []ui.Attr{{Name: "class", Value: "status status-" + r.Status}}, ui.Child(r.Status)))))
		}
	}))))))
}

// TestBenchPageGolden pins the benchmark page's HTML: rendering it faster
// must not change a byte.
func TestBenchPageGolden(t *testing.T) {
	got, err := ui.String(context.Background(), benchPage())
	if err != nil {
		t.Fatal(err)
	}
	const golden = "testdata/benchpage.golden.html"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("the page renders differently:\n got %s\nwant %s", got, want)
	}
	if !strings.Contains(got, `<a href="mailto:user7@example.com">user7@example.com</a>`) {
		t.Fatalf("no row link in %s", got)
	}
}

func BenchmarkPageBuild(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = benchPage()
	}
}

func BenchmarkPageRender(b *testing.B) {
	n := benchPage()
	ctx := context.Background()
	var sb strings.Builder
	b.ReportAllocs()
	for b.Loop() {
		sb.Reset()
		if err := n.Render(ctx, &sb); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPageWrite renders the page with its rows built beforehand: the
// HTML writing alone.
func BenchmarkPageWrite(b *testing.B) {
	var rows ui.Group
	ctx := context.Background()
	if err := ui.Walk(ctx, benchPage(), rowCollector{&rows}); err != nil {
		b.Fatal(err)
	}
	n := benchLayout("Users", ui.El("table", nil, ui.El("tbody", nil, rows)))
	var buf bytes.Buffer
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := n.Render(ctx, &buf); err != nil {
			b.Fatal(err)
		}
	}
}

// rowCollector keeps the tr elements a walk meets.
type rowCollector struct{ rows *ui.Group }

func (c rowCollector) Open(e *ui.Element) error {
	if e.Tag == "tr" {
		*c.rows = append(*c.rows, e)
	}
	return nil
}
func (rowCollector) Close(*ui.Element) error               { return nil }
func (rowCollector) Text(string) error                     { return nil }
func (rowCollector) Raw(string) error                      { return nil }
func (rowCollector) Opaque(context.Context, ui.Node) error { return nil }

// BenchmarkPage builds and renders the page into a reused buffer.
func BenchmarkPage(b *testing.B) {
	ctx := context.Background()
	var buf bytes.Buffer
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := benchPage().Render(ctx, &buf); err != nil {
			b.Fatal(err)
		}
	}
}
