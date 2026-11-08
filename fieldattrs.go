package ui

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
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

// Spec is a field's details in one attribute, the single attributes'
// settings as its members: what a form shows of the field and what its
// messages say. On a model's field it describes every form of the model;
// on a form type's ui.Meta member, that form's (Meta):
//
//	Name Option[string] @orm.Char{Max: 100, Unique: true} @ui.Spec{
//		Required:    "Name is required.",
//		Placeholder: "e.g. Blue mug",
//		Msg:         ui.Msgs{"unique": "A product with this name already exists."},
//	}
//
// A Spec states only what a model's attributes don't: Min and Max may
// tighten the field's limits, Choices narrow its choices, never widen
// them. Required is the message of a required field, and makes an
// optional one (an Option, a NULL column) required; Optional makes a
// required one optional. The single attributes (@ui.Required,
// @ui.Placeholder, @ui.Msg, …) set one member each: one setting given by
// both is a compile error, as one given by a tag and an attribute is.
type Spec struct {
	Label, Help, Placeholder string
	// Widget is the field's control, by name for now: "textarea",
	// "radio", "password", … (any, so widget values can join the names).
	Widget       any
	Autocomplete string
	// Required is the message a required field shows when left empty.
	Required string
	// Optional makes a field its type or model would require optional.
	Optional bool
	// Min and Max bound a text's length, or a number's value: 0 is none.
	Min, Max int
	// Choices are the values the field may take, of the model's when it
	// has some.
	Choices []string
	// Msg are the field's messages, by rule: "required", "min", "max",
	// "len", "pattern", "oneof", "choice", "email", "unique", "number",
	// "whole", "format", "digits", "file".
	Msg Msgs
	// Disabled and ReadOnly show the field's value without letting the
	// visitor change it: a submit never sets it.
	Disabled, ReadOnly bool
}

// Msgs are a field's messages by rule, Spec's Msg.
type Msgs map[string]string

// Meta is a form type's member describing its T's field of the same name:
// the attributes written after it are that field's in this form, over
// the model's. Its type holds nothing, so a form type's Name of type
// ui.Meta is not a field of its own, and Form.Name is still T's:
//
//	type ProductForm struct {
//		ui.FormFor[Product]
//
//		Name ui.Meta @ui.Spec{Required: "Name is required.", Placeholder: "e.g. Blue mug"}
//		Note ui.Meta @ui.Widget("textarea")
//	}
//
// Different forms of one model word the same field their own way.
type Meta struct{}

// msgRules are the rules a field's message may be for.
var msgRules = []string{"required", "min", "max", "len", "pattern", "oneof", "choice", "email", "unique", "number", "whole", "format", "digits", "file"}

func (x Spec) Describe(s *vfield.Spec) {
	w := ""
	if v := reflect.ValueOf(x.Widget); v.Kind() == reflect.String {
		w = v.String()
	}
	for dst, v := range map[*string]string{&s.Label: x.Label, &s.Help: x.Help, &s.Placeholder: x.Placeholder, &s.Widget: w, &s.Autocomplete: x.Autocomplete} {
		if v != "" {
			*dst = v
		}
	}
	if x.Required != "" {
		s.Required, s.Optional = true, false
		s.SetMessage("required", x.Required)
	}
	if x.Optional {
		s.Required, s.Optional = false, true
	}
	if n := float64(x.Min); x.Min > 0 && (s.Min == nil || n > *s.Min) {
		s.Min = vfield.Float(n)
	}
	if n := float64(x.Max); x.Max > 0 && (s.Max == nil || n < *s.Max) {
		s.Max = vfield.Float(n)
	}
	if x.Choices != nil {
		var out []vfield.Choice
		for _, v := range x.Choices {
			c := vfield.Choice{Value: v, Label: v}
			for _, had := range s.Choices {
				if had.Value == v {
					c = had
				}
			}
			out = append(out, c)
		}
		s.Choices = out
	}
	for r, text := range x.Msg {
		s.SetMessage(r, text)
	}
}

// fits reports a Spec that would loosen what the attributes before it
// said of the field (s): a Max above theirs, a Min below, a choice they
// don't have; and one whose members contradict each other.
func (x Spec) fits(s vfield.Spec) error {
	switch {
	case x.Required != "" && x.Optional:
		return fmt.Errorf("ui.Spec: Required and Optional: a field is one or the other")
	case x.Widget != nil && reflect.TypeOf(x.Widget).Kind() != reflect.String:
		return fmt.Errorf("ui.Spec: Widget is a control's name (\"textarea\", \"radio\", …), not a %T", x.Widget)
	case x.Min < 0 || x.Max < 0:
		return fmt.Errorf("ui.Spec: Min and Max are 0 or more")
	case x.Max > 0 && s.Max != nil && float64(x.Max) > *s.Max:
		return fmt.Errorf("ui.Spec: Max %d is above the model's %s: a form may tighten the field's limits, not loosen them", x.Max, strconv.FormatFloat(*s.Max, 'f', -1, 64))
	case x.Min > 0 && s.Min != nil && float64(x.Min) < *s.Min:
		return fmt.Errorf("ui.Spec: Min %d is below the model's %s: a form may tighten the field's limits, not loosen them", x.Min, strconv.FormatFloat(*s.Min, 'f', -1, 64))
	}
	if x.Choices != nil && s.Choices != nil {
		for _, v := range x.Choices {
			if !slices.ContainsFunc(s.Choices, func(c vfield.Choice) bool { return c.Value == v }) {
				return fmt.Errorf("ui.Spec: the choice %q isn't one of the model's: a form may narrow the field's choices, not add to them", v)
			}
		}
	}
	for r := range x.Msg {
		if !slices.Contains(msgRules, r) {
			return fmt.Errorf("ui.Spec: Msg %q is no rule: the rules are %s", r, strings.Join(msgRules, ", "))
		}
	}
	return nil
}

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
func (Spec) vukaTargets() [vuka.OnField]struct{}         { return [vuka.OnField]struct{}{} }

// describe gives a form's field what its attributes say (vfield.Describer),
// under what its tags say, and its attributes' checks. A FormFor's field
// is required by its type: a plain field is, a number by its input not left
// empty; an Option, a pointer, a slice and a bool aren't, but @ui.Required
// makes them (a bool then must be ticked).
//
// The attributes apply in their order of precedence: what another
// library's (a model's orm.Char) implies, then ui's own, then the form
// type's (form, its ui.Meta member's): a later one's settings win.
func (ft *formType) describe(f *formField, sf reflect.StructField, attrs, form []any) error {
	attrs = byPrecedence(attrs, form)
	var s vfield.Spec
	for _, a := range attrs {
		if x, ok := a.(Spec); ok {
			if err := x.fits(s); err != nil {
				return err
			}
			f.disabled, f.readOnly = f.disabled || x.Disabled, f.readOnly || x.ReadOnly
		}
		if d, ok := a.(vfield.Describer); ok {
			d.Describe(&s)
		}
		if d, ok := a.(fieldDefault); ok {
			if v, ok := d.FieldDefault(); ok {
				f.dflt = reflect.ValueOf(v)
			}
		}
	}
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

// fieldDefault is a field attribute giving a new record's starting value
// for the field: a column's default (orm.Char{Default: "draft"}).
type fieldDefault interface{ FieldDefault() (any, bool) }

var uiPkg = reflect.TypeFor[Spec]().PkgPath()

// byPrecedence is a field's attributes in the order they apply: other
// libraries' first, then ui's, each as written, then form's.
func byPrecedence(attrs, form []any) []any {
	out := make([]any, 0, len(attrs)+len(form))
	for _, own := range []bool{false, true} {
		for _, a := range attrs {
			if t := reflect.TypeOf(a); (t != nil && t.PkgPath() == uiPkg) == own {
				out = append(out, a)
			}
		}
	}
	return append(out, form...)
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
