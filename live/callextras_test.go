package live_test

import (
	"context"
	"io"
	"strconv"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// CartSummary is a stateless component the shop opens under one key.
type CartSummary struct{ Items int }

func (c CartSummary) View() ui.Node {
	return ui.El("p", []ui.Attr{a("id", "cart-n")}, ui.Text(strconv.Itoa(c.Items)+" items"))
}

func (c CartSummary) Render(ctx context.Context, w io.Writer) error { return c.View().Render(ctx, w) }

func init() { ui.RegisterCall(CartSummary{}) }

// ShopSide opens previews Into its own region.
type ShopSide struct{ ui.Live }

func (s *ShopSide) Render() ui.Node {
	return ui.El("aside", nil,
		ui.El("button", []ui.Attr{a("id", "preview"), a("onClick", ui.On(ui.JS.Call(ProductNote{ID: 1}, ui.Into("#side"))))}, ui.Text("preview")),
		ui.El("button", []ui.Attr{a("id", "preview2"), a("onClick", ui.On(ui.JS.Call(ProductNote{ID: 2}, ui.Into("#side"))))}, ui.Text("preview 2")),
		ui.El("div", []ui.Attr{a("id", "side")}))
}

type Storefront struct {
	ui.Live
	items, last, deleted int
	side                 bool
}

func (p *Storefront) Add()                 { p.items++ }
func (p *Storefront) Toggle()              { p.side = !p.side }
func (p *Storefront) Create() (int, error) { return 41, nil }
func (p *Storefront) Created(id int)       { p.last = id }

func (p *Storefront) Render() ui.Node {
	b := func(id string, ch ui.Commands) ui.Node {
		return ui.El("button", []ui.Attr{a("id", id), a("onClick", ui.On(ch))}, ui.Text(id))
	}
	var side ui.Node
	if p.side {
		side = ui.Component("side#1", nil, &ShopSide{})
	}
	return ui.El("main", nil,
		b("add", ui.JS.Push(p.Add)),
		b("cart", ui.JS.Call(CartSummary{Items: p.items}, ui.CallKey("cart"))),
		b("drawer", ui.JS.Call(CartSummary{Items: 1}, ui.As(ui.Drawer{Side: "left"}), ui.SizeLg)),
		b("pop", ui.JS.Call(CartSummary{Items: 2}, ui.As(ui.Popover{}))),
		b("delete", ui.JS.Call(ui.ConfirmBox{Title: "Delete?", Text: "It can't be undone.", Danger: true, OnYes: func() { p.deleted++ }})),
		b("saved", ui.JS.Call(ui.Notice{Text: "Saved", Variant: ui.AlertSuccess})),
		b("create", ui.JS.Push(p.Create).Success(p.Created)),
		b("toggle", ui.JS.Push(p.Toggle)),
		side,
		ui.El("i", []ui.Attr{a("id", "out")}, ui.Textf("%d %d", p.last, p.deleted)),
	)
}

func joinStorefront(t *testing.T) *livetest.Client {
	t.Helper()
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("shop#1", nil, &Storefront{}) })
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func attrOf(t *testing.T, e livetest.Element, name string) string {
	t.Helper()
	v, _ := e.Attr(name)
	return v
}

func TestCallKeyUpdates(t *testing.T) {
	c := joinStorefront(t)
	c.Click("#cart")
	c.Click("#add")
	c.Click("#add")
	if err := c.Click("#cart"); err != nil {
		t.Fatal(err)
	}
	if c.Dialogs() != 1 || c.Text("#cart-n") != "2 items" {
		t.Fatalf("a second Call with the key updates the first: %d %q", c.Dialogs(), c.Text("#cart-n"))
	}
}

func TestCallPresentations(t *testing.T) {
	c := joinStorefront(t)
	c.Click("#drawer")
	d := c.Dialog()
	if attrOf(t, d, "class") != "vk-drawer vk-drawer--left" || attrOf(t, d, "data-vk-size") != "lg" || attrOf(t, d, "data-vk-present") != "modal" {
		t.Errorf("As(Drawer) and a Size: %s", c.HTML())
	}
	c.Escape()
	c.Click("#pop")
	if attrOf(t, c.Dialog(), "data-vk-present") != "popover" {
		t.Errorf("As(Popover): %s", c.HTML())
	}
	c.Escape()
	c.Click("#saved")
	n := c.Dialog()
	if attrOf(t, n, "data-vk-present") != "toast" || attrOf(t, n, "data-vk-timeout") != "4000" || n.Text() == "" {
		t.Errorf("a Notice: %s", c.HTML())
	}
}

func TestCallConfirmBox(t *testing.T) {
	c := joinStorefront(t)
	c.Click("#delete")
	if attrOf(t, c.Dialog(), "class") != "vk-dialog vk-dialog--sm vk-confirm" {
		t.Fatalf("ConfirmBox in its dialog: %s", c.HTML())
	}
	c.Click("[data-vk-no]")
	if c.Dialogs() != 0 || c.Text("#out") != "0 0" {
		t.Fatalf("No: %s", c.HTML())
	}
	c.Click("#delete")
	if err := c.Click("[data-vk-yes]"); err != nil {
		t.Fatal(err)
	}
	if c.Dialogs() != 0 || c.Text("#out") != "0 1" {
		t.Fatalf("Yes runs OnYes and closes: %s", c.HTML())
	}
}

func TestCallInto(t *testing.T) {
	c := joinStorefront(t)
	c.Click("#toggle")
	c.Click("#preview")
	d := c.Dialog()
	if attrOf(t, d, "data-vk-present") != "inline" || c.Text("#note-text") != "product 1 " {
		t.Fatalf("Into a region, in place: %s", c.HTML())
	}
	c.Click("#preview2")
	if c.Dialogs() != 1 || c.Text("#note-text") != "product 2 " {
		t.Fatalf("a second Call Into it replaces the first: %d %s", c.Dialogs(), c.HTML())
	}
	c.Click("#toggle")
	if c.Dialogs() != 0 {
		t.Fatalf("it goes with the instance that opened it: %s", c.HTML())
	}
}

func TestTypedSuccess(t *testing.T) {
	c := joinStorefront(t)
	if err := c.Click("#create"); err != nil || c.Text("#out") != "41 0" {
		t.Fatalf("Success's method takes the push's result: %v %s", err, c.HTML())
	}
}
