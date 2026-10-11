package ui

import (
	"net/url"
	"strings"

	"github.com/a-h/templ"
)

// LoadableURL reports whether a browser may be sent to u by script, as
// the live runtime's navigations and redirects do: a path or a URL
// relative to the page, or an absolute http or https URL. A javascript:,
// vbscript: or data: URL loaded that way would run or show in the page's
// origin, and other schemes hand the visitor to another program; the
// runtime refuses them too.
func LoadableURL(u string) bool {
	// browsers drop leading spaces and controls, and tabs and newlines
	// anywhere, before they read a URL's scheme
	u = strings.TrimLeftFunc(u, func(r rune) bool { return r <= ' ' })
	u = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, u)
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	switch strings.ToLower(p.Scheme) {
	case "http", "https":
		return true
	case "":
		first, _, _ := strings.Cut(u, "/")
		return !strings.Contains(first, ":")
	}
	return false
}

// navURL is href for a Navigate or Patch command: href itself when the
// browser may load it (LoadableURL), else templ's failed-sanitization URL,
// which the runtime refuses to load.
func navURL(href string) string {
	if LoadableURL(href) {
		return href
	}
	return string(templ.FailedSanitizationURL)
}

// jsAttrName checks the attribute name of a SetAttr, RemoveAttr or
// ToggleAttr command, panicking (as a chain built wrong does) on one the
// commands may not change: an event handler attribute (on…), whose value
// would run as script, and iframe's srcdoc, a document of the page's
// origin. A name holding what no attribute name holds panics too.
func jsAttrName(cmd, name string) string {
	if !validName(name) {
		panic("ui: JS " + cmd + " " + quote(name) + ": not an attribute name")
	}
	if l := strings.ToLower(name); strings.HasPrefix(l, "on") || l == "srcdoc" {
		panic("ui: JS " + cmd + " " + quote(name) + ": the commands don't set event handler attributes or srcdoc; use an on… attribute of the element")
	}
	return name
}

// jsAttrValue is value for attribute name of a SetAttr or ToggleAttr
// command: a URL attribute's value sanitized as templ sanitizes an href,
// any other as it is.
func jsAttrValue(name, value string) string {
	if urlAttrName(name) {
		return string(templ.URL(value))
	}
	return value
}

// urlAttrName reports whether name is an attribute holding a URL on the
// elements that have it: the attributes a JS command can't know the tag of.
func urlAttrName(name string) bool {
	switch strings.ToLower(name) {
	case "href", "src", "action", "formaction", "xlink:href", "poster", "data":
		return true
	}
	return false
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }
