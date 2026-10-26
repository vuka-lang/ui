package ui_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

var errDown = errors.New("store down")

type store struct{ err error }

func (s store) Users() ([]string, error) { return []string{"ann", "bob"}, s.err }

// usersList is what the compiler writes for a shape whose markup is
// <ul>for _, u := range s.Users() { <li>{u}</li> }</ul>.
type usersList struct{ s store }

func (l usersList) View() ui.Node {
	var lis []ui.Node
	for _, u := range ui.Must(l.s.Users()) {
		lis = append(lis, ui.HTML(`<li>{u}</li>`, u))
	}
	return ui.HTML(`<ul>{lis}</ul>`, lis)
}

func (l usersList) Render(ctx context.Context, w io.Writer) error { return l.View().Render(ctx, w) }

func TestMust(t *testing.T) {
	ctx := context.Background()
	out, err := ui.String(ctx, ui.El("div", nil, usersList{}))
	if err != nil || out != "<div><ul><li>ann</li><li>bob</li></ul></div>" {
		t.Fatalf("%q, %v", out, err)
	}
	failing := usersList{store{errDown}}
	for name, render := range map[string]func() error{
		// inside a Node's render, called by the Node itself, in a render tree
		"Render": func() error { return ui.El("div", nil, failing).Render(ctx, io.Discard) },
		"String": func() error { _, err := ui.String(ctx, failing); return err },
		"BuildTree": func() error {
			_, err := ui.BuildTree(ctx, ui.El("div", nil, failing), false)
			return err
		},
		"RenderPage": func() error { _, err := ui.RenderPage(ctx, failing, io.Discard); return err },
	} {
		if err := render(); err != errDown {
			t.Errorf("%s: %v, want the call's error", name, err)
		}
	}
}

func TestMustOtherPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != "boom" {
			t.Fatalf("recovered %v, want the panic itself", r)
		}
	}()
	ui.String(context.Background(), ui.NodeFunc(func(context.Context, io.Writer) error { panic("boom") }))
}

func TestCatch(t *testing.T) {
	run := func(f func()) (err error) {
		defer ui.Catch(&err)
		f()
		return nil
	}
	if err := run(func() { ui.Must(0, errDown) }); err != errDown {
		t.Fatalf("Catch: %v", err)
	}
	if err := run(func() {}); err != nil {
		t.Fatalf("Catch without a panic: %v", err)
	}
	func() {
		defer func() {
			err, ok := ui.Failed(recover())
			if !ok || err != errDown {
				t.Fatalf("Failed: %v, %v", err, ok)
			}
		}()
		ui.Must("", errDown)
	}()
	if _, ok := ui.Failed("boom"); ok {
		t.Fatal("Failed(another panic)")
	}
}
