package ui

import (
	"context"
	"strings"
	"testing"
)

// What Temporary takes for another value: a set of the field changing what
// it holds, not an edit of the same slice in place.
func TestTempStamp(t *testing.T) {
	rows := []string{"a", "b", "c"}
	same := func(a, b any) bool { return tempStamp(a) == tempStamp(b) }
	if !same(rows, rows) {
		t.Error("the same slice is another value")
	}
	if same(rows, append([]string(nil), rows...)) {
		t.Error("a copy is the same value")
	}
	if same(rows, rows[:2]) {
		t.Error("another length is the same value")
	}
	edited := rows
	edited[0] = "z"
	if !same(rows, edited) {
		t.Error("an edit in place is another value")
	}
	if same(1, 2) || !same(7, 7) || same("x", "y") || !same("x", "x") || same(1, int64(1)) {
		t.Error("scalars compare by value and type")
	}
	type version struct {
		N    int
		Rows []string
	}
	if !same(version{1, rows}, version{1, rows}) || same(version{1, rows}, version{2, rows}) {
		t.Error("a struct compares its fields")
	}
	if !same(nil, nil) || same(nil, rows) || tempStamp(nil) == "" {
		t.Error("nil")
	}
}

// Outside a live tree, Temporary is its content.
func TestTemporaryHTML(t *testing.T) {
	n := Temporary(TemporaryProps{Of: 1, Children: HTML(`<table><tr><td>{x}</td></tr></table>`, "a<b")})
	var b strings.Builder
	if err := n.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "<table><tr><td>a&lt;b</td></tr></table>" {
		t.Fatalf("got %s", got)
	}
}
