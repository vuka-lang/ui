package ui

import (
	"maps"
	"testing"
)

// A msg: message may hold commas: one ends a message only when the next
// rule's name= follows (vuka#231).
func TestParseMsgs(t *testing.T) {
	for _, c := range []struct {
		tag  string
		want map[string]string
	}{
		{"required=Enter your name", map[string]string{"required": "Enter your name"}},
		{"required=Enter your name, as on your ID", map[string]string{"required": "Enter your name, as on your ID"}},
		{"required=Enter your name, as on your ID,max=Too long, sorry", map[string]string{"required": "Enter your name, as on your ID", "max": "Too long, sorry"}},
		{"required=a, b, max=c", map[string]string{"required": "a, b", "max": "c"}},
		{`required=Say a\, max=1 then,email=x`, map[string]string{"required": "Say a, max=1 then", "email": "x"}},
		{`required=back\\slash`, map[string]string{"required": `back\slash`}},
		{"required=form.name.required,len=form.code.len", map[string]string{"required": "form.name.required", "len": "form.code.len"}},
		{"required=x=y", map[string]string{"required": "x=y"}},
	} {
		if got := parseMsgs(c.tag); !maps.Equal(got, c.want) {
			t.Errorf("%q: got %q, want %q", c.tag, got, c.want)
		}
	}
}
