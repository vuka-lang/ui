package ui_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/vuka/v2"
	"github.com/vuka-lang/vuka/v2/field"
)

// registration is a plain struct a FormFor edits, its fields described by
// attributes, as the compiler registers them.
type registration struct {
	Email    string
	Password string
	Plan     string
	Referrer vuka.Option[string]
	Terms    bool
	Seats    int
}

func init() {
	vuka.RegisterFields[registration](map[string][]any{
		"Email":    {ui.Email(""), ui.Label("Email address")},
		"Password": {ui.Min(12), ui.Widget("password")},
		"Plan":     {ui.OneOf("free", "pro")},
		"Referrer": {ui.Max(10)},
		"Terms":    {ui.Required("Accept the terms to continue")},
	})
	vuka.RegisterFields[product](map[string][]any{
		"ID":    {key{}},
		"Name":  {ui.Max(20), uniqueName{}, ui.Placeholder("e.g. Desk lamp")},
		"Price": {ui.Min(0)},
		"Notes": {ui.Widget("textarea")},
	})
}

func (s registration) Clean() error {
	if s.Plan == "pro" && s.Referrer.IsSome() {
		return ui.FieldError(vuka.RefTo(func(s *registration) *vuka.Option[string] { return &s.Referrer }), "Referrals are for the free plan")
	}
	return nil
}

type registrationPage struct {
	ui.Live
	Form   ui.FormFor[registration]
	keep   bool
	joined *[]registration
}

func (p *registrationPage) Submit(ctx context.Context, in registration) error {
	if in.Email == "taken@example.com" {
		return ui.FieldError("Email", "That email is already registered")
	}
	*p.joined = append(*p.joined, in)
	return nil
}

func (p *registrationPage) Render() ui.Node {
	return ui.Form{For: ui.For(&p.Form), OnSubmit: p.Submit, Keep: p.keep}
}

func joinRegistration(t *testing.T, p *registrationPage) *livetest.Client {
	t.Helper()
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "registration", Props: p, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestFormForPlain: a FormFor of a plain struct takes its fields' rules
// from their attributes and types, its handler the checked value; a
// FieldError is its field's; success empties the form, unless Keep.
func TestFormForPlain(t *testing.T) {
	var joined []registration
	c := joinRegistration(t, &registrationPage{joined: &joined})
	html := c.HTML()
	for _, want := range []string{
		`for="registration-email">Email address`,
		`name="email" id="registration-email" type="email"`,
		`name="password" id="registration-password" type="password"`,
		`<option value="free"`,
		`for="registration-referrer">Referrer<span class="vk-field__marker"> (optional)</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	send(t, c, submitTarget, "submit", formValues(html, map[string]string{"email": "", "password": "short", "plan": "gold", "seats": ""}))
	html = c.HTML()
	for _, want := range []string{"Enter your email address", "Password must be 12 characters or more", "Select plan from the list", "Accept the terms to continue", "Enter seats"} {
		if !strings.Contains(html, want) {
			t.Errorf("no error %q in\n%s", want, html)
		}
	}
	good := map[string]string{"email": "ada@example.com", "password": "a long password", "plan": "pro", "referrer": "", "terms": "true", "seats": "0"}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range good {
			m[a] = b
		}
		m[k] = v
		return m
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), with("referrer", "friend")))
	if len(joined) != 0 || !strings.Contains(c.HTML(), "Referrals are for the free plan") {
		t.Fatalf("Clean's FieldError isn't its field's: %v\n%s", joined, c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), with("email", "taken@example.com")))
	if len(joined) != 0 || !strings.Contains(c.HTML(), "That email is already registered") {
		t.Fatalf("the handler's FieldError isn't its field's:\n%s", c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), good))
	if len(joined) != 1 || joined[0].Email != "ada@example.com" || joined[0].Referrer.IsSome() || !joined[0].Terms || joined[0].Seats != 0 {
		t.Fatalf("joined %+v\n%s", joined, c.HTML())
	}
	if strings.Contains(c.HTML(), `value="ada@example.com"`) {
		t.Errorf("a successful submit leaves its values:\n%s", c.HTML())
	}

	joined = nil
	c = joinRegistration(t, &registrationPage{joined: &joined, keep: true})
	send(t, c, submitTarget, "submit", formValues(c.HTML(), with("referrer", "")))
	if len(joined) != 1 || !strings.Contains(c.HTML(), `value="ada@example.com"`) {
		t.Errorf("keep: %v\n%s", joined, c.HTML())
	}
}

// key marks a record's primary key, as orm.PK does.
type key struct{}

func (key) Describe(s *field.Spec) { s.Key = true }

// uniqueName refuses a name another product holds, as orm.Char{Unique}
// does.
type uniqueName struct{}

func (uniqueName) Check(ctx context.Context, v field.Value) error {
	rec := v.Record.(*product)
	for _, p := range *products {
		if p.Name == v.Value && p.ID != rec.ID {
			return &field.Invalid{Rule: "unique"}
		}
	}
	return nil
}

var products *[]product

// product is a model, as an orm model saves itself.
type product struct {
	ID     int64
	Name   string
	Price  int64
	Status string
	Notes  vuka.Option[string]
	Stock  int // a column no form sets
}

func (p *product) StatusChoices() []ui.Option {
	return []ui.Option{{Value: "draft", Label: "Draft"}, {Value: "live", Label: "Live"}}
}

var saves []string

func (p *product) Save(context.Context) error {
	p.ID = int64(len(*products) + 1)
	*products = append(*products, *p)
	saves = append(saves, "insert")
	return nil
}

func (p *product) SaveFields(_ context.Context, fields ...any) error {
	var names []string
	for _, f := range fields {
		names = append(names, f.(string))
	}
	saves = append(saves, "update "+strings.Join(names, ","))
	for i := range *products {
		if (*products)[i].ID == p.ID {
			stock := (*products)[i].Stock
			(*products)[i] = *p
			(*products)[i].Stock = stock
		}
	}
	return nil
}

func (p *product) Refresh(context.Context) error {
	for _, q := range *products {
		if q.ID == p.ID {
			*p = q
		}
	}
	return nil
}

type productsPage struct {
	ui.Live
	Form ui.FormFor[product]
}

func (p *productsPage) Add(ctx context.Context) error { return p.Form.Load(ctx, product{}) }

func (p *productsPage) Edit(ctx context.Context, id int64) error {
	for _, q := range *products {
		if q.ID == id {
			return p.Form.Load(ctx, q)
		}
	}
	return nil
}

func (p *productsPage) Save(ctx context.Context) error { return p.Form.Save(ctx) }

func (p *productsPage) Render() ui.Node {
	title := "Edit product"
	if p.Form.IsNew() {
		title = "Add product"
	}
	return ui.Fragment(ui.El("h1", nil, ui.Text(title)), ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save, Bound: []string{"Name", "Price", "Status", "Notes"},
		Children: ui.Fragment(
			ui.Field{For: ui.For(&p.Form.Value.Name, &p.Form)},
			ui.Field{For: ui.For(&p.Form.Value.Price, &p.Form)},
			ui.Field{For: ui.For(&p.Form.Value.Status, &p.Form)},
			ui.Field{For: ui.For(&p.Form.Value.Notes, &p.Form)},
		)})
}

// TestFormForModel: a model's FormFor inserts a new record and updates only
// its own fields of a loaded one, then reads it again; its Checker refuses
// a value another record holds, but not the record's own.
func TestFormForModel(t *testing.T) {
	products, saves = &[]product{{ID: 1, Name: "Kettle", Price: 30, Status: "live", Stock: 5}}, nil
	page := &productsPage{}
	page.Form.Load(context.Background(), product{})
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "products", Props: page, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	for _, want := range []string{"<h1>Add product</h1>", `placeholder="e.g. Desk lamp"`, `<textarea`, `<option value="live"`, `name="price" id="product-price" type="text" value=""`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	if strings.Contains(html, `name="iD"`) || strings.Contains(html, `name="stock"`) {
		t.Errorf("a field the markup doesn't bind renders:\n%s", html)
	}
	send(t, c, submitTarget, "submit", formValues(html, map[string]string{"name": "Kettle", "price": "", "status": "live", "stock": "99"}))
	if len(saves) != 0 || !strings.Contains(c.HTML(), "Name is already taken") || !strings.Contains(c.HTML(), "Enter price") {
		t.Fatalf("a taken name or an empty price saved: %v\n%s", saves, c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Desk lamp", "price": "0", "status": "draft", "notes": "", "stock": "99"}))
	if !slices.Equal(saves, []string{"insert"}) || len(*products) != 2 {
		t.Fatalf("saves %v, products %+v\n%s", saves, *products, c.HTML())
	}
	lamp := (*products)[1]
	if lamp.Name != "Desk lamp" || lamp.Notes.IsSome() || lamp.Stock != 0 {
		t.Errorf("inserted %+v", lamp)
	}
	if !strings.Contains(c.HTML(), "<h1>Edit product</h1>") || !strings.Contains(c.HTML(), `name="price" id="product-price" type="text" value="0"`) {
		t.Errorf("a saved record's form:\n%s", c.HTML())
	}

	(*products)[1].Stock = 7 // changed meanwhile
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Desk lamp", "price": "45", "status": "live", "notes": "Brass"}))
	if !slices.Equal(saves, []string{"insert", "update Name,Price,Status,Notes"}) {
		t.Fatalf("saves %v\n%s", saves, c.HTML())
	}
	if got := (*products)[1]; got.Price != 45 || got.Notes.Value() != "Brass" || got.Stock != 7 {
		t.Errorf("updated %+v", got)
	}
}

// TestFormForFixed: a FormFor never edits its record's key.
func TestFormForFixed(t *testing.T) {
	var f ui.FormFor[product]
	_, err := ui.String(context.Background(), ui.Form{For: ui.For(&f), Bound: []string{"ID", "Name"}})
	if err == nil || !strings.Contains(err.Error(), "ID is the record's primary key, never a form's field") {
		t.Errorf("binding the key: %v", err)
	}
	_, err = ui.String(context.Background(), ui.Form{For: ui.For(&f), Bound: []string{}, Children: ui.Text("x")})
	if err == nil || !strings.Contains(err.Error(), "@ui.Fields") {
		t.Errorf("a form binding nothing the compiler sees: %v", err)
	}
	var plain ui.FormFor[registration]
	if err := plain.Save(context.Background()); !errors.Is(err, ui.ErrFormInvalid) {
		t.Errorf("saving a form that fails its checks: %v", err)
	}
	plain.Load(context.Background(), registration{Email: "ada@example.com", Password: "a long password", Plan: "free", Terms: true, Seats: 1})
	if err := plain.Save(context.Background()); err == nil || !strings.Contains(err.Error(), "has no Save and SaveFields methods") {
		t.Errorf("saving a form whose T has no Save: %v", err)
	}
}
