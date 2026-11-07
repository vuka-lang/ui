package ui

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/vuka-lang/vuka/v2"
	vfield "github.com/vuka-lang/vuka/v2/field"
)

// The field attributes of forms: what a form field's tags say (label:,
// help:, validate:, msg:, …), as typed values written after the field.
// A model's own attributes describe its fields the same way (orm.Char{Max:
// 100} is a maxlength and a rule), so a form of a model states only what
// the model doesn't:
//
//	type Signup struct {
//		Email    string         @ui.Email @ui.Label("Email address")
//		Password string         @ui.Min(12) @ui.Widget("password")
//		Plan     string         @ui.OneOf("free", "pro")
//		Referrer Option[string] @ui.Max(40)
//		Terms    bool           @ui.Required("Accept the terms to continue")
//	}
//
// A field of a ui.FormFor is required by its type: a plain field must be
// filled, an Option may be left empty (None), and a bool unticked is false
// — @ui.Required on a bool means it must be ticked. Min and Max are a
// text's length and a number's value. A setting given by both a tag and an
// attribute is a compile error: each has one source.
type (
	// Label is the field's label: the field's name in words otherwise.
	Label string
	// Help is the hint below its control.
	Help string
	// Placeholder is the placeholder of its control.
	Placeholder string
	// Widget is its control, as the input: tag names them: "textarea",
	// "radio", "password", "email", "date", …
	Widget string
	// Autocomplete is its control's autocomplete token.
	Autocomplete string
	// Required makes the field required, its message the text when there
	// is one: @ui.Required, @ui.Required("Enter your name"). On a bool it
	// must be ticked.
	Required string
	// Email checks the value is an email address, its message the text
	// when there is one.
	Email string
	// Min and Max bound a text's length, or a number's value.
	Min float64
	Max float64
	// Pattern is a regular expression the whole value matches.
	Pattern string
)

// MsgAttr is Msg's attribute.
type MsgAttr struct{ Rule, Text string }

// Msg gives the field its own message for a rule: @ui.Msg("max", "Keep it
// under {n} characters").
func Msg(rule, text string) MsgAttr { return MsgAttr{rule, text} }

// OneOfAttr is OneOf's attribute.
type OneOfAttr []vfield.Choice

// OneOf is the values the field may take, a select's options (radios with
// @ui.Widget("radio")): @ui.OneOf("free", "pro").
func OneOf(values ...string) OneOfAttr {
	out := make(OneOfAttr, len(values))
	for i, v := range values {
		out[i] = vfield.Choice{Value: v, Label: v}
	}
	return out
}

func (l Label) Describe(s *vfield.Spec)        { s.Label = string(l) }
func (h Help) Describe(s *vfield.Spec)         { s.Help = string(h) }
func (p Placeholder) Describe(s *vfield.Spec)  { s.Placeholder = string(p) }
func (w Widget) Describe(s *vfield.Spec)       { s.Widget = string(w) }
func (a Autocomplete) Describe(s *vfield.Spec) { s.Autocomplete = string(a) }
func (m MsgAttr) Describe(s *vfield.Spec)      { s.SetMessage(m.Rule, m.Text) }
func (o OneOfAttr) Describe(s *vfield.Spec)    { s.Choices = o }
func (p Pattern) Describe(s *vfield.Spec)      { s.Pattern = string(p) }
func (n Min) Describe(s *vfield.Spec)          { s.Min = vfield.Float(float64(n)) }
func (n Max) Describe(s *vfield.Spec)          { s.Max = vfield.Float(float64(n)) }

func (r Required) Describe(s *vfield.Spec) {
	s.Required, s.Optional = true, false
	s.SetMessage("required", string(r))
}

func (e Email) Describe(s *vfield.Spec) {
	s.Email = true
	s.SetMessage("email", string(e))
}

// FieldsAttr is Fields' attribute.
type FieldsAttr []string

// Fields names the fields a ui.FormFor edits, for markup the compiler
// can't see into (a form handed to a component of its own): written on the
// form's field, @ui.Fields(Product.Name, Product.Price), it takes the place
// of the fields the markup binds. A field is a field reference, or its Go
// path as text ("Address.City").
func Fields(fields ...any) FieldsAttr {
	out := make(FieldsAttr, 0, len(fields))
	for _, f := range fields {
		switch f := f.(type) {
		case string:
			out = append(out, f)
		case interface{ FieldPath() vuka.FieldPath }:
			out = append(out, f.FieldPath().String())
		case interface{ Name() string }:
			out = append(out, f.Name())
		}
	}
	return out
}

func (f FieldsAttr) Apply(o *FieldOptions) { o.FormFields = f }

// Where the attributes go: fields; vuka refuses them anywhere else.
func (Label) vukaTargets() [vuka.OnField]struct{}        { return [vuka.OnField]struct{}{} }
func (Help) vukaTargets() [vuka.OnField]struct{}         { return [vuka.OnField]struct{}{} }
func (Placeholder) vukaTargets() [vuka.OnField]struct{}  { return [vuka.OnField]struct{}{} }
func (Widget) vukaTargets() [vuka.OnField]struct{}       { return [vuka.OnField]struct{}{} }
func (Autocomplete) vukaTargets() [vuka.OnField]struct{} { return [vuka.OnField]struct{}{} }
func (MsgAttr) vukaTargets() [vuka.OnField]struct{}      { return [vuka.OnField]struct{}{} }
func (OneOfAttr) vukaTargets() [vuka.OnField]struct{}    { return [vuka.OnField]struct{}{} }
func (Pattern) vukaTargets() [vuka.OnField]struct{}      { return [vuka.OnField]struct{}{} }
func (Min) vukaTargets() [vuka.OnField]struct{}          { return [vuka.OnField]struct{}{} }
func (Max) vukaTargets() [vuka.OnField]struct{}          { return [vuka.OnField]struct{}{} }
func (Required) vukaTargets() [vuka.OnField]struct{}     { return [vuka.OnField]struct{}{} }
func (Email) vukaTargets() [vuka.OnField]struct{}        { return [vuka.OnField]struct{}{} }
func (FieldsAttr) vukaTargets() [vuka.OnField]struct{}   { return [vuka.OnField]struct{}{} }

// describe gives a form's field what its attributes say (vfield.Describer),
// under what its tags say, and its attributes' checks. A FormFor's field
// is required by its type: a plain field is, a number by its input not left
// empty; an Option, a pointer, a slice and a bool aren't, but @ui.Required
// makes them (a bool then must be ticked).
func (ft *formType) describe(f *formField, sf reflect.StructField, attrs []any) error {
	s := vfield.Describe(attrs)
	for _, a := range attrs {
		if c, ok := a.(vfield.Checker); ok {
			f.checkers = append(f.checkers, c)
		}
		if v, ok := a.(interface{ Validate(any) error }); ok {
			f.validators = append(f.validators, v.Validate)
		}
	}
	switch {
	case s.Key:
		f.fixed, f.key = "the record's primary key", true
	case s.Auto:
		f.fixed = "a value the database or the program sets"
	}
	tagged := func(k string) bool { _, ok := sf.Tag.Lookup(k); return ok }
	if s.Label != "" && !tagged("label") {
		f.label = s.Label
	}
	if s.Help != "" && !tagged("help") {
		f.help = s.Help
	}
	if s.Placeholder != "" && !tagged("placeholder") {
		f.placeholder = s.Placeholder
	}
	if !tagged("input") {
		f.inputTag = s.Widget
	}
	if !tagged("autocomplete") {
		f.autocomplete = s.Autocomplete
	}
	f.lower = s.Lower
	for r, text := range s.Messages {
		if f.msgs[r] == "" {
			if f.msgs == nil {
				f.msgs = map[string]string{}
			}
			f.msgs[r] = text
		}
	}
	for _, c := range s.Choices {
		f.options = append(f.options, Option{Value: c.Value, Label: c.Label})
	}
	add := func(r rule) {
		if !f.hasRule(r.name) {
			f.rules = append(f.rules, r)
		}
	}
	base := valueType(sf.Type)
	required := s.Required
	if ft.typed && !s.Required && !s.Optional {
		k := sf.Type.Kind()
		required = !isOption(sf.Type) && k != reflect.Pointer && k != reflect.Slice && k != reflect.Map && k != reflect.Bool && sf.Type != fileType
	}
	switch {
	case !required || f.hasRule("required"):
	case ft.typed && (isInt(base) || isFloat(base)):
		f.needsInput = true
	default:
		f.rules = append([]rule{{name: "required"}}, f.rules...)
	}
	if s.Email {
		add(rule{name: "email"})
	}
	bound := func(name string, n *float64) {
		if n != nil {
			add(rule{name: name, n: *n, param: strconv.FormatFloat(*n, 'f', -1, 64)})
		}
	}
	bound("min", s.Min)
	bound("max", s.Max)
	if s.Pattern != "" && !f.hasRule("pattern") {
		re, err := regexp.Compile(`^(?:` + s.Pattern + `)$`)
		if err != nil {
			return fmt.Errorf("the pattern %q: %w", s.Pattern, err)
		}
		f.rules = append(f.rules, rule{name: "pattern", param: s.Pattern, re: re})
	}
	return nil
}

var optionPkg = reflect.TypeFor[vuka.Option[int]]().PkgPath()

// isOption reports whether t is a vuka.Option.
func isOption(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.PkgPath() == optionPkg && strings.HasPrefix(t.Name(), "Option[")
}

// optionParts are the value and the present flag of an Option, settable
// when v is addressable.
func optionParts(v reflect.Value) (val, ok reflect.Value) {
	if !v.CanAddr() {
		c := reflect.New(v.Type()).Elem()
		c.Set(v)
		v = c
	}
	f0, f1 := v.Field(0), v.Field(1)
	return reflect.NewAt(f0.Type(), unsafe.Pointer(f0.UnsafeAddr())).Elem(),
		reflect.NewAt(f1.Type(), unsafe.Pointer(f1.UnsafeAddr())).Elem()
}

// valueType is the type of the value a field of type t holds: through
// pointers and Options.
func valueType(t reflect.Type) reflect.Type {
	for {
		switch {
		case t.Kind() == reflect.Pointer:
			t = t.Elem()
		case isOption(t):
			t = t.Field(0).Type
		default:
			return t
		}
	}
}
