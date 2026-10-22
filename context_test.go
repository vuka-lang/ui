package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

type contextUserKey struct{}

func TestContextNodePerRender(t *testing.T) {
	node := ui.Ctx(func(ctx context.Context) ui.Node {
		return ui.F(928301, "Es(h)", ui.El("p", []ui.Attr{{Name: "class", Value: "user"}}, ui.Text(ctx.Value(contextUserKey{}).(string))))
	})
	for _, name := range []string{"alice", "<bob>"} {
		ctx := context.WithValue(context.Background(), contextUserKey{}, name)
		got, err := ui.String(ctx, node)
		want := name
		if name == "<bob>" {
			want = "&lt;bob&gt;"
		}
		if err != nil || got != `<p class="user">`+want+`</p>` {
			t.Fatalf("render: %q, %v", got, err)
		}
		tree, err := ui.BuildTree(ctx, node, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(tree.Dyn) == 0 || !strings.Contains(got, want) {
			t.Fatalf("lost dynamic frame: %#v", tree)
		}
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	node := ui.Ctx(func(ctx context.Context) ui.Node {
		return ui.Try(nil, ctx.Err())
	})
	if _, err := ui.String(ctx, node); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}

func TestContextConcurrentRenderIsolation(t *testing.T) {
	node := ui.Ctx(func(ctx context.Context) ui.Node { return ui.Text(ctx.Value(contextUserKey{}).(string)) })
	for _, name := range []string{"alice", "bob", "carol"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.WithValue(context.Background(), contextUserKey{}, name)
			for i := 0; i < 20; i++ {
				got, err := ui.String(ctx, node)
				if err != nil || got != name {
					t.Fatalf("identity crossed renders: %q, %v", got, err)
				}
			}
		})
	}
}
