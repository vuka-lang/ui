package ui

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
