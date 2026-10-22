package ui_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/vuka-lang/ui/v2"
)

// events records what a Walk sends.
type events []string

func (e *events) Open(el *ui.Element) error  { *e = append(*e, "<"+el.Tag); return nil }
func (e *events) Close(el *ui.Element) error { *e = append(*e, "/"+el.Tag); return nil }
func (e *events) Text(s string) error        { *e = append(*e, "t:"+s); return nil }
func (e *events) Raw(s string) error         { *e = append(*e, "r:"+s); return nil }
func (e *events) Opaque(ctx context.Context, n ui.Node) error {
	s, err := ui.String(ctx, n)
	*e = append(*e, "o:"+s)
	return err
}

func TestWalk(t *testing.T) {
	opaque := templ.Raw("<i>t</i>")
	tests := []struct {
		name string
		n    ui.Node
		want string
	}{
		{"element", ui.El("p", nil, ui.Text("a"), ui.El("br", nil)), "<p t:a <br /p"},
		{"flattened", ui.Fragment(ui.Child([]any{"a", 1}), ui.Nodes(func(add func(ui.Node)) { add(ui.Text("b")) }), nil), "t:a t:1 t:b"},
		{"raw", ui.Safe("<b>"), "r:<b>"},
		{"opaque", ui.El("div", nil, opaque), "<div o:<i>t</i> /div"},
		{"try", ui.Try(ui.Text("x"), nil), "t:x"},
		{"boundary replays", ui.ErrorBoundary(nil, ui.El("p", nil, ui.Text("a")), opaque), "<p t:a /p o:<i>t</i>"},
		{"boundary fallback", ui.Fragment(ui.Text("before"), ui.ErrorBoundary(
			func(err error) ui.Node { return ui.Text("fb:" + err.Error()) },
			ui.El("p", nil, ui.Text("lost")), ui.Try(nil, errBoom))), "t:before t:fb:boom"},
		{"boundary opaque error", ui.ErrorBoundary(func(error) ui.Node { return ui.Text("fb") }, failing()), "t:fb"},
		{"nested boundaries", ui.ErrorBoundary(func(error) ui.Node { return ui.Text("outer") },
			ui.ErrorBoundary(func(error) ui.Node { return ui.Text("inner") }, failing()), ui.Text("x")), "t:inner t:x"},
	}
	for _, tt := range tests {
		var e events
		if err := ui.Walk(context.Background(), tt.n, &e); err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got := strings.Join(e, " "); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	var e events
	if err := ui.Walk(context.Background(), ui.El("div", nil, ui.Text("a"), ui.Try(nil, errBoom), ui.Text("b")), &e); !errors.Is(err, errBoom) || len(e) != 2 {
		t.Errorf("error walk: %v %v", err, e)
	}
	if err := ui.Walk(context.Background(), ui.El("img", nil, ui.Text("x")), &e); err == nil {
		t.Error("void children walked")
	}
}

// The HTML a Node renders is a Walk into an HTMLRenderer.
func TestWalkHTMLMatchesRender(t *testing.T) {
	trees := []ui.Node{
		table(20),
		ui.El("html", nil, ui.El("body", nil,
			ui.El("script", nil, ui.Text("</script>")), ui.El("style", nil, ui.Text("a</b")),
			ui.ErrorBoundary(func(err error) ui.Node { return ui.Child(err) }, ui.Text("<ok>"), templ.Raw("<i>t</i>")),
			ui.El("a", []ui.Attr{{Name: "href", Value: "javascript:x"}, {Name: "style", Value: ui.Style{"color": "red"}}}, ui.Safe("&amp;")))),
	}
	for i, n := range trees {
		want, err := ui.String(context.Background(), n)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := ui.Walk(context.Background(), n, ui.NewHTMLRenderer(context.Background(), &b)); err != nil {
			t.Fatal(err)
		}
		if b.String() != want {
			t.Errorf("tree %d:\nwalk   %s\nrender %s", i, b.String(), want)
		}
	}
}

// A plain io.Writer (no WriteString) works too.
type plainWriter struct{ w io.Writer }

func (p plainWriter) Write(b []byte) (int, error) { return p.w.Write(b) }

func TestHTMLRendererPlainWriter(t *testing.T) {
	var b strings.Builder
	n := ui.El("p", nil, ui.Text("<x>"), templ.Raw("<i></i>"))
	if err := ui.Walk(context.Background(), n, ui.NewHTMLRenderer(context.Background(), plainWriter{&b})); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "<p>&lt;x&gt;<i></i></p>" {
		t.Fatal(got)
	}
}

func TestExportedTypes(t *testing.T) {
	el, ok := ui.El("div", []ui.Attr{{Name: "id", Value: "x"}}, ui.Text("a")).(*ui.Element)
	if !ok || el.Tag != "div" || len(el.Attrs) != 1 || el.Children[0] != ui.TextNode("a") {
		t.Fatalf("%#v", el)
	}
	for _, c := range []struct {
		n    ui.Node
		want string
	}{
		{ui.Safe("x"), "ui.RawHTML"}, {ui.Fragment(), "ui.Group"}, {ui.Nodes(nil), "ui.Builder"},
		{ui.Try(nil, nil), "ui.TryNode"}, {ui.ErrorBoundary(nil), "ui.Boundary"}, {ui.Child(nil), "ui.Group"},
	} {
		if got := fmt.Sprintf("%T", c.n); got != c.want {
			t.Errorf("%s, want %s", got, c.want)
		}
	}
}
