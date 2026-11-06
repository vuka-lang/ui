package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/uigen"
)

// TestShowAttr: show hides an element while its value doesn't hold, a
// state's kept in the browser; transition carries its phases.
func TestShowAttr(t *testing.T) {
	open := uigen.State("o", false)
	out, _ := page(t, ui.HTML(`<div show={open} transition={tr}>a</div><p show={on}>b</p><p show={off}>c</p><i show={!open} transition={cl}></i>`,
		open, ui.Scale(150*time.Millisecond), true, false, uigen.Not(open),
		ui.Animate(200*time.Millisecond, ui.Enter("transition", "opacity-0", "opacity-100"), ui.Leave("transition", "opacity-100", "opacity-0"))))
	out = strings.ReplaceAll(out, "&#34;", `"`)
	for _, want := range []string{
		`<div hidden data-vk-v="{"i":false,"s":"c0:o:1"}"`,
		`data-vk-transition="{"ms":150,"enter":{"s":[{"transition-duration":"150ms"`,
		`{"opacity":"0","transform":"scale(0.95)"},{"opacity":"1","transform":"none"}]}`,
		`<p>b</p><p hidden>c</p>`,
		`<i data-vk-v="["!",`,
		`"enter":{"c":["transition","opacity-0","opacity-100"]},"leave":{"c":["transition","opacity-100","opacity-0"]}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
}
