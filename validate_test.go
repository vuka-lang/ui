package ui

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type vLevel struct {
	Code string `json:"code" validate:"required,len=3"`
}

type vFilter struct {
	Years  int      `json:"years" validate:"min=1,max=40"`
	Name   string   `json:"name" validate:"omitempty,min=2"`
	Kind   string   `json:"kind" validate:"oneof=full part"`
	Email  string   `json:"email" validate:"omitempty,email"`
	Tags   []string `json:"tags" validate:"max=2"`
	Level  vLevel   `json:"level"`
	Extra  *vLevel  `json:"extra"`
	Items  []vLevel `json:"items"`
	Parent *vFilter `json:"parent"`
}

func TestValidate(t *testing.T) {
	ok := vFilter{Years: 3, Kind: "full", Level: vLevel{Code: "abc"}}
	if err := Validate(ok); err != nil {
		t.Fatalf("valid: %v", err)
	}
	bad := vFilter{Years: 0, Name: "a", Kind: "x", Email: "nope", Tags: []string{"a", "b", "c"},
		Level: vLevel{Code: "ab"}, Extra: &vLevel{}, Items: []vLevel{{Code: "abc"}, {Code: "abcd"}},
		Parent: &vFilter{Years: 50, Kind: "part", Level: vLevel{Code: "xyz"}}}
	err := Validate(&bad)
	e := ErrorOf(err)
	want := map[string]string{
		"years":        "at least 1",
		"name":         "at least 2 characters",
		"kind":         "one of full, part",
		"email":        "an email address",
		"tags":         "at most 2 items",
		"level.code":   "exactly 3 characters",
		"extra.code":   "required",
		"items.1.code": "exactly 3 characters",
		"parent.years": "at most 40",
	}
	if e.Status != 422 || !reflect.DeepEqual(e.Fields, want) {
		t.Errorf("got %+v", e)
	}
	if err := Validate([]vLevel{{Code: "abc"}, {}}); ErrorOf(err).Fields["1.code"] != "required" {
		t.Errorf("slice: %v", err)
	}
}

type vForm struct {
	Email string `form:"mail" json:"email" validate:"required"`
	Token string `form:"token" json:"-" validate:"required"`
	Skip  string `form:"-" validate:"required"`
	Level vLevel `form:"lvl"`
}

func TestValidateTags(t *testing.T) {
	want := map[string]string{"mail": "required", "token": "required", "lvl.code": "required"}
	if e := ErrorOf(ValidateTags(vForm{}, "form", "json")); !reflect.DeepEqual(e.Fields, want) {
		t.Errorf("form names: %+v", e.Fields)
	}
	want = map[string]string{"email": "required", "Skip": "required", "Level.code": "required"}
	if e := ErrorOf(Validate(vForm{})); !reflect.DeepEqual(e.Fields, want) {
		t.Errorf("json names: %+v", e.Fields)
	}
}

type vBadTag struct {
	N int `validate:"min=x"`
}

type vUnknown struct {
	Inner struct {
		S string `validate:"shout"`
	}
}

func TestCheckValidate(t *testing.T) {
	if err := CheckValidate(reflect.TypeFor[vFilter]()); err != nil {
		t.Error(err)
	}
	if err := CheckValidate(reflect.TypeFor[vBadTag]()); err == nil || !strings.Contains(err.Error(), "min takes a number") {
		t.Errorf("bad min: %v", err)
	}
	if err := CheckValidate(reflect.TypeFor[[]vUnknown]()); err == nil || !strings.Contains(err.Error(), `unknown rule "shout"`) {
		t.Errorf("unknown: %v", err)
	}
}

type vPage struct {
	Q       string `json:"q" bind:"q" validate:"max=5"`
	Page    int    `json:"page" bind:"page" validate:"min=1"`
	Where   vLevel `json:"where" bind:"where"`
	changed int
	called  int
}

func (p *vPage) LiveSchema(l *LiveSchema)                    { l.OnChange(p.onChange) }
func (p *vPage) onChange(context.Context, Changed) error     { p.changed++; return nil }
func (p *vPage) Search(ctx context.Context, f vFilter) error { p.called++; return nil }
func (p *vPage) Pick(items []vLevel, n int) error            { p.called++; return nil }

type vBadPage struct {
	Q string `json:"q" bind:"q" validate:"maxx=5"`
}

type vBadArg struct{}

func (p *vBadArg) Run(b vBadTag) error { return nil }

func TestSetBoundValidates(t *testing.T) {
	p := &vPage{Page: 1}
	x := reflect.ValueOf(p)
	err := SetBound(context.Background(), x, json.RawMessage(`{"q":"toolong","page":3}`))
	e := ErrorOf(err)
	if e.Status != 422 || !reflect.DeepEqual(e.Fields, map[string]string{"q": "at most 5 characters"}) {
		t.Errorf("q: %+v", e)
	}
	if p.Q != "" || p.Page != 1 || p.changed != 0 {
		t.Errorf("an invalid set changed the page: %+v", p)
	}
	err = SetBound(context.Background(), x, json.RawMessage(`{"q":"toolong","page":0,"where":{"code":""}}`))
	if f := ErrorOf(err).Fields; len(f) != 3 || f["page"] != "at least 1" || f["where.code"] != "required" {
		t.Errorf("all keys: %+v", f)
	}
	if err := SetBound(context.Background(), x, json.RawMessage(`{"q":"ok","page":2,"where":{"code":"abc"}}`)); err != nil {
		t.Fatal(err)
	}
	if p.Q != "ok" || p.Page != 2 || p.changed != 1 {
		t.Errorf("valid set: %+v", p)
	}
	if err := CheckIsland(reflect.TypeFor[vBadPage]()); err == nil || !strings.Contains(err.Error(), "unknown rule") {
		t.Errorf("bad field rule: %v", err)
	}
	if err := CheckIsland(reflect.TypeFor[vBadArg]()); err == nil || !strings.Contains(err.Error(), "vBadArg.Run") {
		t.Errorf("bad arg rule: %v", err)
	}
	if err := CheckIsland(reflect.TypeFor[vPage]()); err != nil {
		t.Error(err)
	}
}

func TestCallValidates(t *testing.T) {
	p := &vPage{}
	x := reflect.ValueOf(p)
	ms := map[string]IslandMethod{}
	for _, m := range IslandMethods(x.Type()) {
		ms[m.Name] = m
	}
	err := ms["search"].Call(context.Background(), x, json.RawMessage(`[{"years":0,"kind":"full","level":{"code":"x"}}]`))
	e := ErrorOf(err)
	if e.Status != 422 || !reflect.DeepEqual(e.Fields, map[string]string{"years": "at least 1", "level.code": "exactly 3 characters"}) || p.called != 0 {
		t.Errorf("search: %+v, called %d", e, p.called)
	}
	err = ms["pick"].Call(context.Background(), x, json.RawMessage(`[[{"code":"abc"},{"code":""}], 2]`))
	if f := ErrorOf(err).Fields; !reflect.DeepEqual(f, map[string]string{"1.code": "required"}) || p.called != 0 {
		t.Errorf("pick: %+v", f)
	}
	if err := ms["search"].Call(context.Background(), x, json.RawMessage(`[{"years":2,"kind":"part","level":{"code":"abc"}}]`)); err != nil || p.called != 1 {
		t.Errorf("valid call: %v", err)
	}
}
