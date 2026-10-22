package ui_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// namedMount has the mount hook of earlier releases, found by its name.
type namedMount struct {
	ui.Live
	mounted bool
}

func (p *namedMount) Mount(ctx context.Context) error { p.mounted = true; return nil }
func (p *namedMount) Render() ui.Node                 { return nil }

// namedChanged has the OnChange hook of earlier releases.
type namedChanged struct {
	ui.Live
	Q string `json:"q"`
}

func (p *namedChanged) LiveSchema(l *ui.LiveSchema) { l.Field(&p.Q, ui.Bind{}) }
func (p *namedChanged) Changed()                    {}
func (p *namedChanged) Render() ui.Node             { return nil }

// namedLoader has a deferred group's loader of earlier releases.
type namedLoader struct {
	ui.Live
	Rows []string `json:"rows"`
}

func (p *namedLoader) LiveSchema(l *ui.LiveSchema)        { l.Field(&p.Rows, ui.Defer("rows")) }
func (p *namedLoader) LoadRows(ctx context.Context) error { return nil }
func (p *namedLoader) Render() ui.Node                    { return nil }

// declaredMount is a page whose method named Mount is declared its hook,
// as the compiler writes @ui.Mount on it.
type declaredMount struct {
	ui.Live
	mounted bool
}

func (p *declaredMount) LiveSchema(l *ui.LiveSchema)     { l.Mount(p.Mount) }
func (p *declaredMount) Mount(ctx context.Context) error { p.mounted = true; return nil }
func (p *declaredMount) Render() ui.Node                 { return nil }

func TestNamedHooks(t *testing.T) {
	p := &namedMount{}
	err := ui.MountLive(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "E0337") || !strings.Contains(err.Error(), "@ui.Mount") || p.mounted {
		t.Errorf("Mount by its name: %v (mounted %v), want E0337 naming @ui.Mount, not run", err, p.mounted)
	}
	if err := ui.CheckIsland(reflect.TypeFor[*namedChanged]()); err == nil || !strings.Contains(err.Error(), "E0337") || !strings.Contains(err.Error(), "@ui.OnChange") {
		t.Errorf("Changed by its name: %v, want E0337 naming @ui.OnChange", err)
	}
	err = ui.CheckIsland(reflect.TypeFor[*namedLoader]())
	if err == nil || !strings.Contains(err.Error(), "E0324") || !strings.Contains(err.Error(), `mark LoadRows @ui.Loads("rows")`) {
		t.Errorf("a loader by its name: %v, want E0324 naming @ui.Loads", err)
	}
	d := &declaredMount{}
	if err := ui.MountLive(context.Background(), d); err != nil || !d.mounted {
		t.Errorf("Mount declared a hook: %v (mounted %v)", err, d.mounted)
	}
	var names []string
	for _, m := range ui.IslandMethods(reflect.TypeOf(d)) {
		names = append(names, m.Name)
	}
	if len(names) != 0 {
		t.Errorf("callable: %v; a hook isn't", names)
	}
}

// staticPage is a stateless page with a mount hook and an OnChange hook.
type staticPage struct {
	Q       string `json:"q"`
	mounted int
	changes []string
}

func (p *staticPage) LiveSchema(l *ui.LiveSchema) {
	l.Field(&p.Q, ui.Bind{Debounce: ui.NoDebounce})
	l.Mount(p.mount)
	l.OnChange(p.changed)
}

func (p *staticPage) mount(ctx context.Context) error { p.mounted++; return nil }

func (p *staticPage) changed(ctx context.Context, c ui.Changed) error {
	p.changes = append(p.changes, strings.Join(c.Names(), ","))
	return nil
}

type staticNamed struct{ n int }

func (p *staticNamed) Mount() { p.n++ }

func TestMountPage(t *testing.T) {
	p := &staticPage{}
	if err := ui.CheckHooks(reflect.TypeOf(p)); err != nil {
		t.Fatal(err)
	}
	if err := ui.MountPage(context.Background(), p); err != nil || p.mounted != 1 {
		t.Fatalf("mount: %v, %d", err, p.mounted)
	}
	if err := ui.SetBound(context.Background(), reflect.ValueOf(p), json.RawMessage(`{"q":"go"}`)); err != nil || strings.Join(p.changes, ";") != "q" {
		t.Errorf("set: %v, changes %v", err, p.changes)
	}
	if err := ui.MountPage(context.Background(), &struct{ N int }{}); err != nil {
		t.Errorf("a page without hooks: %v", err)
	}
	s := &staticNamed{}
	if err := ui.CheckHooks(reflect.TypeOf(s)); err == nil || !strings.Contains(err.Error(), "E0337") {
		t.Errorf("CheckHooks: %v, want E0337", err)
	}
	if err := ui.MountPage(context.Background(), s); err == nil || s.n != 0 {
		t.Errorf("MountPage: %v (ran %d), want E0337 and no run", err, s.n)
	}
}
