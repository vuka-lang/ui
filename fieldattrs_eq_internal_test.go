package ui

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/vuka-lang/vuka/v2"
)

// The single field attributes and what vuka fix attributes writes them as
// (the tags, Spec's Choices and Pattern, a message-carrying Required on a
// number as Spec's Required) describe one form field alike: on a plain
// form and on a FormFor.

type attrsSignup struct {
	Email    string
	Password string
	Plan     string
	Tier     string
	Code     string
	Note     string
	Seats    int
	Referrer vuka.Option[string]
	Terms    bool
	Rate     float64
}

type tagsSignup struct {
	Email    string              `label:"Work email" autocomplete:"email" validate:"email" msg:"email=Enter a work address"`
	Password string              `input:"password" validate:"min=12" msg:"min=At least {n} characters\\, please"`
	Plan     string              `help:"Pick one"`
	Tier     string              `placeholder:"e.g. gold"`
	Code     string              `validate:"required,max=8" msg:"required=Enter the code"`
	Note     string              `input:"textarea"`
	Seats    int                 `validate:"max=50"`
	Referrer vuka.Option[string] `validate:"required"`
	Terms    bool                `validate:"required" msg:"required=Accept the terms"`
	Rate     float64             `validate:"min=0.5"`
}

func init() {
	vuka.RegisterFields[attrsSignup](map[string][]any{
		"Email":    {Label("Work email"), Autocomplete("email"), Email("Enter a work address")},
		"Password": {Widget("password"), Min(12), Msg("min", "At least {n} characters, please")},
		"Plan":     {Help("Pick one"), OneOf("free", "pro plus")},
		"Tier":     {Placeholder("e.g. gold"), Pattern("[a-z]+")},
		"Code":     {Required("Enter the code"), Max(8)},
		"Note":     {Widget("textarea")},
		"Seats":    {Max(50), Required("Seats, please")},
		"Referrer": {Required("")},
		"Terms":    {Required("Accept the terms")},
		"Rate":     {Min(0.5)},
	})
	vuka.RegisterFields[tagsSignup](map[string][]any{
		"Plan":  {Spec{Choices: []string{"free", "pro plus"}}},
		"Tier":  {Spec{Pattern: "[a-z]+"}},
		"Seats": {Spec{Required: "Seats, please"}},
	})
}

type (
	attrsForm struct {
		FormState
		attrsSignup
	}
	tagsForm struct {
		FormState
		tagsSignup
	}
)

// fieldShape is what a form shows and checks of a field.
func fieldShape(f *formField) string {
	var rules []string
	for _, r := range f.rules {
		rules = append(rules, r.name+"="+r.param)
	}
	return fmt.Sprintf("label=%q help=%q placeholder=%q input=%q/%q autocomplete=%q rules=%v msgs=%v options=%v needsInput=%v",
		f.label, f.help, f.placeholder, f.input, f.inputTag, f.autocomplete, rules, f.msgs, f.options, f.needsInput)
}

func TestFieldAttrsAsTags(t *testing.T) {
	for _, c := range []struct {
		name       string
		attrs, tag reflect.Type
	}{
		{"form", reflect.TypeFor[attrsForm](), reflect.TypeFor[tagsForm]()},
		{"FormFor", reflect.TypeFor[FormFor[attrsSignup]](), reflect.TypeFor[FormFor[tagsSignup]]()},
	} {
		a, b := formTypeOf(c.attrs), formTypeOf(c.tag)
		if a.err != nil || b.err != nil {
			t.Fatalf("%s: %v / %v", c.name, a.err, b.err)
		}
		if len(a.fields) != len(b.fields) {
			t.Fatalf("%s: %d fields, %d", c.name, len(a.fields), len(b.fields))
		}
		for i := range a.fields {
			if x, y := fieldShape(a.fields[i]), fieldShape(b.fields[i]); x != y {
				t.Errorf("%s %s:\nattributes %s\ntags       %s", c.name, a.fields[i].goName, x, y)
			}
		}
	}
}
