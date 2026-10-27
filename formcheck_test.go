package ui_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/forms"
)

// newPet is an input with no FormState: the tags and methods alone.
type newPet struct {
	Name  string `label:"Pet's name" validate:"required,max=10"`
	Owner string `validate:"required,email"`
	Age   int
}

func (p *newPet) CleanName(ctx context.Context, store *injStore) error {
	for _, r := range store.rows {
		if r == p.Name {
			return forms.Errorf("That name is taken")
		}
	}
	return nil
}

func (p *newPet) Clean() error {
	if p.Age > 30 {
		return forms.Errorf("No pet is that old").At("age")
	}
	return nil
}

func TestCheckForm(t *testing.T) {
	store := &injStore{rows: []string{"rex"}}
	ctx := ui.WithInjector(context.Background(), injector{reflect.TypeFor[*injStore](): store})
	cases := []struct {
		in   newPet
		want []string // path, rule, message
	}{
		{newPet{Name: "tom", Owner: "a@b.co"}, nil},
		{newPet{Owner: "nope"}, []string{"name", "required", "Enter pet's name", "owner", "email", "Enter an email address in the correct format, like name@example.com"}},
		{newPet{Name: "rex", Owner: "a@b.co", Age: 40}, []string{"age", "", "No pet is that old", "name", "", "That name is taken"}},
	}
	for _, c := range cases {
		errs, err := ui.CheckForm(ctx, &c.in)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, e := range errs {
			got = append(got, e.Path, e.Rule, e.Message)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("CheckForm(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := ui.CheckForm(ctx, newPet{}); err == nil {
		t.Error("CheckForm of a struct value: no error")
	}
}
