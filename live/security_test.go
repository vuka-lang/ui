package live_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/live/livetest"
)

// Order is a record page checking in Mount that the visitor may see the
// order its props name; the visitor may see order 1 only.
type Order struct {
	ui.Live
	ID      string
	mounts  *[]string
	deleted *[]string
}

func (o *Order) LiveSchema(l *ui.LiveSchema) { l.Mount(o.mount) }

func (o *Order) mount(context.Context) error {
	*o.mounts = append(*o.mounts, o.ID)
	if o.ID != "1" {
		return errors.New("not your order")
	}
	return nil
}

func (o *Order) Cancel() { *o.deleted = append(*o.deleted, o.ID) }

func (o *Order) Render() ui.Node {
	return ui.El("main", nil, ui.El("p", []ui.Attr{a("id", "at")}, ui.Text("order "+o.ID)),
		ui.El("button", []ui.Attr{a("id", "cancel"), a("onClick", ui.On(o.Cancel))}, ui.Text("cancel")))
}

// CheckedOrder takes the new URL in an Update, which checks again.
type CheckedOrder struct{ Order }

type orderParams struct{ ID string }

func (o *CheckedOrder) Update(p orderParams) error {
	if p.ID != "1" {
		return errors.New("not your order")
	}
	return nil
}

// OtherUpdate has an Update, but not one taking the router's message.
type OtherUpdate struct{ Order }

func (o *OtherUpdate) Update(n int) {}

// A patch the client asks for keeps a page only when the router says an
// Update takes the URL, and that Update takes it; otherwise the page mounts
// again, so a check in Mount sees the new URL (and refuses it).
func TestPatchMountsUnlessUpdateTakesURL(t *testing.T) {
	for _, kind := range []string{"no Update", "Update of another message", "Update taking the URL"} {
		for _, v := range []int{1, 2} {
			t.Run(fmt.Sprint(kind, " v", v), func(t *testing.T) {
				var mounts, deleted []string
				route := func(u string) live.Route {
					id := strings.TrimPrefix(u, "/orders/")
					o := Order{ID: id, mounts: &mounts, deleted: &deleted}
					var c ui.Stateful
					switch kind {
					case "no Update":
						c = &o
					case "Update of another message":
						c = &OtherUpdate{o}
					default:
						c = &CheckedOrder{o}
					}
					rt := live.Route{Render: func(context.Context) ui.Node { return ui.Component("order", nil, c) }}
					if kind != "no Update" {
						rt.Updates = map[ui.Stateful]any{c: orderParams{id}}
					}
					return rt
				}
				s := live.NewSession(context.Background(), route("/orders/1").Render)
				s.SetRouter(route)
				c, err := livetest.Join(s, v)
				if err != nil {
					t.Fatal(err)
				}
				m, _ := c.Patch("/orders/2")
				if m.Type != live.Error && m.Error == "" {
					t.Fatalf("a patch to another visitor's order: %+v", m)
				}
				c.Send(live.ClientMessage{Type: live.EventMsg, Ref: 9, Event: ui.Event{Target: "c1:0", Type: "click"}})
				if len(deleted) != 0 {
					t.Fatalf("an event acted on order %v", deleted)
				}
				want := []string{"1", "2"}
				if kind == "Update taking the URL" {
					want = []string{"1"} // kept: its Update refused the URL
				}
				if fmt.Sprint(mounts) != fmt.Sprint(want) {
					t.Fatalf("mounts %v, want %v", mounts, want)
				}
			})
		}
	}
}

// The handler of a page kept by a patch is the page's, whose Update ran.
func TestPatchKeepsPageWithUpdate(t *testing.T) {
	var mounts, deleted []string
	route := func(u string) live.Route {
		id := strings.TrimPrefix(u, "/orders/")
		c := &CheckedOrder{Order{ID: id, mounts: &mounts, deleted: &deleted}}
		return live.Route{Render: func(context.Context) ui.Node { return ui.Component("order", nil, c) }, Updates: map[ui.Stateful]any{c: orderParams{id}}}
	}
	s := live.NewSession(context.Background(), route("/orders/1").Render)
	s.SetRouter(route)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := c.Patch("/orders/1"); err != nil || m.Error != "" || len(mounts) != 1 {
		t.Fatalf("%+v %v %v", m, err, mounts)
	}
}

// SetErrorText decides what the browser reads of handlers', Mount's and
// Update's errors and panics; a PublicError's message passes.
func TestErrorText(t *testing.T) {
	var logged []string
	hide := func(err error) string {
		logged = append(logged, err.Error())
		if m, ok := live.PublicMessage(err); ok {
			return m
		}
		return "internal error"
	}
	secret := errors.New(`pq: duplicate key (SELECT password_hash FROM users)`)
	page := func(context.Context) ui.Node {
		return ui.El("main", nil,
			ui.El("button", []ui.Attr{a("onClick", ui.On(func() error { return secret }))}),
			ui.El("button", []ui.Attr{a("onClick", ui.On(func() { panic("at /srv/app/secret.go:12") }))}),
			ui.El("button", []ui.Attr{a("onClick", ui.On(func() error { return public{"title is taken"} }))}),
			ui.El("input", []ui.Attr{a("onInput", ui.On(func(n int) {}))}))
	}
	s := live.NewSession(context.Background(), page)
	s.SetErrorText(hide)
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ target, value, want string }{
		{"c0:0", "", "internal error"},
		{"c0:1", "", "internal error"},
		{"c0:2", "", "title is taken"},
		{"c0:3", "x", `live: input event: "x" isn't a whole number`},
		{"c0:9", "", `live: no handler "c0:9"`},
	} {
		m, _ := c.Send(live.ClientMessage{Type: live.EventMsg, Ref: 1, Event: ui.Event{Target: tc.target, Type: "input", Value: tc.value}})
		if m.Error != tc.want {
			t.Errorf("%s: %q, want %q", tc.target, m.Error, tc.want)
		}
	}
	if len(logged) != 5 || !strings.Contains(logged[0], "password_hash") || !strings.Contains(logged[1], "secret.go") {
		t.Fatalf("the hook sees the real errors: %q", logged)
	}

	// Mount's error and panic on a join, and without the hook, the text.
	var mounts, deleted []string
	o := &Order{ID: "2", mounts: &mounts, deleted: &deleted}
	s = live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("o", nil, o) })
	s.SetErrorText(hide)
	if _, err := livetest.Join(s, 2); err == nil || !strings.Contains(err.Error(), "internal error") || strings.Contains(err.Error(), "not your order") {
		t.Fatalf("Mount's error: %v", err)
	}
	s = live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("o", nil, &Panicky{}) })
	s.SetErrorText(hide)
	if _, err := livetest.Join(s, 1); err == nil || !strings.Contains(err.Error(), "internal error") {
		t.Fatalf("Mount's panic: %v", err)
	}
	s = live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("o", nil, &Panicky{}) })
	if _, err := livetest.Join(s, 1); err == nil || !strings.Contains(err.Error(), "Mount panicked: boom") {
		t.Fatalf("without SetErrorText: %v", err)
	}
}

type public struct{ msg string }

func (p public) Error() string         { return p.msg }
func (p public) PublicMessage() string { return p.msg }

type Panicky struct{ ui.Live }

func (p *Panicky) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }
func (p *Panicky) mount(context.Context) error { panic("boom") }
func (p *Panicky) Render() ui.Node             { return ui.El("p", nil) }

// A router's redirect to a URL the browser would run is refused.
func TestRedirectOnlyHTTP(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.El("p", nil) })
	s.SetRouter(func(u string) live.Route { return live.Route{Redirect: strings.TrimPrefix(u, "/out?to=")} })
	if _, err := livetest.Join(s, 2); err != nil {
		t.Fatal(err)
	}
	for to, ok := range map[string]bool{
		"/elsewhere": true, "https://example.com/x": true, "HTTP://example.com": true, "page?x=1": true,
		"javascript:alert(1)": false, " JavaScript:alert(1)": false, "java\tscript:alert(1)": false,
		"data:text/html,<script>alert(1)</script>": false, "vbscript:msgbox": false, "mailto:a@b.c": false,
	} {
		m := s.Handle(live.ClientMessage{Type: live.Navigate, Ref: 1, URL: "/out?to=" + to})
		if got := m.Type == live.Redirect; got != ok {
			t.Errorf("%q: %+v", to, m)
		}
	}
}
