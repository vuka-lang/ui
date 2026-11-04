package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/vuka/v2/diag"
)

// usersCard is a component rendered by its own markup, deferring an
// exported and an unexported field.
type usersCard struct {
	Live
	Users []string
	count int
	Title string
}

func (c *usersCard) LiveSchema(l *LiveSchema) {
	l.Field(&c.Users, Defer("users"))
	l.Field(&c.count, Defer("users"))
	l.Loads("users", c.loadUsers)
}

func (c *usersCard) loadUsers(ctx context.Context) error {
	c.Users, c.count = []string{"ada", "alan"}, 2
	c.Title = "dropped"
	return nil
}

func (c *usersCard) Render() Node { return Text("") }

func TestStartDeferredTemplate(t *testing.T) {
	c := &usersCard{Title: "Users"}
	if err := MountLive(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !c.Loading("users") || c.LoadErr("users") != nil {
		t.Fatal("not loading after the mount")
	}
	ds := StartDeferred(c)
	if len(ds) != 1 || ds[0].Group != "Users" || len(StartDeferred(c)) != 0 {
		t.Fatalf("started %d", len(ds))
	}
	if err := ds[0].Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.count != 0 {
		t.Fatal("the loader ran on the component, not a copy")
	}
	ds[0].Apply(nil)
	if c.Loading("Users") || c.count != 2 || len(c.Users) != 2 || c.Title != "Users" {
		t.Errorf("applied: %+v", c)
	}
	typ := reflect.TypeFor[*usersCard]()
	if IsDeferred(typ, 3) || !IsDeferred(typ, 1) || !IsDeferred(typ, 2) {
		t.Error("IsDeferred")
	}
}

func TestDeferError(t *testing.T) {
	cause := errors.New("db: down")
	var err error = &DeferError{Group: "users", Message: "error", Err: cause}
	if err.Error() != "error" || !errors.Is(err, cause) {
		t.Errorf("%v", err)
	}
}

func TestLoadingWithoutGroups(t *testing.T) {
	var l Live
	err := func() (err error) {
		defer Catch(&err)
		l.Loading("users")
		return nil
	}()
	if diag.Of(err) != "E0325" || !strings.Contains(err.Error(), `defers no group "users"`) {
		t.Errorf("got %v", err)
	}
}

type deferBoundCard struct {
	Live
	q string
}

func (c *deferBoundCard) LiveSchema(l *LiveSchema) { l.Field(&c.q, URL("q"), Defer("q")) }
func (c *deferBoundCard) Render() Node             { return nil }

func TestDeferTemplateURL(t *testing.T) {
	err := MountLive(context.Background(), &deferBoundCard{})
	if diag.Of(err) != "E0323" || !strings.Contains(err.Error(), "drop URL or Defer") {
		t.Errorf("got %v", err)
	}
}
