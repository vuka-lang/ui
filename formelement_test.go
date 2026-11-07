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

type stockItem struct {
	Name     string
	Category string
	Rating   int
	Notes    string
}

func init() {
	vuka.RegisterFields[stockItem](map[string][]any{
		"Name":     {ui.Required(""), ui.Help("As shown in the shop")},
		"Category": {ui.Required("")},
		"Rating":   {ui.Min(1), ui.Max(5)},
	})
}

func (stockItem) CategoryChoices() []ui.Option {
	return []ui.Option{{Value: "tea", Label: "Tea", Group: "Drinks"}, {Value: "coffee", Label: "Coffee", Group: "Drinks"}, {Value: "cake", Label: "Cake"}}
}

type stockPage struct {
	ui.Live
	Form  ui.FormFor[stockItem]
	Other ui.FormFor[stockItem]
	ID    string
	twice bool
	saved int
}

func (p *stockPage) Save(ctx context.Context, in stockItem) error {
	p.saved++
	return nil
}

// Render is what the compiler writes for
//
//	<form for={Form} onSubmit={Save} id={ID} class="stack" autocomplete="off">
//		<label for={Form.Name}/>
//		<input model={Form.Name}/>
//		<select model={Form.Category}></select>
//		<input type="range" model={Form.Rating}/>
//	</form>
//	<button form={Form}>Save</button>
func (p *stockPage) Render() ui.Node {
	form := ui.FormElement{For: ui.For(&p.Form, p), OnSubmit: p.Save, ID: p.ID, Class: "stack",
		Attrs: []ui.Attr{{Name: "autocomplete", Value: "off"}}, Bound: []string{"Name", "Category", "Rating"},
		Children: ui.HTML(`<label for={a}>{b}</label><input {c...}/><select {d...}>{e}</select><input type="range" {f...}/>`,
			ui.ModelID(&p.Form.Value.Name, &p.Form, p), ui.ModelLabel(&p.Form.Value.Name, &p.Form, p),
			ui.Model(ui.ModelSpec{Tag: "input"}, &p.Form.Value.Name, &p.Form, p),
			ui.Model(ui.ModelSpec{Tag: "select"}, &p.Form.Value.Category, &p.Form, p), ui.ModelOptions(&p.Form.Value.Category, &p.Form, p),
			ui.Model(ui.ModelSpec{Tag: "input", Type: "range", Own: "type"}, &p.Form.Value.Rating, &p.Form, p))}
	var other ui.Node
	if p.twice {
		other = ui.FormElement{For: ui.For(&p.Other, p), Bound: []string{"Name"},
			Children: ui.HTML(`<input {a...}/>`, ui.Model(ui.ModelSpec{Tag: "input"}, &p.Other.Value.Name, &p.Other, p))}
	}
	return ui.HTML(`<main>{a}{b}<button form={c}>Save</button></main>`, form, other, ui.ModelID(&p.Form, p))
}

func renderStock(t *testing.T, p *stockPage) (string, error) {
	t.Helper()
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "stock", Props: p, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		return "", err
	}
	return c.HTML(), nil
}

// TestFormElement: a <form for> is <ui.Form for>'s form on the page's own
// element — its class only the page's, its other attributes passed on —
// with an id generated from the form's type, and the controls' own
// attributes.
func TestFormElement(t *testing.T) {
	html, err := renderStock(t, &stockPage{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<form class="stack" method="post" id="stock-item" novalidate data-vk-form="stock-item"`,
		`autocomplete="off"`,
		`<label for="stock-item-name">Name</label>`,
		`aria-required="true"`,
		`<option value="">Choose…</option><optgroup label="Drinks"><option value="tea">Tea</option><option value="coffee">Coffee</option></optgroup><option value="cake">Cake</option>`,
		`type="range"`, `min="1" max="5"`,
		`<button form="stock-item">Save</button>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s\nin %s", want, html)
		}
	}
	if strings.Contains(html, "vk-form ") || strings.Contains(html, `class="vk-form`) {
		t.Errorf("a <form for> has ui.Form's class:\n%s", html)
	}
}

// TestFormElementWrittenID: an id written on the form is its element's,
// and form={Form} names it; the controls' ids keep the form's identity.
func TestFormElementWrittenID(t *testing.T) {
	html, err := renderStock(t, &stockPage{ID: "edit-item"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="edit-item"`, `data-vk-form="stock-item"`, `<button form="edit-item">`, `for="stock-item-name"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s\nin %s", want, html)
		}
	}
}

// TestFormElementTwice: two forms of one type on a page without keys
// would share their ids, so the render fails, saying to key them.
func TestFormElementTwice(t *testing.T) {
	_, err := renderStock(t, &stockPage{twice: true})
	if err == nil || !strings.Contains(err.Error(), "give each a key") {
		t.Fatalf("two unkeyed forms of one type: err = %v", err)
	}
}

// TestFieldAccessors: a component laying out its own label, hint and
// error reads them from its Model.
func TestFieldAccessors(t *testing.T) {
	p := &stockPage{}
	ref := ui.For(&p.Form.Value.Name, &p.Form, p)
	out, err := ui.String(context.Background(), ui.HTML(`<p>{a}|{b}|{c}</p>`, ui.LabelText(ref), ui.HelpText(ref), ui.ErrorText(ref)))
	if err != nil {
		t.Fatal(err)
	}
	if out != `<p>Name|As shown in the shop|</p>` {
		t.Errorf("got %s", out)
	}
}

// TestFieldsetAttrs: a <fieldset for> of a field with choices carries its
// error state and name; one of a struct field, nothing.
func TestFieldsetAttrs(t *testing.T) {
	p := &stockPage{}
	out, err := ui.String(context.Background(), ui.HTML(`<fieldset {a...}><legend>{b}</legend></fieldset>`,
		ui.FieldsetAttrs(&p.Form.Value.Category, &p.Form, p), ui.ModelLabel(&p.Form.Value.Notes, &p.Form, p)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-vk-name="category"`, `<legend>Notes</legend>`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}

type resetPage struct {
	ui.Live
	Form   ui.FormFor[stockItem]
	resets int
}

func (p *resetPage) Reset(ctx context.Context) error { p.resets++; return nil }

func (p *resetPage) Render() ui.Node {
	return ui.FormElement{For: ui.For(&p.Form, p), OnReset: p.Reset, OnSuccess: ui.JS.Hide(ui.Sel("#dialog")), OnError: ui.JS.Show(ui.Sel("#help")),
		Bound: []string{"Name"}, Children: ui.HTML(`<input {a...}/>`, ui.Model(ui.ModelSpec{Tag: "input"}, &p.Form.Value.Name, &p.Form, p))}
}

// TestFormElementEvents: a form's OnSuccess and OnError chains are its
// element's, for the browser to run after a submit's reply; a reset (sent
// as a submit with the "reset" intent) puts the loaded values back and
// runs OnReset.
func TestFormElementEvents(t *testing.T) {
	p := &resetPage{}
	p.Form.Load(context.Background(), stockItem{Name: "Tea", Category: "tea", Rating: 3})
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "reset", Props: p, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	for _, want := range []string{`data-vk-js-vksuccess=`, `data-vk-js-vkerror=`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in %s", want, html)
		}
	}
	p.Form.Value.Name = "Coffee"
	send(t, c, submitTarget, "submit", formValues(html, map[string]string{"name": "Cake", ui.FormIntentField: "reset"}))
	if p.Form.Value.Name != "Tea" || p.resets != 1 {
		t.Errorf("after a reset: name %q, %d OnReset calls", p.Form.Value.Name, p.resets)
	}
}
