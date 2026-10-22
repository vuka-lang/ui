package ui

import "strconv"

// DataTableProps is DataTable, by its former name.
//
// Deprecated: use DataTable: ui.DataTable[T]{…}, or
// <ui.DataTable …> in a template.
type DataTableProps[T any] = DataTable[T]

// sortAria is a sort control's aria-sort: whether its column is the sorted
// one, and which way.
func sortAria(sorted bool, dir string) string {
	switch {
	case !sorted:
		return "none"
	case dir == "desc":
		return "descending"
	}
	return "ascending"
}

// pagerWindow is how many numbered buttons a pager shows: window, 5 by
// default.
func pagerWindow(window int) int {
	if window < 1 {
		return 5
	}
	return window
}

// pageTo is a pager button's click: onPage called with page n. It is none
// for a disabled button or a pager without a handler.
func pageTo(onPage any, n int, enabled bool) []Attr {
	if !enabled || onPage == nil {
		return nil
	}
	return on("onClick", JS.Push(onPage, Value(strconv.Itoa(n))))
}

// sortHeader is a sortable column's header: a button calling onSort with its
// key, marked when its column is the sorted one.
func sortHeader[T any](c Column[T], sort, dir string, onSort any) Node {
	icon := Icon("caret-up", "vk-sort__ind vk-sort__ind--active")
	if c.Sort != sort {
		icon = Icon("caret-up-down", "vk-sort__ind")
	} else if dir == "desc" {
		icon = Icon("caret-down", "vk-sort__ind vk-sort__ind--active")
	}
	return HTML(`<button type="button" class="vk-sort" aria-sort={sortAria(c.Sort == sort, dir)} onClick={JS.Push(onSort, Value(c.Sort))}>{c.Header}{icon}</button>`, sortAria(c.Sort == sort, dir), JS.Push(onSort, Value(c.Sort)), c.Header, icon)
}

// pager is a page's numbered buttons between previous and next.
func pager[T any](page Page[T], window int, onPage any) Node {
	if page.Pages <= 1 {
		return HTML(`<div class="vk-pager"/>`)
	}
	var buttons []Node
	for _, n := range page.Window(pagerWindow(window)) {
		buttons = append(buttons, pagerButton(strconv.Itoa(n), "Page "+strconv.Itoa(n), n, true, n == page.Page, onPage))
	}
	prev := pagerButton("‹", "Previous page", page.Page-1, page.HasPrev, false, onPage)
	next := pagerButton("›", "Next page", page.Page+1, page.HasNext, false, onPage)
	return HTML(`<nav class="vk-pager" aria-label="Pagination">{prev}{buttons}{next}</nav>`, prev, buttons, next)
}

func pagerButton(label, aria string, target int, enabled, current bool, onPage any) Node {
	return HTML(`<button type="button" aria-label={aria} aria-current={when(current, "page")} disabled={!enabled || onPage == nil} {pageTo(onPage, target, enabled)...}>{label}</button>`, aria, when(current, "page"), !enabled || onPage == nil, pageTo(onPage, target, enabled), label)
}
