package ui

import (
	"context"
	"slices"

	"github.com/vuka-lang/orm/v2"
)

// Source says how LoadPage pages a model's rows for a DataTable:
//
//	func (p *Books) load(ctx context.Context, q ui.TableQuery) (ui.Page[Book], error) {
//		return ui.LoadPage(ctx, q, ui.Source[Book]{
//			Search: func(qs orm.QuerySet[Book], term string) orm.QuerySet[Book] {
//				return qs.Filter(orm.Q{"title__icontains": term})
//			},
//			Sorts:   map[string]string{"title": "title", "author": "author__name"},
//			Default: "-published",
//		})
//	}
type Source[T any] struct {
	// Rows is the set to page; every row of T when nil.
	Rows *orm.QuerySet[T]
	// Search narrows the rows to those matching a non-empty search term.
	// Without it the search is ignored.
	Search func(qs orm.QuerySet[T], term string) orm.QuerySet[T]
	// Sorts maps a column's sort key (q.Sort) to the field it orders by; a
	// key it doesn't list is ignored. Without Sorts a key the table's
	// columns declare (q.Sortable) is the field's name itself, so
	// <slot:column sort="title"> is written once.
	Sorts map[string]string
	// Default orders the rows when no sort applies: a field name, a leading
	// - for descending.
	Default string
}

// LoadPage is the page q asks for of src's rows: counted before the search
// (Page.All) and after it (Page.Total), ordered by the sort the table sent
// or Default, and cut by the database to q's page, the last page when q
// asks for one past it.
func LoadPage[T any](ctx context.Context, q TableQuery, src Source[T]) (Page[T], error) {
	rows := orm.Objects[T]().Filter()
	if src.Rows != nil {
		rows = *src.Rows
	}
	all, err := rows.Count(ctx)
	if err != nil {
		return Page[T]{}, err
	}
	found := rows
	if q.Search != "" && src.Search != nil {
		found = src.Search(found, q.Search)
	}
	total := all
	if q.Search != "" && src.Search != nil {
		if total, err = found.Count(ctx); err != nil {
			return Page[T]{}, err
		}
	}
	if order := src.order(q); order != "" {
		found = found.OrderBy(order)
	}
	size := max(q.Size, 1)
	p := NewPage[T](nil, int(total), q.Page, size)
	p.All = int(all)
	if total == 0 {
		return p, nil
	}
	if p.Items, err = found.Limit(size).Offset((p.Page - 1) * size).All(ctx); err != nil {
		return Page[T]{}, err
	}
	return p, nil
}

// order is the field q's sort orders by, a leading - for descending, else
// Default.
func (src Source[T]) order(q TableQuery) string {
	field, ok := src.Sorts[q.Sort]
	if src.Sorts == nil && q.Sort != "" && slices.Contains(q.Sortable, q.Sort) {
		field, ok = q.Sort, true
	}
	if !ok || field == "" {
		return src.Default
	}
	if q.Desc {
		return "-" + field
	}
	return field
}
