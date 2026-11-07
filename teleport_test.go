package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

func TestTeleport(t *testing.T) {
	for _, c := range []struct {
		t    ui.Teleport
		want string
	}{
		{ui.Teleport{Children: ui.Text("hi")}, `<vk-teleport data-vk-teleport="body" style="display: contents;">hi</vk-teleport>`},
		{ui.Teleport{To: "#overlays", ID: "menu", Children: ui.Text("hi")}, `<vk-teleport id="menu" data-vk-teleport="#overlays" style="display: contents;">hi</vk-teleport>`},
	} {
		if got := render(t, c.t); got != c.want {
			t.Errorf("got  %s\nwant %s", got, c.want)
		}
	}
	for _, to := range []string{"main", "#", "#a b", ".x"} {
		var b strings.Builder
		if err := (ui.Teleport{To: to}).Render(context.Background(), &b); err == nil && !strings.Contains(b.String(), "Teleport") {
			t.Errorf("to=%q rendered %q", to, b.String())
		}
	}
}

// teleportFormPage writes a struct form's field in a Teleport: the field is
// still the form's.
type teleportFormPage struct {
	ui.Live
	Form ui.FormFor[registration]
}

func (p *teleportFormPage) Submit(context.Context, registration) error { return nil }

func (p *teleportFormPage) Render() ui.Node {
	return ui.Form{For: ui.For(&p.Form), OnSubmit: p.Submit, Children: ui.Teleport{Children: ui.Field{For: ui.For(&p.Form.Value.Email, &p.Form)}}}
}

func TestTeleportKeepsForm(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "registration", Props: &teleportFormPage{}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	tp := strings.Index(html, "<vk-teleport")
	if form := strings.Index(html, "<form"); form < 0 || tp < form || !strings.Contains(html[tp:], `id="registration-email"`) {
		t.Fatalf("the field in the Teleport isn't the form's:\n%s", html)
	}
	send(t, c, submitTarget, "submit", formValues(html, map[string]string{"email": ""}))
	if html = c.HTML(); !strings.Contains(html[strings.Index(html, "<vk-teleport"):], "Enter your email address") {
		t.Fatalf("the form's check isn't shown in the Teleport:\n%s", html)
	}
}
