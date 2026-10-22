package live_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

// TestRenderPageMatchesSession checks ui.RenderPage against a session's
// first render: the same HTML for a page with nothing live, and Live for
// one with something.
func TestRenderPageMatchesSession(t *testing.T) {
	inc := func() {}
	opaque := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<i data-vk-js-click="[]">x</i>`)
		return err
	})
	cases := []struct {
		name           string
		page           ui.Node
		live, js, opaq bool
	}{
		{name: "plain", page: ui.El("html", nil, ui.El("body", []ui.Attr{a("class", "x")}, ui.Text("a < b"), ui.El("a", []ui.Attr{a("href", "javascript:x")}, ui.Text("l"))))},
		{name: "keyed", page: ui.El("ul", nil, ui.El("li", []ui.Attr{a("key", 7)}, ui.Text("seven")), ui.El("li", []ui.Attr{a("key", "a&b")}, ui.Text("ab")))},
		{name: "js", page: ui.El("button", []ui.Attr{a("onClick", ui.On(ui.JS.Toggle(ui.Ref("menu"))))}, ui.Text("menu")), js: true},
		{name: "push", page: ui.El("div", nil, ui.El("button", []ui.Attr{a("onClick", ui.On(ui.JS.Toggle(ui.Ref("menu")).Push(inc)))}, ui.Text("menu"))), live: true, js: true},
		{name: "handler", page: ui.El("div", nil, ui.El("button", []ui.Attr{a("onClick", ui.On(inc))}, ui.Text("+"))), live: true},
		{name: "component", page: ui.El("main", nil, ui.Text("before"), ui.Component("site", nil, &Counter{Label: "c"})), live: true},
		{name: "boundary", page: ui.ErrorBoundary(func(error) ui.Node { return ui.Text("failed") }, ui.Component("site", nil, &Counter{Label: "c"})), live: true},
		{name: "templ", page: ui.El("div", nil, opaque), opaq: true},
		{name: "safe", page: ui.El("div", nil, ui.Safe(`<b data-vk-js-click="[]">x</b>`)), opaq: true},
		{name: "safe-plain", page: ui.El("div", nil, ui.Safe(`<b>x</b>`))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			info, err := ui.RenderPage(context.Background(), c.page, &b)
			if err != nil {
				t.Fatal(err)
			}
			if info.Live != c.live || !c.live && (info.JS != c.js || info.Opaque != c.opaq) {
				t.Fatalf("info %+v, want live %v js %v opaque %v", info, c.live, c.js, c.opaq)
			}
			if c.live {
				return
			}
			s := live.NewSession(context.Background(), func(context.Context) ui.Node { return c.page })
			defer s.Close()
			want, err := s.Render()
			if err != nil {
				t.Fatal(err)
			}
			if b.String() != want {
				t.Fatalf("RenderPage wrote\n%s\nthe session\n%s", b.String(), want)
			}
		})
	}
}
