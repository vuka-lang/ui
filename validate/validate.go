// Package validate is the rules a live page's field keeps, as attributes
// of its LiveSchema:
//
//	l.Field(&d.Q, ui.Bind{}, validate.Required{}, validate.Max(100))
//
// or in Vuka, after the field: Q string @ui.Bind{} @validate.Required
// @validate.Max(100). A set breaking one is refused, the field's message
// in the component's live.error.
package validate

import (
	"strconv"
	"strings"

	"github.com/vuka-lang/ui/v2"
)

type (
	// Required refuses the zero value: a nil pointer, "", 0, false, an
	// empty slice.
	Required struct{}
	// OmitEmpty skips the other rules for the zero value.
	OmitEmpty struct{}
	// Email is an email address.
	Email struct{}
	// Min and Max bound a string's characters, a number's value, a slice's
	// or map's items.
	Min float64
	Max float64
	// Len is exactly that many of the same.
	Len int
	// OneOf is one of the values.
	OneOf []string
)

func (Required) Apply(o *ui.FieldOptions)  { add(o, "required") }
func (OmitEmpty) Apply(o *ui.FieldOptions) { add(o, "omitempty") }
func (Email) Apply(o *ui.FieldOptions)     { add(o, "email") }
func (m Min) Apply(o *ui.FieldOptions)     { add(o, "min="+num(float64(m))) }
func (m Max) Apply(o *ui.FieldOptions)     { add(o, "max="+num(float64(m))) }
func (n Len) Apply(o *ui.FieldOptions)     { add(o, "len="+strconv.Itoa(int(n))) }
func (v OneOf) Apply(o *ui.FieldOptions)   { add(o, "oneof="+strings.Join(v, "|")) }

func add(o *ui.FieldOptions, rule string) { o.Validate = append(o.Validate, rule) }

func num(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
