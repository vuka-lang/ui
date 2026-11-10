package tmpltest

import (
	"context"
	"io"
	"strconv"

	"github.com/vuka-lang/ui/v2"
)

// PetEditor is a component docs.vuka.html opens with ui.JS.Call.
type PetEditor struct {
	ID     int
	OnSave ui.Action
}

func (e PetEditor) View() ui.Node {
	return ui.El("form", nil, ui.Text("pet "+strconv.Itoa(e.ID)), ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(e.OnSave)}}, ui.Text("Save")))
}

func (e PetEditor) Render(ctx context.Context, w io.Writer) error { return e.View().Render(ctx, w) }

func init() { ui.RegisterCall(PetEditor{}) }
