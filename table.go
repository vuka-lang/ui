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

// TableProps configures a Table over rows of type T.
type TableProps[T any] struct {
	Columns []Column[T]
	Rows    []T
	// RowKey gives each row a stable key for the live diff (the row's id).
	RowKey func(T) any
	// RowHref, when set, makes a row navigate to it on click (over the live
	// connection when connected).
	RowHref func(T) string
	// RowActions, when set, adds a trailing column rendering per-row controls.
	RowActions func(T) Node
	// Selectable adds a leading checkbox column; the checkboxes are named
	// SelectName (default "selected") with the row's RowKey as value, for a
	// form or a server handler to read.
	Selectable bool
	SelectName string
	// Empty is shown in place of the body when there are no rows; a default
	// message is used when nil.
	Empty Node
	Class string
}

// Table renders rows of type T as an HTML table styled with the --ui-*
// tokens. It is presentational — DataTable adds paging, search and sort:
//
//	ui.Table(ui.TableProps[Book]{
//		Columns: []ui.Column[Book]{
//			{Header: ui.Text("Title"), Cell: func(b Book) ui.Node { return ui.Text(b.Title) }},
//			{Header: ui.Text("Author"), Cell: func(b Book) ui.Node { return ui.Text(b.Author) }},
//		},
//		Rows:   books,
//		RowKey: func(b Book) any { return b.ID },
//	})
func Table[T any](p TableProps[T]) Node {
	class := "vk-table"
	if p.Class != "" {
		class += " " + p.Class
	}
	span := len(p.Columns)
	if p.Selectable {
		span++
	}
	if p.RowActions != nil {
		span++
	}

	table := El("table", []Attr{{"class", class}},
		El("thead", nil, El("tr", nil, headerCells(p)...)),
		tableBody(p, span),
	)
	return El("div", []Attr{{"class", "vk-table-wrap"}}, table)
}

func headerCells[T any](p TableProps[T]) []Node {
	var cells []Node
	if p.Selectable {
		cells = append(cells, El("th", []Attr{{"class", "vk-col--center"}, {"scope", "col"}}))
	}
	for _, c := range p.Columns {
		cells = append(cells, El("th", thAttrs(c), c.Header))
	}
	if p.RowActions != nil {
		cells = append(cells, El("th", []Attr{{"class", "vk-col--right"}, {"scope", "col"}, {"aria-label", "Actions"}}))
	}
	return cells
}

func thAttrs[T any](c Column[T]) []Attr {
	return []Attr{{"scope", "col"}, {"class", colClass(c)}}
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

func tableBody[T any](p TableProps[T], span int) Node {
	if len(p.Rows) == 0 {
		empty := p.Empty
		if empty == nil {
			empty = Text("Nothing to show.")
		}
		return El("tbody", nil, El("tr", nil,
			El("td", []Attr{{"colspan", span}, {"class", "vk-table__empty"}}, empty)))
	}
	name := p.SelectName
	if name == "" {
		name = "selected"
	}
	var rows []Node
	for _, row := range p.Rows {
		var cells []Node
		if p.Selectable {
			cb := []Attr{{"type", "checkbox"}, {"name", name}}
			if p.RowKey != nil {
				cb = append(cb, Attr{"value", Text(p.RowKey(row))})
			}
			cells = append(cells, El("td", []Attr{{"class", "vk-col--center"}}, El("input", cb)))
		}
		for _, c := range p.Columns {
			cells = append(cells, El("td", []Attr{{"class", colClass(c)}}, cellNode(c, row)))
		}
		if p.RowActions != nil {
			cells = append(cells, El("td", []Attr{{"class", "vk-col--right"}}, p.RowActions(row)))
		}

		attrs := []Attr{}
		if p.RowKey != nil {
			attrs = append(attrs, Attr{"key", p.RowKey(row)})
		}
		if p.RowHref != nil {
			attrs = append(attrs, Attr{"class", "vk-row--link"}, Attr{"onClick", On(JS.Navigate(p.RowHref(row)))})
		}
		rows = append(rows, El("tr", attrs, cells...))
	}
	return El("tbody", nil, rows...)
}

func cellNode[T any](c Column[T], row T) Node {
	if c.Cell == nil {
		return nil
	}
	return c.Cell(row)
}
