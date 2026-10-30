package ui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/uigen"
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

type propsSignup struct {
	ui.FormState
	Email    string
	Password string `input:"new-password"`
}

type PropsLogin struct {
	Email string
	Code  string `input:"one-time-code"`
	Token string `json:"-"`
}

type propsCard struct {
	Holder string
	PIN    string `input:"password"`
	When   time.Time
}

type propsJoin struct {
	ui.Live
	Title  string
	Form   propsSignup
	Ptr    *propsSignup
	Login  PropsLogin
	Secret string `input:"current-password"`
	Cards  []propsCard
	ByName map[string]propsCard
	PropsLogin
}

// A form's values, password and one-time-code fields, and json:"-" fields
// never reach a script's props, at any depth (vuka#229).
func TestPropsLeaveSecretsOut(t *testing.T) {
	when := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	got := ui.Props(&propsJoin{
		Title:      "Join",
		Form:       propsSignup{Email: "a@shop.test", Password: "hunter2-form"},
		Ptr:        &propsSignup{Password: "hunter2-ptr"},
		Login:      PropsLogin{Email: "b@shop.test", Code: "hunter2-code", Token: "hunter2-token"},
		Secret:     "hunter2-top",
		Cards:      []propsCard{{Holder: "Ann", PIN: "hunter2-slice", When: when}},
		ByName:     map[string]propsCard{"ann": {Holder: "Ann", PIN: "hunter2-map"}},
		PropsLogin: PropsLogin{Email: "c@shop.test", Code: "hunter2-embedded"},
	})
	if strings.Contains(got, "hunter2") {
		t.Fatalf("a secret reached the props: %s", got)
	}
	for _, want := range []string{`"Title":"Join"`, `"Login":{"Email":"b@shop.test"}`, `"Cards":[{"Holder":"Ann","When":"2026-01-02T00:00:00Z"}]`, `"ByName":{"ann":{"Holder":"Ann","When":"0001-01-01T00:00:00Z"}}`, `"Email":"c@shop.test"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	for _, gone := range []string{`"Form"`, `"Ptr"`, `"Secret"`} {
		if strings.Contains(got, gone) {
			t.Errorf("%s in %s", gone, got)
		}
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
	c := statesComp{open: uigen.State("t:1", true), Count: uigen.State("t:2", 2)}
	got, err := ui.String(context.Background(), ui.El("div", []ui.Attr{{Name: "data-vk-s", Value: ui.States(&c)}}))
	if err != nil || !strings.Contains(got, `&#34;open&#34;:{`) || !strings.Contains(got, `&#34;n&#34;:{`) || !strings.Contains(got, `&#34;$vk&#34;:&#34;state&#34;`) {
		t.Fatalf("got %s %v", got, err)
	}
}

func TestCSSVars(t *testing.T) {
	got, err := ui.String(context.Background(), ui.El("p", []ui.Attr{{Name: "style", Value: ui.CSSVars("color: red", "--accent", "#0a0", "--n", 3)}}))
	if want := `<p style="--accent:#0a0;--n:3;color:red;"></p>`; err != nil || got != want {
		t.Fatalf("got %s %v, want %s", got, err, want)
	}
}

type propsIsland struct {
	Holder string
	Form   propsSignup
	Card   propsCard
	PIN    string `input:"password"`
}

// An island's props leave the same secrets out, its dates still tagged.
func TestIslandPropsLeaveSecretsOut(t *testing.T) {
	when := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	b, err := ui.PropsJSON(context.Background(), propsIsland{Holder: "Ann", Form: propsSignup{Password: "hunter2-form"},
		Card: propsCard{Holder: "Ann", PIN: "hunter2-card", When: when}, PIN: "hunter2-top"})
	got := string(b)
	if err != nil || strings.Contains(got, "hunter2") || strings.Contains(got, `"Form"`) ||
		!strings.Contains(got, `"Card":{"Holder":"Ann","When":{"$vk":"date","v":"2026-01-02T00:00:00Z"}}`) {
		t.Fatalf("got %s %v", got, err)
	}
}
