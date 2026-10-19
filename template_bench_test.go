package ui_test

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/vuka-lang/ui"
)

// The benchmark page of page_bench_test.go written with ui.HTML, as a
// person or the compiler writes it.

func benchLayoutHTML(title string, content ui.Node) ui.Node {
	return ui.HTML(`
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{title} · Bench</title>
  </head>
  <body class="page">
    <header class="top">
      <nav><a href="/">Home</a> <a href="/page">Users</a></nav>
    </header>
    <main class="content">{content}</main>
    <footer class="foot">Benchmark page</footer>
  </body>
</html>`, title, content)
}

func benchContentHTML(rows ui.Node) ui.Node {
	return ui.HTML(`
<h1 class="title">Users</h1>
<table class="table">
  <thead>
    <tr><th>ID</th><th>Name</th><th>Email</th><th>Role</th><th>Status</th></tr>
  </thead>
  <tbody>{rows}</tbody>
</table>`, rows)
}

func benchRowTemplate(r benchRow) ui.Node {
	return ui.HTML(`
<tr class="row">
  <td class="num">{strconv.Itoa(r.ID)}</td>
  <td class="name">{r.Name}</td>
  <td class="email"><a href={"mailto:" + r.Email}>{r.Email}</a></td>
  <td class="role">{r.Role}</td>
  <td class={"status status-" + r.Status}>{r.Status}</td>
</tr>`, strconv.Itoa(r.ID), r.Name, "mailto:"+r.Email, r.Email, r.Role, "status status-"+r.Status, r.Status)
}

func benchPageTemplate() ui.Node {
	return benchLayoutHTML("Users", benchContentHTML(ui.Each(benchRows(), benchRowTemplate)))
}

func TestTemplateBenchPageGolden(t *testing.T) {
	got, err := ui.String(context.Background(), benchPageTemplate())
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/benchpage.golden.html")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("the template page renders differently:\n got %s\nwant %s", got, want)
	}
}

func BenchmarkTemplatePage(b *testing.B) {
	ctx := context.Background()
	var buf bytes.Buffer
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := benchPageTemplate().Render(ctx, &buf); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTemplatePageBuild(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = benchPageTemplate()
	}
}

func BenchmarkTemplatePageRender(b *testing.B) {
	n := benchPageTemplate()
	ctx := context.Background()
	var buf bytes.Buffer
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := n.Render(ctx, &buf); err != nil {
			b.Fatal(err)
		}
	}
}
