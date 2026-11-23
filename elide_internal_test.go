package ui

import (
	"context"
	"testing"

	"github.com/a-h/templ"
)

// An empty attribute set and a class whose only holes are empty
// ClassMerges cost no dynamic: the frame is the one the markup without
// them makes, so a component's Fallthrough adds nothing to the live wire
// when nothing fell through.
func TestElideEmptySlots(t *testing.T) {
	frameOf := func(n Node) *lazyFrame {
		t.Helper()
		lf, ok := n.(*lazyFrame)
		if !ok {
			t.Fatalf("not a lazy frame: %T", n)
		}
		return lf
	}
	for _, c := range []struct {
		name      string
		with, was Node
	}{
		{"nil set", HTML(`<table {f...}>{x}</table>`, []Attr(nil), "x"), HTML(`<table>{x}</table>`, "x")},
		{"empty set", HTML(`<table {f...}>{x}</table>`, []Attr{}, "x"), HTML(`<table>{x}</table>`, "x")},
		{"empty templ set", HTML(`<table {f...}>{x}</table>`, templ.Attributes{}, "x"), HTML(`<table>{x}</table>`, "x")},
		{"class", HTML(`<p class="card{c}">{x}</p>`, ClassMerge(""), "x"), HTML(`<p class="card">{x}</p>`, "x")},
		{"both", HTML(`<p {f...} class="card{c}" id={id}>{x}</p>`, []Attr(nil), ClassMerge(""), "i", "x"), HTML(`<p class="card" id={id}>{x}</p>`, "i", "x")},
	} {
		with, was := frameOf(c.with), frameOf(c.was)
		if with.shape != was.shape || len(with.dyns) != len(was.dyns) {
			t.Errorf("%s: shape %q with %d dynamics, want %q with %d", c.name, with.shape, len(with.dyns), was.shape, len(was.dyns))
		}
		a, err := String(context.Background(), c.with)
		b, _ := String(context.Background(), c.was)
		if err != nil || a != b {
			t.Errorf("%s: %q %v, want %q", c.name, a, err, b)
		}
	}

	// a value in the slot keeps it, and the variant's fingerprint is its own
	src := `<p {f...} class="card{c}">{x}</p>`
	full := frameOf(HTML(src, []Attr{{Name: "id", Value: "a"}}, ClassMerge("mt-4"), "x"))
	elided := frameOf(HTML(src, []Attr(nil), ClassMerge(""), "x"))
	half := frameOf(HTML(src, []Attr(nil), ClassMerge("mt-4"), "x"))
	if len(full.dyns) != 3 || len(elided.dyns) != 1 || len(half.dyns) != 2 {
		t.Fatalf("dynamics: full %d, elided %d, half %d", len(full.dyns), len(elided.dyns), len(half.dyns))
	}
	if full.fp == elided.fp || full.fp == half.fp || elided.fp == half.fp {
		t.Errorf("fingerprints shared: %x %x %x", full.fp, elided.fp, half.fp)
	}
	if s, _ := String(context.Background(), HTML(src, []Attr{{Name: "id", Value: "a"}}, ClassMerge("mt-4"), "x")); s != `<p id="a" class="card mt-4">x</p>` {
		t.Errorf("full: %s", s)
	}
	// a non-ClassMerge hole that is empty is a dynamic still
	if lf := frameOf(HTML(`<p class="card{c}">{x}</p>`, "", "x")); len(lf.dyns) != 2 {
		t.Errorf("a string hole elided: %d dynamics", len(lf.dyns))
	}
}

// A slot after an elided one keeps the site it has in the full template,
// so what a block or component in it places stays the same instance
// whether or not something fell through.
func TestElideKeepsSites(t *testing.T) {
	src := `<div {f...}>{rows}</div>`
	render := func(f []Attr) string {
		lf := HTML(src, f, []Node{Text("a")}).(*lazyFrame)
		var site string
		var find func(ns []Node)
		find = func(ns []Node) {
			for _, n := range ns {
				switch n := n.(type) {
				case *list:
					site = n.site
				case *Element:
					find(n.Children)
				}
			}
		}
		find(lf.frame().(*Frame).Kids)
		return site
	}
	full, elided := render([]Attr{{Name: "id", Value: "a"}}), render(nil)
	if full == "" || full != elided {
		t.Errorf("site %q elided, %q full", elided, full)
	}
}
