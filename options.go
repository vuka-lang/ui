package ui

// OptionGroup is a labelled group of options, an <optgroup>: a method
// loading a select's options may return []OptionGroup for grouped choices.
//
// Deprecated: return []Option with each option's Group set: consecutive
// options of one Group share an <optgroup>, as they do in a select
// showing a field's choices.
type OptionGroup struct {
	Label   string
	Options []Option
}

// optionJSON is an option as the runtime takes it: {"value", "label"}, or a
// group, {"group", "options"}.
type optionJSON struct {
	Value   string       `json:"value,omitempty"`
	Label   string       `json:"label,omitempty"`
	Group   string       `json:"group,omitempty"`
	Options []optionJSON `json:"options,omitempty"`
}

func optionsJSON(os []Option, gs []OptionGroup) []optionJSON {
	out := []optionJSON{}
	for _, o := range os {
		oj := optionJSON{Value: o.Value, Label: o.Label}
		if o.Group == "" {
			out = append(out, oj)
			continue
		}
		// consecutive options of one Group share its <optgroup>
		if n := len(out); n > 0 && out[n-1].Options != nil && out[n-1].Group == o.Group {
			out[n-1].Options = append(out[n-1].Options, oj)
			continue
		}
		out = append(out, optionJSON{Group: o.Group, Options: []optionJSON{oj}})
	}
	for _, g := range gs {
		og := optionJSON{Group: g.Label, Options: []optionJSON{}}
		for _, o := range g.Options {
			og.Options = append(og.Options, optionJSON{Value: o.Value, Label: o.Label})
		}
		out = append(out, og)
	}
	return out
}
