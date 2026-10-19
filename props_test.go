package ui_test

import (
	"context"
	"strings"
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

type statesComp struct {
	Title string
	open  ui.State[bool]
	Count ui.State[int] `json:"n"`
}

func TestStates(t *testing.T) {
	if ui.States(salesProps{}) != nil {
		t.Fatal("a struct with no state has States")
	}
	c := statesComp{open: ui.NewState("t:1", true), Count: ui.NewState("t:2", 2)}
	got, err := ui.String(context.Background(), ui.El("div", []ui.Attr{{Name: "data-vk-s", Value: ui.States(&c)}}))
	if err != nil || !strings.Contains(got, `&#34;open&#34;:{`) || !strings.Contains(got, `&#34;n&#34;:{`) || !strings.Contains(got, `&#34;$vk&#34;:&#34;state&#34;`) {
		t.Fatalf("got %s %v", got, err)
	}
}
