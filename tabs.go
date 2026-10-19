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

// Tabs is a tab bar underlining the active tab; each tab may carry a count:
//
//	<ui.Tabs tabs={[]ui.Tab{
//		{Label: "Placements", Href: "/tasks?type=placements", Active: true, Count: ui.Count(42)},
//		{Label: "Permits", Href: "/tasks?type=permits", Count: ui.Count(7)},
//	}}/>
type Tabs struct {
	Tabs  []Tab
	Class string
}

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
