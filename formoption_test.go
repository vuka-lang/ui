package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/vuka/v2"
)

type delivery struct {
	Note vuka.Option[string]
}

func init() {
	vuka.RegisterFields[delivery](map[string][]any{"Note": {ui.Max(10)}})
}

type deliveryPage struct {
	ui.Live
	Form  ui.FormFor[delivery]
	saved *[]delivery
}

func (p *deliveryPage) Submit(ctx context.Context, in delivery) error {
	*p.saved = append(*p.saved, in)
	return nil
}

// Render is what the compiler writes for
// <ui.Form for={Form} onSubmit={Submit} keep><input model={Form.Note}/></ui.Form>.
func (p *deliveryPage) Render() ui.Node {
	return ui.Form{For: ui.For(&p.Form, p), OnSubmit: p.Submit, Keep: true, Bound: []string{"Note"},
		Children: ui.HTML(`<input {a...}/>`, ui.Model(ui.ModelSpec{Tag: "input"}, &p.Form.Value.Note, &p.Form, p))}
}

// TestModelOption: a control's model on an Option field shows the value
// it holds, binds an empty input as None and any other as Some, trimmed
// and held to the field's attributes.
func TestModelOption(t *testing.T) {
	var saved []delivery
	p := &deliveryPage{saved: &saved}
	p.Form.Load(context.Background(), delivery{Note: vuka.Some("fragile")})
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "deliveries", Props: p, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if html := c.HTML(); !strings.Contains(html, `name="note"`) || !strings.Contains(html, `value="fragile"`) {
		t.Fatalf("the loaded value doesn't render:\n%s", html)
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"note": "  "}))
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"note": " gift wrap "}))
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"note": "leave at the door"}))
	if len(saved) != 2 || saved[0].Note.IsSome() || saved[1].Note != vuka.Some("gift wrap") {
		t.Fatalf("saved %+v", saved)
	}
	if !strings.Contains(c.HTML(), "10 characters or less") {
		t.Errorf("a value over the field's limit saves:\n%s", c.HTML())
	}
}
