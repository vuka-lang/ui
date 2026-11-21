package ui_test

import (
	"context"
	"net/url"
	"reflect"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2"
)

type optionQuery struct {
	ui.Live
	Limit vuka.Option[int64]  `json:"limit"`
	Name  vuka.Option[string] `json:"name"`
	Size  vuka.Option[int]    `json:"size"`
}

func (p *optionQuery) LiveSchema(l *ui.LiveSchema) {
	l.Field(&p.Limit, ui.URL("limit"))
	l.Field(&p.Name, ui.URL("name"))
	l.Field(&p.Size, ui.URL("size").Default(10))
}

func (p *optionQuery) Render() ui.Node { return ui.Text("") }

type optionOfSlice struct {
	ui.Live
	Ids vuka.Option[[]int] `json:"ids"`
}

func (p *optionOfSlice) LiveSchema(l *ui.LiveSchema) { l.Field(&p.Ids, ui.URL("ids")) }

func (p *optionOfSlice) Render() ui.Node { return ui.Text("") }

// TestURLOption: an Option field is kept in the query: None is left out,
// Some(0) and Some("") are written, a default given as the T is Some, and the
// URL fills it again as it changes.
func TestURLOption(t *testing.T) {
	if err := ui.CheckIsland(reflect.TypeFor[*optionQuery]()); err != nil {
		t.Fatal(err)
	}
	if err := ui.CheckIsland(reflect.TypeFor[*optionOfSlice]()); err == nil {
		t.Error("an Option of a slice has no text form")
	}
	p := &optionQuery{}
	var f ui.URLFields
	fill := func(q string) {
		t.Helper()
		v, err := url.ParseQuery(q)
		if err != nil {
			t.Fatal(err)
		}
		f.Fill(context.Background(), p, nil, "", false, v)
	}
	check := func(limit vuka.Option[int64], name vuka.Option[string], size vuka.Option[int]) {
		t.Helper()
		if p.Limit != limit || p.Name != name || p.Size != size {
			t.Errorf("filled %v %v %v, want %v %v %v", p.Limit, p.Name, p.Size, limit, name, size)
		}
	}
	fill("")
	check(vuka.None[int64](), vuka.None[string](), vuka.Some(10))
	fill("limit=0&name=&size=3")
	check(vuka.Some[int64](0), vuka.Some(""), vuka.Some(3))
	fill("limit=x&name=a")
	check(vuka.None[int64](), vuka.Some("a"), vuka.Some(10))
	fill("")
	check(vuka.None[int64](), vuka.None[string](), vuka.Some(10))

	q := url.Values{"other": {"x"}}
	edit := func(change func()) {
		t.Helper()
		f.Snapshot(p)
		change()
		f.Changes(p, nil, "", q)
	}
	edit(func() { p.Limit, p.Name, p.Size = vuka.Some[int64](0), vuka.Some(""), vuka.Some(3) })
	if want := (url.Values{"limit": {"0"}, "name": {""}, "size": {"3"}, "other": {"x"}}); !reflect.DeepEqual(q, want) {
		t.Errorf("Some(0) and Some(\"\") are written: %v", q)
	}
	edit(func() { p.Limit, p.Name, p.Size = vuka.None[int64](), vuka.None[string](), vuka.Some(10) })
	if want := (url.Values{"other": {"x"}}); !reflect.DeepEqual(q, want) {
		t.Errorf("None and the default are left out: %v", q)
	}
	if got := ui.QueryText(vuka.Some(0)); !reflect.DeepEqual(got, []string{"0"}) {
		t.Errorf("QueryText(Some(0)) = %q", got)
	}
	if got := ui.QueryText(vuka.None[int]()); got != nil {
		t.Errorf("QueryText(None) = %q", got)
	}
}
