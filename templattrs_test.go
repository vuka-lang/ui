package ui

import (
	"testing"

	"github.com/a-h/templ"
)

func TestTemplAttrs(t *testing.T) {
	base := templ.Attributes{"data-slot": "trigger", "onClick": func() {}}
	got := TemplAttrs(nil, base, Attr{Name: "aria-label", Value: `Say "hi" & go`}, Attr{Name: "hidden", Value: true},
		Attr{Name: "data-n", Value: 3}, Attr{Name: "onMouseEnter", Value: On(func() {})}, Attr{Name: "inert", Value: false})
	want := templ.Attributes{"data-slot": "trigger", "aria-label": `Say "hi" & go`, "hidden": true, "data-n": "3"}
	if len(got) != len(want) {
		t.Fatalf("%v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %v, want %v (%v)", k, got[k], v, got)
		}
	}
	if _, ok := base["onClick"]; !ok || len(base) != 2 {
		t.Fatal("base was modified")
	}
}
