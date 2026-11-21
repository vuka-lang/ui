package ui

import (
	"context"
	"io"
)

// Column describes one column of a Table or DataTable over rows of type T.
//
// In a template each <slot:column> inside the tag is one Column, in order:
// its attributes set the fields (a quoted header is text) and its content,
// with let naming the row, is Cell:
//
//	<ui.DataTable page={Page} onPage={GoTo}>
//	  <slot:column header="Title" let={b}><b>{b.Title}</b></slot:column>
//	  <slot:column header="Pages" align="right" let={b}>{b.Pages}</slot:column>
//	</ui.DataTable>
//
// A <slot:header> inside a <slot:column> gives a header with markup.
type Column[T any] struct {
	// Header is the heading cell's content.
	Header Node
	// Cell renders a row's cell in this column.
	Cell func(T) Node
	// Align is "", "right" or "center".
	Align string
	// Class is added to every cell (header and body) in this column.
	Class string
	// Sort, when set, is the column's sort key: DataTable renders the header
	// as a sort control that calls its OnSort with this key. Table ignores it.
	Sort string
}

// TableProps is Table, by its former name.
//
// Deprecated: use Table: ui.Table[T]{…}, or <ui.Table …> in a
// template.
type TableProps[T any] = Table[T]

func colClass(c Column[any]) string {
	class := ""
	switch c.Align {
	case "right":
		class = "vk-col--right"
	case "center":
		class = "vk-col--center"
	}
	return withClass(class, c.Class)
}

func cellOf(c Column[any], row any) Node {
	if c.Cell == nil {
		return nil
	}
	return c.Cell(row)
}

// Table renders rows of type T as an HTML table styled with the --ui-*
// tokens. It is presentational — DataTable adds paging, search and sort:
//
//	<ui.Table columns={cols} rows={books} rowKey={func(b Book) any { return b.ID }}/>
//
// with
//
//	cols := []ui.Column[Book]{
//		{Header: ui.Text("Title"), Cell: func(b Book) ui.Node { return ui.Text(b.Title) }},
//		{Header: ui.Text("Author"), Cell: func(b Book) ui.Node { return ui.Text(b.Author) }},
//	}
type Table[T any] struct {
	Columns []Column[T]
	Rows    []T
	// RowKey gives each row a stable key for the live diff (the row's id).
	RowKey func(T) any
	// RowHref, when set, makes a row navigate to it on click (over the live
	// connection when connected).
	RowHref func(T) string
	// RowActions, when set, adds a trailing column rendering per-row controls.
	RowActions func(T) Node

	// Selectable adds a leading checkbox column. With OnSelect the boxes are
	// live (ticked by RowSelected, toggled through OnSelect); without it they
	// are plain form checkboxes named SelectName (default "selected") with the
	// row's RowKey as value, for a form or handler to read.
	Selectable bool
	SelectName string
	// RowSelected is a live checkbox's ticked state, and highlights the row.
	RowSelected func(T) bool
	// OnSelect is the live handler a row's checkbox calls on change; it
	// receives the row and returns the handler to run. The checkbox fires
	// onChange with its new ticked state, so the returned handler should take
	// that bool (func(ctx, bool) or func(bool)) and set the row's selection
	// to it — reading the real box state, not toggling a server-side guess.
	OnSelect func(T) any
	// OnSelectAll is the header checkbox's onChange handler; it takes the new
	// ticked state (func(ctx, bool) / func(bool)) — select the page when true,
	// clear it when false.
	OnSelectAll any
	// AllSelected ticks the header checkbox; SomeSelected marks it mixed.
	AllSelected  bool
	SomeSelected bool

	// Index adds a leading row-number column. IndexFrom is the number before
	// the first row (0 numbers the first row 1); DataTable sets it from the
	// page.
	Index     bool
	IndexFrom int

	// Empty is shown in place of the body when there are no rows; a default
	// message is used when nil. EmptyIcon is shown above it.
	Empty     Node
	EmptyIcon Node

	// Flush drops the table's outer border and side padding so it sits flush
	// in a Card (use with Card.NoPad).
	Flush bool
	Class string

	// rowAttrs are attributes of a row's tr: a DataTable's, for the browser.
	rowAttrs func(T) []Attr
}

// View is the table's markup, table's: it is compiled once for every row
// type, since t's rows are erased to any first.
func (t Table[T]) View() Node { return table{t.erase()}.View() }

func (t Table[T]) Render(ctx context.Context, w io.Writer) error { return t.View().Render(ctx, w) }

// erase is t over rows of type any: each row a *T into t.Rows, and each
// function taking a row its own taking that pointer.
func (t Table[T]) erase() Table[any] {
	if e, ok := any(t).(Table[any]); ok {
		return e
	}
	return Table[any]{
		Columns: eraseColumns(t.Columns), Rows: eraseRows(t.Rows),
		RowKey: eraseRow(t.RowKey), RowHref: eraseRow(t.RowHref), RowActions: eraseRow(t.RowActions),
		Selectable: t.Selectable, SelectName: t.SelectName,
		RowSelected: eraseRow(t.RowSelected), OnSelect: eraseRow(t.OnSelect), OnSelectAll: t.OnSelectAll,
		AllSelected: t.AllSelected, SomeSelected: t.SomeSelected,
		Index: t.Index, IndexFrom: t.IndexFrom,
		Empty: t.Empty, EmptyIcon: t.EmptyIcon,
		Flush: t.Flush, Class: t.Class,
		rowAttrs: eraseRow(t.rowAttrs),
	}
}

// The row type is erased at the Table and DataTable boundary: rows become
// pointers into the caller's slice, held as any (which costs no allocation
// per row), and the functions given a row get it back as a T.

func eraseRows[T any](rows []T) []any {
	if rows == nil {
		return nil
	}
	out := make([]any, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out
}

func eraseRow[T, R any](f func(T) R) func(any) R {
	if f == nil {
		return nil
	}
	return func(row any) R { return f(*row.(*T)) }
}

func eraseColumns[T any](cols []Column[T]) []Column[any] {
	if cols == nil {
		return nil
	}
	out := make([]Column[any], len(cols))
	for i, c := range cols {
		out[i] = Column[any]{Header: c.Header, Cell: eraseRow(c.Cell), Align: c.Align, Class: c.Class, Sort: c.Sort}
	}
	return out
}
