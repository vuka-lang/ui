package forms

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A Catalogue gives the text of a message key: an app's own wording
// (Messages), or its translations (Translations).
type Catalogue interface {
	// Message is key's text, its placeholders ({label}, {n}, plural forms)
	// still in it; ok is false when the catalogue has no such key.
	Message(ctx context.Context, key string) (text string, ok bool)
}

// A LocaleCatalogue is a Catalogue that knows the language of the text it
// gives, which chooses the text's plural forms: Translations is one.
type LocaleCatalogue interface {
	Catalogue
	// MessageIn is key's text and its language tag ("sw", "pt-BR").
	MessageIn(ctx context.Context, key string) (text, locale string, ok bool)
}

// Messages is a catalogue of texts by key, in one language:
//
//	forms.Messages{
//		"validate.required": "{Label} can't be empty",
//		"signup.agree":      "Accept the terms to continue",
//		"Full name":         "Your name", // a label, by its text
//	}
//
// A key is a message key — the default wording's ("validate.required",
// "form.problem", see DefaultMessages), or one an app names in a msg: tag,
// SetError, Fail or Errorf — or a label's, hint's or legend's text, which
// the catalogue's entry translates.
type Messages map[string]string

// Message is the text of key.
func (m Messages) Message(_ context.Context, key string) (string, bool) {
	s, ok := m[key]
	return s, ok
}

// Translations are catalogues by language tag ("en", "sw", "pt-BR"): a
// message is looked up in the catalogue of the request's first language
// that has it (WithLocale), a regional tag falling back to its language's
// ("pt-BR" to "pt"):
//
//	ui.Configure(ui.Config{Messages: forms.Translations{
//		"sw": forms.Messages{"validate.required": "Weka {label}", "Full name": "Jina kamili"},
//		"fr": forms.Messages{"validate.required": "Saisissez {label}"},
//	}})
//
// A key no language of the request's has falls back to the default
// English wording.
type Translations map[string]Catalogue

// Message is the text of key in ctx's languages.
func (t Translations) Message(ctx context.Context, key string) (string, bool) {
	s, _, ok := t.MessageIn(ctx, key)
	return s, ok
}

// MessageIn is the text of key in the first of ctx's languages that has
// it, and that language.
func (t Translations) MessageIn(ctx context.Context, key string) (string, string, bool) {
	for _, tag := range Locales(ctx) {
		for _, try := range []string{tag, baseLanguage(tag)} {
			for name, c := range t {
				if c != nil && strings.EqualFold(name, try) {
					if s, ok := c.Message(ctx, key); ok {
						return s, name, true
					}
				}
			}
		}
	}
	return "", "", false
}

func baseLanguage(tag string) string {
	base, _, _ := strings.Cut(tag, "-")
	base, _, _ = strings.Cut(base, "_")
	return base
}

type catalogueKey struct{}
type baseKey struct{}
type localeKey struct{}

// catalogues are the catalogues ctx resolves keys with, the last added
// first.
type catalogues struct {
	c    Catalogue
	next *catalogues
}

// WithCatalogue is ctx resolving message keys with c before the
// catalogues ctx had, and the default wording.
func WithCatalogue(ctx context.Context, c Catalogue) context.Context {
	if c == nil {
		return ctx
	}
	next, _ := ctx.Value(catalogueKey{}).(*catalogues)
	return context.WithValue(ctx, catalogueKey{}, &catalogues{c, next})
}

// WithBase is ctx resolving message keys with c after its catalogues
// (WithCatalogue) and before the default wording: an app's catalogue,
// which a request's own override. Package ui adds its Config.Messages so.
func WithBase(ctx context.Context, c Catalogue) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, baseKey{}, c)
}

// WithLocale is ctx whose messages are in the languages tags name, the
// preferred first ("sw", "en-GB"): what Translations look up. Package web
// sets a request's from its Accept-Language header; a middleware choosing
// another (a signed-in visitor's setting) sets it before.
func WithLocale(ctx context.Context, tags ...string) context.Context {
	return context.WithValue(ctx, localeKey{}, slices.Clone(tags))
}

// Locales are ctx's languages, the preferred first; none when ctx names
// none.
func Locales(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	tags, _ := ctx.Value(localeKey{}).([]string)
	return tags
}

// Detach is a context carrying only what ctx's messages are resolved
// with — its catalogues and languages — for a value that renders messages
// after ctx's request has ended.
func Detach(ctx context.Context) context.Context {
	out := context.Background()
	if ctx == nil {
		return out
	}
	if c := ctx.Value(catalogueKey{}); c != nil {
		out = context.WithValue(out, catalogueKey{}, c)
	}
	if c := ctx.Value(baseKey{}); c != nil {
		out = context.WithValue(out, baseKey{}, c)
	}
	if l := ctx.Value(localeKey{}); l != nil {
		out = context.WithValue(out, localeKey{}, l)
	}
	return out
}

// lookup is key's text: from ctx's catalogues, its base, then the default
// wording; locale is the language of the text ("" when its catalogue
// didn't say).
func lookup(ctx context.Context, key string) (text, locale string, ok bool) {
	from := func(c Catalogue) (string, string, bool) {
		if lc, ok := c.(LocaleCatalogue); ok {
			return lc.MessageIn(ctx, key)
		}
		s, ok := c.Message(ctx, key)
		return s, "", ok
	}
	if ctx != nil {
		for cs, _ := ctx.Value(catalogueKey{}).(*catalogues); cs != nil; cs = cs.next {
			if s, l, ok := from(cs.c); ok {
				return s, l, true
			}
		}
		if c, _ := ctx.Value(baseKey{}).(Catalogue); c != nil {
			if s, l, ok := from(c); ok {
				return s, l, true
			}
		}
	}
	if s, ok := defaults[key]; ok {
		return s, "en", true
	}
	return "", "", false
}

// Translate is text in ctx's language: the catalogue's entry for it when
// it has one, text itself otherwise. A label, a hint or a legend is
// translated by its text ("Full name"), the form components' own words by
// their keys ("form.problem", "form.optional").
func Translate(ctx context.Context, text string) string {
	s, locale, ok := lookup(ctx, text)
	if !ok {
		return text
	}
	return fill(s, "", nil, localeFor(ctx, locale))
}

// Text is the message a visitor reads for e, label being its field's label
// ("Email", "Full name"): e's key looked up in ctx's catalogues, the app's
// (WithBase), then the default wording; a key none has is the message
// itself. The label is translated (Translate), then put in the
// placeholders with e.Params: {label} is the label in a sentence ("full
// name"), {Label} as it starts one ("Full name"), {n} and the rule's other
// parameters their values. A plural form chooses its words by a number,
// in the message's language:
//
//	"{Label} must be {n, plural, one {# character} other {# characters}} or less"
//
// It is the one place a message is made, so a catalogue (an app's, a
// translation's) changes every message the same way: the field's error, the
// error summary and tests' expectations.
func Text(ctx context.Context, e Error, label string) string {
	key := e.Key
	if key == "" {
		key = DefaultKey(e)
	}
	text, locale, ok := lookup(ctx, key)
	if !ok {
		text = key
	}
	if label != "" {
		label = Translate(ctx, label)
	}
	return fill(text, label, e.Params, localeFor(ctx, locale))
}

// localeFor is the language a message's plural forms follow: its
// catalogue's, else the request's first.
func localeFor(ctx context.Context, locale string) string {
	if locale != "" {
		return locale
	}
	if l := Locales(ctx); len(l) > 0 {
		return l[0]
	}
	return "en"
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
// instructions saying what to do, never "invalid", "please" or "oops"; and
// the form components' own words.
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
	"validate.choice":          "Select {label} from the list",
	"validate.max.string":      "{Label} must be {n, plural, one {# character} other {# characters}} or less",
	"validate.min.string":      "{Label} must be {n, plural, one {# character} other {# characters}} or more",
	"validate.len.string":      "{Label} must be {n, plural, one {# character} other {# characters}}",
	"validate.max.number":      "{Label} must be {n} or less",
	"validate.min.number":      "{Label} must be {n} or more",
	"validate.len.number":      "{Label} must be {n}",
	"validate.max.list":        "Select {n} or fewer {label}",
	"validate.min.list":        "Select {n} or more {label}",
	"validate.len.list":        "Select {n} {label}",
	"validate.oneof":           "{Label} must be one of: {values}",
	"validate.oneof.choice":    "Select {label}",
	"validate.digits":          "{Label} must only include numbers",

	"form.problem":       "There is a problem",
	"form.error":         "Error:",
	"form.optional":      "(optional)",
	"form.required":      "(required)",
	"form.save":          "Save",
	"form.update":        "Update",
	"form.show-password": "Show password",
	"form.leave":         "You have changes that are not saved. Leave this page?",
}

// DefaultMessages is a copy of the default wording, by key: what an app's
// catalogue overrides, and a translation translates.
func DefaultMessages() Messages { return maps.Clone(defaults) }

// fill puts label and params into text's placeholders, its plural forms
// chosen in locale's language.
func fill(text, label string, params map[string]string, locale string) string {
	if !strings.Contains(text, "{") {
		return text
	}
	text = plurals(text, params, locale)
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

// plurals replaces each plural form in text, {n, plural, one {…} other
// {…}}, with the words its number's category (in locale's language)
// chooses, "#" in them being the number. =0, =1 choose by an exact value.
func plurals(text string, params map[string]string, locale string) string {
	var b strings.Builder
	for {
		i := strings.Index(text, "{")
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		name, rest, ok := pluralHead(text[i+1:])
		if !ok {
			b.WriteString(text[:i+1])
			text = text[i+1:]
			continue
		}
		forms, after, ok := pluralForms(rest)
		if !ok {
			b.WriteString(text[:i+1])
			text = text[i+1:]
			continue
		}
		b.WriteString(text[:i])
		n := params[name]
		b.WriteString(strings.ReplaceAll(choosePlural(forms, n, locale), "#", n))
		text = after
	}
}

// pluralHead reads "n, plural," at the start of s: the parameter's name and
// what follows.
func pluralHead(s string) (name, rest string, ok bool) {
	name, rest, ok = strings.Cut(s, ",")
	name = strings.TrimSpace(name)
	if !ok || name == "" || strings.ContainsAny(name, "{} ") {
		return "", "", false
	}
	kind, rest, ok := strings.Cut(rest, ",")
	if !ok || strings.TrimSpace(kind) != "plural" {
		return "", "", false
	}
	return name, rest, true
}

// pluralForms reads the forms of a plural, "one {…} other {…}}": each
// selector's words, and what follows the plural's closing brace.
func pluralForms(s string) (map[string]string, string, bool) {
	out := map[string]string{}
	for {
		s = strings.TrimLeftFunc(s, unicode.IsSpace)
		if strings.HasPrefix(s, "}") {
			return out, s[1:], len(out) > 0
		}
		j := strings.IndexByte(s, '{')
		if j <= 0 {
			return nil, "", false
		}
		sel := strings.TrimSpace(s[:j])
		depth, k := 0, j
		for ; k < len(s); k++ {
			if s[k] == '{' {
				depth++
			} else if s[k] == '}' {
				if depth--; depth == 0 {
					break
				}
			}
		}
		if k == len(s) {
			return nil, "", false
		}
		out[sel] = s[j+1 : k]
		s = s[k+1:]
	}
}

// choosePlural is the form of forms number n (as text) takes in locale's
// language: an exact =n, its category's, else other.
func choosePlural(forms map[string]string, n, locale string) string {
	if s, ok := forms["="+n]; ok {
		return s
	}
	if f, err := strconv.ParseFloat(n, 64); err == nil {
		frac := 0
		if _, d, ok := strings.Cut(n, "."); ok {
			frac = len(d)
		}
		if s, ok := forms[PluralCategory(locale, f, frac)]; ok {
			return s
		}
	}
	return forms["other"]
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
