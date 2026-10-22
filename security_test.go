package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

const failed = "about:invalid#TemplFailedSanitizationURL"

// Every attribute a browser loads or follows as a URL is sanitized, as
// templ sanitizes an href, whatever the case of its name.
func TestURLAttributes(t *testing.T) {
	evil := "javascript:alert(document.cookie)"
	for _, tc := range []struct{ tag, attr string }{
		{"a", "href"}, {"link", "href"}, {"form", "action"}, {"object", "data"},
		{"iframe", "src"}, {"frame", "src"}, {"embed", "src"}, {"source", "src"}, {"track", "src"},
		{"video", "src"}, {"audio", "src"}, {"img", "src"}, {"input", "src"}, {"script", "src"},
		{"video", "poster"}, {"button", "formaction"}, {"input", "formaction"},
		{"area", "href"}, {"base", "href"}, {"use", "href"}, {"image", "href"}, {"svg", "href"},
		{"use", "xlink:href"}, {"a", "xlink:href"}, {"A", "HREF"}, {"IFRAME", "Src"}, {"button", "formAction"},
	} {
		for _, v := range []any{evil, ui.Text(evil), strings.ToUpper(evil), " " + evil} {
			if _, isText := v.(ui.Node); isText {
				continue
			}
			got, err := ui.String(context.Background(), ui.El(tc.tag, []ui.Attr{{Name: tc.attr, Value: v}}))
			if err != nil || !strings.Contains(got, failed) || strings.Contains(strings.ToLower(got), "javascript") {
				t.Errorf("<%s %s=%q>: %s %v", tc.tag, tc.attr, v, got, err)
			}
		}
	}
	for _, tc := range []struct {
		n    ui.Node
		want string
	}{
		{ui.El("img", []ui.Attr{{Name: "src", Value: "data:image/png;base64,AAAA"}}), `<img src="data:image/png;base64,AAAA">`},
		{ui.El("video", []ui.Attr{{Name: "poster", Value: "/p.png"}}), `<video poster="/p.png"></video>`},
		{ui.El("iframe", []ui.Attr{{Name: "src", Value: "data:text/html,<script>alert(1)</script>"}}), `<iframe src="` + failed + `"></iframe>`},
		{ui.El("img", []ui.Attr{{Name: "src", Value: "data:text/html,x"}}), `<img src="` + failed + `">`},
		{ui.El("img", []ui.Attr{{Name: "alt", Value: "javascript:"}}), `<img alt="javascript:">`},
		{ui.El("iframe", []ui.Attr{{Name: "src", Value: "https://example.com/embed"}}), `<iframe src="https://example.com/embed"></iframe>`},
	} {
		if got, err := ui.String(context.Background(), tc.n); err != nil || got != tc.want {
			t.Errorf("got %s %v, want %s", got, err, tc.want)
		}
	}
}

// JS commands carry no URL a browser would run, and change no event
// handler attribute.
func TestJSCommandsSanitize(t *testing.T) {
	render := func(ch ui.JSChain) string {
		s, err := ui.String(context.Background(), ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(ch)}}))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	for _, ch := range []ui.JSChain{
		ui.JS.Navigate("javascript:void(window.pwned=1)"),
		ui.JS.Patch(" JAVASCRIPT:alert(1)"),
		ui.JS.Navigate("data:text/html,<script>alert(1)</script>"),
		ui.JS.Navigate("vbscript:msgbox"),
		ui.JS.SetAttr(ui.Sel("#x"), "href", "javascript:alert(1)"),
		ui.JS.SetAttr(ui.Sel("#x"), "SRC", "javascript:alert(1)"),
		ui.JS.ToggleAttr(ui.Sel("#x"), "formaction", "javascript:a()", "/ok"),
		ui.JS.On(ui.Sel("#x")).SetAttr("xlink:href", "javascript:alert(1)"),
		ui.JS.On(ui.This).Navigate("javascript:alert(1)"),
	} {
		if got := render(ch); strings.Contains(strings.ToLower(got), "javascript:") || strings.Contains(got, "data:text") || strings.Contains(got, "vbscript") || !strings.Contains(got, failed) {
			t.Errorf("unsanitized: %s", got)
		}
	}
	if got := render(ui.JS.Navigate("/pets/7?x=1").Patch("https://example.com/a").SetAttr(ui.This, "title", "javascript: a word")); !strings.Contains(got, "/pets/7?x=1") || !strings.Contains(got, "https://example.com/a") || !strings.Contains(got, "javascript: a word") {
		t.Errorf("sanitized too much: %s", got)
	}
	for _, f := range []func(){
		func() { ui.JS.SetAttr(ui.This, "onmouseover", "alert(1)") },
		func() { ui.JS.SetAttr(ui.This, "ONCLICK", "alert(1)") },
		func() { ui.JS.SetAttr(ui.This, "srcdoc", "<script>alert(1)</script>") },
		func() { ui.JS.ToggleAttr(ui.This, "onfocus", "a()", "b()") },
		func() { ui.JS.RemoveAttr(ui.This, "onclick") },
		func() { ui.JS.On(ui.This).SetAttr("onload", "x") },
		func() { ui.JS.SetAttr(ui.This, `x" onclick="a()`, "x") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("no panic")
				}
			}()
			f()
		}()
	}
}

func TestLoadableURL(t *testing.T) {
	for u, ok := range map[string]bool{
		"/a": true, "a/b?c=d:e": true, "?q=1": true, "#top": true, "http://x.y": true, "HTTPS://x.y/p": true, "//x.y/p": true,
		"javascript:x": false, "\x01javascript:x": false, "jav\tascript:x": false, "data:,x": false, "vbscript:x": false,
		"mailto:a@b": false, "about:blank": false, "a:b": false,
	} {
		if ui.LoadableURL(u) != ok {
			t.Errorf("LoadableURL(%q) != %v", u, ok)
		}
	}
}
