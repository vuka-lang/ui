package live

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// seenCard defers comments until its fallback is seen.
type seenCard struct {
	ui.Live
	Comments []string
}

func (c *seenCard) LiveSchema(l *ui.LiveSchema) {
	l.Field(&c.Comments, ui.Defer("comments").WhenSeen())
	l.Loads("comments", c.loadComments)
}

func (c *seenCard) loadComments(ctx context.Context) error {
	c.Comments = []string{"first"}
	return nil
}

func (c *seenCard) Render() ui.Node {
	if c.Loading("comments") {
		return ui.Skeleton{For: "comments"}
	}
	return ui.Text("comments " + strings.Join(c.Comments, ","))
}

func seenSession() *Session {
	return NewSession(context.Background(), func(ctx context.Context) ui.Node {
		return ui.El("main", nil, ui.Component("card", nil, &seenCard{}))
	})
}

// A group loading when seen waits for the browser to name it.
func TestDeferredSeen(t *testing.T) {
	s := seenSession()
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `data-vk-seen="comments"`) {
		t.Fatalf("no marked fallback: %s", html)
	}
	if ds := s.Deferred(); len(ds) != 0 {
		t.Fatalf("Deferred started %d", len(ds))
	}
	if ds := s.DeferredSeen([]string{"nope"}); len(ds) != 0 {
		t.Fatalf("an unknown group started %d", len(ds))
	}
	ds := s.DeferredSeen([]string{"comments"})
	if len(ds) != 1 || len(s.DeferredSeen([]string{"comments"})) != 0 {
		t.Fatalf("started %d, then more", len(ds))
	}
	ds[0].Run(context.Background())
	p, err := s.Loaded(ds[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Updates) != 1 || !strings.Contains(p.Updates[0].HTML, "comments first") {
		t.Fatalf("patch: %+v", p)
	}
}

// With no connection to ask over, LoadDeferred loads it too.
func TestLoadDeferredSeen(t *testing.T) {
	s := seenSession()
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadDeferred(context.Background()); err != nil {
		t.Fatal(err)
	}
	if html := s.HTML(); !strings.Contains(html, "comments first") {
		t.Fatalf("not loaded: %s", html)
	}
}
