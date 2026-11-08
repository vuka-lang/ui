package widget

import (
	"context"
	"io"

	"github.com/vuka-lang/ui/v2"
)

// Related is the control of an orm.FK field: a select of the related
// model's rows, labelled by their String(). An optional relation
// (@orm.Null) has an empty first option. A <Field>Choices method
// returning a query narrows the rows, and a submitted key outside it is
// refused ("Select a valid choice").
type Related struct {
	Props
	Choices []Choice[any]
	Class   string
}

func (w Related) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, func(ctx context.Context) ui.Node {
		return ui.El("select", w.control("select", "", cls("vk-select", w.Class)), options(ctx, w.Props, w.Choices, false)...)
	})
}

// RelatedSearch is the control of an orm.FK field whose model has too
// many rows to list: a search box, and the rows whose label holds what
// was typed (up to Limit, 20 when 0) to choose from. The submitted key is
// checked against the field's rows, its <Field>Choices query's when it
// has one, as Related's is.
type RelatedSearch struct {
	Props
	// Limit caps the rows a search shows; 20 when 0.
	Limit int
	// Search is the field's rows whose label holds the text.
	Search func(ctx context.Context, q string) []Choice[any]
	Class  string
}

// FieldLayout: a field group shows the search and its rows as a
// fieldset.
func (RelatedSearch) FieldLayout() string { return "fieldset" }

func (w RelatedSearch) Render(ctx context.Context, out io.Writer) error {
	return render(ctx, out, &w, w.view)
}

func (w RelatedSearch) view(ctx context.Context) ui.Node {
	q := w.Model.PartValue("search")
	value, label := w.Model.Chosen(ctx)
	search := ui.El("input", []ui.Attr{
		{Name: "class", Value: cls("vk-input", w.Class)},
		{Name: "type", Value: "search"},
		{Name: "name", Value: w.Model.Part("search")},
		{Name: "id", Value: w.Model.ID() + "-search"},
		{Name: "value", Value: q},
		{Name: "placeholder", Value: nonEmpty(w.Placeholder)},
		{Name: "autocomplete", Value: "off"},
		{Name: "disabled", Value: w.Disabled},
	})
	var choices []Choice[any]
	if q != "" && w.Search != nil {
		choices = w.Search(ctx, q)
	}
	shown := false
	for _, c := range choices {
		shown = shown || c.Value == value
	}
	if value != "" && !shown {
		// the current row stays a choice, whatever the search
		choices = append([]Choice[any]{{Value: value, Label: label, Selected: true}}, choices...)
	}
	if !w.Required && value != "" {
		choices = append(choices, Choice[any]{Value: "", Label: "—"})
	}
	return ui.El("div", []ui.Attr{{Name: "class", Value: "vk-related-search"}}, search, items(w.Props, choices, "radio"))
}
