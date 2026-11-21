package ui

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strconv"
)

// DataTable is a Table with a search box, sortable headers and a pager.
// It gets its rows in one of four ways, with the same columns and markup:
//
// From a store (Load): the table reads its page, size, search and sort from
// the page's URL and calls Load with them; Load returns that page (see
// LoadPage, which pages an orm QuerySet). Sorting, paging and searching
// write the URL, so each is a patch of the live page, a link without one:
//
//	<ui.DataTable load={Books} searchPlaceholder="Find books">
//		<slot:column header="Title" sort="title" let={b}>{b.Title}</slot:column>
//		<slot:actions let={b}><ui.ActionMenu icon="trash" danger on={ui.JS.Push(Delete, ui.Value(b.ID))}>Delete</ui.ActionMenu></slot:actions>
//	</ui.DataTable>
//
// From a slice (Rows): every row; the table cuts, finds (SearchBy) and orders
// (Sorts) them itself, from the URL as Load does.
//
// In the browser (Client with Rows): every row is in the page and the
// browser searches and pages them, with no request.
//
// From a live view's events (Page): it renders the page it is given and
// calls the On… handlers the host supplies:
//
//	<ui.DataTable columns={cols} page={p.page.Get()} search={p.query.Get()}
//		onSearch={p.Search} onSort={p.SortBy} onPage={p.GoTo}/>
//
// The On… handlers are live event handlers (a func, as an on… attribute
// takes); leaving one nil drops its control, so a DataTable can offer just
// paging, or paging with search and sort.
//
// The table's root carries ID ("records" unless set), so
// ui.JS.Reload("#records") renders it again, calling Load again.
type DataTable[T any] struct {
	Columns []Column[T]
	// Page is the current page of rows (see web.Paginate).
	Page Page[T]

	// OnPage is called with a 1-based page number when a pager control is
	// used; without it the pager renders as static text.
	OnPage any
	// OnSize is called with a new page size when the size select changes;
	// shown only with Sizes.
	OnSize any
	// Sizes are the page-size options: offered when OnSize is set, and by a
	// table with Load or Rows (10, 25, 50 and 100 without them).
	Sizes []int
	// OnSearch is called with the search box's value as the user types;
	// without it no search box is shown.
	OnSearch any
	// Search is the current search value (kept in the box across renders).
	Search            string
	SearchPlaceholder string
	// OnSort is called with a column's Sort key when its header is clicked;
	// columns with no Sort key stay plain.
	OnSort any
	// Sort and SortDir ("asc"/"desc") mark the sorted column for the header
	// indicator.
	Sort    string
	SortDir string

	// Window is how many numbered pager buttons to show; default 5.
	Window int

	// Passed through to the underlying Table.
	RowKey       func(T) any
	RowHref      func(T) string
	RowActions   func(T) Node
	Selectable   bool
	SelectName   string
	RowSelected  func(T) bool
	OnSelect     func(T) any
	OnSelectAll  any
	AllSelected  bool
	SomeSelected bool
	// Index adds a row-number column numbered from the page's first row.
	Index     bool
	Empty     Node
	EmptyIcon Node
	// Flush drops the table's border so it sits flush in a Card.
	Flush bool
	Class string

	// Toolbar is extra controls shown in the bar beside the search box
	// (filters, a "New" button).
	Toolbar Node
	// Tabs, when set, are shown in the bar above the table.
	Tabs Node

	// ID is the id of the table's root, what ui.JS.Reload("#"+ID) renders
	// again; "records" unless set. Two tables on a page need two.
	ID string
	// Key prefixes the table's URL parameters (q, page, size, sort, dir):
	// "orders" keeps them as orders.page, so two tables on one page page
	// apart.
	Key string
	// Load returns the page q asks for: what the table renders, with q read
	// from the page's URL. An error is logged and shown as the table's
	// error state.
	Load func(ctx context.Context, q TableQuery) (Page[T], error)
	// Rows is every row, for the table to page itself: on the server from
	// the URL, or in the browser with Client.
	Rows []T
	// Client pages and searches Rows in the browser, with no request.
	Client bool
	// SearchBy is the text a row of Rows is found by; without it Rows has
	// no search box (Client finds a row by what it shows).
	SearchBy func(T) string
	// Sorts orders Rows by a column's sort key: the value a row sorts by
	// (numbers and times by value, anything else as text).
	Sorts map[string]func(T) any
	// NoSearch drops the search box of a table with Load.
	NoSearch bool
	// Actions is a row's menu: its ActionMenu items, shown behind a button
	// in a trailing column (<slot:actions let={row}>).
	Actions func(T) Node
	// Loading marks the table busy (aria-busy), as it is while one of its
	// controls waits for the server.
	Loading bool
}

// View is the table's markup, dataTable's: it is compiled once for every
// row type, since d's rows are erased to any first.
func (d DataTable[T]) View() Node { return dataTable{d.erase()}.View() }

func (d DataTable[T]) Render(ctx context.Context, w io.Writer) error { return d.View().Render(ctx, w) }

// erase is d over rows of type any, as Table.erase.
func (d DataTable[T]) erase() DataTable[any] {
	if e, ok := any(d).(DataTable[any]); ok {
		return e
	}
	e := DataTable[any]{
		Columns: eraseColumns(d.Columns), Page: erasePage(d.Page),
		OnPage: d.OnPage, OnSize: d.OnSize, Sizes: d.Sizes,
		OnSearch: d.OnSearch, Search: d.Search, SearchPlaceholder: d.SearchPlaceholder,
		OnSort: d.OnSort, Sort: d.Sort, SortDir: d.SortDir, Window: d.Window,
		RowKey: eraseRow(d.RowKey), RowHref: eraseRow(d.RowHref), RowActions: eraseRow(d.RowActions),
		Selectable: d.Selectable, SelectName: d.SelectName,
		RowSelected: eraseRow(d.RowSelected), OnSelect: eraseRow(d.OnSelect), OnSelectAll: d.OnSelectAll,
		AllSelected: d.AllSelected, SomeSelected: d.SomeSelected,
		Index: d.Index, Empty: d.Empty, EmptyIcon: d.EmptyIcon, Flush: d.Flush, Class: d.Class,
		Toolbar: d.Toolbar, Tabs: d.Tabs, ID: d.ID, Key: d.Key,
		Rows: eraseRows(d.Rows), Client: d.Client, SearchBy: eraseRow(d.SearchBy),
		NoSearch: d.NoSearch, Actions: eraseRow(d.Actions), Loading: d.Loading,
	}
	if d.Load != nil {
		e.Load = func(ctx context.Context, q TableQuery) (Page[any], error) {
			p, err := d.Load(ctx, q)
			return erasePage(p), err
		}
	}
	if d.Sorts != nil {
		e.Sorts = make(map[string]func(any) any, len(d.Sorts))
		for k, by := range d.Sorts {
			e.Sorts[k] = eraseRow(by)
		}
	}
	return e
}

func erasePage[T any](p Page[T]) Page[any] {
	return Page[any]{Items: eraseRows(p.Items), Total: p.Total, Page: p.Page, Size: p.Size, Pages: p.Pages, HasPrev: p.HasPrev, HasNext: p.HasNext, All: p.All}
}

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
func sortHeader(c Column[any], sort, dir string, onSort any) Node {
	icon := Icon("caret-up", "vk-sort__ind vk-sort__ind--active")
	if c.Sort != sort {
		icon = Icon("caret-up-down", "vk-sort__ind")
	} else if dir == "desc" {
		icon = Icon("caret-down", "vk-sort__ind vk-sort__ind--active")
	}
	return HTML(`<button type="button" class="vk-sort" aria-sort={sortAria(c.Sort == sort, dir)} onClick={JS.Push(onSort, Value(c.Sort))}>{c.Header}{icon}</button>`, sortAria(c.Sort == sort, dir), JS.Push(onSort, Value(c.Sort)), c.Header, icon)
}

// pager is a page's numbered buttons between previous and next.
func pager(page Page[any], window int, onPage any) Node {
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
func (d dataTable) sortable() []string {
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
func (d dataTable) sizes() []int {
	if len(d.Sizes) > 0 {
		return d.Sizes
	}
	return tableSizes
}

// resolve is the table's content for the URL ctx renders: its query read,
// the page loaded (the last page when the URL asks for one past it).
func (d dataTable) resolve(ctx context.Context) Node {
	noteTable(ctx, d.Key)
	u := urlOf(ctx, d.Key)
	q := u.query(d.sizes(), d.sortable())
	b := dataTableBody{D: d, URL: u, Query: q, Server: true}
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
