package ui

import "strconv"

// Tab is one tab of a Tabs bar. It is driven either by Href (navigation) or
// by On (a live event handler); set one.
type Tab struct {
	Label string
	// Icon is an optional named icon before the label, e.g. "users".
	Icon string
	// Count, when set, shows as a badge beside the label (see ui.Count).
	Count *int
	// Href makes the tab a link. Patch asks the live page to re-bind the same
	// route with the new URL rather than mount afresh.
	Href  string
	Patch bool
	// On is a live event handler the tab calls when it has no Href.
	On any
	// Active marks the current tab.
	Active bool
	ID     string
}

// Count boxes n for a Tab's badge.
func Count(n int) *int { return &n }

// TabsProps is Tabs, by its former name.
//
// Deprecated: use Tabs: ui.Tabs{…}, or <ui.Tabs …> in a template.
type TabsProps = Tabs

func (t Tab) classes() string {
	if t.Active {
		return "vk-tab vk-tab--active"
	}
	return "vk-tab"
}

func (t Tab) selected() string { return strconv.FormatBool(t.Active) }

// link is a link tab's data-vk-link: a navigation, or a patch of the page.
func (t Tab) link() any {
	if t.Patch {
		return "patch"
	}
	return true
}

// node is the tab: a link, or a button calling On.
func (t Tab) node() Node {
	if t.Href != "" {
		return HTML(`<a class={t.classes()} href={t.Href} data-vk-link={t.link()} role="tab" aria-selected={t.selected()} id={nonEmpty(t.ID)}>{t.label()}</a>`,
			t.classes(),
			t.Href,
			t.link(),
			t.selected(),
			nonEmpty(t.ID),
			t.label(),
		)
	}
	return HTML(`<button type="button" class={t.classes()} role="tab" aria-selected={t.selected()} id={nonEmpty(t.ID)} {on("onClick", t.On)...}>{t.label()}</button>`, t.classes(), t.selected(), nonEmpty(t.ID), on("onClick", t.On), t.label())
}

// label is the tab's icon, label and count.
func (t Tab) label() Node {
	var icon, count Node
	if t.Icon != "" {
		icon = HTML(`<span class="vk-tab__icon">{Icon(t.Icon)}</span>`, Icon(t.Icon))
	}
	if t.Count != nil {
		count = HTML(`<span class="vk-tab__count">{*t.Count}</span>`, *t.Count)
	}
	return HTML(`
{icon}
<span class="vk-tab__label">{t.Label}</span>
{count}`, icon, t.Label, count)
}
