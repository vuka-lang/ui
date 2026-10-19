package ui_test

import (
	"context"
	"testing"

	"github.com/vuka-lang/ui"
)

type salesProps struct {
	ui.Live
	Title    string
	Week     []int `json:"week"`
	Zoom     func(int)
	Children ui.Node
	hidden   int
}

func TestProps(t *testing.T) {
	if got, want := ui.Props(&salesProps{Title: "W", Week: []int{1}, hidden: 3}), `{"Title":"W","week":[1]}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestJSON(t *testing.T) {
	got, err := ui.String(context.Background(), ui.El("script", nil, ui.Safe("let x = "), ui.JSON(map[string]string{"a": "</script>"})))
	if want := `<script>let x = {"a":"\u003c/script\u003e"}</script>`; err != nil || got != want {
		t.Fatalf("got %s %v, want %s", got, err, want)
	}
}
