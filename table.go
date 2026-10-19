package ui

import "strings"

// Column describes one column of a Table or DataTable over rows of type T.
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
}

// TableProps is Table, by its former name.
//
// Deprecated: use Table: ui.Table[T]{…}, or <ui.Table …> in a
// template.
type TableProps[T any] = Table[T]

func (t Table[T]) classes() string {
	c := "vk-table"
	if t.Flush {
		c += " vk-table--flush"
	}
	return withClass(c, t.Class)
}

// span is the number of columns, the leading and trailing ones included.
func (t Table[T]) span() int {
	n := len(t.Columns)
	for _, on := range []bool{t.Selectable, t.Index, t.RowActions != nil} {
		if on {
			n++
		}
	}
	return n
}

func (t Table[T]) selectName() string {
	if t.SelectName == "" {
		return "selected"
	}
	return t.SelectName
}

// mixed is the header checkbox's state when some rows but not all are
// selected.
func (t Table[T]) mixed() []Attr {
	if !t.SomeSelected || t.AllSelected {
		return nil
	}
	return []Attr{{"aria-checked", "mixed"}, {"data-vk-mixed", "true"}}
}

// key is a row's key for the live diff, nil without RowKey.
func (t Table[T]) key(row T) any {
	if t.RowKey == nil {
		return nil
	}
	return t.RowKey(row)
}

func (t Table[T]) rowClass(row T) any {
	var c []string
	if t.RowHref != nil {
		c = append(c, "vk-row--link")
	}
	if t.RowSelected != nil && t.RowSelected(row) {
		c = append(c, "vk-row--selected")
	}
	return nonEmpty(strings.Join(c, " "))
}

func (t Table[T]) rowNav(row T) []Attr {
	if t.RowHref == nil {
		return nil
	}
	return on("onClick", JS.Navigate(t.RowHref(row)))
}

// selectValue is a row checkbox's value: its key, as text.
func (t Table[T]) selectValue(row T) any {
	if t.RowKey == nil {
		return nil
	}
	return Text(t.RowKey(row))
}

// selectBox is how a row's checkbox is bound: live, ticked by RowSelected
// and calling OnSelect, or a plain form field named SelectName.
func (t Table[T]) selectBox(row T) []Attr {
	if t.OnSelect == nil {
		return []Attr{{"name", t.selectName()}}
	}
	var a []Attr
	if t.RowSelected != nil && t.RowSelected(row) {
		a = append(a, Attr{"checked", true})
	}
	return append(a, on("onChange", t.OnSelect(row))...)
}

func colClass[T any](c Column[T]) string {
	class := ""
	switch c.Align {
	case "right":
		class = "vk-col--right"
	case "center":
		class = "vk-col--center"
	}
	if c.Class != "" {
		if class != "" {
			class += " "
		}
		class += c.Class
	}
	return class
}

func (c Column[T]) cell(row T) Node {
	if c.Cell == nil {
		return nil
	}
	return c.Cell(row)
}

func (t Table[T]) empty() Node {
	if t.Empty == nil {
		return Text("Nothing to show.")
	}
	return t.Empty
}
