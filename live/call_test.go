package live_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// ProductNote is a stateless component a Call opens: an Action prop it
// forwards to its button.
type ProductNote struct {
	ID     int
	Note   any // a read: the browser's
	OnSave ui.Action
}

func (n ProductNote) View() ui.Node {
	note, _ := n.Note.(string)
	return ui.El("div", []ui.Attr{a("id", "note")},
		ui.El("p", []ui.Attr{a("id", "note-text")}, ui.Textf("product %d %s", n.ID, note)),
		ui.El("button", []ui.Attr{a("id", "note-save"), a("onClick", ui.On(n.OnSave))}, ui.Text("Save")))
}

func (n ProductNote) Render(ctx context.Context, w io.Writer) error { return n.View().Render(ctx, w) }

// OrderEditor is a stateful component a Call opens, in a ui.Dialog: its
// Mount refuses order 0, and its Save closes it from the server.
type OrderEditor struct {
	ui.Live
	ID      int
	OnSave  ui.Action
	OnError ui.Action
	qty     int
	log     *[]string
}

func (e *OrderEditor) LiveSchema(l *ui.LiveSchema) { l.Mount(e.mount) }

func (e *OrderEditor) mount(ctx context.Context) error {
	if e.ID == 0 {
		return publicError("no such order")
	}
	e.qty = e.ID * 10
	return nil
}

func (e *OrderEditor) More() { e.qty++ }

func (e *OrderEditor) Save(ctx context.Context) {
	if f, ok := e.OnSave.(func()); ok {
		f()
	}
	ui.Close(ctx)
}

func (e *OrderEditor) Unmount() { unmounted++ }

var unmounted int

func (e *OrderEditor) Render() ui.Node {
	return ui.Dialog{Title: ui.Textf("Order %d", e.ID), Children: ui.El("div", nil,
		ui.El("output", []ui.Attr{a("id", "qty")}, ui.Text(strconv.Itoa(e.qty))),
		ui.El("button", []ui.Attr{a("id", "more"), a("onClick", ui.On(e.More))}, ui.Text("+")),
		ui.El("button", []ui.Attr{a("id", "order-save"), a("onClick", ui.On(e.Save))}, ui.Text("Save")),
		ui.El("button", []ui.Attr{a("id", "cancel"), a("onClick", ui.JS.Close())}, ui.Text("Cancel")))}
}

func init() {
	ui.RegisterCall(ProductNote{})
	ui.RegisterCall(OrderEditor{})
}

// Orders opens both from its rows.
type Orders struct {
	ui.Live
	saved int
}

func (p *Orders) Open(ctx context.Context) error { return ui.Call(ctx, OrderEditor{ID: 2}) }

func (p *Orders) Render() ui.Node {
	b := func(id string, ch ui.Commands) ui.Node {
		return ui.El("button", []ui.Attr{a("id", id), a("onClick", ui.On(ch))}, ui.Text(id))
	}
	return ui.El("main", nil,
		ui.El("input", []ui.Attr{a("id", "q"), a("value", "red")}),
		b("note", ui.JS.Call(ProductNote{ID: 7, Note: ui.Sel("#q").Value(), OnSave: func() { p.saved++ }}).
			Success(logStep("success")).Closed(logStep("closed"))),
		b("edit", ui.JS.Call(OrderEditor{ID: 3, OnSave: func() { p.saved += 10 }}).Success(logStep("success")).Error(logStep("error")).After(logStep("after")).Closed(logStep("closed"))),
		b("bad", ui.JS.Call(OrderEditor{ID: 0, OnError: logStep("on error")}).Error(ui.JS.SetText("#err", ui.LoadError))),
		b("server", ui.JS.Push(p.Open)),
		ui.El("p", []ui.Attr{a("id", "err")}),
		ui.El("i", []ui.Attr{a("id", "saved")}, ui.Text(strconv.Itoa(p.saved))),
	)
}

func joinOrders(t *testing.T) (*livetest.Client, *[]string) {
	t.Helper()
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("orders#1", nil, &Orders{}) })
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	var steps []string
	c.RegisterFunc("log", func(_ livetest.CallContext, args []any) (any, error) {
		steps = append(steps, args[0].(string))
		return nil, nil
	})
	return c, &steps
}

func TestCallStateless(t *testing.T) {
	c, steps := joinOrders(t)
	if err := c.Click("#note"); err != nil {
		t.Fatal(err)
	}
	d := c.Dialog()
	if !d.Exists() || c.Text("#note-text") != "product 7 red" {
		t.Fatalf("the note opened with its props and read: %s", c.HTML())
	}
	if cls, _ := d.Attr("class"); cls != "vk-call" {
		t.Errorf("a component without a presentation of its own is in the dialog frame: %q", cls)
	}
	if got := strings.Join(*steps, ","); got != "success" {
		t.Errorf("Success once it is on screen: %s", got)
	}
	if err := c.Click("#note-save"); err != nil || c.Text("#saved") != "1" {
		t.Fatalf("the Action prop runs the page's closure: %v %s", err, c.HTML())
	}
	if err := c.Click(".vk-call__close"); err != nil {
		t.Fatal(err)
	}
	if c.Dialogs() != 0 || strings.Join(*steps, ",") != "success,closed" {
		t.Fatalf("the frame's close: %d %v %s", c.Dialogs(), *steps, c.HTML())
	}
}

func TestCallStateful(t *testing.T) {
	unmounted = 0
	c, steps := joinOrders(t)
	if err := c.Click("#edit"); err != nil {
		t.Fatal(err)
	}
	d := c.Dialog()
	if cls, _ := d.Attr("class"); cls != "vk-dialog" || c.Text("#qty") != "30" || !strings.Contains(d.Text(), "Order 3") {
		t.Fatalf("a ui.Dialog root is the presentation, mounted: %q %s", cls, c.HTML())
	}
	if err := c.Click("#more"); err != nil || c.Text("#qty") != "31" {
		t.Fatalf("the instance keeps its state: %v %s", err, c.HTML())
	}
	if err := c.Click("#order-save"); err != nil {
		t.Fatal(err)
	}
	if c.Dialogs() != 0 || c.Text("#saved") != "10" || unmounted != 1 {
		t.Fatalf("Save ran the page's closure and ui.Close closed it, unmounting it: %d %s %d", c.Dialogs(), c.Text("#saved"), unmounted)
	}
	if got := strings.Join(*steps, ","); got != "success,after,closed" {
		t.Errorf("hooks: %s", got)
	}
	*steps = nil
	c.Click("#edit")
	c.Click("#edit")
	if c.Dialogs() != 2 {
		t.Fatalf("a second Call stacks: %d", c.Dialogs())
	}
	if err := c.Escape(); err != nil || c.Dialogs() != 1 {
		t.Fatalf("Escape closes the top one: %v %d", err, c.Dialogs())
	}
	if err := c.Click("#cancel"); err != nil || c.Dialogs() != 0 || unmounted != 3 {
		t.Fatalf("ui.JS.Close: %v %d %d", err, c.Dialogs(), unmounted)
	}
}

func TestCallMountFails(t *testing.T) {
	c, steps := joinOrders(t)
	if err := c.Click("#bad"); err != nil {
		t.Fatal(err)
	}
	d := c.Dialog()
	if cls, _ := d.Attr("class"); cls != "vk-call vk-call--failed" || c.Text(".vk-call__error") != "This couldn't open." {
		t.Fatalf("the error state in the slot: %q %s", cls, c.HTML())
	}
	if c.Text("#err") != "no such order" || strings.Join(*steps, ",") != "on error" {
		t.Fatalf("Error with the Mount's message, the OnError prop run: %q %v", c.Text("#err"), *steps)
	}
	c.Click(".vk-call__close")
	if c.Dialogs() != 0 {
		t.Fatal("the error state closes")
	}
}

func TestCallFromServer(t *testing.T) {
	c, _ := joinOrders(t)
	if err := c.Click("#server"); err != nil || c.Text("#qty") != "20" {
		t.Fatalf("ui.Call: %v %s", err, c.HTML())
	}
	if err := c.Call(ProductNote{ID: 9}); err != nil || c.Dialogs() != 2 || c.Text("#note-text") != "product 9 " {
		t.Fatalf("livetest Call: %v %s", err, c.HTML())
	}
	if err := c.Call(struct{ ID int }{}); err == nil {
		t.Fatal("an unregistered component")
	}
}

var tokenOf = regexp.MustCompile(`&#34;t&#34;:&#34;([^&]+)&#34;`)

// TestCallRefused: a token the session didn't render — another session's,
// one with a prop changed, one naming a component no Call names — opens
// nothing.
func TestCallRefused(t *testing.T) {
	a1, _ := joinOrders(t)
	b, _ := joinOrders(t)
	tok := tokenOf.FindStringSubmatch(a1.HTML())[1]
	tok = html.UnescapeString(tok)
	open := func(c *livetest.Client, token string) error {
		data, _ := json.Marshal(map[string]any{"key": "x1"})
		m, err := c.Event(ui.Event{Target: live.RootID + ":vkcall", Type: "vkcall", Value: token, Data: data})
		if err != nil {
			return err
		}
		if m.Error != "" {
			return errors.New(m.Error)
		}
		return nil
	}
	if err := open(a1, tok); err != nil || a1.Dialogs() != 1 {
		t.Fatalf("its own session: %v", err)
	}
	if err := open(b, tok); err == nil || b.Dialogs() != 0 {
		t.Fatalf("another session's token: %v", err)
	}
	body, sig, _ := strings.Cut(tok, ".")
	raw, _ := base64URL(body)
	forged := strings.Replace(raw, `"ID":7`, `"ID":8`, 1)
	if forged == raw {
		t.Fatalf("no ID in %q", raw)
	}
	if err := open(a1, encode64(forged)+"."+sig); err == nil {
		t.Fatal("a changed prop")
	}
	other := strings.Replace(raw, strings.Split(raw, "\x00")[1], "AAAAAAAAAAAA", 1)
	if err := open(a1, encode64(other)+"."+sig); err == nil {
		t.Fatal("an unregistered component's name")
	}
}

func base64URL(s string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return string(b), err
}

func encode64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

type publicError string

func (e publicError) Error() string         { return string(e) }
func (e publicError) PublicMessage() string { return string(e) }
