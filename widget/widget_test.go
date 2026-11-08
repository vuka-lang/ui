package widget_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/ui/v2/widget"
	"github.com/vuka-lang/vuka/v2"
)

// category is a related model's row; ref an FK to one, as orm.FK is to a
// form: its key as text, and its model's rows (FormRows).
type category struct {
	ID   int64
	Name string
	Live bool
}

func (c category) String() string { return c.Name }

var categories = []category{{1, "Lamps", true}, {2, "Kettles", true}, {3, "Retired", false}, {4, "Lanterns", true}}

type ref struct{ id int64 }

func (r ref) MarshalText() ([]byte, error) {
	if r.id == 0 {
		return nil, nil
	}
	return []byte(strconv.FormatInt(r.id, 10)), nil
}

func (r *ref) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*r = ref{}
		return nil
	}
	n, err := strconv.ParseInt(string(b), 10, 64)
	*r = ref{n}
	return err
}

func (ref) FormRows() any { return rows{func(category) bool { return true }} }

// rows is a query of categories, as an orm query is to a form.
type rows struct{ keep func(category) bool }

func (q rows) FormChoices(_ context.Context, search string, limit int) (keys, labels []string, out []any, err error) {
	for _, c := range categories {
		if !q.keep(c) || search != "" && !strings.Contains(strings.ToLower(c.Name), strings.ToLower(search)) {
			continue
		}
		if limit > 0 && len(keys) == limit {
			break
		}
		keys, labels, out = append(keys, strconv.FormatInt(c.ID, 10)), append(labels, c.Name), append(out, c)
	}
	return keys, labels, out, nil
}

func (q rows) FormChoice(_ context.Context, key string) (string, any, bool, error) {
	for _, c := range categories {
		if strconv.FormatInt(c.ID, 10) == key && q.keep(c) {
			return c.Name, c, true, nil
		}
	}
	return "", nil, false, nil
}

// amount is a value two inputs edit.
type amount struct {
	Cents    int64
	Currency string
}

func (a amount) MarshalText() ([]byte, error) { return []byte(fmt.Sprint(a.Cents, a.Currency)), nil }
func (a *amount) UnmarshalText([]byte) error  { return errors.New("not text") }

type item struct {
	Name     string
	Category ref
	Shelf    vuka.Option[ref]
	Notes    string
	Price    amount
	Size     string
}

type itemForm struct {
	ui.FormFor[item]
	Notes ui.Meta
	Size  ui.Meta
}

func init() {
	vuka.RegisterFields[itemForm](map[string][]any{
		"Notes": {ui.Spec{Widget: widget.Textarea{Rows: 7}}},
		"Size":  {ui.Spec{Widget: widget.Radios{}}},
	})
}

// ShelfChoices narrows the shelf's rows: a query, the allow-list too.
func (f *itemForm) ShelfChoices() rows { return rows{func(c category) bool { return c.Live }} }

func (f *itemForm) SizeChoices() []ui.Option {
	return []ui.Option{{Value: "s", Label: "Small"}, {Value: "l", Label: "Large"}}
}

// moneyInputs is a widget of two inputs, written as an app's own: the
// form fills it as its markup renders it (Label here).
type moneyInputs struct{ widget.Props }

func (moneyInputs) Format(v amount) map[string]string {
	if v.Currency == "" {
		return map[string]string{}
	}
	return map[string]string{"amount": fmt.Sprintf("%d.%02d", v.Cents/100, v.Cents%100), "currency": v.Currency}
}

func (moneyInputs) Parse(in map[string]string) (amount, error) {
	if in["amount"] == "" {
		return amount{}, nil
	}
	whole, frac, _ := strings.Cut(in["amount"], ".")
	n, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil || len(frac) != 2 {
		return amount{}, errors.New("Enter the price as 12.50")
	}
	return amount{n, in["currency"]}, nil
}

func (w moneyInputs) Render(ctx context.Context, out io.Writer) error {
	return ui.Fragment(
		ui.El("span", []ui.Attr{{Name: "class", Value: "label"}}, ui.Text(w.Label)),
		ui.El("input", []ui.Attr{{Name: "name", Value: w.Model.Part("amount")}, {Name: "value", Value: w.Model.PartValue("amount")}}),
		ui.El("input", []ui.Attr{{Name: "name", Value: w.Model.Part("currency")}, {Name: "value", Value: w.Model.PartValue("currency")}}),
		ui.El("span", []ui.Attr{{Name: "class", Value: "err"}}, ui.Text(w.Error)),
	).Render(ctx, out)
}

type itemPage struct {
	ui.Live
	Form  itemForm
	saved *[]item
}

func (p *itemPage) Save(ctx context.Context) error {
	*p.saved = append(*p.saved, p.Form.Value)
	return nil
}

func (p *itemPage) Render() ui.Node {
	return ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save, Keep: true, Children: ui.Fragment(
		ui.Field{For: ui.For(&p.Form.Value.Name, &p.Form)},
		ui.Field{For: ui.For(&p.Form.Value.Category, &p.Form)},
		ui.Field{For: ui.For(&p.Form.Value.Shelf, &p.Form)},
		ui.Field{For: ui.For(&p.Form.Value.Notes, &p.Form)},
		ui.Field{For: ui.For(&p.Form.Value.Size, &p.Form)},
		ui.HTML(`<div class="price">{w}</div>`, moneyInputs{Props: widget.Props{Model: ui.For(&p.Form.Value.Price, &p.Form)}}),
	)}
}

var (
	submitTarget = regexp.MustCompile(`data-vk-on-submit="([^"]+)"`)
	inputTarget  = regexp.MustCompile(`data-vk-on-input="([^"]+)"`)
)

func hidden(html, name string) string {
	m := regexp.MustCompile(`name="` + name + `" value="([^"]*)"`).FindStringSubmatch(html)
	if m == nil {
		return ""
	}
	return strings.NewReplacer("&amp;", "&", "&#34;", `"`, "&#39;", "'").Replace(m[1])
}

func values(html string, fields map[string]string) url.Values {
	v := url.Values{}
	for _, h := range []string{ui.FormIDField, ui.FormIdemField, ui.FormFieldsField, ui.FormUsedField} {
		v.Set(h, hidden(html, h))
	}
	for k, x := range fields {
		v.Set(k, x)
	}
	return v
}

func join(t *testing.T, p ui.Stateful) *livetest.Client {
	t.Helper()
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return &ui.ComponentNode{Site: "items", Props: p, Routed: true} })
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func send(t *testing.T, c *livetest.Client, re *regexp.Regexp, typ string, form url.Values) {
	t.Helper()
	m := re.FindStringSubmatch(c.HTML())
	if m == nil {
		t.Fatalf("no %s handler in %s", typ, c.HTML())
	}
	if _, err := c.Event(ui.Event{Target: m[1], Type: typ, Form: form}); err != nil {
		t.Fatal(err)
	}
}

func contains(t *testing.T, html string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(html, w) {
			t.Errorf("missing %s in\n%s", w, html)
		}
	}
}

// TestWidgets: an FK field is a select of its model's rows; a Choices
// query narrows them and is the allow-list; a @ui.Spec widget is the
// field's control; a widget of several inputs parses its parts.
func TestWidgets(t *testing.T) {
	var saved []item
	c := join(t, &itemPage{saved: &saved})
	html := c.HTML()
	contains(t, html,
		`<select class="vk-select" name="category" id="item-category"`,
		`<option value="1">Lamps</option><option value="2">Kettles</option><option value="3">Retired</option>`,
		`<option value="4">Lanterns</option></select>`,
		`<textarea class="vk-textarea" rows="7" name="notes" id="item-notes"`,
		`<fieldset class="vk-fieldset vk-field"`,
		`<input class="vk-check__box" id="item-size" value="s" name="size" type="radio"`,
		`<label class="vk-check__label" for="item-size-2">Large</label>`,
		`<span class="label">Price</span><input name="price:amount" value=""><input name="price:currency" value="">`,
	)
	if strings.Contains(html, `name="shelf"`) && strings.Contains(html[strings.Index(html, `name="shelf"`):], `>Retired<`) && strings.Index(html[strings.Index(html, `name="shelf"`):], `>Retired<`) < strings.Index(html[strings.Index(html, `name="shelf"`):], `</select>`) {
		t.Errorf("the shelf's query keeps Retired out:\n%s", html)
	}

	good := map[string]string{"name": "Desk lamp", "category": "1", "shelf": "4", "notes": "Bright", "size": "l", "price:amount": "12.50", "price:currency": "TZS"}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range good {
			m[a] = b
		}
		m[k] = v
		return m
	}
	send(t, c, submitTarget, "submit", values(c.HTML(), with("shelf", "3")))
	if len(saved) != 0 {
		t.Fatalf("a shelf outside its query was taken: %+v", saved)
	}
	contains(t, c.HTML(), "Select shelf from the list")

	send(t, c, submitTarget, "submit", values(c.HTML(), with("price:amount", "12")))
	if len(saved) != 0 {
		t.Fatalf("an unparsed price was taken: %+v", saved)
	}
	contains(t, c.HTML(), `<span class="err">Enter the price as 12.50</span>`, `name="price:amount" value="12"`)

	send(t, c, submitTarget, "submit", values(c.HTML(), good))
	if len(saved) != 1 {
		t.Fatalf("not saved:\n%s", c.HTML())
	}
	got := saved[0]
	if got.Category.id != 1 || !got.Shelf.IsSome() || got.Shelf.Unwrap().id != 4 || got.Size != "l" || got.Price != (amount{1250, "TZS"}) {
		t.Errorf("saved %+v", got)
	}
	contains(t, c.HTML(), `<option value="1" selected>Lamps</option>`, `name="price:amount" value="12.50"`)
}

// picker is an app's widget for a category: typed Choices (their rows
// categories) and Value.
type picker struct {
	widget.Props
	Value   ref
	Choices []widget.Choice[category]
}

func (w picker) Render(ctx context.Context, out io.Writer) error {
	var kids []ui.Node
	for _, c := range w.Choices {
		live := ""
		if c.Row.Live {
			live = " live"
		}
		kids = append(kids, ui.El("button", []ui.Attr{{Name: "name", Value: w.Model.Path()}, {Name: "value", Value: c.Value}, {Name: "aria-pressed", Value: c.Selected}}, ui.Text(c.Label+live)))
	}
	return ui.El("div", []ui.Attr{{Name: "class", Value: "picker"}, {Name: "data-value", Value: w.Value.id}}, kids...).Render(ctx, out)
}

// wrongPicker's Choices are of another row type than the field's.
type wrongPicker struct {
	widget.Props
	Choices []widget.Choice[item]
}

func (wrongPicker) Render(context.Context, io.Writer) error { return nil }

type shelving struct {
	Name   string
	Main   ref
	Spare  ref
	Wrong  ref
	Placed bool
	Lookup ref
}

type shelvingPage struct {
	ui.Live
	Form ui.FormFor[shelving]
}

func (p *shelvingPage) Render() ui.Node {
	ref := func(f any) ui.FieldRef { return ui.For(f, &p.Form) }
	return ui.Form{For: ui.For(&p.Form), Children: ui.Fragment(
		ui.Field{For: ref(&p.Form.Value.Name)},
		ui.HTML(`<div class="main">{a}</div>`, picker{Props: widget.Props{Model: ref(&p.Form.Value.Main)}}),
		ui.HTML(`<div class="spare">{b}</div>`, widget.Select{Props: widget.Props{Model: ref(&p.Form.Value.Spare), Label: "Mine"}}),
		ui.Field{For: ref(&p.Form.Value.Placed)},
		ui.Field{For: ref(&p.Form.Value.Lookup)},
	)}
}

// TestCustomWidgets: an app's widget is filled by name and type, its
// typed Choices carrying the rows; one of the wrong row type fails the
// render naming both; Use is the app's widget for a type; RelatedSearch
// shows the rows a search finds.
func TestCustomWidgets(t *testing.T) {
	widget.Use[bool](widget.Switch{})
	widget.Use[ref](widget.RelatedSearch{Limit: 2})
	t.Cleanup(func() {
		ui.UseWidget(reflectType[bool](), nil)
		ui.UseWidget(reflectType[ref](), nil)
	})
	c := join(t, &shelvingPage{Form: ui.FormFor[shelving]{Value: shelving{Main: ref{2}}}})
	html := c.HTML()
	contains(t, html,
		`<div class="picker" data-value="2"><button name="main" value="1">Lamps live</button><button name="main" value="2" aria-pressed>Kettles live</button><button name="main" value="3">Retired</button>`,
		`<input class="vk-check__box vk-switch" role="switch" name="placed" id="shelving-placed" type="checkbox" value="true"`,
		`<label class="vk-check__label" for="shelving-placed">Placed</label>`,
		`<input class="vk-input" type="search" name="lookup:search" id="shelving-lookup-search" value="" autocomplete="off">`,
		`<option value="">Choose…</option><option value="1">Lamps</option>`,
		`value="name,main,spare,placed,lookup.`,
	)
	if !strings.Contains(html, `class="spare"><select class="vk-select" name="spare"`) {
		t.Errorf("Use[ref] took a widget the markup named:\n%s", html)
	}
	var f ui.FormFor[shelving]
	_, err := ui.String(context.Background(), ui.Form{For: ui.For(&f), Children: ui.HTML(`<p>{w}</p>`, wrongPicker{Props: widget.Props{Model: ui.For(&f.Value.Wrong, &f)}})})
	if err == nil || !strings.Contains(err.Error(), "its Choices' Row is a widget_test.item, and the rows of Wrong's choices are widget_test.category") {
		t.Errorf("a widget of another row type: %v", err)
	}
	send(t, c, inputTarget, "input", values(c.HTML(), map[string]string{"name": "x", "lookup:search": "la"}))
	html = c.HTML()
	contains(t, html, `value="la"`, `<label class="vk-check__label" for="shelving-lookup">Lamps</label>`, `>Lanterns</label>`)
	if strings.Contains(html, `>Kettles</label>`) {
		t.Errorf("the search shows rows it doesn't match:\n%s", html)
	}
	send(t, c, inputTarget, "input", values(c.HTML(), map[string]string{"name": "x", "lookup:search": "la", "lookup": "4"}))
	send(t, c, inputTarget, "input", values(c.HTML(), map[string]string{"name": "x", "lookup:search": "ket", "lookup": "4"}))
	contains(t, c.HTML(), `value="4" name="lookup" type="radio" checked`, `>Lanterns</label>`, `>Kettles</label>`)
}

func reflectType[T any]() reflect.Type { return reflect.TypeFor[T]() }

// fieldError and summary are an app's own error message and summary.
type fieldError struct{ widget.ErrorProps }

func (e fieldError) Render(ctx context.Context, out io.Writer) error {
	return ui.El("em", e.Attrs, ui.Text(e.Model.Path()+": "+e.Message)).Render(ctx, out)
}

type summary struct{ widget.SummaryProps }

func (s summary) Render(ctx context.Context, out io.Writer) error {
	var kids []ui.Node
	for _, it := range s.Items {
		kids = append(kids, ui.El("a", []ui.Attr{{Name: "href", Value: it.Href}}, ui.Text(it.Text)))
	}
	return ui.El("section", append(s.Attrs, ui.Attr{Name: "role", Value: "alert"}), append([]ui.Node{ui.Text(s.Title)}, kids...)...).Render(ctx, out)
}

type signupPage struct {
	ui.Live
	Form ui.FormFor[shelving]
}

func (p *signupPage) Render() ui.Node {
	return ui.Form{For: ui.For(&p.Form), Children: ui.Fragment(
		widget.Summary{},
		widget.Field{For: ui.For(&p.Form.Value.Name, &p.Form)},
		widget.Error{For: ui.For(&p.Form.Value.Main, &p.Form)},
		widget.Select{Props: widget.Props{Model: ui.For(&p.Form.Value.Main, &p.Form)}},
	)}
}

// TestFormWidgets: the form's components from package widget, the app's
// own error message and summary in their place.
func TestFormWidgets(t *testing.T) {
	widget.UseError(fieldError{})
	widget.UseSummary(summary{})
	t.Cleanup(func() { widget.UseError(nil); widget.UseSummary(nil) })
	c := join(t, &signupPage{})
	send(t, c, submitTarget, "submit", values(c.HTML(), map[string]string{"name": "", "main": ""}))
	contains(t, c.HTML(),
		`<section class="vk-summary" id="shelving-summary" tabindex="-1" autofocus data-vk-summary role="alert">There is a problem<a href="#shelving-name">Enter name</a>`,
		`<em class="vk-field__error" id="shelving-name-error" data-vk-error="validate.required">name: Enter name</em>`,
		`<em class="vk-field__error" id="shelving-main-error" data-vk-error="validate.required.choice">main: Select main</em>`,
		`aria-describedby="shelving-main-error"`,
	)
}
