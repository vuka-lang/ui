package ui_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

func TestCallIntoBareTokenRoundTrip(t *testing.T) {
	ctx := callCtx("bare")
	step := openStep(t, ctx, ui.JS.Call(productCard{ID: 5}, ui.Into("#host", ui.Bare())))
	call, err := ui.ParseCall(ctx, "bare", tokenOf(t, step), nil)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := call.Value(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ui.RenderCall(ctx, "bare", comp, call.Options()...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `data-vk-call="bare"`) || !strings.Contains(got, `data-vk-present="inline"`) {
		t.Fatal(got)
	}
	for _, chrome := range []string{"vk-call-inline", "vk-dialog__header", "vk-dialog__body", "vk-dialog__close"} {
		if strings.Contains(got, chrome) {
			t.Errorf("Bare kept %s: %s", chrome, got)
		}
	}
	if !strings.Contains(got, "5") {
		t.Fatalf("component content lost: %s", got)
	}
}

func TestCallIntoOwnPresentation(t *testing.T) {
	for name, view := range map[string]ui.Node{
		"dialog": productDialog{ID: 4},
		"drawer": ui.Drawer{Title: ui.Text("Details"), Children: ui.Text("content")},
		"toast":  ui.Toast{Title: "Saved", Children: ui.Text("content")},
		"notice": ui.Notice{Text: "content"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ui.String(context.Background(), ui.CallView("own", view, nil, nil, ui.Into("#host")))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "vk-call-inline") {
				t.Fatalf("presentation root got extra chrome: %s", got)
			}
			if !strings.Contains(got, `data-vk-call="own"`) {
				t.Fatalf("call identity lost: %s", got)
			}
			if name == "dialog" || name == "drawer" {
				if !strings.HasPrefix(got, "<dialog ") {
					t.Fatalf("native presentation was lost: %s", got)
				}
			}
		})
	}
}

func TestCallIntoBareFailure(t *testing.T) {
	got, err := ui.String(context.Background(), ui.CallView("failed", nil, nil, errors.New("failed"), ui.Into("#host", ui.Bare())))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `role="alert"`) || strings.Contains(got, "vk-dialog__") || strings.Contains(got, "vk-call-inline") {
		t.Fatal(got)
	}
}
