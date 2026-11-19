package ui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/vuka/v2"
)

type zeroKind struct {
	ui.FormState
	Kind int `label:"Kind" validate:"required"`
}

func (f *zeroKind) KindChoices(context.Context) ([]ui.Option, error) {
	return []ui.Option{{Value: "0", Label: "Normal"}, {Value: "1", Label: "Delegated"}}, nil
}

type zeroRank struct {
	ui.FormState
	Rank int `label:"Rank" validate:"required"`
}

func (f *zeroRank) RankChoices(context.Context) ([]ui.Option, error) {
	return []ui.Option{{Value: "1", Label: "First"}, {Value: "2", Label: "Second"}}, nil
}

type zeroOpt struct {
	ui.FormState
	Kind vuka.Option[int] `label:"Kind" validate:"required"`
}

func (f *zeroOpt) KindChoices(context.Context) ([]ui.Option, error) {
	return []ui.Option{{Value: "0", Label: "Normal"}, {Value: "1", Label: "Delegated"}}, nil
}

func renderForm(t *testing.T, form any) string {
	t.Helper()
	html, err := ui.String(context.Background(), ui.Form{For: ui.For(form)})
	if err != nil {
		t.Fatal(err)
	}
	return html
}

func TestSelectIntZeroChoice(t *testing.T) {
	html := renderForm(t, &zeroKind{})
	if !strings.Contains(html, `value="0" selected`) || strings.Contains(html, `>Choose`) {
		t.Errorf("0 doesn't select the option valued 0, or a placeholder is added:\n%s", html)
	}
	html = renderForm(t, &zeroKind{Kind: 1})
	if !strings.Contains(html, `value="1" selected`) || strings.Contains(html, `value="0" selected`) {
		t.Errorf("1 doesn't select Delegated alone:\n%s", html)
	}
}

func TestSelectIntZeroWithoutZeroChoice(t *testing.T) {
	html := renderForm(t, &zeroRank{})
	if !strings.Contains(html, `<option value="">`) || strings.Contains(html, " selected") {
		t.Errorf("a zero with no option valued 0 changed:\n%s", html)
	}
}

func TestSelectOptionZero(t *testing.T) {
	html := renderForm(t, &zeroOpt{Kind: vuka.Some(0)})
	if !strings.Contains(html, `value="0" selected`) {
		t.Errorf("Some(0) doesn't select 0:\n%s", html)
	}
	html = renderForm(t, &zeroOpt{})
	if strings.Contains(html, " selected") || !strings.Contains(html, `<option value="">`) {
		t.Errorf("None selects an option:\n%s", html)
	}
}
