package ui

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/vuka-lang/ui/v2/forms"
)

// formType is what a form struct (one embedding FormState) declares: its
// fields in order, each with its path, label, control and rules, and its
// Clean methods.
type formType struct {
	t       reflect.Type
	name    string  // the type's name in kebab case: an id's first part ("signup")
	state   []int   // the FormState field's index
	stateAt uintptr // and its offset in the struct
	fields  []*formField
	byPath  map[string]*formField
	clean   *reflect.Method            // Clean(ctx, deps…) error
	cleans  map[string]*reflect.Method // Clean<Field>, by path
	err     error
}

// formField is one field of a form: a value the visitor edits (a leaf), or a
// struct holding more (a group, its fields' paths under its own).
type formField struct {
	path   string // dotted, as every error and id names it: "email", "address.city"
	goName string // its Go name ("Email"), and its Go path's ("Address.City")
	goPath string
	index  []int   // from the form struct
	offset uintptr // from the form struct's start
	typ    reflect.Type
	group  bool // a struct of fields: a fieldset

	label, help, placeholder string
	input                    string // the input: tag, or what the type gives ("text", "checkbox", "date", …)
	inputTag                 string // the input: tag as written
	autocomplete             string
	inputmode                string
	raw                      bool // whitespace kept: trim:"-", or a password or code
	rules                    []rule
	msgs                     map[string]string // the msg: tag, rule → key
	choices                  *reflect.Method   // <Field>Choices
}

var (
	formStateType   = reflect.TypeFor[FormState]()
	textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()
	formTypes       sync.Map // reflect.Type → *formType
)

// formTypeOf is t's form description; t is a struct embedding FormState.
func formTypeOf(t reflect.Type) *formType {
	if v, ok := formTypes.Load(t); ok {
		return v.(*formType)
	}
	ft := buildFormType(t)
	v, _ := formTypes.LoadOrStore(t, ft)
	return v.(*formType)
}

// isForm reports whether t is a struct embedding FormState.
func isForm(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	f, ok := t.FieldByName("FormState")
	return ok && f.Anonymous && f.Type == formStateType
}

func buildFormType(t reflect.Type) *formType {
	ft := &formType{t: t, name: kebab(t.Name()), byPath: map[string]*formField{}, cleans: map[string]*reflect.Method{}}
	sf, _ := t.FieldByName("FormState")
	ft.state, ft.stateAt = sf.Index, sf.Offset
	ft.addFields(t, nil, 0, "", "")
	pt := reflect.PointerTo(t)
	for i := 0; i < pt.NumMethod(); i++ {
		m := pt.Method(i)
		if _, own := formStateType.MethodByName(m.Name); own {
			continue
		}
		if _, own := reflect.PointerTo(formStateType).MethodByName(m.Name); own {
			continue
		}
		switch name := m.Name; {
		case name == "Clean":
			ft.clean = &m
		case strings.HasPrefix(name, "Clean"):
			if f := ft.byGoPath(strings.TrimPrefix(name, "Clean")); f != nil && !f.group {
				ft.cleans[f.path] = &m
			}
		case strings.HasSuffix(name, "Choices"):
			if f := ft.byGoPath(strings.TrimSuffix(name, "Choices")); f != nil && !f.group {
				f.choices = &m
				if f.inputTag == "" && f.input != "checkboxes" {
					f.input = "select"
				}
			}
		}
	}
	return ft
}

// byGoPath is the field a method's name names: Email for CleanEmail,
// AddressCity for CleanAddressCity (its Go path without the dots).
func (ft *formType) byGoPath(name string) *formField {
	for _, f := range ft.fields {
		if strings.ReplaceAll(f.goPath, ".", "") == name {
			return f
		}
	}
	return nil
}

func (ft *formType) addFields(t reflect.Type, index []int, offset uintptr, path, goPath string) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.Type == formStateType || !sf.IsExported() && !sf.Anonymous {
			continue
		}
		idx := append(append([]int(nil), index...), i)
		name, _, _ := strings.Cut(sf.Tag.Get("form"), ",")
		if name == "-" {
			continue
		}
		if sf.Anonymous {
			// an embedded struct's fields are the form's own
			if st := sf.Type; st.Kind() == reflect.Struct && !isLeaf(st) {
				ft.addFields(st, idx, offset+sf.Offset, path, goPath)
			}
			continue
		}
		if name == "" {
			name = lowerCamel(sf.Name)
		}
		f := &formField{
			path: joinPath(path, name), goName: sf.Name, goPath: joinPath(goPath, sf.Name),
			index: idx, offset: offset + sf.Offset, typ: sf.Type,
			label: sf.Tag.Get("label"), help: sf.Tag.Get("help"), placeholder: sf.Tag.Get("placeholder"),
		}
		if f.label == "" {
			f.label = words(sf.Name)
		}
		rules, err := parseRules(sf.Tag.Get("validate"))
		if err != nil && ft.err == nil {
			ft.err = within(t.Name()+"."+sf.Name, err)
		}
		f.rules = rules
		if m := sf.Tag.Get("msg"); m != "" {
			f.msgs = map[string]string{}
			for _, part := range strings.Split(m, ",") {
				if r, key, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
					f.msgs[r] = key
				}
			}
		}
		if base := derefType(sf.Type); base.Kind() == reflect.Struct && !isLeaf(base) {
			f.group = true
			ft.fields = append(ft.fields, f)
			ft.byPath[f.path] = f
			if sf.Type.Kind() == reflect.Struct {
				ft.addFields(sf.Type, idx, f.offset, f.path, f.goPath)
			}
			continue
		}
		f.control(sf)
		ft.fields = append(ft.fields, f)
		ft.byPath[f.path] = f
	}
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// isLeaf reports whether a struct type is one value, not a group of fields:
// a time.Time, a TextUnmarshaler.
func isLeaf(t reflect.Type) bool {
	return t == timeType || reflect.PointerTo(t).Implements(textUnmarshaler)
}

// control decides a field's input, autocomplete and inputmode from its
// tags, type and name.
func (f *formField) control(sf reflect.StructField) {
	input := sf.Tag.Get("input")
	f.inputTag = input
	base := derefType(f.typ)
	if input == "" {
		switch {
		case base == timeType:
			input = "date"
		case base.Kind() == reflect.Bool:
			input = "checkbox"
		case base.Kind() == reflect.Slice && base.Elem().Kind() != reflect.Uint8:
			input = "checkboxes"
		case base.Kind() == reflect.String && (f.hasRule("email") || strings.EqualFold(sf.Name, "email")):
			input = "email"
		default:
			input = "text"
		}
	}
	f.input = input
	switch input {
	case "password", "new-password", "current-password", "one-time-code":
		f.raw = true
	}
	if t := sf.Tag.Get("trim"); t == "-" {
		f.raw = true
	}
	switch {
	case input == "numeric":
		f.inputmode = "numeric"
	case input == "text" && isInt(base):
		f.inputmode = "numeric"
	case input == "text" && isFloat(base):
		f.inputmode = "decimal"
	}
	f.autocomplete = sf.Tag.Get("autocomplete")
	if f.autocomplete == "" {
		f.autocomplete = inferAutocomplete(sf.Name, input)
	}
}

func (f *formField) hasRule(name string) bool {
	for _, r := range f.rules {
		if r.name == name {
			return true
		}
	}
	return false
}

func (f *formField) rule(name string) (rule, bool) {
	for _, r := range f.rules {
		if r.name == name {
			return r, true
		}
	}
	return rule{}, false
}

// required reports whether the field must be filled.
func (f *formField) required() bool { return f.hasRule("required") }

// htmlType is the type attribute an <input> for the field has.
func (f *formField) htmlType() string {
	switch f.input {
	case "password", "new-password", "current-password":
		return "password"
	case "one-time-code", "numeric", "text", "select", "textarea":
		return "text"
	case "checkboxes":
		return "checkbox"
	}
	return f.input
}

func isInt(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

func isFloat(t reflect.Type) bool { return t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64 }

// inferAutocomplete is the autocomplete token a field's name and input
// suggest (WCAG 1.3.5); "" when none fits.
func inferAutocomplete(name, input string) string {
	switch input {
	case "password", "current-password":
		return "current-password"
	case "new-password":
		return "new-password"
	case "one-time-code":
		return "one-time-code"
	case "email":
		return "email"
	case "tel":
		return "tel"
	}
	switch strings.ToLower(name) {
	case "name", "fullname":
		return "name"
	case "firstname", "givenname", "forename":
		return "given-name"
	case "lastname", "familyname", "surname":
		return "family-name"
	case "username", "login":
		return "username"
	case "phone", "telephone", "mobile", "tel":
		return "tel"
	case "postcode", "zip", "zipcode", "postalcode":
		return "postal-code"
	case "country":
		return "country"
	case "city", "town":
		return "address-level2"
	case "address", "street", "streetaddress":
		return "street-address"
	case "organization", "organisation", "company":
		return "organization"
	case "birthday", "birthdate", "dob", "dateofbirth":
		return "bday"
	case "website", "url", "homepage":
		return "url"
	}
	return ""
}

// lowerCamel is a Go name as a path element: Email is email, FirstName
// firstName, URL url, UserID userID.
func lowerCamel(s string) string {
	r := []rune(s)
	i := 0
	for i < len(r) && unicode.IsUpper(r[i]) {
		i++
	}
	switch {
	case i == 0:
		return s
	case i == len(r):
		return strings.ToLower(s)
	case i > 1:
		i-- // URLPath: the P starts the next word
	}
	return strings.ToLower(string(r[:i])) + string(r[i:])
}

// words is a Go name as a label: FirstName is "First name", UserID "User ID".
func words(s string) string {
	var out []string
	r := []rune(s)
	start := 0
	for i := 1; i <= len(r); i++ {
		if i == len(r) || unicode.IsUpper(r[i]) && (!unicode.IsUpper(r[i-1]) || i+1 < len(r) && unicode.IsLower(r[i+1])) {
			out = append(out, string(r[start:i]))
			start = i
		}
	}
	for i := 1; i < len(out); i++ {
		if strings.ToUpper(out[i]) != out[i] || utf8.RuneCountInString(out[i]) == 1 {
			out[i] = strings.ToLower(out[i])
		}
	}
	return strings.Join(out, " ")
}

// kebab is a name as part of an id: SignupForm is signup-form, address.city
// address-city.
func kebab(s string) string {
	var b strings.Builder
	r := []rune(s)
	for i, c := range r {
		switch {
		case c == '.' || c == '_' || c == ' ':
			b.WriteByte('-')
		case unicode.IsUpper(c):
			if i > 0 && r[i-1] != '.' && (!unicode.IsUpper(r[i-1]) || i+1 < len(r) && unicode.IsLower(r[i+1])) {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(c))
		case unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-':
			b.WriteRune(c)
		}
	}
	return b.String()
}

// fieldAt is the field of the form at offset off whose type is t: what a
// pointer to one of its fields names.
func (ft *formType) fieldAt(off uintptr, t reflect.Type) *formField {
	for _, f := range ft.fields {
		if f.offset == off && f.typ == t {
			return f
		}
	}
	return nil
}

// errorFor is a field's error, its key from the msg: tag when the tag
// names the rule.
func (f *formField) errorFor(r rule, kind string) forms.Error {
	e := forms.Error{Path: f.path, Rule: r.name, Params: map[string]string{}}
	if r.param != "" {
		e.Params["n"] = r.param
	}
	if kind != "" {
		e.Params["kind"] = kind
	}
	if r.name == "oneof" {
		e.Params["values"] = strings.Join(r.set, ", ")
	}
	if k := f.msgs[r.name]; k != "" {
		e.Key = k
	}
	return e
}

// String is the field as a diagnostic names it.
func (f *formField) String() string { return fmt.Sprintf("%s (%s)", f.goPath, f.path) }
