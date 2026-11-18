package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// Dialog's Size picks a width class: "xl" and "full" join "sm" and "lg", the
// stylesheet has a rule for each that beats the dialog's own width, and any
// other size is the default.
func TestDialogSize(t *testing.T) {
	for _, c := range []struct{ size, class string }{
		{"", "vk-dialog"}, {"md", "vk-dialog"}, {"huge", "vk-dialog"},
		{"sm", "vk-dialog vk-dialog--sm"}, {"lg", "vk-dialog vk-dialog--lg"},
		{"xl", "vk-dialog vk-dialog--xl"}, {"full", "vk-dialog vk-dialog--full"},
	} {
		out, err := ui.String(context.Background(), ui.Dialog{Size: c.size, Open: true})
		if want := `<dialog class="` + c.class + `" open>`; err != nil || !strings.HasPrefix(out, want) {
			t.Errorf("Size %q: %s %v, want %s", c.size, out, err, want)
		}
	}
	for _, rule := range []string{
		"dialog.vk-dialog--sm{width:min(24rem,",
		"dialog.vk-dialog--lg{width:min(48rem,",
		"dialog.vk-dialog--xl{width:min(72rem,calc(100vw - 2rem))}",
		"dialog.vk-dialog--full{width:calc(100vw - 2rem);height:calc(100vh - 2rem)}",
	} {
		if !strings.Contains(ui.Styles, rule) {
			t.Errorf("the stylesheet has no %q", rule)
		}
	}
}
