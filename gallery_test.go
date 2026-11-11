package ui_test

import (
	"context"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/widget"
)

var galleryOutput = flag.String("gallery-output", "", "write the dev gallery HTML to this directory")
var galleryCSS = flag.String("gallery-css", "", "compare with this stylesheet instead of ui.Styles")

type galleryForm struct {
	ui.FormState
	Text   string
	Flag   bool
	Many   []string
	Upload ui.File
}

// Gallery cases extend the kit's existing component/state matrix with every
// built-in widget and the newer native presentations. No app is needed.
func galleryCases() []kitCase {
	cases := kitCases()
	for _, c := range []struct {
		name string
		node ui.Node
	}{
		{"dialog", ui.Dialog{Open: true, Title: ui.Text("Dialog"), Children: ui.Text("Content"), Footer: ui.Button{Children: ui.Text("Done")}}},
		{"drawer", ui.Drawer{Title: ui.Text("Drawer"), Children: ui.Text("Content")}},
		{"popover", ui.Popover{Title: ui.Text("Popover"), Children: ui.Text("Content")}},
		{"notice", ui.Notice{Text: "Saved"}},
		{"confirm", ui.ConfirmBox{Title: "Continue?", Text: "Confirm this action"}},
	} {
		node := c.node
		cases = append(cases, kitCase{c.name, func() ui.Node { return node }})
	}
	widgets := []any{widget.Text{}, widget.Email{}, widget.Password{}, widget.Number{}, widget.Tel{}, widget.URL{}, widget.Search{}, widget.Textarea{}, widget.Date{}, widget.DateTime{}, widget.Time{}, widget.Checkbox{}, widget.Switch{}, widget.Select{}, widget.MultiSelect{}, widget.Radios{}, widget.Checkboxes{}, widget.Related{}, widget.RelatedSearch{}, widget.File{}}
	for _, prototype := range widgets {
		for _, state := range []string{"default", "disabled", "error"} {
			typ := reflect.TypeOf(prototype)
			cases = append(cases, kitCase{"widget/" + typ.Name() + "/" + state, func() ui.Node {
				f := &galleryForm{Text: "Example", Flag: true, Many: []string{"one"}}
				field := ui.For(&f.Text, f)
				switch typ.Name() {
				case "Checkbox", "Switch":
					field = ui.For(&f.Flag, f)
				case "MultiSelect", "Checkboxes":
					field = ui.For(&f.Many, f)
				case "File":
					field = ui.For(&f.Upload, f)
				}
				props := widget.Props{Model: field, Label: typ.Name(), Placeholder: "Enter a value", Disabled: state == "disabled"}
				if state == "error" {
					props.Error = "Check this value"
				}
				v := reflect.New(typ).Elem()
				v.FieldByName("Props").Set(reflect.ValueOf(props))
				if choices := v.FieldByName("Choices"); choices.IsValid() {
					choices.Set(reflect.ValueOf([]widget.Choice[any]{{Value: "one", Label: "One"}, {Value: "two", Label: "Two"}}))
				}
				return ui.Form{For: ui.For(f), Children: v.Interface().(ui.Node)}
			}})
		}
	}
	return cases
}

func TestGallery(t *testing.T) {
	if *galleryOutput == "" {
		t.Skip("dev-only fixture: pass -gallery-output=gallery/rendered")
	}
	css := ui.Styles
	if *galleryCSS != "" {
		b, err := os.ReadFile(*galleryCSS)
		if err != nil {
			t.Fatal(err)
		}
		css = string(b)
	}
	var rows strings.Builder
	for i, c := range galleryCases() {
		// A real in-memory live session supplies the same form/event context as
		// a host app. Mounting/render errors fail the test, rather than hiding rows.
		client := join(t, 1, func(context.Context) ui.Node { return ui.El("main", nil, c.node()) })
		body := client.HTML()
		if strings.Contains(body, "data-vk-error") && strings.Contains(body, "E0") {
			t.Fatalf("%s failed to render: %s", c.name, body)
		}
		fmt.Fprintf(&rows, `<tr data-case="%s"><th scope="row">%s</th><td><div class="sample" id="sample-%d">%s</div></td></tr>`, html.EscapeString(c.name), html.EscapeString(c.name), i, body)
	}
	if err := os.MkdirAll(*galleryOutput, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		page := fmt.Sprintf(`<!doctype html><html data-theme="%s"><meta charset="utf-8"><title>Vuka kit gallery — %s</title><style>%s</style><style>
body{margin:0;padding:24px;background:var(--ui-bg);color:var(--ui-fg);font:16px Arial,sans-serif}h1{font-size:24px}table.gallery{border-collapse:collapse;width:100%%;table-layout:fixed}table.gallery>tbody>tr>th{width:230px;text-align:left;font:12px monospace;padding:12px;vertical-align:top;border-bottom:1px solid var(--ui-border)}table.gallery>tbody>tr>td{padding:12px;border-bottom:1px solid var(--ui-border)}.sample{position:relative;transform:translateZ(0);isolation:isolate;min-height:90px;overflow:hidden;padding:8px}.sample:has(dialog),.sample:has(.vk-modal-root){height:280px}.sample:has(.vk-toasts){height:150px}.sample dialog{position:relative!important;inset:auto!important;max-height:260px}.sample img{max-width:100%%;max-height:120px}*,*::before,*::after{animation-play-state:paused!important;caret-color:transparent!important}
</style><h1>Vuka kit — %s</h1><p>Kit variants and states; every built-in widget. Hover/focus are forced by the capture script. Unsupported states are omitted.</p><table class="gallery"><tbody>%s</tbody></table><script>document.querySelectorAll('dialog').forEach(d=>d.setAttribute('open',''));document.querySelectorAll('.vk-modal-root').forEach(d=>d.style.display='flex');document.querySelectorAll('img').forEach(im=>im.src='data:image/svg+xml,%%3Csvg xmlns="http://www.w3.org/2000/svg" width="160" height="90"%%3E%%3C/svg%%3E');</script></html>`, theme, theme, css, theme, rows.String())
		if err := os.WriteFile(filepath.Join(*galleryOutput, theme+".html"), []byte(page), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
