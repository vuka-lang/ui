package ui_test

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// productForm is a form type: a FormFor of product with hooks, and a field
// of its own, before the FormFor, that is no form field.
type productForm struct {
	Mode string
	ui.FormFor[product]
}

var inits int

func (f *productForm) Init(ctx context.Context) error {
	inits++
	if f.IsNew() {
		f.Value.Price, f.Value.Status = 10, "draft"
	}
	return nil
}

func (f *productForm) Clean(ctx context.Context) error {
	switch {
	case f.Value.Name == "Closed":
		return errors.New("The catalog is closed")
	case f.Value.Status == "live" && f.Value.Notes.IsNone():
		return ui.FieldError("Notes", "A live product needs notes")
	}
	return nil
}

type hookedPage struct {
	ui.Live
	Form productForm
}

func (p *hookedPage) Save(ctx context.Context) error { return p.Form.Save(ctx) }

func (p *hookedPage) Edit(ctx context.Context) error { return p.Form.Load(ctx, (*products)[0]) }

var closedSave error

func (p *hookedPage) Close(ctx context.Context) error {
	p.Form.Value.Name = "Closed"
	closedSave = p.Form.Save(ctx)
	return nil
}

func (p *hookedPage) Render() ui.Node {
	return ui.Fragment(
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(p.Edit)}}, ui.Text("edit")),
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(p.Close)}}, ui.Text("close")),
		ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save, Bound: []string{"Name", "Price", "Status", "Notes"},
			Children: ui.Fragment(
				ui.Field{For: ui.For(&p.Form.Value.Name, &p.Form)},
				ui.Field{For: ui.For(&p.Form.Value.Price, &p.Form)},
				ui.Field{For: ui.For(&p.Form.Value.Status, &p.Form)},
				ui.Field{For: ui.For(&p.Form.Value.Notes, &p.Form)},
			)})
}

// TestFormForHooks: a struct embedding a FormFor is a form of its T; Load
// runs its Init, and its Clean runs once the other checks passed, its
// FieldError under its field, another error the form's, either stopping
// the save.
func TestFormForHooks(t *testing.T) {
	products, saves, inits = &[]product{{ID: 1, Name: "Kettle", Price: 30, Status: "live", Stock: 5}}, nil, 0
	ctx := context.Background()
	page := &hookedPage{}
	if err := ui.Attach(&page.Form).Load(ctx, product{}); err != nil {
		t.Fatal(err)
	}
	if inits != 1 || page.Form.Value.Price != 10 || page.Form.Dirty() {
		t.Fatalf("Init: %d runs, value %+v, dirty %v", inits, page.Form.Value, page.Form.Dirty())
	}
	s := live.NewSession(ctx, func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "products", Props: page, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	if !strings.Contains(html, `name="price" id="product-price" type="text" value="10"`) || strings.Contains(html, `name="mode"`) {
		t.Fatalf("a form type's form:\n%s", html)
	}
	send(t, c, submitTarget, "submit", formValues(html, map[string]string{"name": "Desk lamp", "price": "", "status": "live", "notes": ""}))
	if len(saves) != 0 || !strings.Contains(c.HTML(), "Enter price") || strings.Contains(c.HTML(), "A live product needs notes") {
		t.Fatalf("Clean ran before the field rules passed: %v\n%s", saves, c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Desk lamp", "price": "45", "status": "live", "notes": ""}))
	if len(saves) != 0 || !strings.Contains(c.HTML(), "A live product needs notes") {
		t.Fatalf("Clean's FieldError: %v\n%s", saves, c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Closed", "price": "45", "status": "draft", "notes": ""}))
	if len(saves) != 0 || !strings.Contains(c.HTML(), "The catalog is closed") || strings.Contains(c.HTML(), "A live product needs notes") {
		t.Fatalf("Clean's error: %v\n%s", saves, c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Desk lamp", "price": "45", "status": "live", "notes": "Brass"}))
	if !slices.Equal(saves, []string{"insert"}) || strings.Contains(c.HTML(), "The catalog is closed") {
		t.Fatalf("saves %v\n%s", saves, c.HTML())
	}
	if inits != 1 || page.Form.IsNew() {
		t.Errorf("after the save: %d Init runs, new %v", inits, page.Form.IsNew())
	}

	// loaded after its page attached it, a form type runs Init without Attach
	clicks := regexp.MustCompile(`data-vk-on-click="([^"]+)"`).FindAllStringSubmatch(c.HTML(), -1)
	if _, err := c.Event(ui.Event{Target: clicks[0][1], Type: "click"}); err != nil {
		t.Fatal(err)
	}
	if inits != 2 || !strings.Contains(c.HTML(), `value="Kettle"`) {
		t.Fatalf("Load of a row: %d Init runs\n%s", inits, c.HTML())
	}
	if _, err := c.Event(ui.Event{Target: clicks[1][1], Type: "click"}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(closedSave, ui.ErrFormInvalid) || !slices.Equal(saves, []string{"insert"}) || !strings.Contains(c.HTML(), "The catalog is closed") {
		t.Errorf("Save with Clean failing: %v, saves %v\n%s", closedSave, saves, c.HTML())
	}
}

type failingInit struct{ ui.FormFor[product] }

func (*failingInit) Init(context.Context) error { return errors.New("no catalog") }

type badHook struct{ ui.FormFor[product] }

func (*badHook) Init(n int) error { return nil }

// TestFormForHookErrors: Init's error is Load's; a hook of another
// signature is the form's error, naming the one it should have.
func TestFormForHookErrors(t *testing.T) {
	ctx := context.Background()
	var f failingInit
	if err := ui.Attach(&f).Load(ctx, product{}); err == nil || err.Error() != "no catalog" {
		t.Errorf("Init's error: %v", err)
	}
	var b badHook
	err := ui.Attach(&b).Load(ctx, product{})
	if err == nil || !strings.Contains(err.Error(), "want func (f *badHook) Init(ctx context.Context) error") {
		t.Errorf("a hook of another signature: %v", err)
	}
}
