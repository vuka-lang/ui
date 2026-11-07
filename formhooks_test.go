package ui_test

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/forms"
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

// CleanPrice is a form type's own Clean<Field>, run once Price's rules
// passed.
func (f *productForm) CleanPrice(ctx context.Context) error {
	if f.Value.Price == 13 {
		return forms.Errorf("Not 13")
	}
	return nil
}

func (f *productForm) Clean(ctx context.Context) error {
	switch {
	case f.Value.Name == "Closed":
		return forms.Errorf("The catalog is closed")
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

// TestFormForHooks: a struct embedding a FormFor is a form of its T; a new
// form runs its Init at its first render, with no Load, a Load again; its Clean and
// Clean<Field> methods run as T's would, a FieldError under its field, a
// form error the form's, either stopping the save.
func TestFormForHooks(t *testing.T) {
	products, saves, inits = &[]product{{ID: 1, Name: "Kettle", Price: 30, Status: "live", Stock: 5}}, nil, 0
	ctx := context.Background()
	page := &hookedPage{}
	s := live.NewSession(ctx, func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "products", Props: page, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	// a new form, no Load: Init ran at its first render
	if inits != 1 || !strings.Contains(html, `name="price" id="product-price" type="text" value="10"`) || strings.Contains(html, `name="mode"`) {
		t.Fatalf("a form type's form:\n%s", html)
	}
	send(t, c, submitTarget, "submit", formValues(html, map[string]string{"name": "Desk lamp", "price": "", "status": "live", "notes": ""}))
	if len(saves) != 0 || !strings.Contains(c.HTML(), "Enter price") || !strings.Contains(c.HTML(), "A live product needs notes") {
		t.Fatalf("the rules' and Clean's errors: %v\n%s", saves, c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Desk lamp", "price": "13", "status": "draft", "notes": ""}))
	if len(saves) != 0 || !strings.Contains(c.HTML(), "Not 13") {
		t.Fatalf("the form type's CleanPrice: %v\n%s", saves, c.HTML())
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
	if inits != 1 {
		t.Errorf("after the save: %d Init runs", inits)
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

type failingPage struct {
	ui.Live
	Form failingInit
}

func (p *failingPage) Render() ui.Node { return ui.Form{For: ui.For(&p.Form), Bound: []string{"Name"}} }

// TestFormForInitTriggers: Init runs once per instance of the form — at its
// first render, at a Load, after a Reset — and before a Save of a form
// never rendered; its error is the render's, and leaves the form for the
// next trigger.
func TestFormForInitTriggers(t *testing.T) {
	ctx := context.Background()
	products, saves, inits = &[]product{}, nil, 0
	var f productForm
	ui.Attach(&f)
	f.Value.Name = "Lamp"
	if err := f.Save(ctx); err != nil || inits != 1 || !slices.Equal(saves, []string{"insert"}) || (*products)[0].Status != "draft" {
		t.Errorf("Save of a form never rendered: %v, %d runs, %+v", err, inits, f.Value)
	}
	if _, err := ui.String(ctx, ui.Form{For: ui.For(&f), Bound: []string{"Name"}}); err != nil || inits != 1 {
		t.Errorf("a render after Init ran: %v, %d runs", err, inits)
	}
	f.Value.Name = "Desk"
	f.Reset()
	if _, err := ui.String(ctx, ui.Form{For: ui.For(&f), Bound: []string{"Name"}}); err != nil || inits != 2 || f.Value.Name != "Lamp" {
		t.Errorf("a render after Reset: %v, %d runs, %+v", err, inits, f.Value)
	}
	p := &failingPage{}
	if _, err := ui.String(ctx, p.Render()); err == nil || !strings.Contains(err.Error(), "no catalog") {
		t.Errorf("Init's error at render: %v", err)
	}
}

type failingInit struct{ ui.FormFor[product] }

func (*failingInit) Init(context.Context) error { return errors.New("no catalog") }

type badHook struct{ ui.FormFor[product] }

func (*badHook) Init(n int) error { return nil }

type twoChoices struct{ ui.FormFor[product] }

func (*twoChoices) StatusChoices() []ui.Option { return nil }

// TestFormForHookErrors: Init's error is Load's; a hook of another
// signature is the form's error, naming the one it should have, as is a
// method both the form type and its T declare.
func TestFormForHookErrors(t *testing.T) {
	ctx := context.Background()
	var f failingInit
	if err := ui.Attach(&f).Load(ctx, product{}); err == nil || err.Error() != "no catalog" {
		t.Errorf("Init's error: %v", err)
	}
	var b badHook
	err := ui.Attach(&b).Load(ctx, product{})
	if err == nil || !strings.Contains(err.Error(), "want func (f *badHook) Init(ctx context.Context, deps…) error") {
		t.Errorf("a hook of another signature: %v", err)
	}
	var two twoChoices
	err = ui.Attach(&two).Load(ctx, product{})
	if err == nil || !strings.Contains(err.Error(), "twoChoices.StatusChoices and product.StatusChoices") {
		t.Errorf("a method the form type and its T declare: %v", err)
	}
}

// modalPage holds its form in a hidden element, as a closed modal does,
// which the browser shows.
type modalPage struct {
	ui.Live
	Form  productForm
	eager bool
}

func (p *modalPage) Edit(ctx context.Context) error { return p.Form.Load(ctx, (*products)[0]) }

func (p *modalPage) Render() ui.Node {
	return ui.Fragment(
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(p.Edit)}}, ui.Text("edit")),
		ui.El("div", []ui.Attr{{Name: "style", Value: ui.Style{"display": "none"}}},
			ui.Form{For: ui.For(&p.Form), Eager: p.eager, Bound: []string{"Name", "Price"}, Children: ui.Fragment(
				ui.Field{For: ui.For(&p.Form.Value.Name, &p.Form)},
				ui.Field{For: ui.For(&p.Form.Value.Price, &p.Form)},
			)}))
}

func joinModal(t *testing.T, p *modalPage) *livetest.Client {
	t.Helper()
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "modal", Props: p, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var initTarget = regexp.MustCompile(`data-vk-on-vkinit="([^"]+)"`)

// TestFormForInitHidden: a form rendered hidden runs Init when the browser
// first shows it (its vkinit event), busy until then; eager runs it at the
// first render; a Load in the event that shows it runs Init once, and the
// form renders without the marker, so the browser sends no vkinit.
func TestFormForInitHidden(t *testing.T) {
	products, saves, inits = &[]product{{ID: 1, Name: "Kettle", Price: 30, Status: "live"}}, nil, 0
	c := joinModal(t, &modalPage{})
	html := c.HTML()
	m := initTarget.FindStringSubmatch(html)
	if inits != 0 || m == nil || !strings.Contains(html, `data-vk-init aria-busy="true"`) || strings.Contains(html, `value="10"`) {
		t.Fatalf("a hidden form ran Init, or isn't marked: %d runs\n%s", inits, html)
	}
	if _, err := c.Event(ui.Event{Target: m[1], Type: "vkinit"}); err != nil {
		t.Fatal(err)
	}
	html = c.HTML()
	if inits != 1 || !strings.Contains(html, `name="price" id="product-price" type="text" value="10"`) || strings.Contains(html, "data-vk-init") {
		t.Fatalf("vkinit: %d runs\n%s", inits, html)
	}

	inits = 0
	joinModal(t, &modalPage{eager: true})
	if inits != 1 {
		t.Errorf("eager: %d runs", inits)
	}

	inits = 0
	c = joinModal(t, &modalPage{})
	edit := regexp.MustCompile(`data-vk-on-click="([^"]+)"`).FindStringSubmatch(c.HTML())
	if _, err := c.Event(ui.Event{Target: edit[1], Type: "click"}); err != nil {
		t.Fatal(err)
	}
	if html := c.HTML(); inits != 1 || !strings.Contains(html, `value="Kettle"`) || strings.Contains(html, "data-vk-init") {
		t.Errorf("Load and show in one event: %d runs\n%s", inits, html)
	}
}
