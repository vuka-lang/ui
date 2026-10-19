package ui

// The components are a .go file declaring each one's type and a .vuka.html
// file holding its markup; vuka gen compiles the markup to the x_html.go
// files beside it, committed so the module builds with plain go build. The
// vuka command is pinned in tools.mod, a development-only module file.
//
//go:generate go tool -modfile=tools.mod vuka gen -inplace ./...

// The components' shared helpers.

// withClass is base with extra after it, space-separated, when extra is set.
func withClass(base, extra string) string {
	if extra == "" {
		return base
	}
	return base + " " + extra
}

// when is v when cond holds, else nil: an attribute written only then.
func when(cond bool, v any) any {
	if cond {
		return v
	}
	return nil
}

// nonEmpty is s, or nil when s is empty: an attribute written only when set.
func nonEmpty(s string) any { return when(s != "", s) }

// on is the attribute event (onClick, onInput, …) set to the handler h —
// a func or JS commands held as any, which an on… attribute's hole doesn't
// take — as an attribute set: <button {on("onClick", b.On)...}>. It is
// empty when h is nil.
func on(event string, h any) []Attr {
	if h == nil {
		return nil
	}
	return []Attr{{event, On(h)}}
}
