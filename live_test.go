package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/vuka-lang/ui"
)

type tally struct {
	ui.Live
	Start int
	n     int
}

func (t *tally) Mount() { t.n += t.Start }
func (t *tally) Inc()   { t.n++ }
func (t *tally) Render() ui.Node {
	return ui.El("b", []ui.Attr{{Name: "onClick", Value: ui.On(t.Inc)}}, ui.Child(t.n))
}

func TestStaticStateful(t *testing.T) {
	n := ui.El("li", []ui.Attr{{Name: "key", Value: 7}}, ui.Component("x#1", nil, &tally{Start: 2}))
	for range 2 { // each render mounts a copy: the node is unchanged
		if got := render(t, n); got != `<li><b>2</b></li>` {
			t.Fatalf("got %s", got)
		}
	}
	script := ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(templ.ComponentScript{Call: "go()"})}})
	if got := render(t, script); got != `<button onClick="go()"></button>` {
		t.Fatalf("a script stays a script: %s", got)
	}
	var w recorder
	if err := ui.Walk(context.Background(), n, &w); err != nil || w.String() != "<li><b>2</b></li>" {
		t.Fatalf("walk: %q %v", w.String(), err)
	}
}

// recorder writes what a walk sends as bare tags and text.
type recorder struct{ b []byte }

func (r *recorder) Open(e *ui.Element) error  { r.b = append(r.b, "<"+e.Tag+">"...); return nil }
func (r *recorder) Close(e *ui.Element) error { r.b = append(r.b, "</"+e.Tag+">"...); return nil }
func (r *recorder) Text(s string) error       { r.b = append(r.b, s...); return nil }
func (r *recorder) Raw(s string) error        { r.b = append(r.b, s...); return nil }
func (r *recorder) Opaque(ctx context.Context, n ui.Node) error {
	s, err := ui.String(ctx, n)
	r.b = append(r.b, s...)
	return err
}
func (r *recorder) String() string { return string(r.b) }

type badMount struct{ ui.Live }

func (*badMount) Mount(int)       {}
func (*badMount) Render() ui.Node { return nil }

type ptrLive struct{ *ui.Live }

func (*ptrLive) Render() ui.Node { return nil }

// What the compiler no longer knows about is checked when it renders.
func TestRuntimeChecks(t *testing.T) {
	cases := []struct {
		n    ui.Node
		want string
	}{
		{ui.El("b", []ui.Attr{{Name: "onClick", Value: ui.On(func(a, b int) {})}}),
			"ui: onClick takes a func(), a func taking the event's value (a string, number or bool), a form (a struct, or url.Values) or a ui.Event, optionally after a context.Context, returning nothing or an error; this is func(int, int)"},
		{ui.El("b", []ui.Attr{{Name: "onClick", Value: ui.On(42)}}), "; this is int"},
		{ui.Component("x#1", nil, &badMount{}), "error[E0329]: *ui_test.badMount's Mount is func(int)\n  = help: Mount takes nothing or a context.Context, and returns nothing or an error"},
		{ui.Component("x#2", nil, &ptrLive{}), "error[E0329]: *ui_test.ptrLive embeds *ui.Live\n  = help: embed ui.Live by value"},
	}
	for _, c := range cases {
		_, err := ui.String(context.Background(), c.n)
		if err == nil || !strings.HasSuffix(err.Error(), c.want) && !strings.Contains(err.Error(), c.want) {
			t.Errorf("got %v, want …%s", err, c.want)
		}
	}
	for _, fn := range []any{func() {}, func(context.Context, string) error { return nil }, func(ui.Event) {}, func(*struct{ A int }) {}, func(map[string][]string) {}} {
		if err := ui.CheckHandler(fn); err != nil {
			t.Errorf("%T: %v", fn, err)
		}
	}
}
