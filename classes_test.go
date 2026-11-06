package ui_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2/diag"
)

func TestMergeClasses(t *testing.T) {
	for _, c := range []struct{ name, base, extra, want string }{
		// spacing
		{"padding", "px-4 py-2", "px-2", "py-2 px-2"},
		{"a side refines the whole", "p-4", "px-2", "p-4 px-2"},
		{"the whole replaces its sides", "px-2 pt-1", "p-4", "p-4"},
		{"px replaces pl, pr", "pl-1 pr-2 pt-3", "px-4", "pt-3 px-4"},
		{"margin", "m-2 mt-4", "m-0", "m-0"},
		{"negative margin", "-mt-2", "mt-4", "mt-4"},
		{"gap", "gap-2 gap-x-4", "gap-6", "gap-6"},
		{"space", "space-y-2", "space-y-4", "space-y-4"},
		// sizing
		{"width", "w-full max-w-md", "w-1/2", "max-w-md w-1/2"},
		{"height, min height", "min-h-4 h-8", "h-32", "min-h-4 h-32"},
		{"size over width and height", "w-4 h-4", "size-8", "size-8"},
		{"arbitrary length", "w-4", "w-[calc(100%-2rem)]", "w-[calc(100%-2rem)]"},
		// colours
		{"background colour", "bg-gray-50", "bg-red-50", "bg-red-50"},
		{"background opacity", "bg-current/10", "bg-white", "bg-white"},
		{"theme colours", "border-border bg-muted", "bg-red-50", "border-border bg-red-50"},
		{"text colour and size apart", "text-sm text-gray-700", "text-red-800", "text-sm text-red-800"},
		{"text size and colour apart", "text-sm text-gray-700", "text-lg", "text-gray-700 text-lg"},
		{"text size with a line height", "text-sm/6", "text-base", "text-base"},
		{"arbitrary colour", "text-red-500", "text-[#0af]", "text-[#0af]"},
		{"arbitrary size", "text-sm", "text-[13px]", "text-[13px]"},
		{"text align", "text-left text-sm", "text-center", "text-sm text-center"},
		{"border colour and width apart", "border border-gray-200", "border-red-500", "border border-red-500"},
		{"border width", "border", "border-2", "border-2"},
		{"border side", "border-t-2", "border-t-4", "border-t-4"},
		{"border width over sides", "border-b-2", "border-0", "border-0"},
		{"border style", "border-solid", "border-dashed", "border-dashed"},
		{"ring", "ring-2 ring-blue-500", "ring-red-500", "ring-2 ring-red-500"},
		{"shadow and its colour", "shadow-md shadow-black/10", "shadow-lg", "shadow-black/10 shadow-lg"},
		{"gradient stops", "from-blue-500 to-red-500", "from-green-500", "to-red-500 from-green-500"},
		// display, position, layout
		{"display", "flex items-start", "hidden", "items-start hidden"},
		{"display, grid", "block", "grid", "grid"},
		{"position", "relative", "absolute", "absolute"},
		{"inset", "top-0 left-0", "inset-0", "inset-0"},
		{"flex direction and wrap apart", "flex-row flex-wrap", "flex-col", "flex-wrap flex-col"},
		{"flex over grow", "grow", "flex-1", "flex-1"},
		{"align", "items-start justify-between", "items-center", "justify-between items-center"},
		{"justify items apart from content", "justify-between", "justify-items-center", "justify-between justify-items-center"},
		{"grid columns", "grid-cols-2", "grid-cols-3", "grid-cols-3"},
		{"overflow", "overflow-x-auto", "overflow-hidden", "overflow-hidden"},
		{"z", "z-10", "z-50", "z-50"},
		// typography
		{"font weight and family apart", "font-sans font-medium", "font-bold", "font-sans font-bold"},
		{"font family", "font-sans", "font-mono", "font-mono"},
		{"leading", "leading-6", "leading-tight", "leading-tight"},
		{"font size over leading", "leading-6", "text-sm", "text-sm"},
		{"tracking", "tracking-tight", "tracking-wide", "tracking-wide"},
		{"text transform", "uppercase", "normal-case", "normal-case"},
		{"italic", "italic", "not-italic", "not-italic"},
		{"decoration", "underline", "no-underline", "no-underline"},
		{"whitespace", "whitespace-nowrap", "whitespace-normal", "whitespace-normal"},
		{"truncate", "truncate", "text-ellipsis", "text-ellipsis"},
		// borders, effects
		{"rounded", "rounded-md", "rounded-full", "rounded-full"},
		{"a corner refines rounded", "rounded-md", "rounded-t-none", "rounded-md rounded-t-none"},
		{"rounded over a corner", "rounded-tl-lg", "rounded", "rounded"},
		{"opacity", "opacity-50", "opacity-100", "opacity-100"},
		{"animation", "animate-pulse", "animate-none", "animate-none"},
		{"transition", "transition", "transition-colors", "transition-colors"},
		{"cursor", "cursor-pointer", "cursor-not-allowed", "cursor-not-allowed"},
		// variants
		{"under the same variant", "hover:bg-gray-50", "hover:bg-red-50", "hover:bg-red-50"},
		{"under another variant", "bg-gray-50", "hover:bg-red-50", "bg-gray-50 hover:bg-red-50"},
		{"variants in any order", "dark:hover:bg-gray-800", "hover:dark:bg-black", "hover:dark:bg-black"},
		{"breakpoints", "md:px-4 px-2", "md:px-6", "px-2 md:px-6"},
		{"important apart", "!px-4", "px-2", "!px-4 px-2"},
		{"motion", "animate-pulse motion-reduce:animate-none", "animate-spin", "motion-reduce:animate-none animate-spin"},
		{"arbitrary property", "[mask-type:luminance]", "[mask-type:alpha]", "[mask-type:alpha]"},
		// kept
		{"not Tailwind", "vk-alert vk-alert--info", "px-2", "vk-alert vk-alert--info px-2"},
		{"an app's own", "card", "card shadow", "card card shadow"},
		{"group and peer", "group peer", "group/item", "group peer group/item"},
		{"nothing to merge", "px-4 py-2", "", "px-4 py-2"},
		{"no base", "", "px-2", "px-2"},
		{"white space", "  px-4\n py-2 ", "px-2", "py-2 px-2"},
	} {
		if got := ui.MergeClasses(c.base, c.extra); got != c.want {
			t.Errorf("%s: MergeClasses(%q, %q) = %q, want %q", c.name, c.base, c.extra, got, c.want)
		}
	}
}

func TestMergeClassesMany(t *testing.T) {
	if got := ui.MergeClasses("px-4 text-sm", "px-2", "text-lg px-1"); got != "text-lg px-1" {
		t.Errorf("got %q", got)
	}
	if got := ui.MergeClasses(); got != "" {
		t.Errorf("none: %q", got)
	}
}

// A kit component's class is the caller's: its utilities win.
func TestKitClassMerged(t *testing.T) {
	got, err := ui.String(context.Background(), ui.Skeleton{Class: "rounded-full h-32"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `class="vk-skeleton block bg-current/10 animate-pulse motion-reduce:animate-none min-h-4 rounded-full h-32"`) {
		t.Errorf("got %s", got)
	}
}

var alert = ui.Variants{
	Base: "flex items-start gap-2 rounded-lg border px-4 py-3 text-sm",
	Variants: map[string]map[string]string{
		"tone": {"error": "border-red-200 bg-red-50 text-red-800", "info": "border-border bg-gray-50"},
		"size": {"sm": "px-2 py-1 text-xs", "md": ""},
	},
	Defaults: map[string]string{"tone": "info"},
}

func TestVariants(t *testing.T) {
	for _, c := range []struct {
		pairs []string
		want  string
	}{
		{nil, "flex items-start gap-2 rounded-lg border px-4 py-3 text-sm border-border bg-gray-50"},
		{[]string{"tone", ""}, "flex items-start gap-2 rounded-lg border px-4 py-3 text-sm border-border bg-gray-50"},
		{[]string{"tone", "error"}, "flex items-start gap-2 rounded-lg border px-4 py-3 text-sm border-red-200 bg-red-50 text-red-800"},
		{[]string{"tone", "error", "size", "sm"}, "flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 text-red-800 px-2 py-1 text-xs"},
		{[]string{"size", "md", "class", "px-6 bg-white"}, "flex items-start gap-2 rounded-lg border py-3 text-sm border-border px-6 bg-white"},
		{[]string{"class", ""}, "flex items-start gap-2 rounded-lg border px-4 py-3 text-sm border-border bg-gray-50"},
	} {
		if got := alert.Class(c.pairs...); got != c.want {
			t.Errorf("Class(%q) = %q\nwant %q", c.pairs, got, c.want)
		}
	}
}

func TestVariantsUnknown(t *testing.T) {
	for _, c := range []struct {
		pairs   []string
		message string
		suggest string
	}{
		{[]string{"tone", "eror"}, `the variant tone has no value "eror": it is "error" or "info"`, `"error"`},
		{[]string{"tones", "info"}, `the variants have no "tones": they are "size" or "tone"`, `"tone"`},
		{[]string{"tone", "warning"}, `the variant tone has no value "warning": it is "error" or "info"`, ""},
	} {
		err := func() (err error) {
			defer ui.Catch(&err)
			alert.Class(c.pairs...)
			return nil
		}()
		d, ok := err.(*diag.Diagnostic)
		if diag.Of(err) != "E0371" || !ok || d.Message != c.message || d.Suggest != c.suggest {
			t.Errorf("Class(%q): %#v", c.pairs, err)
		}
	}
}

var benchClasses = []struct{ name, base, extra string }{
	{"kit", "vk-skeleton block rounded-md bg-current/10 animate-pulse motion-reduce:animate-none min-h-4", "h-32 rounded-full"},
	{"utilities", "flex items-start gap-2 rounded-lg border px-4 py-3 text-sm border-border bg-gray-50 hover:bg-gray-100 dark:bg-gray-900", "px-2 bg-white md:px-6"},
}

func BenchmarkMergeClasses(b *testing.B) {
	for _, c := range benchClasses {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ui.MergeClasses(c.base, c.extra)
			}
		})
	}
}

// BenchmarkMergeClassesCold merges lists never merged before: what the
// cache holds after its first render.
func BenchmarkMergeClassesCold(b *testing.B) {
	for _, c := range benchClasses {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				i++
				ui.MergeClasses(c.base, c.extra, "x"+strconv.Itoa(i))
			}
		})
	}
}

func BenchmarkVariants(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		alert.Class("tone", "error", "class", "px-2")
	}
}
