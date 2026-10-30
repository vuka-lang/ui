package ui_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

type shop struct{ Region string }

type geo struct{ calls int }

func (s *shop) CitiesOf(ctx context.Context, country string, g *geo) ([]ui.Option, error) {
	g.calls++
	if country == "" {
		return nil, errors.New("no country")
	}
	return []ui.Option{{Value: country + "-1", Label: s.Region + " " + country}}, nil
}

func (s shop) Sizes(n int, gift bool) []ui.OptionGroup {
	return []ui.OptionGroup{{Label: "Small", Options: []ui.Option{{Value: "s", Label: "S"}}}, {Label: map[bool]string{true: "Gift", false: "Plain"}[gift]}}
}

func (s *shop) Plain() string { return "" }

func init() {
	ui.RegisterOptions((*shop).CitiesOf)
	ui.RegisterOptions(shop.Sizes)
}

type keyHost struct{ key []byte }

func (h keyHost) FormToken(context.Context) string { return "" }
func (h keyHost) FormKey() []byte                  { return h.key }
func (h keyHost) FormStore() ui.FormStore          { return nil }

type geoInjector struct{ g *geo }

func (i geoInjector) Inject(t reflect.Type) func(context.Context) (reflect.Value, error) {
	if t != reflect.TypeFor[*geo]() {
		return nil
	}
	return func(context.Context) (reflect.Value, error) { return reflect.ValueOf(i.g), nil }
}

func pageCtx(path string) context.Context {
	ctx := ui.WithFormHost(context.Background(), keyHost{[]byte("k")})
	return ui.WithPageURL(ctx, &url.URL{Path: path}, nil)
}

// TestJSOptions: an Options step carries its method's reference, signed for
// the page's path, and its arguments; the page answers a call naming it
// (ParseOptionsCall) with the method's options.
func TestJSOptions(t *testing.T) {
	s := &shop{Region: "East"}
	ctx := pageCtx("/checkout")
	ch := ui.JS.Options(ui.Sel("#city"), s.CitiesOf, ui.This.Value()).OnError(ui.JS.Show(ui.Ref("err")))
	b, err := ui.EncodeJS(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	var ops [][]json.RawMessage
	if err := json.Unmarshal(b, &ops); err != nil || len(ops) != 1 {
		t.Fatalf("%s: %v", b, err)
	}
	var a struct {
		To      map[string]string `json:"to"`
		M       string            `json:"m"`
		Args    []json.RawMessage `json:"args"`
		OnError json.RawMessage   `json:"on_error"`
	}
	if err := json.Unmarshal(ops[0][1], &a); err != nil || string(ops[0][0]) != `"options"` {
		t.Fatalf("%s: %v", b, err)
	}
	if a.To["sel"] != "#city" || len(a.Args) != 1 || string(a.Args[0]) != `{"$read":{"value":true}}` || string(a.OnError) != `[["show",{"to":{"ref":"err"}}]]` {
		t.Errorf("the step: %s", b)
	}
	if _, err := ui.ParseOptionsCall(pageCtx("/admin"), a.M, `["tz"]`); !errors.Is(err, ui.ErrOptionsRef) {
		t.Errorf("a reference another page rendered is taken: %v", err)
	}
	if _, err := ui.ParseOptionsCall(ctx, strings.Split(a.M, ".")[0]+".forged", `["tz"]`); !errors.Is(err, ui.ErrOptionsRef) {
		t.Errorf("a forged reference is taken: %v", err)
	}
	if _, err := ui.ParseOptionsCall(ctx, a.M, `["tz", "x"]`); err == nil || !strings.Contains(err.Error(), "takes 1 arguments, not 2") {
		t.Errorf("too many arguments: %v", err)
	}
	call, err := ui.ParseOptionsCall(ctx, a.M, `["tz"]`)
	if err != nil {
		t.Fatal(err)
	}
	if call.Receiver() != reflect.TypeFor[shop]() || call.Method() != "shop.CitiesOf" {
		t.Errorf("the call names %s %s", call.Receiver(), call.Method())
	}
	g := &geo{}
	cctx := ui.WithInjector(ctx, geoInjector{g})
	out, err := call.Call(cctx, reflect.ValueOf(&shop{Region: "West"}))
	j, _ := json.Marshal(out)
	if err != nil || string(j) != `[{"value":"tz-1","label":"West tz"}]` || g.calls != 1 {
		t.Errorf("the call: %s, %v (%d calls)", j, err, g.calls)
	}
	call, _ = ui.ParseOptionsCall(ctx, a.M, `[""]`)
	if _, err := call.Call(cctx, reflect.ValueOf(&shop{})); err == nil || err.Error() != "no country" {
		t.Errorf("the method's error: %v", err)
	}
	if _, err := call.Call(ctx, reflect.ValueOf(&shop{})); err == nil || !strings.Contains(err.Error(), "is not provided") {
		t.Errorf("a dependency nothing provides: %v", err)
	}

	// A route's scope: good for the route's other paths, not another route's.
	scoped := func(path, scope string) context.Context { return ui.WithOptionsScope(pageCtx(path), scope) }
	sb, err := ui.EncodeJS(scoped("/orgs/1", "/orgs/{org}"), ui.JS.Options(ui.This, s.CitiesOf, "tz"))
	if err != nil {
		t.Fatal(err)
	}
	ref := regexp.MustCompile(`"m":"([^"]+)"`).FindStringSubmatch(string(sb))[1]
	if _, err := ui.ParseOptionsCall(scoped("/orgs/2", "/orgs/{org}"), ref, `["tz"]`); err != nil {
		t.Errorf("another path of the route: %v", err)
	}
	if _, err := ui.ParseOptionsCall(scoped("/orgs/1", "/teams/{team}"), ref, `["tz"]`); !errors.Is(err, ui.ErrOptionsRef) {
		t.Errorf("another route: %v", err)
	}

	// Value receivers, reads parsed into numbers and bools, groups.
	b, err = ui.EncodeJS(ctx, ui.JS.On(ui.Ref("size")).Options(s.Sizes, ui.Sel("#n").Value(), ui.Sel("#gift").Checked()))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(b, &ops)
	json.Unmarshal(ops[0][1], &a)
	call, err = ui.ParseOptionsCall(ctx, a.M, `["3", true]`)
	if err != nil {
		t.Fatal(err)
	}
	out, err = call.Call(ctx, reflect.ValueOf(&shop{}))
	j, _ = json.Marshal(out)
	if err != nil || string(j) != `[{"group":"Small","options":[{"value":"s","label":"S"}]},{"group":"Gift"}]` {
		t.Errorf("groups: %s, %v", j, err)
	}
	call, _ = ui.ParseOptionsCall(ctx, a.M, `["three", true]`)
	if _, err := call.Call(ctx, reflect.ValueOf(&shop{})); err == nil || !strings.Contains(err.Error(), `can't take "three" as a int`) {
		t.Errorf("an argument that doesn't parse: %v", err)
	}

	// Methods vuka didn't register, and wrong arguments, fail the render.
	if _, err := ui.EncodeJS(ctx, ui.JS.Options(ui.Sel("#x"), s.Plain)); err == nil || !strings.Contains(err.Error(), "isn't a method vuka's compiler registered") {
		t.Errorf("an unregistered method: %v", err)
	}
	if _, err := ui.EncodeJS(ctx, ui.JS.Options(ui.Sel("#x"), s.CitiesOf)); err == nil || !strings.Contains(err.Error(), "takes 1 arguments, not 0") {
		t.Errorf("a missing argument: %v", err)
	}
	for _, bad := range []any{func() {}, func(shop) string { return "" }, func(*shop, int, context.Context) []ui.Option { return nil }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("RegisterOptions(%T) doesn't panic", bad)
				}
			}()
			ui.RegisterOptions(bad)
		}()
	}
}

func TestJSSetValueReads(t *testing.T) {
	tests := []struct {
		ch   ui.JSChain
		want string
	}{
		{ui.JS.SetValue(ui.Sel("#ship-city"), ui.Sel("#bill-city").Value()), `[["set_value",{"to":{"sel":"#ship-city"},"value":{"$read":{"to":{"sel":"#bill-city"},"value":true}}}]]`},
		{ui.JS.SetValue(ui.Ref("agree"), true), `[["set_value",{"to":{"ref":"agree"},"value":true}]]`},
		{ui.JS.On(ui.Ref("agree")).SetValue(ui.This.Checked()), `[["set_value",{"to":{"ref":"agree"},"value":{"$read":{"checked":true}}}]]`},
	}
	for _, tt := range tests {
		if got := encode(t, tt.ch); got != tt.want {
			t.Errorf("got  %s\nwant %s", got, tt.want)
		}
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "SetValue takes a string, a bool or a read") {
			t.Errorf("SetValue(42): %v", r)
		}
	}()
	ui.JS.SetValue(ui.This, 42)
}

type reportHost struct {
	keyHost
	reports []ui.FormReport
}

func (h *reportHost) FormReport(_ context.Context, r ui.FormReport) { h.reports = append(h.reports, r) }

// TestFormReportChoices: a form's report carries the choices its render
// computed for the fields depending on others, from the values it has.
func TestFormReportChoices(t *testing.T) {
	h := &reportHost{keyHost: keyHost{[]byte("k")}}
	ctx := ui.WithFormHost(context.Background(), h)
	f := place{Country: "ke"}
	if _, err := ui.String(ctx, ui.Form{For: ui.For(&f)}); err != nil {
		t.Fatal(err)
	}
	if len(h.reports) != 1 {
		t.Fatalf("%d reports", len(h.reports))
	}
	j, _ := json.Marshal(h.reports[0].Choices)
	if string(j) != `{"city":[{"value":"nbo","label":"Nairobi"}]}` {
		t.Errorf("the report's choices: %s", j)
	}
}
