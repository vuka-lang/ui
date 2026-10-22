package ui

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
