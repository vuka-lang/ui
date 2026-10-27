// Package forms is the error model of vuka's forms: what is wrong with a
// field or a whole form, as data — its path, the rule it broke, the rule's
// parameters and a message key — and the one function that turns an error
// into the words a visitor reads.
//
// A form is a Go struct embedding ui.FormState; package ui binds, validates
// and renders it. This package holds what both a form's own methods and the
// rest of an app need without the component library: Errorf for a Clean
// method's error, Live to tell a check run while the visitor types from one
// run on submit, and Text for the message — through the app's message
// catalogue (Messages, Translations) in the request's language (WithLocale).
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
	"strconv"
	"strings"
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
// it: its Key is format formatted with args. The key is looked up in the
// message catalogue (see Text); one the catalogue doesn't have is the
// message as written, so write it as a sentence a visitor can act on: "That
// email address is already registered", or as a key a catalogue gives the
// words of: "signup.email.taken".
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

// Param is n as a rule parameter's text: 80, not 80.000000.
func Param(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }
