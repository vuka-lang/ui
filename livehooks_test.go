package ui_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
)

type ordersPage struct {
	ui.Live
	Q       string   `json:"q"`
	Rows    []string `json:"rows"`
	mounted int
	changes []string
}

func newOrdersPage() *ordersPage {
	p := &ordersPage{}
	p.OnMount((*ordersPage).mount)
	p.Loads("rows", (*ordersPage).loadRows)
	p.OnChange((*ordersPage).changed)
	return p
}

func (p *ordersPage) LiveSchema(l *ui.LiveSchema) {
	l.Field(&p.Q, ui.Bind{Debounce: ui.NoDebounce})
	l.Field(&p.Rows, ui.Defer("rows"))
}

func (p *ordersPage) mount() { p.mounted++ }

func (p *ordersPage) loadRows(ctx context.Context) error {
	p.Rows = []string{p.Q}
	return nil
}

func (p *ordersPage) changed(c ui.Changed) error {
	p.changes = append(p.changes, strings.Join(c.Names(), ","))
	return nil
}

func (p *ordersPage) Cancel(id int64) error { return nil }

func (p *ordersPage) Render() ui.Node { return ui.Text(p.Q) }

func TestConstructorHooks(t *testing.T) {
	p := newOrdersPage()
	typ := reflect.TypeOf(p)
	if err := ui.CheckIsland(typ); err != nil {
		t.Fatal(err)
	}
	if err := ui.MountLive(context.Background(), p); err != nil || p.mounted != 1 {
		t.Fatalf("mount: %v, %d", err, p.mounted)
	}
	x := reflect.ValueOf(p)
	if err := ui.SetBound(context.Background(), x, json.RawMessage(`{"q":"go"}`)); err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.changes, ";") != "q" {
		t.Errorf("changes %v", p.changes)
	}
	ui.LoadsFrom(typ, nil).Load(context.Background(), x, nil)
	if strings.Join(p.Rows, ",") != "go" {
		t.Errorf("loaded %v", p.Rows)
	}
	var names []string
	for _, m := range ui.IslandMethods(typ) {
		names = append(names, m.Name)
	}
	if strings.Join(names, " ") != "cancel" {
		t.Errorf("callable: %v (Live's methods and the hooks aren't)", names)
	}
	zero := &ordersPage{}
	if err := ui.MountLive(context.Background(), zero); err != nil || zero.mounted != 1 {
		t.Errorf("a page built without its constructor has its type's hooks: %v, %d", err, zero.mounted)
	}
}

type valueHook struct {
	ui.Live
}

func (p *valueHook) mount()          {}
func (p *valueHook) Render() ui.Node { return nil }

type wrongHook struct {
	ui.Live
}

func (p *wrongHook) mount(n int) error { return nil }
func (p *wrongHook) Render() ui.Node   { return nil }

type twoHooks struct {
	ui.Live
}

func (p *twoHooks) mount() error    { return nil }
func (p *twoHooks) start() error    { return nil }
func (p *twoHooks) Render() ui.Node { return nil }

type exportedHook struct {
	ui.Live
}

func (p *exportedHook) Start()          {}
func (p *exportedHook) Render() ui.Node { return nil }

type lateLoader struct {
	ui.Live
	Rows []string `json:"rows"`
}

func newLateLoader() *lateLoader {
	p := &lateLoader{}
	p.Loads("rows", (*lateLoader).loadRows)
	return p
}

func (p *lateLoader) LiveSchema(l *ui.LiveSchema) { l.Field(&p.Rows, ui.Defer("rows")) }
func (p *lateLoader) loadRows()                   {}
func (p *lateLoader) Render() ui.Node             { return nil }

func TestConstructorHooksAfterBoot(t *testing.T) {
	typ := reflect.TypeFor[*lateLoader]()
	if err := ui.CheckIsland(typ); err == nil || !strings.Contains(err.Error(), "E0324") {
		t.Fatalf("before the constructor: %v, want E0324", err)
	}
	newLateLoader()
	if err := ui.CheckIsland(typ); err != nil {
		t.Errorf("after the constructor: %v", err)
	}
}

type bothHooks struct {
	ui.Live
}

func (p *bothHooks) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }
func (p *bothHooks) mount(context.Context) error { return nil }
func (p *bothHooks) Render() ui.Node             { return nil }

func TestConstructorHookErrors(t *testing.T) {
	byValue := &valueHook{}
	byValue.OnMount(byValue.mount)
	wrong := &wrongHook{}
	wrong.OnMount((*wrongHook).mount)
	group := &valueHook{}
	group.Loads("two words", (*valueHook).mount)
	first, second := &twoHooks{}, &twoHooks{}
	first.OnMount((*twoHooks).mount)
	second.OnMount((*twoHooks).start)
	exported := &exportedHook{}
	exported.OnMount((*exportedHook).Start)
	both := &bothHooks{}
	both.OnMount((*bothHooks).mount)
	for _, c := range []struct {
		name string
		page ui.Stateful
		want string
	}{
		{"a method value", byValue, "doesn't take the page first"},
		{"a wrong signature", wrong, "is given func(*ui_test.wrongHook, int) error"},
		{"a group that isn't a name", group, "a group is a name"},
		{"two mount hooks for a type", second, "declared twice"},
		{"a constructor and a LiveSchema", both, "by its constructor and by its LiveSchema"},
		{"an exported method", exported, "an exported method"},
	} {
		err := ui.MountLive(context.Background(), c.page)
		if err == nil || !strings.Contains(err.Error(), "E0336") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want E0336 %q", c.name, err, c.want)
		}
	}
}
