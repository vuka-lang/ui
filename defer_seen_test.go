package ui

import (
	"context"
	"strings"
	"testing"
)

// seenCard defers two groups: users at once, and comments once its
// fallback is seen.
type seenCard struct {
	Live
	Users    []string
	Comments []string
	note     string
}

func (c *seenCard) LiveSchema(l *LiveSchema) {
	l.Field(&c.Users, Defer("users"))
	l.Field(&c.Comments, Defer("comments").WhenSeen())
	l.Field(&c.note, Defer("comments"))
	l.Loads("users", c.loadUsers)
	l.Loads("comments", c.loadComments)
}

func (c *seenCard) loadUsers(ctx context.Context) error { c.Users = []string{"ada"}; return nil }
func (c *seenCard) loadComments(ctx context.Context) error {
	c.Comments, c.note = []string{"first"}, "n"
	return nil
}

func (c *seenCard) Render() Node { return Text("") }

func TestDeferWhenSeen(t *testing.T) {
	c := &seenCard{}
	if err := MountLive(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	ds := StartDeferred(c)
	if len(ds) != 1 || ds[0].Group != "Users" {
		t.Fatalf("StartDeferred started %d, want users alone", len(ds))
	}
	if !c.Loading("comments") {
		t.Fatal("comments not loading")
	}
	for _, names := range [][]string{nil, {"users"}, {"nope"}, {""}, {"bad name"}} {
		if got := StartSeen(c, names); len(got) != 0 {
			t.Fatalf("StartSeen(%q) started %d", names, len(got))
		}
	}
	ds = StartSeen(c, []string{"comments", "comments"})
	if len(ds) != 1 || ds[0].Group != "Comments" {
		t.Fatalf("StartSeen started %d", len(ds))
	}
	if len(StartSeen(c, []string{"Comments"})) != 0 || len(StartDeferred(c)) != 0 {
		t.Fatal("a group started twice")
	}
	if err := ds[0].Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	ds[0].Apply(nil)
	if c.Loading("comments") || c.note != "n" || len(c.Comments) != 1 {
		t.Errorf("applied: %+v", c)
	}
}

func TestStartAllDeferred(t *testing.T) {
	c := &seenCard{}
	if err := MountLive(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if ds := StartAllDeferred(c); len(ds) != 2 {
		t.Fatalf("started %d, want both groups", len(ds))
	}
}

func TestSkeletonFor(t *testing.T) {
	got, err := String(context.Background(), Skeleton{For: "comments", Lines: 2, Attrs: []Attr{{Name: "id", Value: "c"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-vk-seen="comments"`, `id="c"`, "animate-pulse"} {
		if !strings.Contains(got, want) {
			t.Errorf("%s: no %s", got, want)
		}
	}
	got, _ = String(context.Background(), Skeleton{})
	if strings.Contains(got, "data-vk-seen") {
		t.Errorf("%s: marked without For", got)
	}
	got, _ = String(context.Background(), El("div", DeferFallback("comments")))
	if !strings.Contains(got, `data-vk-seen="comments"`) {
		t.Errorf("DeferFallback: %s", got)
	}
}
