package live_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/uigen"
)

// Grid is a generic component as the compiler writes one from a .vuka.html
// file: its LiveSchema is a method of Grid[T], so each instantiation
// declares its own fields and hooks.
type Grid[T any] struct {
	ui.Live
	ID     string
	Rows   []T
	Sort   string
	page   uigen.Assign[int]
	shown  int
	mounts int
}

func (g *Grid[T]) LiveSchema(l *ui.LiveSchema) {
	l.Field(&g.Sort, ui.URL("sort"))
	l.Field(&g.page, ui.URL("page").Default(1))
	l.Mount(g.mount)
}

func (g *Grid[T]) mount(context.Context) error {
	g.mounts++
	g.shown = len(g.Rows)
	return nil
}
func (g *Grid[T]) Next() { g.page.Set(g.page.Get() + 1) }
func (g *Grid[T]) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("p", []ui.Attr{a("id", g.ID)}, ui.Text(fmt.Sprintf("%d rows page %d sort %s mount %d", g.shown, g.page.Get(), g.Sort, g.mounts))),
		ui.El("button", []ui.Attr{a("id", g.ID+"-next"), a("onClick", ui.On(g.Next))}),
		ui.El("button", []ui.Attr{a("id", g.ID+"-push"), a("onClick", ui.On(func(ctx context.Context) {
			ui.EditURL(ctx, func(q url.Values) { q.Set(g.ID+".sort", "push") }, true)
		}))}))
}

type gridTask struct{ Name string }
type gridUser struct{ Email string }

// TestGenericComponent: one generic component, instantiated with two type
// arguments on one page under distinct URL keys, mounts each instance and
// keeps each one's fields in its own URL parameters, through writes, back
// and forward.
func TestGenericComponent(t *testing.T) {
	tasks := []gridTask{{"a"}, {"b"}, {"c"}}
	users := []gridUser{{"x"}}
	render := func() ui.Node {
		return ui.El("main", nil,
			ui.URLKey("tasks", ui.Component("t", nil, &Grid[gridTask]{ID: "tasks", Rows: tasks})),
			ui.URLKey("users", ui.Component("u", nil, &Grid[gridUser]{ID: "users", Rows: users})))
	}
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			s, c := urlSession(t, v, "/p?tasks.page=2&users.sort=email", render)
			check := func(url, tasks, users string) {
				t.Helper()
				if c.URL() != url || c.Text("#tasks") != tasks || c.Text("#users") != users || c.HTML() != s.HTML() {
					t.Fatalf("at %q: tasks %q, users %q; want %q: %q, %q\n%s", c.URL(), c.Text("#tasks"), c.Text("#users"), url, tasks, users, c.HTML())
				}
			}
			check("/p?tasks.page=2&users.sort=email", "3 rows page 2 sort  mount 1", "1 rows page 1 sort email mount 1")

			// Each instance's write goes to its own parameter.
			if err := c.Click("#tasks-next"); err != nil {
				t.Fatal(err)
			}
			check("/p?tasks.page=3&users.sort=email", "3 rows page 3 sort  mount 1", "1 rows page 1 sort email mount 1")
			if err := c.Click("#users-next"); err != nil {
				t.Fatal(err)
			}
			check("/p?tasks.page=3&users.page=2&users.sort=email", "3 rows page 3 sort  mount 1", "1 rows page 2 sort email mount 1")

			// A history entry, then back and forward refill each from the URL.
			if err := c.Click("#users-push"); err != nil {
				t.Fatal(err)
			}
			check("/p?tasks.page=3&users.page=2&users.sort=push", "3 rows page 3 sort  mount 1", "1 rows page 2 sort push mount 1")
			if _, err := c.Back(); err != nil {
				t.Fatal(err)
			}
			check("/p?tasks.page=3&users.page=2&users.sort=email", "3 rows page 3 sort  mount 1", "1 rows page 2 sort email mount 1")
			if _, err := c.Patch("/p?tasks.page=5&tasks.sort=name"); err != nil {
				t.Fatal(err)
			}
			check("/p?tasks.page=5&tasks.sort=name", "3 rows page 5 sort name mount 1", "1 rows page 1 sort  mount 1")
			if _, err := c.Back(); err != nil {
				t.Fatal(err)
			}
			check("/p?tasks.page=3&users.page=2&users.sort=email", "3 rows page 3 sort  mount 1", "1 rows page 2 sort email mount 1")
			if _, err := c.Forward(); err != nil {
				t.Fatal(err)
			}
			check("/p?tasks.page=5&tasks.sort=name", "3 rows page 5 sort name mount 1", "1 rows page 1 sort  mount 1")
		})
	}
}
