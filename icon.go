package ui

import "sort"

// icons is the built-in icon set: name → the inner markup of a 24×24 SVG
// drawn in currentColor. They are hand-drawn line icons, so the library pulls
// in no icon font and no third-party asset; a page styles them through the
// --ui-* text colour like any text. Add your own with RegisterIcon.
var icons = map[string]string{
	"x":                `<path d="M6 6l12 12M18 6L6 18"/>`,
	"check":            `<path d="M5 13l4 4L19 7"/>`,
	"check-circle":     `<circle cx="12" cy="12" r="9"/><path d="M8 12l3 3 5-6"/>`,
	"caret-up":         `<path d="M6 15l6-6 6 6"/>`,
	"caret-down":       `<path d="M6 9l6 6 6-6"/>`,
	"caret-up-down":    `<path d="M8 10l4-4 4 4M8 14l4 4 4-4"/>`,
	"chevron-left":     `<path d="M15 6l-6 6 6 6"/>`,
	"chevron-right":    `<path d="M9 6l6 6-6 6"/>`,
	"plus":             `<path d="M12 5v14M5 12h14"/>`,
	"minus":            `<path d="M5 12h14"/>`,
	"trash":            `<path d="M4 7h16M9 7V5a1 1 0 011-1h4a1 1 0 011 1v2M6 7l1 13a1 1 0 001 1h8a1 1 0 001-1l1-13M10 11v6M14 11v6"/>`,
	"search":           `<circle cx="11" cy="11" r="6"/><path d="M20 20l-3.6-3.6"/>`,
	"users":            `<circle cx="9" cy="8" r="3.2"/><path d="M3.5 19a5.5 5.5 0 0 1 11 0M16 5.2a3.2 3.2 0 0 1 0 6.2M17.4 19a5.5 5.5 0 0 0-2-4.3"/>`,
	"user":             `<circle cx="12" cy="8" r="3.4"/><path d="M5.5 19a6.5 6.5 0 0 1 13 0"/>`,
	"briefcase":        `<rect x="3" y="7" width="18" height="12" rx="2"/><path d="M8 7V5.5A1.5 1.5 0 0 1 9.5 4h5A1.5 1.5 0 0 1 16 5.5V7M3 12h18"/>`,
	"file-text":        `<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h6"/>`,
	"arrows-clockwise": `<path d="M20 12a8 8 0 1 0-2.3 5.6"/><path d="M20 6v5h-5"/>`,
	"signature":        `<path d="M3 16c2.5 0 2.8-8 5-8s2 8 4.5 8 3-5 5.5-5"/><path d="M3 20h18"/>`,
	"list-checks":      `<path d="M4 6l1.4 1.4L8 5M4 16l1.4 1.4L8 14"/><path d="M11 6h9M11 12h9M11 18h9"/>`,
	"list":             `<path d="M4 6h16M4 12h16M4 18h16"/>`,
	"dots":             `<circle cx="5.5" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="18.5" cy="12" r="1.4" fill="currentColor" stroke="none"/>`,
	"dots-vertical":    `<circle cx="12" cy="5.5" r="1.4" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="12" cy="18.5" r="1.4" fill="currentColor" stroke="none"/>`,
	"warning":          `<path d="M12 3L2.5 20h19z"/><path d="M12 10v4M12 17h.01"/>`,
	"warning-circle":   `<circle cx="12" cy="12" r="9"/><path d="M12 8v5M12 16h.01"/>`,
	"info":             `<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>`,
}

// iconClass is an icon's class attribute: vk-icon and the extra classes
// given.
func iconClass(extra []string) string {
	c := "vk-icon"
	for _, x := range extra {
		if x != "" {
			c += " " + x
		}
	}
	return c
}

// RegisterIcon adds or replaces an icon in the built-in set. body is the inner
// markup of a 24×24 SVG, drawn in currentColor (as the built-in icons are).
func RegisterIcon(name, body string) { icons[name] = body }

// IconNames are the names in the built-in set, sorted.
func IconNames() []string {
	out := make([]string, 0, len(icons))
	for n := range icons {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
