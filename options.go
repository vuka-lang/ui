package ui

// OptionGroup is a labelled group of options, an <optgroup>: a method
// loading a select's options returns []OptionGroup for grouped choices.
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
		out = append(out, optionJSON{Value: o.Value, Label: o.Label})
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
