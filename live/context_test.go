package live_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
)

type contextPermissionKey struct{}
type contextPermissions struct{ allowed bool }
type contextPanel struct {
	ui.Live
	n      ui.Assign[int]
	Nested bool
}

func (*contextPanel) Noop() {}
func (p *contextPanel) Render() ui.Node {
	node := ui.Ctx(func(ctx context.Context) ui.Node {
		text := "denied"
		if ctx.Value(contextPermissionKey{}).(*contextPermissions).allowed {
			text = "allowed"
		}
		return ui.El("div", nil, ui.Text(text), ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(p.Noop)}}, ui.Text("refresh")))
	})
	if p.Nested {
		return ui.El("section", nil, node)
	}
	return node
}

func TestContextPermissionsNotSkipped(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, trees := range []bool{false, true} {
			t.Run(map[bool]string{false: "html", true: "trees"}[trees], func(t *testing.T) {
				perms := &contextPermissions{allowed: true}
				ctx := context.WithValue(context.Background(), contextPermissionKey{}, perms)
				s := live.NewSession(ctx, func(context.Context) ui.Node { return ui.Component("panel", nil, &contextPanel{Nested: nested}) })
				defer s.Close()
				if trees {
					if _, err := s.RenderTrees(); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := s.Render(); err != nil {
						t.Fatal(err)
					}
				}
				if !strings.Contains(s.HTML(), "allowed") {
					t.Fatal(s.HTML())
				}
				perms.allowed = false
				if _, err := s.Event(ui.Event{Target: "c1:0"}); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(s.HTML(), "allowed") || !strings.Contains(s.HTML(), "denied") {
					t.Fatalf("stale permissions: %s", s.HTML())
				}
				if strings.Contains(s.HTML(), "<vk-c") {
					t.Fatalf("context hid the root element: %s", s.HTML())
				}
			})
		}
	}
}
