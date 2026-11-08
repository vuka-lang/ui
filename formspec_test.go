package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/vuka/v2"
	"github.com/vuka-lang/vuka/v2/field"
)

// column is a model library's field attribute, as orm.Char is: limits,
// choices, a label and a default.
type column struct {
	Max     int
	Choices []string
	Label   string
	Default any
}

func (c column) Describe(s *field.Spec) {
	if c.Max > 0 {
		s.Max = field.Float(float64(c.Max))
	}
	for _, v := range c.Choices {
		s.Choices = append(s.Choices, field.Choice{Value: v, Label: strings.ToUpper(v[:1]) + v[1:]})
	}
	if c.Label != "" {
		s.Label = c.Label
	}
}

func (c column) FieldDefault() (any, bool) { return c.Default, c.Default != nil }

// purchase is a model whose forms word its fields their own way.
type purchase struct {
	Customer string
	Status   string
	Qty      int
	Ref      string
	Note     vuka.Option[string]
}

// purchaseForm describes purchase's fields in this form: ui.Meta members.
type purchaseForm struct {
	ui.FormFor[purchase]
	Customer ui.Meta
	Status   ui.Meta
	Ref      ui.Meta
	Note     ui.Meta
}

func init() {
	vuka.RegisterFields[purchase](map[string][]any{
		// ui's own attribute wins over the model library's, written first
		"Customer": {ui.Label("Buyer"), column{Max: 40, Label: "Client"}, ui.Placeholder("Model placeholder")},
		"Status":   {column{Max: 10, Choices: []string{"draft", "placed", "shipped"}, Default: "draft"}},
		"Qty":      {column{Default: 1}},
	})
	vuka.RegisterFields[purchaseForm](map[string][]any{
		"Customer": {ui.Spec{Required: "Who is the purchase for?", Placeholder: "e.g. Ada", Max: 12, Msg: ui.Msgs{"max": "{Label}: {n} at most"}}},
		"Status":   {ui.Spec{Choices: []string{"placed", "draft"}, Widget: "radio"}},
		"Ref":      {ui.Spec{ReadOnly: true, Optional: true}},
		"Note":     {ui.Spec{Required: "Add a note", Help: "For the courier"}},
	})
}

type purchasePage struct {
	ui.Live
	Form purchaseForm
	got  *[]purchase
}

func (p *purchasePage) Submit(ctx context.Context, in purchase) error {
	*p.got = append(*p.got, in)
	return nil
}

func (p *purchasePage) Render() ui.Node {
	return ui.Form{For: ui.For(&p.Form), OnSubmit: p.Submit, Bound: []string{"Customer", "Status", "Qty", "Ref", "Note"},
		Children: ui.Fragment(
			ui.Field{For: ui.For(&p.Form.Value.Customer, &p.Form)},
			ui.Field{For: ui.For(&p.Form.Value.Status, &p.Form)},
			ui.Field{For: ui.For(&p.Form.Value.Qty, &p.Form)},
			ui.Field{For: ui.For(&p.Form.Value.Ref, &p.Form)},
			ui.Field{For: ui.For(&p.Form.Value.Note, &p.Form)},
		)}
}

// TestFormSpec: a form type's ui.Meta members describe T's fields in that
// form, over the model's attributes, which ui's own win over; a ui.Spec
// tightens limits and narrows choices; defaults start a new record; a
// read-only field keeps the server's value.
func TestFormSpec(t *testing.T) {
	var got []purchase
	p := &purchasePage{got: &got}
	p.Form.Value.Ref = "R-1"
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "purchases", Props: p, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	for _, want := range []string{
		`>Buyer<`,                                    // the model's ui.Label over its column's
		`placeholder="e.g. Ada"`,                     // the form's over the model's
		`value="placed"`, `>Placed<`, `type="radio"`, // narrowed, the model's labels kept, a radio
		`value="R-1" readonly`,
		`For the courier`,
		`id="purchase-qty" type="text" value="1"`, `value="draft" name="status" type="radio" checked`, // a new record's defaults
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	for _, not := range []string{"Model placeholder", `value="shipped"`, `>Client<`} {
		if strings.Contains(html, not) {
			t.Errorf("%s in\n%s", not, html)
		}
	}
	send(t, c, submitTarget, "submit", formValues(html, map[string]string{"customer": "", "status": "shipped", "qty": "2", "ref": "R-9", "note": ""}))
	html = c.HTML()
	for _, want := range []string{"Who is the purchase for?", "Add a note", "Select status from the list"} {
		if !strings.Contains(html, want) {
			t.Errorf("no error %q in\n%s", want, html)
		}
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"customer": "A much longer name", "status": "placed", "qty": "2", "note": "Ring"}))
	if !strings.Contains(c.HTML(), "Buyer: 12 at most") {
		t.Errorf("the form's Max and its message:\n%s", c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"customer": "Ada", "status": "placed", "qty": "2", "ref": "R-9", "note": "Ring"}))
	if len(got) != 1 || got[0].Ref != "R-1" || got[0].Customer != "Ada" || got[0].Note.UnwrapOr("") != "Ring" {
		t.Fatalf("submitted %+v\n%s", got, c.HTML())
	}
}

type loose struct{ Name string }
type looseMax struct {
	ui.FormFor[loose]
	Name ui.Meta
}
type looseChoice struct {
	ui.FormFor[loose]
	Name ui.Meta
}
type looseMsg struct {
	ui.FormFor[loose]
	Name ui.Meta
}
type looseBoth struct {
	ui.FormFor[loose]
	Name ui.Meta
}
type strayMeta struct {
	ui.FormFor[loose]
	Nmae ui.Meta
}

func init() {
	vuka.RegisterFields[loose](map[string][]any{"Name": {column{Max: 20, Choices: []string{"a", "b"}}}})
	vuka.RegisterFields[looseMax](map[string][]any{"Name": {ui.Spec{Max: 30}}})
	vuka.RegisterFields[looseChoice](map[string][]any{"Name": {ui.Spec{Choices: []string{"a", "c"}}}})
	vuka.RegisterFields[looseMsg](map[string][]any{"Name": {ui.Spec{Msg: ui.Msgs{"maxx": "Too long"}}}})
	vuka.RegisterFields[looseBoth](map[string][]any{"Name": {ui.Spec{Required: "Enter it", Optional: true}}})
}

// TestFormSpecErrors: a ui.Spec loosening the model's limits or choices,
// naming a rule there is none of, or both requiring and not; a ui.Meta
// naming no field of T.
func TestFormSpecErrors(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name string
		load func() error
		want string
	}{
		{"Max", func() error { return ui.Attach(&looseMax{}).Load(ctx, loose{}) }, "Max 30 is above the model's 20"},
		{"Choices", func() error { return ui.Attach(&looseChoice{}).Load(ctx, loose{}) }, `the choice "c" isn't one of the model's`},
		{"Msg", func() error { return ui.Attach(&looseMsg{}).Load(ctx, loose{}) }, `Msg "maxx" is no rule`},
		{"Required", func() error { return ui.Attach(&looseBoth{}).Load(ctx, loose{}) }, "Required and Optional"},
		{"Meta", func() error { return ui.Attach(&strayMeta{}).Load(ctx, loose{}) }, "strayMeta.Nmae is a ui.Meta"},
	} {
		if err := c.load(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}

// TestFormSpecDefaults: defaults start a new record only: a plain
// struct's form that loaded one edits it (it has no key to say it's new).
func TestFormSpecDefaults(t *testing.T) {
	f := ui.Attach(&purchaseForm{})
	if err := f.Load(context.Background(), purchase{Qty: 3}); err != nil {
		t.Fatal(err)
	}
	if f.Value.Status != "" || f.Value.Qty != 3 || f.Dirty() {
		t.Errorf("a loaded record: %+v, dirty %v", f.Value, f.Dirty())
	}
}

type widgetValue struct{ Rows int }
type looseWidget struct {
	ui.FormFor[loose]
	Name ui.Meta
}

func init() {
	vuka.RegisterFields[looseWidget](map[string][]any{"Name": {ui.Spec{Widget: widgetValue{4}}}})
}

// TestFormSpecWidget: Widget takes a widget or a control's name; another
// value is the form's error, not ignored.
func TestFormSpecWidget(t *testing.T) {
	if err := ui.Attach(&looseWidget{}).Load(context.Background(), loose{}); err == nil || !strings.Contains(err.Error(), "Widget is a widget (widget.Textarea{Rows: 4}) or a control's name") {
		t.Errorf("a widget value: %v", err)
	}
}
