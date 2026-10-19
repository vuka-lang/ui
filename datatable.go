package ui

import "strconv"

// DataTable is a Table with a search box, sortable headers and a pager,
// driven from a live view's events. It renders a page (ui.Page[T], e.g. from
// web.Paginate) and calls the handlers the host supplies:
//
//	<ui.DataTable columns={cols} page={p.page.Get()} search={p.query.Get()}
//		onSearch={p.Search} onSort={p.SortBy} onPage={p.GoTo}/>
//
// The On… handlers are live event handlers (a func, as an on… attribute
// takes); leaving one nil drops its control, so a DataTable can offer just
// paging, or paging with search and sort.
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
	// Sizes are the page-size options offered when OnSize is set.
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
}

// DataTableProps is DataTable, by its former name.
//
// Deprecated: use DataTable: ui.DataTable[T]{…}, or
// <ui.DataTable …> in a template.
type DataTableProps[T any] = DataTable[T]

// table is the Table of the page's rows, its sortable columns' headers sort
// controls.
func (d DataTable[T]) table() Table[T] {
	cols := d.Columns
	if d.OnSort != nil {
		cols = make([]Column[T], len(d.Columns))
		copy(cols, d.Columns)
		for i := range cols {
			if cols[i].Sort != "" {
				cols[i].Header = sortHeader(cols[i], d.Sort, d.SortDir, d.OnSort)
			}
		}
	}
	return Table[T]{
		Columns: cols, Rows: d.Page.Items, RowKey: d.RowKey, RowHref: d.RowHref,
		RowActions: d.RowActions, Selectable: d.Selectable, SelectName: d.SelectName,
		RowSelected: d.RowSelected, OnSelect: d.OnSelect, OnSelectAll: d.OnSelectAll,
		AllSelected: d.AllSelected, SomeSelected: d.SomeSelected,
		Index: d.Index, IndexFrom: d.Page.From() - 1,
		Empty: d.Empty, EmptyIcon: d.EmptyIcon, Flush: d.Flush,
	}
}

func (d DataTable[T]) placeholder() string {
	if d.SearchPlaceholder == "" {
		return "Search…"
	}
	return d.SearchPlaceholder
}

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
