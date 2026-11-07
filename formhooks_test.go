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
// form runs its Init as its page is created, a Load again; its Clean and
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
	// a new form, no Load: Init ran as the page was created
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

// mountedPage's Mount changes what its form's Init set.
type mountedPage struct {
	ui.Live
	Form productForm
}

func (p *mountedPage) LiveSchema(l *ui.LiveSchema)     { l.Mount(p.Mount) }
func (p *mountedPage) Mount(ctx context.Context) error { p.Form.Value.Price = 20; return nil }
func (p *mountedPage) Render() ui.Node                 { return nil }

type failingPage struct {
	ui.Live
	Form failingInit
}

func (p *failingPage) Render() ui.Node { return nil }

// TestFormForInitMount: a page's new form runs Init before the page's
// Mount, which may change what it set, and stays clean; Init's error fails
// the mount; a form Load gives a new record runs it again.
func TestFormForInitMount(t *testing.T) {
	ctx := context.Background()
	inits = 0
	p := &mountedPage{}
	if err := ui.MountLive(ctx, p); err != nil {
		t.Fatal(err)
	}
	if inits != 1 || p.Form.Value.Price != 20 || p.Form.Value.Status != "draft" || p.Form.Dirty() {
		t.Errorf("Init then Mount: %d runs, %+v, dirty %v", inits, p.Form.Value, p.Form.Dirty())
	}
	if err := p.Form.Load(ctx, product{}); err != nil || inits != 2 || p.Form.Value.Price != 10 {
		t.Errorf("Load: %v, %d runs, %+v", err, inits, p.Form.Value)
	}
	if err := ui.MountLive(ctx, &failingPage{}); err == nil || !strings.Contains(err.Error(), "no catalog") {
		t.Errorf("Init's error at mount: %v", err)
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
