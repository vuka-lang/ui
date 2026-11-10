package ui_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// productCard is a stateless component a Call opens over HTTP.
type productCard struct {
	ID     int64
	Name   string
	Query  any
	OnSave ui.Action
}

func (p productCard) View() ui.Node {
	q, _ := p.Query.(string)
	return ui.El("article", nil, ui.Textf("%d %s %s", p.ID, p.Name, q), ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(p.OnSave)}}, ui.Text("Save")))
}

func (p productCard) Render(ctx context.Context, w io.Writer) error { return p.View().Render(ctx, w) }

// productDialog's root is a ui.Dialog: its presentation.
type productDialog struct{ ID int64 }

func (p productDialog) View() ui.Node {
	return ui.HTML("\n{d}\n", ui.Dialog{Title: ui.Text("Product"), Children: ui.Textf("product %d", p.ID)})
}

func (p productDialog) Render(ctx context.Context, w io.Writer) error { return p.View().Render(ctx, w) }

// withSlot has a slot, which a Call can't send.
type withSlot struct {
	Body     ui.Node
	Children ui.Node
	Rows     []struct{ Cell ui.Node }
	Fn       func()
}

func (withSlot) Render(context.Context, io.Writer) error { return nil }

type notCalled struct{ ID int }

func (notCalled) Render(context.Context, io.Writer) error { return nil }

func init() {
	ui.RegisterCall(productCard{})
	ui.RegisterCall(productDialog{})
	ui.RegisterCall(withSlot{})
}

func (o *orders) Close(ctx context.Context) { ui.Close(ctx) }

func (o *orders) Open(ctx context.Context) error { return ui.Call(ctx, productCard{ID: 3}) }

func init() { ui.RegisterPush((*orders).Close); ui.RegisterPush((*orders).Open) }

func callCtx(token string) context.Context {
	return ui.WithPageURL(ui.WithFormHost(context.Background(), callHost{token}), &url.URL{Path: "/products"}, nil)
}

type callHost struct{ tok string }

func (h callHost) FormToken(context.Context) string { return h.tok }
func (callHost) FormKey() []byte                    { return []byte("call test key") }
func (callHost) FormStore() ui.FormStore            { return nil }

// openStep is a chain's open step's arguments.
func openStep(t *testing.T, ctx context.Context, ch ui.JSChain) map[string]json.RawMessage {
	t.Helper()
	b, err := ui.EncodeJS(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	var ops [][]json.RawMessage
	if err := json.Unmarshal(b, &ops); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
	for _, o := range ops {
		if string(o[0]) == `"open"` {
			var a map[string]json.RawMessage
			json.Unmarshal(o[1], &a)
			return a
		}
	}
	t.Fatalf("no open in %s", b)
	return nil
}

func tokenOf(t *testing.T, a map[string]json.RawMessage) string {
	var tok string
	if err := json.Unmarshal(a["t"], &tok); err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestCallEncode(t *testing.T) {
	ctx := callCtx("visitor-1")
	a := openStep(t, ctx, ui.JS.Call(productCard{ID: 7, Name: "Lamp", Query: ui.Sel("#q").Value(), OnSave: (&orders{}).Remove}).
		Success(ui.JS.Hide("#x")).Closed(ui.JS.Focus(ui.This)).After(ui.JS.Show("#y")))
	for _, k := range []string{"t", "reads", "csrf", "on_ok", "closed", "after"} {
		if a[k] == nil {
			t.Errorf("no %s in %v", k, a)
		}
	}
	if a["h"] != nil || string(a["csrf"]) != `"visitor-1"` {
		t.Errorf("a page that isn't live opens over HTTP: %v", a)
	}
	c, err := ui.ParseCall(ctx, "visitor-1", tokenOf(t, a), json.RawMessage(`{"Query":"red"}`))
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.Value(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := v.(*productCard)
	if p.ID != 7 || p.Name != "Lamp" || p.Query != "red" {
		t.Errorf("props: %+v", p)
	}
	ops, err := ui.EncodeJS(ctx, p.OnSave.(ui.JSChain))
	if err != nil || !strings.Contains(string(ops), `"push",{"m":`) {
		t.Errorf("the Action prop pushes the page's method over HTTP: %s %v", ops, err)
	}
	html, failed := ui.RenderCall(ctx, "k1", v)
	if failed != nil || !strings.HasPrefix(html, `<dialog data-vk-call="k1" class="vk-call" data-vk-present="modal">`) || !strings.Contains(html, "7 Lamp red") {
		t.Errorf("the default frame: %s %v", html, failed)
	}
}

func TestCallPresentation(t *testing.T) {
	ctx := callCtx("v")
	c, err := ui.ParseCall(ctx, "v", tokenOf(t, openStep(t, ctx, ui.JS.Call(productDialog{ID: 4}))), nil)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := c.Value(nil)
	html, err := ui.RenderCall(ctx, "k2", v)
	if err != nil || !strings.HasPrefix(html, `<dialog class="vk-dialog" data-vk-call="k2" data-vk-present="modal">`) || strings.Contains(html, "vk-call\"") || strings.Count(html, "<dialog") != 1 {
		t.Errorf("a ui.Dialog root is the presentation: %s %v", html, err)
	}
	out, err := ui.String(ctx, ui.Dialog{Title: ui.Text("Plain"), Open: true})
	if err != nil || !strings.HasPrefix(out, `<dialog class="vk-dialog" open>`) || strings.Contains(out, "data-vk-call") {
		t.Errorf("a Dialog outside a Call: %s %v", out, err)
	}
}

// TestCallRefused: a token is only good for the visitor it was rendered for,
// as rendered, naming a component a Call names.
func TestCallRefused(t *testing.T) {
	ctx := callCtx("visitor-1")
	tok := tokenOf(t, openStep(t, ctx, ui.JS.Call(productCard{ID: 7})))
	if _, err := ui.ParseCall(ctx, "visitor-2", tok, nil); !errors.Is(err, ui.ErrCallToken) {
		t.Errorf("another session's token: %v", err)
	}
	body, sig, _ := strings.Cut(tok, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(body)
	parts := strings.Split(string(raw), "\x00")
	forge := func(i int, v string) string {
		p := append([]string(nil), parts...)
		p[i] = v
		return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(p, "\x00"))) + "." + sig
	}
	if _, err := ui.ParseCall(ctx, "visitor-1", forge(2, `{"ID":8}`), nil); !errors.Is(err, ui.ErrCallToken) {
		t.Errorf("a changed prop: %v", err)
	}
	if _, err := ui.ParseCall(ctx, "visitor-1", forge(1, "AAAAAAAAAAAA"), nil); !errors.Is(err, ui.ErrCallToken) {
		t.Errorf("another component's name: %v", err)
	}
	if _, err := ui.ParseCall(ctx, "visitor-1", tok, json.RawMessage(`{"Name":"x"}`)); err == nil {
		t.Error("a read of a prop the token doesn't read")
	}
	if _, err := ui.EncodeJS(ctx, ui.JS.Call(notCalled{})); err == nil || !strings.Contains(err.Error(), "isn't a component vuka's compiler registered") {
		t.Errorf("an unregistered component: %v", err)
	}
	if _, err := ui.ParseCall(ctx, "visitor-1", tok, nil); err != nil {
		t.Errorf("the token as rendered: %v", err)
	}
}

func TestCallProps(t *testing.T) {
	ctx := callCtx("v")
	for _, tt := range []struct {
		c    withSlot
		want string
	}{
		{withSlot{Body: ui.Text("x")}, "Body is markup"},
		{withSlot{Children: ui.Text("x")}, "Children is markup"},
		{withSlot{Rows: []struct{ Cell ui.Node }{{}}}, "Rows is markup"},
		{withSlot{Fn: func() {}}, "Fn is a func()"},
	} {
		if _, err := ui.EncodeJS(ctx, ui.JS.Call(tt.c)); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v", tt.want, err)
		}
	}
	if _, err := ui.EncodeJS(ctx, ui.JS.Call(productCard{OnSave: func() {}})); err == nil || !strings.Contains(err.Error(), "only a live page keeps") {
		t.Errorf("a closure on a page that isn't live: %v", err)
	}
	for name, f := range map[string]func(){
		"Closed after a Push":     func() { ui.JS.Push((&orders{}).Remove).Closed(ui.JS.Hide("#x")) },
		"Into after a Call":       func() { ui.JS.Call(productCard{}).Into("#x") },
		"an option with a name":   func() { ui.JS.Call("confetti", ui.CallOption(nil)) },
		"an argument after props": func() { ui.JS.Call(productCard{}, 3) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s doesn't panic", name)
				}
			}()
			f()
		}()
	}
}

// TestCallFromPush: a method a page that isn't live pushes opens a
// component (ui.Call) and closes the one it came from (ui.Close): its reply
// says so.
func TestCallFromPush(t *testing.T) {
	ctx := callCtx("v")
	o := &orders{}
	for m, want := range map[string]string{"Close": `"close":true`, "Open": `"run":[["open",{"t":`} {
		h := map[string]any{"Close": o.Close, "Open": o.Open}[m]
		a := pushStep(t, ctx, ui.JS.Push(h))
		var ref string
		json.Unmarshal(a["m"], &ref)
		call, err := ui.ParsePushCall(ctx, ref, "[]", "")
		if err != nil {
			t.Fatal(err)
		}
		res, err := call.Call(ctx, reflect.New(call.Receiver()))
		j, _ := json.Marshal(res)
		if err != nil || !strings.Contains(string(j), want) {
			t.Errorf("%s: %s %v", m, j, err)
		}
	}
	if err := ui.Call(context.Background(), productCard{}); !errors.Is(err, ui.ErrNoCaller) {
		t.Errorf("ui.Call outside a handler: %v", err)
	}
}

// TestCallOptions: a Call's options travel in its token, and the
// component renders as they say.
func TestCallOptions(t *testing.T) {
	ctx := callCtx("v")
	a := openStep(t, ctx, ui.JS.Call(productCard{ID: 5}, ui.Into("#side"), ui.CallKey("product 5"), ui.SizeXl))
	var key, into string
	json.Unmarshal(a["key"], &key)
	json.Unmarshal(a["into"], &into)
	if into != "#side" || !strings.HasPrefix(key, "k") || len(key) > 40 {
		t.Errorf("the step's key and into: %q %q", key, into)
	}
	c, err := ui.ParseCall(ctx, "v", tokenOf(t, a), nil)
	if err != nil {
		t.Fatal(err)
	}
	if sel, _ := c.Into(); sel != "#side" || c.Key() != key {
		t.Errorf("the token's options: %q %q", sel, c.Key())
	}
	v, _ := c.Value(nil)
	html, _ := ui.RenderCall(ctx, key, v, c.Options()...)
	if !strings.HasPrefix(html, `<section class="vk-call-inline" data-vk-call="`+key+`" data-vk-size="xl" data-vk-present="inline">`) {
		t.Errorf("Into renders in place: %s", html)
	}
	c, _ = ui.ParseCall(ctx, "v", tokenOf(t, openStep(t, ctx, ui.JS.Call(productDialog{ID: 4}, ui.As(ui.Drawer{})))), nil)
	v, _ = c.Value(nil)
	html, _ = ui.RenderCall(ctx, "k", v, c.Options()...)
	if !strings.HasPrefix(html, `<dialog class="vk-drawer vk-drawer--right" data-vk-call="k" data-vk-present="modal"><div class="vk-dialog__header"><h2 class="vk-dialog__title">Product</h2>`) || strings.Count(html, "<dialog") != 1 {
		t.Errorf("As takes the root's title and content: %s", html)
	}
	for name, f := range map[string]func(){
		"As with a title": func() { ui.As(ui.Drawer{Title: ui.Text("x")}) },
		"Into a class":    func() { ui.Into(".side") },
		"a Size":          func() { ui.JS.Call(productCard{}, ui.Size("huge")) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s doesn't panic", name)
				}
			}()
			f()
		}()
	}
}
