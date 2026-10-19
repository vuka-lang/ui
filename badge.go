package ui

// Badge is a small status pill:
//
//	<ui.Badge variant={ui.Success}>Signed</ui.Badge>
//	<ui.Badge variant={ui.Danger} soft>Failed</ui.Badge>
type Badge struct {
	// Variant is the tone: Default, Primary, Success, Danger, Warning or Info.
	Variant Variant
	// Soft tints the badge (a light background with coloured text) instead of
	// filling it.
	Soft     bool
	Class    string
	Children Node
}

// BadgeProps is Badge, by its former name.
//
// Deprecated: use Badge: ui.Badge{…}, or <ui.Badge …> in a template.
type BadgeProps = Badge

func (b Badge) classes() string {
	class := "vk-badge"
	if b.Variant != "" {
		class += " vk-badge--" + string(b.Variant)
	}
	if b.Soft {
		class += " vk-badge--soft"
	}
	return withClass(class, b.Class)
}
