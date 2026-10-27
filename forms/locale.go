package forms

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// A PluralRule is a language's plural category of a number n, written
// with frac digits after its point: "one", "few", "many" or "other", as
// the Unicode CLDR names them ("zero" and "two" too, for the languages
// having them).
type PluralRule func(n float64, frac int) string

var (
	pluralMu   sync.RWMutex
	pluralRule = map[string]PluralRule{}
)

// SetPlural sets the plural rule of a language ("ar", "cy"), for one the
// built-in rules don't cover or cover differently. The built-in rules
// follow CLDR for English and the languages counting as it does (one for
// 1, other for the rest: the default), French, Portuguese and Hindi (one
// for 0 and 1), Russian, Ukrainian and Belarusian, Polish, Czech and
// Slovak (one, few, many), and the languages with one form (Chinese,
// Japanese, Korean, Indonesian, Malay, Thai, Vietnamese).
func SetPlural(lang string, rule PluralRule) {
	pluralMu.Lock()
	pluralRule[strings.ToLower(lang)] = rule
	pluralMu.Unlock()
}

// PluralCategory is the CLDR plural category of n, written with frac
// digits after its point, in locale's language.
func PluralCategory(locale string, n float64, frac int) string {
	pluralMu.RLock()
	rule := pluralRule[strings.ToLower(locale)]
	if rule == nil {
		rule = pluralRule[strings.ToLower(baseLanguage(locale))]
	}
	pluralMu.RUnlock()
	if rule != nil {
		return rule(n, frac)
	}
	return builtinPlural(strings.ToLower(baseLanguage(locale)), n, frac)
}

func builtinPlural(lang string, n float64, frac int) string {
	whole := frac == 0 && n == math.Trunc(n)
	i := int64(math.Abs(math.Trunc(n)))
	switch lang {
	case "zh", "ja", "ko", "id", "ms", "th", "vi", "lo", "my", "km":
		return "other"
	case "fr", "pt", "hi", "bn", "fa", "am", "zu", "kn", "gu", "mr":
		if i == 0 || i == 1 {
			return "one"
		}
		return "other"
	case "ru", "uk", "be":
		if !whole {
			return "other"
		}
		switch {
		case i%10 == 1 && i%100 != 11:
			return "one"
		case i%10 >= 2 && i%10 <= 4 && (i%100 < 12 || i%100 > 14):
			return "few"
		}
		return "many"
	case "pl":
		if !whole {
			return "other"
		}
		switch {
		case i == 1:
			return "one"
		case i%10 >= 2 && i%10 <= 4 && (i%100 < 12 || i%100 > 14):
			return "few"
		}
		return "many"
	case "cs", "sk":
		switch {
		case !whole:
			return "many"
		case i == 1:
			return "one"
		case i >= 2 && i <= 4:
			return "few"
		}
		return "other"
	}
	if whole && i == 1 {
		return "one"
	}
	return "other"
}

// ParseAcceptLanguage is the languages an Accept-Language header names, the
// preferred first: "sw-TZ, en;q=0.8, *;q=0.1" is ["sw-TZ", "en"]. A
// wildcard and a language weighted 0 are left out.
func ParseAcceptLanguage(h string) []string {
	type tag struct {
		name string
		q    float64
	}
	var tags []tag
	for _, part := range strings.Split(h, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		name = strings.TrimSpace(name)
		if name == "" || name == "*" || len(name) > 35 {
			continue
		}
		q := 1.0
		for _, p := range strings.Split(params, ";") {
			if k, v, ok := strings.Cut(strings.TrimSpace(p), "="); ok && strings.TrimSpace(k) == "q" {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = f
				}
			}
		}
		if q > 0 {
			tags = append(tags, tag{name, q})
		}
		if len(tags) == 16 {
			break
		}
	}
	slices.SortStableFunc(tags, func(a, b tag) int { return cmp.Compare(b.q, a.q) })
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.name
	}
	return out
}
