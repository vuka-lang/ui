package ui

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// IsUpdate reports whether m, a method of a live component's type, is one
// of its Update methods, which a session calls with a message and the
// browser never does: Update, or an overload of it named after the type of
// the message it takes last, as vuka names Update's overloads and as a
// person writes them —
//
//	func (c *Chat) UpdateNewMessage(m NewMessage)              { … }
//	func (c *Chat) UpdateTyping(ctx context.Context, t Typing) { … }
//
// Update__T, the name older vuka commands gave an overload, is one too.
func IsUpdate(m reflect.Method) bool {
	switch {
	case m.Name == "Update" || strings.HasPrefix(m.Name, "Update__"):
		return true
	case !strings.HasPrefix(m.Name, "Update") || m.Type.NumIn() < 2:
		return false
	}
	return m.Name == "Update"+TypeName(m.Type.In(m.Type.NumIn()-1))
}

// TypeName is t written as part of a Go name, as vuka names an overload
// after its parameters' types: int is Int, *Order Order, []Order Orders,
// map[string]int MapStringInt, any Any.
func TypeName(t reflect.Type) string {
	if t.Name() != "" {
		name, args, generic := strings.Cut(t.Name(), "[")
		if generic {
			for _, a := range splitTypeArgs(strings.TrimSuffix(args, "]")) {
				name += typeNameText(a)
			}
		}
		return upperFirst(name)
	}
	switch t.Kind() {
	case reflect.Pointer:
		return TypeName(t.Elem())
	case reflect.Slice, reflect.Array:
		return plural(TypeName(t.Elem()))
	case reflect.Map:
		return "Map" + TypeName(t.Key()) + TypeName(t.Elem())
	case reflect.Chan:
		return "Chan" + TypeName(t.Elem())
	case reflect.Func:
		return "Func"
	case reflect.Interface:
		if t.NumMethod() == 0 {
			return "Any"
		}
		return "Interface"
	case reflect.Struct:
		return "Struct"
	}
	return upperFirst(t.String())
}

// typeNameText is TypeName of a type argument as reflect writes it in a
// generic type's name: "*example.com/shop.Order".
func typeNameText(s string) string {
	switch {
	case strings.HasPrefix(s, "*"):
		return typeNameText(s[1:])
	case strings.HasPrefix(s, "[]"):
		return plural(typeNameText(s[2:]))
	case s == "interface {}":
		return "Any"
	}
	name, args, generic := strings.Cut(s, "[")
	name = name[strings.LastIndexByte(name, '.')+1:]
	if generic {
		for _, a := range splitTypeArgs(strings.TrimSuffix(args, "]")) {
			name += typeNameText(a)
		}
	}
	return upperFirst(name)
}

func splitTypeArgs(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// plural is a name's plural, as Go writes one: Ints, Addresses, Entries.
func plural(s string) string {
	switch {
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "z"),
		strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	case len(s) > 1 && s[len(s)-1] == 'y' && !strings.ContainsRune("aeiouAEIOU", rune(s[len(s)-2])):
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}
