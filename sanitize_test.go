package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// TestSanitize: what the default policy keeps, written again well-formed.
func TestSanitize(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{``, ``},
		{`plain text`, `plain text`},
		{`<p>Hello <b>bold</b> <em>em</em></p>`, `<p>Hello <b>bold</b> <em>em</em></p>`},
		{`<h2 title="t">Title</h2><hr><br>`, `<h2 title="t">Title</h2><hr/><br/>`},
		{`<ul><li>one<li>two</ul>`, `<ul><li>one</li><li>two</li></ul>`},
		{`<p>unclosed <b>bold`, `<p>unclosed <b>bold</b></p>`},
		{`<a href="https://example.com/a?b=1&amp;c=2">link</a>`, `<a href="https://example.com/a?b=1&amp;c=2">link</a>`},
		{`<a href="/posts/7">rel</a> <a href="#top">frag</a> <a href="mailto:a@example.com">m</a>`, `<a href="/posts/7">rel</a> <a href="#top">frag</a> <a href="mailto:a@example.com">m</a>`},
		{`<a href="//example.com/x">proto-relative</a>`, `<a href="//example.com/x">proto-relative</a>`},
		{`<img src="https://example.com/a.png" alt="a" width="9">`, `<img src="https://example.com/a.png" alt="a"/>`},
		{`<table><tr><td colspan="2" rowspan="x">c</td></tr></table>`, `<table><tbody><tr><td colspan="2">c</td></tr></tbody></table>`},
		{`<pre><code>if a &lt; b {}</code></pre>`, `<pre><code>if a &lt; b {}</code></pre>`},
		{`<blockquote>q</blockquote>`, `<blockquote>q</blockquote>`},
		{`a < b & c > d`, `a &lt; b &amp; c &gt; d`},
		{`<p class="x" id="y" data-vk-c="1" style="color:red">p</p>`, `<p>p</p>`},
		{`<custom-el>kept text</custom-el>`, `kept text`},
		{`<font color="red">old</font>`, `old`},
		{`<!-- comment --><p>after</p>`, `<p>after</p>`},
		{`<!DOCTYPE html><p>x</p>`, `<p>x</p>`},
		{`<html><body><p>x</p></body></html>`, `<p>x</p>`},
	} {
		if got := string(ui.Sanitize(tc.in)); got != tc.want {
			t.Errorf("Sanitize(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

// TestSanitizeXSS: nothing of a corpus of attacks runs: no script element,
// no handler, no javascript: or data: URL, no style, whatever the spelling.
func TestSanitizeXSS(t *testing.T) {
	for _, in := range []string{
		`<script>alert(1)</script>`,
		`<SCRIPT SRC=https://evil.example/x.js></SCRIPT>`,
		`<scr<script>ipt>alert(1)</script>`,
		`<img src=x onerror=alert(1)>`,
		`<img src="x" ONERROR="alert(1)">`,
		`<img/src=x/onerror=alert(1)>`,
		`<svg onload=alert(1)>`,
		`<svg><script>alert(1)</script></svg>`,
		`<svg><style><img src=x onerror=alert(1)></style></svg>`,
		`<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`,
		`<math><mi xlink:href="javascript:alert(1)">x</mi></math>`,
		`<a href="javascript:alert(1)">x</a>`,
		`<a href="JaVaScRiPt:alert(1)">x</a>`,
		`<a href=" javascript:alert(1)">x</a>`,
		`<a href="&#x6A;avascript:alert(1)">x</a>`,
		`<a href="&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)">x</a>`,
		`<a href="javascript&colon;alert(1)">x</a>`,
		`<a href="java&#x09;script:alert(1)">x</a>`,
		`<a href="java&#x0A;script:alert(1)">x</a>`,
		"<a href=\"java\tscript:alert(1)\">x</a>",
		"<a href=\"\x01javascript:alert(1)\">x</a>",
		`<a href="vbscript:msgbox(1)">x</a>`,
		`<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`,
		`<img src="data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+">`,
		`<a href="javascript​:alert(1)">x</a>`,
		`<iframe src="https://evil.example"></iframe>`,
		`<iframe srcdoc="&lt;script&gt;alert(1)&lt;/script&gt;"></iframe>`,
		`<object data="javascript:alert(1)"></object>`,
		`<embed src="javascript:alert(1)">`,
		`<form action="javascript:alert(1)"><button formaction="javascript:alert(1)">x</button></form>`,
		`<form><form><input onfocus=alert(1) autofocus></form></form>`,
		`<div style="background:url(javascript:alert(1))">x</div>`,
		`<style>@import "javascript:alert(1)";</style>`,
		`<noscript><p title="</noscript><img src=x onerror=alert(1)>">`,
		`<xmp><img src=x onerror=alert(1)></xmp>`,
		`<noembed><img src=x onerror=alert(1)></noembed>`,
		`<textarea><img src=x onerror=alert(1)></textarea>`,
		`<title><img src=x onerror=alert(1)></title>`,
		`<template><img src=x onerror=alert(1)></template>`,
		`<plaintext><img src=x onerror=alert(1)>`,
		`<p title="</p><img src=x onerror=alert(1)>">t</p>`,
		`<a href="https://ok.example" onclick="alert(1)">x</a>`,
		`<base href="javascript:alert(1)//">`,
		`<link rel=stylesheet href="https://evil.example/x.css">`,
		`<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,
		`<details open ontoggle=alert(1)>`,
		`<img src="x` + "`" + `onerror=alert(1)">`,
		`<img """><script>alert(1)</script>">`,
		`<<script>alert(1)//<</script>`,
		`<p><a href="https://ok.example"><b>unclosed`,
		`</p><img src=x onerror=alert(1)>`,
	} {
		got := string(ui.Sanitize(in))
		if bad := unsafeMarkup(got); bad != "" {
			t.Errorf("Sanitize(%q) = %q: %s", in, got, bad)
		}
		if again := string(ui.Sanitize(got)); again != got {
			t.Errorf("Sanitize(%q) = %q, and sanitized again %q", in, got, again)
		}
	}
}

// unsafeMarkup is what in s, parsed as a browser parses it, could run or
// style the page: an element the default policy doesn't keep, a handler, a
// style, a URL of another scheme.
func unsafeMarkup(s string) string {
	nodes, err := html.ParseFragment(strings.NewReader(s), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return err.Error()
	}
	kept := map[string]bool{}
	for _, e := range ui.DefaultPolicy().Elements {
		kept[e] = true
	}
	var bad string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.CommentNode:
			bad = "a comment"
		case n.Type == html.ElementNode && (n.Namespace != "" || !kept[n.Data]):
			bad = "<" + n.Data + ">"
		case n.Type == html.ElementNode:
			for _, a := range n.Attr {
				v := strings.ToLower(strings.TrimSpace(a.Val))
				if strings.HasPrefix(a.Key, "on") || a.Key == "style" || a.Namespace != "" ||
					(a.Key == "href" || a.Key == "src") && strings.Contains(v, ":") && !strings.HasPrefix(v, "http:") && !strings.HasPrefix(v, "https:") && !strings.HasPrefix(v, "mailto:") {
					bad = a.Key + "=" + a.Val
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return bad
}

// TestSanitizeWith: a Policy adds elements, attributes and schemes, and
// can't take back what is never kept.
func TestSanitizeWith(t *testing.T) {
	p := ui.DefaultPolicy()
	p.Elements = append(p.Elements, "figure", "figcaption", "script", "style", "iframe")
	p.Attrs = append(p.Attrs, "class", "onclick", "style")
	p.Schemes = append(p.Schemes, "tel")
	in := `<figure class="wide" onclick="x()" style="color:red"><figcaption>c</figcaption></figure><a href="tel:+1">call</a><script>x()</script><style>p{}</style><iframe src="https://example.com"></iframe>`
	want := `<figure class="wide"><figcaption>c</figcaption></figure><a href="tel:+1">call</a>`
	if got := string(ui.SanitizeWith(in, p)); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if got := string(ui.Sanitize(in)); got != `c<a>call</a>` {
		t.Fatalf("the default policy unchanged: %q", got)
	}
	if d := ui.DefaultPolicy(); len(d.Attrs) != 1 {
		t.Fatalf("DefaultPolicy is a copy: %v", d.Attrs)
	}
}

// TestRawHTMLRenders: a RawHTML child is written as is, a string escaped,
// and either in an attribute is its value, escaped.
func TestRawHTMLRenders(t *testing.T) {
	body := ui.Sanitize(`<b>bold</b><script>x</script>`)
	got, err := ui.String(context.Background(), ui.HTML(`<article title={title}><div>{body}</div><p>{text}</p></article>`, ui.RawHTML(`<i>"t"</i>`), body, "<b>text</b>"))
	if err != nil {
		t.Fatal(err)
	}
	want := `<article title="&lt;i&gt;&#34;t&#34;&lt;/i&gt;"><div><b>bold</b></div><p>&lt;b&gt;text&lt;/b&gt;</p></article>`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
