package ui_test

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/gallery"
)

var updateKit = flag.Bool("update-kit", false, "rewrite testdata/kit.golden")

// TestKitGolden renders every component and variant, statically and in a
// live session, against testdata/kit.golden.
func TestKitGolden(t *testing.T) {
	var b strings.Builder
	for _, c := range gallery.Kit() {
		b.WriteString("=== " + c.Name + "\n")
		s, err := ui.String(context.Background(), c.Node())
		if err != nil {
			b.WriteString("error: " + err.Error() + "\n")
		} else {
			b.WriteString(s + "\n")
		}
		cl := join(t, 1, func(context.Context) ui.Node { return ui.El("main", nil, c.Node()) })
		b.WriteString("--- live\n" + cl.HTML() + "\n")
	}
	got := b.String()
	const file = "testdata/kit.golden"
	if *updateKit {
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("kit.golden differs at line %d:\n got: %s\nwant: %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("kit.golden differs in length: got %d lines, want %d", len(gl), len(wl))
	}
}

// BenchmarkKit renders every case of TestKitGolden once per iteration.
func BenchmarkKit(b *testing.B) {
	cs := gallery.Kit()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		for _, c := range cs {
			if _, err := ui.String(ctx, c.Node()); err != nil {
				b.Fatal(err)
			}
		}
	}
}
