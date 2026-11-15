package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// A ShowOnce renders the same element with a value and without one: the
// same id and marker, its children and key only while it has a Key.
func TestShowOnceMarkup(t *testing.T) {
	code := ui.El("code", nil, ui.Text("s3cr3t"))
	tests := []struct {
		name string
		n    ui.Node
		want string
	}{
		{"shown", ui.ShowOnce{ID: "token", Key: "k1", Children: code},
			`<div id="vk-once-token" data-vk-once="k1"><code>s3cr3t</code></div>`},
		{"gone", ui.ShowOnce{ID: "token", Children: code},
			`<div id="vk-once-token" data-vk-once=""></div>`},
		{"empty string key", ui.ShowOnce{ID: "token", Key: "", Children: code},
			`<div id="vk-once-token" data-vk-once=""></div>`},
		{"zero int key", ui.ShowOnce{ID: "token", Key: 0, Children: code},
			`<div id="vk-once-token" data-vk-once=""></div>`},
		{"int key", ui.ShowOnce{ID: "token", Key: int64(7), Children: code},
			`<div id="vk-once-token" data-vk-once="7"><code>s3cr3t</code></div>`},
		{"class", ui.ShowOnce{ID: "flash", Key: true, Class: "p-2", Children: ui.Text("Saved")},
			`<div id="vk-once-flash" data-vk-once="true" class="p-2">Saved</div>`},
	}
	for _, tt := range tests {
		if got := render(t, tt.n); got != tt.want {
			t.Errorf("%s:\n got %s\nwant %s", tt.name, got, tt.want)
		}
	}
	if ui.ShowOnceID("token") != "vk-once-token" {
		t.Errorf("ShowOnceID = %q", ui.ShowOnceID("token"))
	}
}

// Without a Key the children aren't rendered at all: a value the page no
// longer has isn't asked for.
func TestShowOnceGoneSkipsChildren(t *testing.T) {
	ran := false
	body := ui.Ctx(func(context.Context) ui.Node { ran = true; return ui.Text("x") })
	render(t, ui.ShowOnce{ID: "token", Children: body})
	if ran {
		t.Error("the children rendered with no Key")
	}
}

// A second value keeps the element's id and changes its key, which is
// what makes the browser take it in place of the first.
func TestShowOnceSecondValue(t *testing.T) {
	k1, k2 := ui.ShowOnceKey(), ui.ShowOnceKey()
	if k1 == "" || k1 == k2 {
		t.Fatalf("ShowOnceKey gave %q and %q", k1, k2)
	}
	a := render(t, ui.ShowOnce{ID: "token", Key: k1, Children: ui.Text("one")})
	b := render(t, ui.ShowOnce{ID: "token", Key: k2, Children: ui.Text("two")})
	for _, s := range []string{a, b} {
		if !strings.HasPrefix(s, `<div id="vk-once-token" data-vk-once="`) {
			t.Errorf("not the same place: %s", s)
		}
	}
	if !strings.Contains(a, `data-vk-once="`+k1+`"`) || !strings.Contains(b, `data-vk-once="`+k2+`"`) {
		t.Errorf("keys not rendered: %s / %s", a, b)
	}
}

func TestShowOnceNeedsID(t *testing.T) {
	if _, err := ui.String(context.Background(), ui.ShowOnce{Key: "k"}); err == nil || !strings.Contains(err.Error(), "needs an ID") {
		t.Errorf("err = %v", err)
	}
}
