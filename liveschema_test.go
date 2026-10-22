package ui_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/validate"
)

type tasksPage struct {
	ui.Live
	Q       string   `json:"q"`
	Status  string   `json:"status"`
	Rows    []string `json:"rows"`
	Total   int      `json:"total"`
	mounted int
	changes []string
}

func (p *tasksPage) LiveSchema(l *ui.LiveSchema) {
	l.Field(&p.Q, ui.Bind{Debounce: 500 * time.Millisecond}, ui.URL("q"), validate.Max(5))
	l.Field(&p.Status, ui.Bind{Debounce: ui.NoDebounce}, validate.OneOf{"open", "done"})
	l.Field(&p.Rows, ui.Defer("lists"))
	l.Field(&p.Total, ui.Defer("lists"))
	l.Loads("lists", p.loadLists)
	l.OnChange(p.changed)
	l.Mount(p.Mount)
}

func (p *tasksPage) Mount(ctx context.Context) error { p.mounted++; return nil }

func (p *tasksPage) changed(ctx context.Context, c ui.Changed) error {
	p.changes = append(p.changes, strings.Join(c.Names(), ","))
	if p.Q == "boom" {
		return errors.New("boom")
	}
	return nil
}

func (p *tasksPage) loadLists(ctx context.Context) error {
	p.Rows, p.Total = []string{p.Q}, 1
	return nil
}

func (p *tasksPage) Archive(id int64) error { return nil }

func (p *tasksPage) Render() ui.Node { return ui.Text(p.Q) }

func TestLiveSchemaFields(t *testing.T) {
	typ := reflect.TypeFor[*tasksPage]()
	if err := ui.CheckIsland(typ); err != nil {
		t.Fatal(err)
	}
	fields, err := ui.IslandFields(typ)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range fields {
		got = append(got, f.Handle+":"+map[bool]string{true: "bind " + f.Debounce.String(), false: "read"}[f.Bind]+":"+f.Defer)
	}
	if want := "q:bind 500ms: status:bind 0s: rows:read:Lists total:read:Lists"; strings.Join(got, " ") != want {
		t.Errorf("fields\n got  %s\n want %s", strings.Join(got, " "), want)
	}
	var names []string
	for _, m := range ui.IslandMethods(typ) {
		names = append(names, m.Name)
	}
	if strings.Join(names, " ") != "archive" {
		t.Errorf("callable: %v (LiveSchema and the hooks aren't)", names)
	}
	f, _ := typ.Elem().FieldByName("Q")
	if u := ui.FieldOptionsOf(typ, f).URL; u != "q" {
		t.Errorf("URL = %q", u)
	}
	if !ui.IsHook(&tasksPage{}, "Mount") || ui.IsHook(&tasksPage{}, "Archive") {
		t.Error("IsHook")
	}
}

func TestLiveSchemaHooks(t *testing.T) {
	p := &tasksPage{}
	if err := ui.MountLive(context.Background(), p); err != nil || p.mounted != 1 {
		t.Fatalf("mount: %v, %d", err, p.mounted)
	}
	x := reflect.ValueOf(p)
	if err := ui.SetBound(context.Background(), x, json.RawMessage(`{"q":"go","status":"open"}`)); err != nil {
		t.Fatal(err)
	}
	if p.Q != "go" || strings.Join(p.changes, ";") != "q,status" {
		t.Errorf("set: %q, changes %v", p.Q, p.changes)
	}
	for _, data := range []string{`{"q":"too long"}`, `{"status":"lost"}`} {
		var bad ui.Invalid
		if err := ui.SetBound(context.Background(), x, json.RawMessage(data)); !errors.As(err, &bad) {
			t.Errorf("%s: %v, want Invalid", data, err)
		}
	}
	if err := ui.SetBound(context.Background(), x, json.RawMessage(`{"q":"boom"}`)); err == nil || err.Error() != "boom" {
		t.Errorf("OnChange's error: %v", err)
	}
	p.Q = "go"
	ui.LoadsFrom(x.Type(), nil).Load(context.Background(), x, nil)
	if strings.Join(p.Rows, ",") != "go" || p.Total != 1 {
		t.Errorf("loaded %v %d", p.Rows, p.Total)
	}
}

type badSchema struct {
	ui.Live
	Q     string
	other string
}

var elsewhere string

func (p *badSchema) LiveSchema(l *ui.LiveSchema) {
	l.Field(&elsewhere, ui.Bind{})
}

func (p *badSchema) Render() ui.Node { return nil }

type badGroup struct {
	ui.Live
	Q string
}

func (p *badGroup) LiveSchema(l *ui.LiveSchema) { l.Loads("two words", p.load) }
func (p *badGroup) load(context.Context) error  { return nil }
func (p *badGroup) Render() ui.Node             { return nil }

type noLoader struct {
	ui.Live
	Rows []string
}

func (p *noLoader) LiveSchema(l *ui.LiveSchema) { l.Field(&p.Rows, ui.Defer("rows")) }
func (p *noLoader) Render() ui.Node             { return nil }

func TestLiveSchemaErrors(t *testing.T) {
	for _, c := range []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeFor[*badSchema](), "E0335"},
		{reflect.TypeFor[*badGroup](), "E0336"},
		{reflect.TypeFor[*noLoader](), "E0324"},
	} {
		err := ui.CheckIsland(c.typ)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %s", c.typ, err, c.want)
		}
	}
	if err := ui.MountLive(context.Background(), &badSchema{}); err == nil {
		t.Error("MountLive of a page whose LiveSchema is wrong")
	}
}
