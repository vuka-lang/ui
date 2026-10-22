package ui_test

import (
	"context"
	"database/sql"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

type injStore struct{ rows []string }

type injUser struct{ Name string }

type injector map[reflect.Type]any

func (m injector) Inject(t reflect.Type) func(context.Context) (reflect.Value, error) {
	v, ok := m[t]
	if !ok {
		return nil
	}
	return func(context.Context) (reflect.Value, error) { return reflect.ValueOf(v), nil }
}

type injForm struct {
	Text string `form:"text" validate:"required"`
}

type injPage struct {
	ui.Live
	log []string
}

func (p *injPage) LiveSchema(l *ui.LiveSchema) {
	l.Mount(p.mount)
	l.OnChange(p.changed)
	l.Loads("stats", p.stats)
}

func (p *injPage) mount(ctx context.Context, s *injStore, u *injUser) error {
	p.log = append(p.log, "mount "+u.Name+" "+strings.Join(s.rows, ","))
	return nil
}

func (p *injPage) changed(c ui.Changed, s *injStore) { p.log = append(p.log, "changed") }

func (p *injPage) stats(s *injStore) error { return nil }

func (p *injPage) Add(ctx context.Context, f injForm, s *injStore) error {
	s.rows = append(s.rows, f.Text)
	return nil
}

func (p *injPage) Pick(id int, u *injUser) { p.log = append(p.log, u.Name) }

func (p *injPage) Fields(v url.Values) {}

func (p *injPage) Total() int { return 0 } // no handler: it returns a value

func (p *injPage) Render() ui.Node { return ui.Text("") }

type badInjPage struct {
	ui.Live
}

func (p *badInjPage) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }

func (p *badInjPage) mount(ctx context.Context, db *sql.DB, n int) error { return nil }

func (p *badInjPage) Add(f injForm, db *sql.DB) {}

// Move takes two values: no event fits it, a method the page's Go calls.
func (p *badInjPage) Move(from, to int, db *sql.DB) {}

func (p *badInjPage) Render() ui.Node { return ui.Text("") }

func TestInjectedHooks(t *testing.T) {
	store, user := &injStore{rows: []string{"a"}}, &injUser{"ada"}
	inj := injector{reflect.TypeFor[*injStore](): store, reflect.TypeFor[*injUser](): user}
	if errs := ui.CheckParams(reflect.TypeFor[injPage](), inj); len(errs) > 0 {
		t.Fatalf("CheckParams: %v", errs)
	}
	ctx := ui.WithInjector(context.Background(), inj)
	p := &injPage{}
	if err := ui.MountLive(ctx, p); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.log, "|"); got != "mount ada a" {
		t.Errorf("mount: %q", got)
	}
	bind := func(t reflect.Type) (reflect.Value, error) { return reflect.ValueOf(injForm{Text: "b"}), nil }
	if err := ui.CallHandler(ctx, p.Add, bind); err != nil || len(store.rows) != 2 {
		t.Errorf("Add: %v, %v", err, store.rows)
	}
	scalar := func(reflect.Type) (reflect.Value, error) { return reflect.ValueOf(7), nil }
	if err := ui.CallHandler(ctx, p.Pick, scalar); err != nil || p.log[len(p.log)-1] != "ada" {
		t.Errorf("Pick: %v, %q", err, p.log)
	}

	// Without the injector, nothing fills the store.
	err := ui.CallHandler(context.Background(), p.Add, bind)
	if err == nil || err.Error() != "Add's 3rd parameter, *ui_test.injStore, is not provided" {
		t.Errorf("no injector: %v", err)
	}
}

func TestCheckParams(t *testing.T) {
	errs := ui.CheckParams(reflect.TypeFor[*badInjPage](), injector{})
	var got []string
	for _, e := range errs {
		if e.File != "inject_test.go" || e.Line == 0 {
			t.Errorf("%s: placed at %s:%d", e.Method, e.File, e.Line)
		}
		got = append(got, e.Error()[strings.Index(e.Error(), ": ")+2:])
	}
	want := []string{
		"badInjPage.mount's 2nd parameter, *sql.DB, is not provided",
		"badInjPage.mount's 3rd parameter, int, can't come from a hook's call, which gives the context",
		"badInjPage.Add's 2nd parameter, *sql.DB, is not provided",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !errs[0].Dependency || !errs[0].Hook || errs[1].Dependency || !errs[2].Dependency || errs[2].Hook {
		t.Errorf("kinds: %+v %+v %+v", errs[0], errs[1], errs[2])
	}
	// What the event can't carry is reported by the call.
	err := ui.CallHandler(context.Background(), (&badInjPage{}).Move, nil)
	if err == nil || err.Error() != "Move's 2nd parameter, int, is a second value from the event, which carries one (the 1st parameter takes it)" {
		t.Errorf("Move: %v", err)
	}
}
