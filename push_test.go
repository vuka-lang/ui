package ui_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2"
)

type quote struct {
	SKU    string
	Qty    int    `validate:"min=1"`
	Coupon string `form:"code"`
}

type place2 struct {
	City   string
	Region string
	Zip    vuka.Option[string]
	Tags   []string
	Gift   bool
}

type orderStore struct{ pushes int }

type orders struct{ Name string }

func (o *orders) Details(ctx context.Context, id int64, st *orderStore) (ui.Node, error) {
	st.pushes++
	if id == 0 {
		return nil, errors.New("no such order")
	}
	return ui.Text("order " + o.Name), nil
}
func (o *orders) Price(sku string, qty int, coupon string) string {
	return sku + ":" + strings.Repeat("x", qty) + coupon
}
func (o *orders) Quote(ctx context.Context, q quote, st *orderStore) (int, error) {
	return q.Qty * 10, nil
}
func (o *orders) Lookup(postcode string) (place2, error) {
	return place2{City: "Arusha", Region: postcode, Tags: []string{"a", "b"}, Gift: true}, nil
}
func (o *orders) Check(name string) ui.Commands {
	return ui.JS.SetText("#name-msg", name+" is free")
}
func (o *orders) Remove(id int64) error  { return nil }
func (o *orders) When() time.Time        { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
func (o *orders) Missing() (string, int) { return "", 0 }

func init() {
	for _, m := range []any{(*orders).Details, (*orders).Price, (*orders).Quote, (*orders).Lookup, (*orders).Check, (*orders).Remove, (*orders).When} {
		ui.RegisterPush(m)
	}
}

type tokenHost struct{ keyHost }

func (tokenHost) FormToken(context.Context) string { return "tok" }

func pushStep(t *testing.T, ctx context.Context, ch ui.JSChain) map[string]json.RawMessage {
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
		if string(o[0]) == `"push"` {
			var a map[string]json.RawMessage
			json.Unmarshal(o[1], &a)
			return a
		}
	}
	t.Fatalf("no push in %s", b)
	return nil
}

// TestPushStep: a Push's arguments, options and Into are told apart by
// type; on a page that isn't live it carries its method's reference and,
// unless it is a read, the CSRF token.
func TestPushStep(t *testing.T) {
	o := &orders{}
	ctx := ui.WithPageURL(ui.WithFormHost(context.Background(), tokenHost{keyHost{[]byte("k")}}), &url.URL{Path: "/orders"}, nil)
	a := pushStep(t, ctx, ui.JS.Push(o.Price, "sku-1", ui.Read(), ui.This.Value(), ui.Timeout(time.Second), ui.Sel("[name=coupon]").Value()).Into("#price"))
	if string(a["args"]) != `["sku-1",{"$read":{"value":true}},{"$read":{"to":{"sel":"[name=coupon]"},"value":true}}]` ||
		string(a["read"]) != "true" || string(a["into"]) != `{"sel":"#price"}` || string(a["timeout"]) != "1000" || a["csrf"] != nil || a["m"] == nil {
		t.Errorf("a read: %v", stringsOf(a))
	}
	a = pushStep(t, ctx, ui.JS.Push(o.Details, ui.Value(int64(7))).Into("#details"))
	if string(a["args"]) != `[7]` || string(a["csrf"]) != `"tok"` || a["read"] != nil {
		t.Errorf("a write, Value its first argument: %v", stringsOf(a))
	}
	a = pushStep(t, ctx, ui.JS.Push(o.Quote).Into("#total"))
	if string(a["form"]) != "true" || a["args"] != nil {
		t.Errorf("a struct from the form: %v", stringsOf(a))
	}
	a = pushStep(t, ctx, ui.JS.Push(o.Remove))
	if string(a["args"]) != `[{"$read":{"value":true}}]` {
		t.Errorf("one value, no argument: the element's: %v", stringsOf(a))
	}
	a = pushStep(t, ctx, ui.JS.Push(o.Details, int64(1), ui.Read()).Into("#posts", ui.Append()))
	if string(a["mode"]) != `"append"` {
		t.Errorf("append: %v", stringsOf(a))
	}
	c1 := pushStep(t, ctx, ui.JS.Push(o.Details, int64(1), ui.Cache(time.Minute)))
	c2 := pushStep(t, ctx, ui.JS.Push(o.Details, int64(1), ui.Read()))
	var r1, r2 string
	json.Unmarshal(c1["m"], &r1)
	json.Unmarshal(c2["m"], &r2)
	c3 := pushStep(t, ctx, ui.JS.Push(o.Details, int64(1)))
	var r3 string
	json.Unmarshal(c3["m"], &r3)
	if !strings.Contains(r1, ".r60.") || !strings.Contains(r2, ".r.") || strings.Count(r3, ".") != 1 || string(c1["read"]) != "true" {
		t.Errorf("a cached read's reference carries its time: %s, %s", r1, r2)
	}
	call, err := ui.ParsePushCall(ctx, r1, `[1]`, "")
	if w, _ := ui.ParsePushCall(ctx, r3, `[1]`, ""); w == nil || w.Read() {
		t.Errorf("a write's reference reads as a read")
	}
	if err != nil || call.Cache() != time.Minute || !call.Read() {
		t.Errorf("the cache: %v, %v", call, err)
	}
	forged := strings.Replace(r1, ".r60.", ".r3600.", 1)
	if _, err := ui.ParsePushCall(ctx, forged, `[1]`, ""); !errors.Is(err, ui.ErrPushRef) {
		t.Errorf("a reference with its cache time changed is taken: %v", err)
	}
	if _, err := ui.ParsePushCall(ctx, strings.Replace(r3, ".", ".r.", 1), `[1]`, ""); !errors.Is(err, ui.ErrPushRef) {
		t.Errorf("a write's reference made a read is taken: %v", err)
	}

	for _, tt := range []struct {
		ch   ui.JSChain
		want string
	}{
		{ui.JS.Push(o.Remove).Into("#x"), "returns no result to put Into a target"},
		{ui.JS.Push(o.Price, "a"), "takes 3 arguments, not 1"},
		{ui.JS.Push(o.Quote, 1, 2), "takes 1 arguments, not 2 (or 0, its ui_test.quote filled from the form)"},
		{ui.JS.Options("#x", o.Details, int64(1)), "returns no options"},
		{ui.JS.Push(o.Missing), "a pushed method returns"},
	} {
		if _, err := ui.EncodeJS(ctx, tt.ch); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v", tt.want, err)
		}
	}
	func() {
		defer func() {
			if r := recover(); r == nil || !strings.Contains(r.(string), "Push before it") {
				t.Errorf("Into without a Push: %v", r)
			}
		}()
		ui.JS.Show("#x").Into("#y")
	}()
}

func stringsOf(m map[string]json.RawMessage) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = string(v)
	}
	return out
}

// TestPushCall: a call binds its arguments in order, a struct from the
// form, and encodes each kind of result as the runtime applies it.
func TestPushCall(t *testing.T) {
	ctx := ui.WithPageURL(ui.WithFormHost(context.Background(), keyHost{[]byte("k")}), &url.URL{Path: "/orders"}, nil)
	st := &orderStore{}
	ctx = ui.WithInjector(ctx, storeInj{st})
	ref := func(ch ui.JSChain) string {
		var r string
		json.Unmarshal(pushStep(t, ctx, ch)["m"], &r)
		return r
	}
	o := &orders{}
	do := func(r, args, form string) (string, error) {
		t.Helper()
		call, err := ui.ParsePushCall(ctx, r, args, form)
		if err != nil {
			return "", err
		}
		out, err := call.Call(ctx, reflect.ValueOf(&orders{Name: "fresh"}))
		j, _ := json.Marshal(out)
		return string(j), err
	}
	tests := []struct {
		ref, args, form, want string
	}{
		{ref(ui.JS.Push(o.Details, int64(1), ui.Read())), `["9"]`, "", `{"html":"order fresh"}`},
		{ref(ui.JS.Push(o.Price, "a", "b", "c")), `["a","3","c"]`, "", `{"value":"a:xxxc"}`},
		{ref(ui.JS.Push(o.Quote)), ``, `{"qty":["4"],"code":["X"]}`, `{"value":"40"}`},
		{ref(ui.JS.Push(o.Lookup, "x")), `["0100"]`, "", `{"fields":{"city":"Arusha","gift":true,"region":"0100","tags":["a","b"]}}`},
		{ref(ui.JS.Push(o.Check, "x")), `["ann"]`, "", `{"js":[["set_text",{"to":{"sel":"#name-msg"},"text":"ann is free"}]]}`},
		{ref(ui.JS.Push(o.Remove, int64(1))), `[1]`, "", `{}`},
		{ref(ui.JS.Push(o.When)), ``, "", `{"value":"2026-01-02"}`},
	}
	for _, tt := range tests {
		got, err := do(tt.ref, tt.args, tt.form)
		if err != nil || got != tt.want {
			t.Errorf("%s %s: got %s, %v; want %s", tt.args, tt.form, got, err, tt.want)
		}
	}
	if st.pushes != 1 {
		t.Errorf("the dependency: %d pushes", st.pushes)
	}
	if _, err := do(tests[0].ref, `[0]`, ""); err == nil || err.Error() != "no such order" {
		t.Errorf("the method's error: %v", err)
	}
	if _, err := do(tests[1].ref, `["a","three","c"]`, ""); err == nil || !strings.Contains(err.Error(), `can't take "three" as a int`) {
		t.Errorf("a read that won't convert: %v", err)
	}
	if _, err := do(tests[2].ref, ``, `{"qty":["0"]}`); err == nil || !errors.As(err, new(ui.Invalid)) {
		t.Errorf("a struct breaking its validate: tags: %v", err)
	}
	if _, err := do(tests[2].ref, ``, `{"qty":["many"]}`); err == nil || !strings.Contains(err.Error(), `qty: "many" isn't a number`) {
		t.Errorf("a form field that won't convert: %v", err)
	}
	if _, err := do(tests[1].ref, `["a"]`, ""); err == nil || !strings.Contains(err.Error(), "takes 3 arguments, not 1") {
		t.Errorf("too few arguments: %v", err)
	}
}

type storeInj struct{ st *orderStore }

func (i storeInj) Inject(t reflect.Type) func(context.Context) (reflect.Value, error) {
	if t != reflect.TypeFor[*orderStore]() {
		return nil
	}
	return func(context.Context) (reflect.Value, error) { return reflect.ValueOf(i.st), nil }
}

// TestCallPush: a live handler with arguments binds them in order and
// returns its encoded result.
func TestCallPush(t *testing.T) {
	o := &orders{Name: "live"}
	out, err := ui.CallPush(context.Background(), o.Price, []json.RawMessage{json.RawMessage(`"s"`), json.RawMessage(`"2"`), json.RawMessage(`"!"`)}, nil)
	j, _ := json.Marshal(out)
	if err != nil || string(j) != `{"value":"s:xx!"}` {
		t.Errorf("got %s, %v", j, err)
	}
	if _, err := ui.CallPush(context.Background(), func(a, b int) {}, []json.RawMessage{json.RawMessage(`1`)}, nil); err == nil || !strings.Contains(err.Error(), "takes 2 arguments, not 1") {
		t.Errorf("too few: %v", err)
	}
}

// TestLoadError: SetText and SetValue take the failed push's message.
func TestLoadError(t *testing.T) {
	if got := encode(t, ui.JS.SetText("#err", ui.LoadError)); got != `[["set_text",{"to":{"sel":"#err"},"text":{"$read":{"error":true}}}]]` {
		t.Errorf("got %s", got)
	}
	if got := encode(t, ui.JS.SetValue("#err", ui.LoadError)); got != `[["set_value",{"to":{"sel":"#err"},"value":{"$read":{"error":true}}}]]` {
		t.Errorf("got %s", got)
	}
}
