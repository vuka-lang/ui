package tmpltest_test

import (
	"context"
	"html"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/internal/tmpltest"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// tag is c placed as its tag in a template is.
func tag(c any) ui.Node { return ui.HTML(`{c}`, c) }

// TestTemplates: ui is a .vuka.html file's target — components bind by
// name, a function component's error fails the render, live components
// render statically.
func TestTemplates(t *testing.T) {
	ctx := context.Background()
	for n, want := range []string{
		`<table class="pets"><tr><td>Rex</td><td>3</td></tr><caption>pets</caption></table>`,
		`<table class="pets"><tr><td>none</td></tr></table>`,
		`<p><img src="/a.png"></p>`,
		`<button>4</button>`,
		`<a href="/pets?page=2" data-vk-link="patch" class="next">Next <b>page</b></a>`,
		`<a href="/pets/7" data-vk-link>Rex</a>`,
	} {
		if got, err := ui.String(ctx, tmpltest.Example{N: n}); err != nil || got != want {
			t.Errorf("%d: got %q (%v), want %q", n, got, err, want)
		}
	}
	if _, err := ui.String(ctx, tmpltest.Example{N: -1}); err == nil || err.Error() != "no avatar" {
		t.Errorf("a component's error fails the render: %v", err)
	}
}

// TestTemplateCommands: a chain is an on… attribute's value, compiled
// through the event check; ref renders as data-vk-ref.
func TestTemplateCommands(t *testing.T) {
	got, err := ui.String(context.Background(), tag(&tmpltest.Dropdown{}))
	want := `<div><button data-vk-js-click="[[&#34;remove_class&#34;,{&#34;to&#34;:{&#34;ref&#34;:&#34;menu&#34;},&#34;names&#34;:[&#34;hidden&#34;]}],[&#34;push&#34;,{}]]">Open</button>` +
		`<ul data-vk-ref="menu" class="hidden" data-vk-js-keydown="[[&#34;key&#34;,{&#34;keys&#34;:[&#34;Escape&#34;]}],[&#34;add_class&#34;,{&#34;names&#34;:[&#34;hidden&#34;]}]]"><li>0</li></ul></div>`
	if err != nil || got != want {
		t.Errorf("got  %s (%v)\nwant %s", got, err, want)
	}
}

func TestTemplateModal(t *testing.T) {
	got, err := ui.String(context.Background(), tag(&tmpltest.Modal{}))
	if err != nil || !strings.Contains(got, `data-vk-ref="dialog"`) || !strings.Contains(got, `data-vk-js-change="[[&#34;if&#34;`) {
		t.Errorf("%s (%v)", got, err)
	}
}

func TestTemplateOptimistic(t *testing.T) {
	fail := false
	c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node { return tag(&tmpltest.Post{Fail: &fail}) }), 2)
	if err != nil {
		t.Fatal(err)
	}
	c.Hold()
	c.Click("button")
	if c.Text("span") != "1" || !c.HasClass("button", "liked") {
		t.Fatalf("predicted: %s", c.HTML())
	}
	c.Deliver()
	if c.Text("span") != "1" || c.Visible("p") {
		t.Fatalf("confirmed: %s", c.HTML())
	}
	fail = true
	c.Click("button")
	if c.Text("span") != "2" {
		t.Fatalf("predicted: %s", c.HTML())
	}
	c.Deliver()
	if c.Text("span") != "1" || !c.Visible("p") || !c.HasClass("button", "liked") {
		t.Fatalf("reverted to the server's count and class, error shown: %s", c.HTML())
	}
}

// TestTemplateDocExamples renders js.go's doc examples (docs.vuka.html),
// statically and in a live session: each chain encodes, its handlers are
// ones an on… attribute takes, and its branches are well formed.
func TestTemplateDocExamples(t *testing.T) {
	got, err := ui.String(context.Background(), tag(&tmpltest.Docs{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"else_if"`, `"exec"`, `"call"`, `"throttle"`, `"debounce"`, `"set_value"`, `"busy"`, `"attr"`, `"and"`, `"some"`, `"into"`, `"read"`, `"$read"`, `"error"`} {
		if !strings.Contains(got, html.EscapeString(want)) {
			t.Errorf("no %s rendered", want)
		}
	}
	c, err := livetest.Join(live.NewSession(context.Background(), func(context.Context) ui.Node { return tag(&tmpltest.Docs{}) }), 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"keep":true`, `"timeout":3000`, `"on_ok"`, `"on_error"`, `"value":0`} {
		if !strings.Contains(c.HTML(), want) && !strings.Contains(c.HTML(), html.EscapeString(want)) {
			t.Errorf("no %s in the live page", want)
		}
	}
}
