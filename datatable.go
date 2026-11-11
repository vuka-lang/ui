package ui

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
)

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

// sortable is the sort keys the table's URL sort takes: its columns' keys
// (with Rows, those Sorts orders by).
func (d DataTable[T]) sortable() []string {
	var keys []string
	for _, c := range d.Columns {
		if c.Sort == "" || slices.Contains(keys, c.Sort) {
			continue
		}
		if d.Load == nil && d.Sorts[c.Sort] == nil {
			continue
		}
		keys = append(keys, c.Sort)
	}
	return keys
}

// sizes is the page sizes offered: Sizes, else 10, 25, 50 and 100.
func (d DataTable[T]) sizes() []int {
	if len(d.Sizes) > 0 {
		return d.Sizes
	}
	return tableSizes
}

// resolve is the table's content for the URL ctx renders: its query read,
// the page loaded (the last page when the URL asks for one past it).
func (d DataTable[T]) resolve(ctx context.Context) Node {
	u := urlOf(ctx, d.Key)
	q := u.query(d.sizes(), d.sortable())
	b := dataTableBody[T]{D: d, URL: u, Query: q, Server: true}
	if d.Load == nil {
		b.Page = pageOf(d.Rows, q, d.SearchBy, d.Sorts)
		b.Page.All = len(d.Rows)
		return b
	}
	p, err := d.Load(ctx, q)
	if err == nil {
		if pages := (p.Total + q.Size - 1) / q.Size; pages > 0 && q.Page > pages {
			q.Page = pages
			b.Query = q
			p, err = d.Load(ctx, q)
		}
	}
	if err != nil {
		slog.ErrorContext(ctx, "ui: a DataTable's Load failed", "table", d.id(), "err", err)
		b.Failed = true
		return b
	}
	all := p.All
	b.Page = NewPage(p.Items, p.Total, q.Page, q.Size)
	b.Page.All = all
	return b
}
