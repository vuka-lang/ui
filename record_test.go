package ui_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
	templruntime "github.com/a-h/templ/runtime"
	"github.com/vuka-lang/ui"
)

// gen is a component written as the vuka command instruments templ's code:
// body records through r and writes its expressions into w.
func gen(body func(ctx context.Context, w *templruntime.Buffer, r ui.Rec) error) ui.Node {
	return templruntime.GeneratedTemplate(func(in templruntime.GeneratedComponentInput) (err error) {
		w, ctx := in.Writer, in.Context
		buf, isBuf := templruntime.GetBuffer(w)
		if !isBuf {
			defer func() {
				if e := templruntime.ReleaseBuffer(buf); err == nil {
					err = e
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		return body(ctx, buf, ui.Record(ctx, buf))
	})
}

// item is <li>{ name }@badge</li>, the badge rendered only when name isn't
// empty, so that the expression and the component sit side by side.
func item(name string, badge ui.Node) ui.Node {
	return gen(func(ctx context.Context, w *templruntime.Buffer, r ui.Rec) error {
		r.S(w, 1, "<li>")
		w.WriteString(templ.EscapeString(name))
		r.D()
		r.Open()
		if name != "" {
			if err := r.Render(badge, ctx, w); err != nil {
				return err
			}
		}
		r.Close()
		return r.S(w, 2, "</li>")
	})
}

// list is <ul>for … { @item }</ul>.
func list(names ...string) ui.Node {
	return gen(func(ctx context.Context, w *templruntime.Buffer, r ui.Rec) error {
		r.S(w, 3, "<ul>")
		r.For()
		for _, n := range names {
			r.Item()
			if err := r.Render(item(n, ui.El("b", nil, ui.Text("new"))), ctx, w); err != nil {
				return err
			}
		}
		r.End()
		return r.S(w, 4, "</ul>")
	})
}

func treeHTML(t *testing.T, n ui.Node) (*ui.Tree, string) {
	t.Helper()
	tr, err := ui.BuildTree(context.Background(), n, true)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	tr.HTML(&b, nil)
	want, err := ui.String(context.Background(), n)
	if err != nil || b.String() != want {
		t.Fatalf("tree HTML %q, render %q %v", b.String(), want, err)
	}
	return tr, b.String()
}

func TestRecord(t *testing.T) {
	tr, html := treeHTML(t, list("a<", "", "c"))
	if html != `<ul><li>a&lt;<b>new</b></li><li></li><li>c<b>new</b></li></ul>` {
		t.Fatalf("html %s", html)
	}
	if len(tr.Statics) != 2 || tr.Statics[0] != "<ul>" {
		t.Fatalf("statics %q", tr.Statics)
	}
	l := tr.Dyn[0].(*ui.TreeList)
	if len(l.Items) != 3 {
		t.Fatalf("items %#v", l.Items)
	}
	// An empty expression keeps the frame's shape: every item has the same
	// statics, whatever it wrote.
	if a, b := l.Items[0].Key, l.Items[1].Key; a != b || len(l.Items[0].Statics) != 3 {
		t.Fatalf("items keyed %x %x, statics %q", a, b, l.Items[0].Statics)
	}
	// The same component in another render keys alike.
	if again, _ := treeHTML(t, list("x")); again.Key != tr.Key {
		t.Fatal("one component, two keys")
	}
}

// TestRecordAround: hand-written components around recorded ones, recorded
// ones writing into a buffer of their own, and errors.
func TestRecordAround(t *testing.T) {
	wrapper := ui.NodeFunc(func(ctx context.Context, w io.Writer) error {
		io.WriteString(w, "<div>")
		if err := list("in").Render(ctx, w); err != nil {
			return err
		}
		_, err := io.WriteString(w, "</div>")
		return err
	})
	if tr, _ := treeHTML(t, wrapper); tr.Statics[0] == "" && len(tr.Dyn) == 1 {
		if _, ok := tr.Dyn[0].(string); ok {
			t.Fatalf("a recorded component in a hand-written one isn't recorded: %#v", tr)
		}
	}
	// Rendered into a string of its own (an attribute value), a recorded
	// component only writes.
	attr := ui.El("p", []ui.Attr{{Name: "title", Value: list("t")}}, list("body"))
	treeHTML(t, ui.F(9, "Ed(h)", attr))

	// An uninstrumented templ component is one dynamic.
	plain := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<i>plain</i>")
		return err
	})
	if tr, _ := treeHTML(t, plain); len(tr.Dyn) != 1 || tr.Dyn[0] != "<i>plain</i>" {
		t.Fatalf("%#v", tr)
	}

	boom := errors.New("boom")
	failing := gen(func(ctx context.Context, w *templruntime.Buffer, r ui.Rec) error {
		r.S(w, 1, "<p>")
		r.Open()
		return r.Render(ui.Try(nil, boom), ctx, w)
	})
	if _, err := ui.BuildTree(context.Background(), list("a", "b"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := ui.BuildTree(context.Background(), ui.F(9, "Ed(h)", ui.El("p", []ui.Attr{{Name: "id", Value: "x"}}, failing)), true); !errors.Is(err, boom) {
		t.Fatalf("error %v", err)
	}
}
