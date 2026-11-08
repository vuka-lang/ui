package ui_test

import (
	"context"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
	"github.com/vuka-lang/vuka/v2"
)

type orderLine struct {
	Product  string
	Quantity int
}

func (l orderLine) CleanQuantity() error {
	if l.Quantity > 9 {
		return ui.FieldError("Quantity", "Order 9 or fewer")
	}
	return nil
}

type basket struct{ Customer string }

type orderForm struct {
	ui.FormFor[basket]
	Lines ui.Rows[orderLine]
}

func init() {
	vuka.RegisterFields[orderForm](map[string][]any{"Lines": {ui.Spec{Min: 1, Max: 2}}})
}

// Clean sees every row.
func (f *orderForm) Clean() error {
	for _, l := range f.Lines.Values() {
		if l.Product == f.Value.Customer {
			return f.Fail("a line names the customer")
		}
	}
	return nil
}

type orderPage struct {
	ui.Live
	Form  orderForm
	saved *[][]orderLine
}

func (p *orderPage) Save(ctx context.Context) error {
	*p.saved = append(*p.saved, p.Form.Lines.Values())
	return nil
}

func (p *orderPage) Render() ui.Node {
	kids := []ui.Node{ui.Field{For: ui.For(&p.Form.Value.Customer, &p.Form)}}
	for _, r := range p.Form.Lines.Each() {
		kids = append(kids,
			ui.Field{For: ui.For(&r.Value.Product, r)},
			ui.Field{For: ui.For(&r.Value.Quantity, r)},
			ui.El("button", []ui.Attr{{Name: "type", Value: "button"}, {Name: "class", Value: "remove"}, {Name: "onClick", Value: p.Form.Lines.Remove(r)}}, ui.Text("Remove")))
	}
	kids = append(kids, ui.El("button", []ui.Attr{{Name: "type", Value: "button"}, {Name: "class", Value: "add"}, {Name: "onClick", Value: p.Form.Lines.Add()}}, ui.Text("Add line")))
	return ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save, Keep: true, Children: ui.Fragment(kids...)}
}

var (
	addTarget    = regexp.MustCompile(`class="add" data-vk-on-click="([^"]+)"`)
	removeTarget = regexp.MustCompile(`class="remove" data-vk-on-click="([^"]+)"`)
)

// TestRows: a form type's rows bind, check and submit with the form; Add
// and Remove keep what was typed in the other rows; Min and Max bound
// them; a row's error shows at its field and in the summary.
func TestRows(t *testing.T) {
	var saved [][]orderLine
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "basket", Props: &orderPage{saved: &saved}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	click := func(re *regexp.Regexp, nth int) {
		t.Helper()
		m := re.FindAllStringSubmatch(c.HTML(), -1)
		if len(m) <= nth {
			t.Fatalf("no button %d in %s", nth, c.HTML())
		}
		if _, err := c.Event(ui.Event{Target: m[nth][1], Type: "click"}); err != nil {
			t.Fatal(err)
		}
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"customer": "Ada"}))
	if len(saved) != 0 || !strings.Contains(c.HTML(), "Lines must have at least 1") {
		t.Fatalf("Min: %v\n%s", saved, c.HTML())
	}
	click(addTarget, 0)
	click(addTarget, 0)
	click(addTarget, 0)
	html := c.HTML()
	if n := strings.Count(html, `class="remove"`); n != 2 {
		t.Fatalf("%d rows past Max 2:\n%s", n, html)
	}
	for _, want := range []string{`name="lines.r1.product" id="order-line-lines-r1-product"`, `name="lines.r2.quantity"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	send(t, c, inputTarget, "input", formValues(c.HTML(), map[string]string{"customer": "Ada", "lines.r1.product": "Lamp", "lines.r1.quantity": "2", "lines.r2.product": "Kettle", "lines.r2.quantity": "1"}))
	click(removeTarget, 1)
	html = c.HTML()
	if !strings.Contains(html, `name="lines.r1.product" id="order-line-lines-r1-product" type="text" value="Lamp"`) || strings.Contains(html, "lines.r2") {
		t.Fatalf("Remove: \n%s", html)
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"customer": "Ada", "lines.r1.product": "Lamp", "lines.r1.quantity": "12"}))
	html = c.HTML()
	if len(saved) != 0 || !strings.Contains(html, `id="order-line-lines-r1-quantity-error"`) || !strings.Contains(html, `<a href="#order-line-lines-r1-quantity">Order 9 or fewer</a>`) {
		t.Fatalf("a row's error: %v\n%s", saved, html)
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"customer": "Lamp", "lines.r1.product": "Lamp", "lines.r1.quantity": "3"}))
	if len(saved) != 0 || !strings.Contains(c.HTML(), "a line names the customer") {
		t.Fatalf("the form's Clean: %v\n%s", saved, c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"customer": "Ada", "lines.r1.product": "Lamp", "lines.r1.quantity": "3"}))
	if len(saved) != 1 || len(saved[0]) != 1 || saved[0][0] != (orderLine{"Lamp", 3}) {
		t.Fatalf("saved %v\n%s", saved, c.HTML())
	}
}

// A model's rows: invoice (a model) and its invoiceItems, each with a back
// reference (itemRef, as an orm.FK is: its key as text, the rows
// referring to a record, a transaction).
type invoice struct {
	ID   int64
	Note string
}

type invoiceItem struct {
	ID      int64
	Invoice itemRef
	Name    string
}

type itemRef struct{ id int64 }

func (r itemRef) MarshalText() ([]byte, error) { return []byte(strconv.FormatInt(r.id, 10)), nil }
func (r *itemRef) UnmarshalText(b []byte) error {
	n, err := strconv.ParseInt(string(b), 10, 64)
	r.id = n
	return err
}

var (
	invoices     []invoice
	invoiceItems []invoiceItem
	ops          []string
)

func (itemRef) FormChildren(_ context.Context, child reflect.Type, field, key string) ([]any, error) {
	ops = append(ops, "children "+child.Name()+"."+field+"="+key)
	var out []any
	for _, it := range invoiceItems {
		if strconv.FormatInt(it.Invoice.id, 10) == key {
			out = append(out, it)
		}
	}
	return out, nil
}

func (itemRef) FormAtomic(ctx context.Context, fn func(context.Context) error) error {
	ops = append(ops, "begin")
	err := fn(ctx)
	ops = append(ops, "end")
	return err
}

func init() {
	vuka.RegisterFields[invoice](map[string][]any{"ID": {key{}}})
	vuka.RegisterFields[invoiceItem](map[string][]any{"ID": {key{}}})
}

func (v *invoice) Save(context.Context) error {
	v.ID = int64(len(invoices) + 1)
	invoices = append(invoices, *v)
	ops = append(ops, "insert invoice")
	return nil
}
func (v *invoice) SaveFields(context.Context, ...any) error {
	ops = append(ops, "update invoice")
	return nil
}

func (it *invoiceItem) Save(context.Context) error {
	it.ID = int64(100 + len(invoiceItems))
	invoiceItems = append(invoiceItems, *it)
	ops = append(ops, "insert "+it.Name+" of "+strconv.FormatInt(it.Invoice.id, 10))
	return nil
}

func (it *invoiceItem) SaveFields(_ context.Context, fields ...any) error {
	for i := range invoiceItems {
		if invoiceItems[i].ID == it.ID {
			invoiceItems[i] = *it
		}
	}
	ops = append(ops, "update "+it.Name)
	return nil
}

func (it *invoiceItem) Delete(context.Context) error {
	ops = append(ops, "delete "+it.Name)
	return nil
}

type invoiceForm struct {
	ui.FormFor[invoice]
	Items ui.Rows[invoiceItem]
}

// TestModelRows: a model's rows are loaded with it and saved with it, in
// one transaction: added ones inserted with their back reference set,
// changed ones updated, removed ones deleted.
func TestModelRows(t *testing.T) {
	ctx := context.Background()
	invoices, invoiceItems, ops = nil, nil, nil
	var f invoiceForm
	ui.Attach(&f)
	if err := f.Load(ctx, invoice{}); err != nil {
		t.Fatal(err)
	}
	f.Value.Note = "first"
	row, _ := f.Items.Append(ctx)
	row.Value.Name = "Lamp"
	if err := f.Save(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ops, "; "); got != "begin; insert invoice; insert Lamp of 1; end" {
		t.Fatalf("a new invoice: %s", got)
	}
	invoiceItems = append(invoiceItems, invoiceItem{ID: 7, Invoice: itemRef{1}, Name: "Kettle"})
	ops = nil
	var g invoiceForm
	ui.Attach(&g)
	if err := g.Load(ctx, invoices[0]); err != nil {
		t.Fatal(err)
	}
	if g.Items.Len() != 2 {
		t.Fatalf("loaded %d rows: %s", g.Items.Len(), ops)
	}
	rows := g.Items.Each()
	rows[0].Value.Name = "Desk lamp"
	g.Items.Delete(rows[1])
	added, _ := g.Items.Append(ctx)
	added.Value.Name = "Mug"
	ops = nil
	if err := g.Save(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ops, "; "); got != "begin; update invoice; delete Kettle; update Desk lamp; insert Mug of 1; end" {
		t.Fatalf("an edited invoice: %s", got)
	}
}
