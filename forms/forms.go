// Package forms is the error model of vuka's forms: what is wrong with a
// field or a whole form, as data — its path, the rule it broke, the rule's
// parameters and a message key — and the one function that turns an error
// into the words a visitor reads.
//
// A form is a Go struct embedding ui.FormState; package ui binds, validates
// and renders it. This package holds what both a form's own methods and the
// rest of an app need without the component library: Errorf for a Clean
// method's error, Live to tell a check run while the visitor types from one
// run on submit, and Text for the message.
//
//	func (f *Signup) CleanEmail(ctx context.Context, users *Users) error {
//		if forms.Live(ctx) {
//			return nil // checked on submit only
//		}
//		if users.Exists(ctx, f.Email) {
//			return forms.Errorf("That email address is already registered")
//		}
//		return nil
//	}
//
// Every error carries one dotted path naming its field — "email",
// "address.city" — the same path the markup's ids, the error summary's
// links, tests and message keys use. A form-level error has the path "".
package forms

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Error is one thing wrong with a form: Path names the field ("email",
// "address.city"; "" for the form as a whole), Rule the rule it broke
// ("required", "max", "email", "number"; "" for a Clean method's error),
// Params the rule's values ({"n": "80"}), and Key the message to show — a
// key of the message catalogue, or the message itself when the catalogue
// has no such key.
type Error struct {
	Path   string
	Rule   string
	Params map[string]string
	Key    string
}

func (e Error) Error() string {
	text := Text(context.Background(), e, "")
	if e.Path == "" {
		return text
	}
	return e.Path + ": " + text
}

// Errors is several errors, as a Clean method may return them.
type Errors []Error

func (es Errors) Error() string {
	var b strings.Builder
	for i, e := range es {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Error())
	}
	return b.String()
}

// Errorf is an error of the form, or of the field whose Clean method returns
// it: its Key is format formatted with args. Without a catalogue (Phase 2 of
// the forms design) the key is the message as written, so write it as a
// sentence a visitor can act on: "That email address is already
// registered".
func Errorf(format string, args ...any) Error {
	if len(args) > 0 {
		format = fmt.Sprintf(format, args...)
	}
	return Error{Key: format}
}

// At is e at path: a Clean method's error for another field than the one it
// checks, or a form-level Clean's error for one field.
func (e Error) At(path string) Error {
	e.Path = path
	return e
}

type liveKey struct{}

// WithLive is ctx for checks run while the visitor is typing, which Live
// reports. Package ui sets it; a test of a Clean method may too.
func WithLive(ctx context.Context) context.Context {
	return context.WithValue(ctx, liveKey{}, true)
}

// Live reports whether the check running with ctx is a live one — the
// visitor is typing, or left a field — rather than the submit's. A Clean
// method that is slow or has side effects returns nil early when it is: its
// error then shows on submit, and stays until the field changes.
func Live(ctx context.Context) bool {
	v, _ := ctx.Value(liveKey{}).(bool)
	return v
}

// A Catalogue gives the text of a message key. Phase 2 of the forms design
// adds translations; until then an app overrides the default wording, or
// names its own keys, with Messages.
type Catalogue interface {
	// Message is key's text, its placeholders ({label}, {n}) still in it;
	// ok is false when the catalogue has no such key.
	Message(ctx context.Context, key string) (text string, ok bool)
}

// Messages is a catalogue of fixed texts by key:
//
//	forms.Messages{"validate.required": "{Label} can't be empty", "signup.agree": "Accept the terms to continue"}
type Messages map[string]string

// Message is the text of key.
func (m Messages) Message(_ context.Context, key string) (string, bool) {
	s, ok := m[key]
	return s, ok
}

type catalogueKey struct{}

// WithCatalogue is ctx resolving message keys with c before the defaults.
func WithCatalogue(ctx context.Context, c Catalogue) context.Context {
	return context.WithValue(ctx, catalogueKey{}, c)
}

// Text is the message a visitor reads for e, label being its field's label
// ("Email", "Full name"): e's key looked up in ctx's catalogue, then in the
// default wording; a key neither has is the message itself. Placeholders are
// filled from label and e.Params: {label} is the label in a sentence ("full
// name"), {Label} as it starts one ("Full name"), {n} and the rule's other
// parameters their values.
//
// It is the one place a message is made, so a catalogue (an app's, a
// translation's) changes every message the same way: the field's error, the
// error summary and tests' expectations.
func Text(ctx context.Context, e Error, label string) string {
	key := e.Key
	if key == "" {
		key = DefaultKey(e)
	}
	text, ok := "", false
	if c, _ := ctx.Value(catalogueKey{}).(Catalogue); c != nil {
		text, ok = c.Message(ctx, key)
	}
	if !ok {
		text, ok = defaults[key]
	}
	if !ok {
		text = key
	}
	return fill(text, label, e.Params)
}

// DefaultKey is the message key of an error that names none: "validate."
// and its rule, with the kind of value the rule measured when the wording
// differs by it ("validate.max.string", "validate.required.choice").
func DefaultKey(e Error) string {
	if e.Rule == "" {
		return "validate.invalid"
	}
	if k := e.Params["kind"]; k != "" {
		return "validate." + e.Rule + "." + k
	}
	return "validate." + e.Rule
}

// defaults is the wording messages have when nothing overrides them: plain
// instructions saying what to do, never "invalid", "please" or "oops".
var defaults = map[string]string{
	"validate.invalid":         "{Label} is not right",
	"validate.required":        "Enter {label}",
	"validate.required.email":  "Enter your email address",
	"validate.required.choice": "Select {label}",
	"validate.required.check":  "Select “{Label}” to continue",
	"validate.required.list":   "Select at least one {label}",
	"validate.email":           "Enter an email address in the correct format, like name@example.com",
	"validate.number":          "Enter a number",
	"validate.whole":           "Enter a whole number",
	"validate.format":          "Enter {label} in the correct format",
	"validate.max.string":      "{Label} must be {n} characters or less",
	"validate.min.string":      "{Label} must be {n} characters or more",
	"validate.len.string":      "{Label} must be {n} characters",
	"validate.max.number":      "{Label} must be {n} or less",
	"validate.min.number":      "{Label} must be {n} or more",
	"validate.len.number":      "{Label} must be {n}",
	"validate.max.list":        "Select {n} or fewer {label}",
	"validate.min.list":        "Select {n} or more {label}",
	"validate.len.list":        "Select {n} {label}",
	"validate.oneof":           "{Label} must be one of: {values}",
	"validate.oneof.choice":    "Select {label}",
	"validate.digits":          "{Label} must only include numbers",
}

// DefaultMessages is a copy of the default wording, by key: what an app's
// catalogue overrides.
func DefaultMessages() Messages { return maps.Clone(defaults) }

// fill puts label and params into text's placeholders.
func fill(text, label string, params map[string]string) string {
	if !strings.Contains(text, "{") {
		return text
	}
	if label == "" {
		label = "this field"
	}
	r := []string{"{Label}", upperFirst(label), "{label}", inSentence(label)}
	for _, k := range slices.Sorted(maps.Keys(params)) {
		r = append(r, "{"+k+"}", params[k])
	}
	return strings.NewReplacer(r...).Replace(text)
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// inSentence is a label as it reads inside a sentence: its first letter
// lowered, unless the first word is an acronym ("URL", "PIN").
func inSentence(s string) string {
	word, _, _ := strings.Cut(s, " ")
	if len(word) > 1 && strings.ToUpper(word) == word {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

// Param is n as a rule parameter's text: 80, not 80.000000.
func Param(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }
