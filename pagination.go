package ui

// Page is one page of a larger list: the items of this page and the counts a
// pager needs. It is a plain value with no dependencies, so a server builds it
// (see web.Paginate) and a view renders it (see DataTable) without either
// side knowing the other.
//
//	p := ui.NewPage(rows, total, page, size)
//	for _, row := range p.Items { … }
//	if p.HasNext { … }
type Page[T any] struct {
	Items   []T  // the items of this page
	Total   int  // how many items the whole list holds
	Page    int  // this page's number, 1-based
	Size    int  // how many items a full page holds
	Pages   int  // how many pages the list has (0 when empty)
	HasPrev bool // whether a page comes before this one
	HasNext bool // whether a page comes after this one
}

// NewPage builds a Page from the items fetched for a page and the total count
// of the list. page is 1-based and clamped to the list; size is the page
// window (at least 1). It computes Pages, HasPrev and HasNext; it does not
// slice items, which the caller has already fetched for the page.
func NewPage[T any](items []T, total, page, size int) Page[T] {
	if size < 1 {
		size = 1
	}
	pages := (total + size - 1) / size
	if page < 1 {
		page = 1
	}
	if pages > 0 && page > pages {
		page = pages
	}
	return Page[T]{
		Items:   items,
		Total:   total,
		Page:    page,
		Size:    size,
		Pages:   pages,
		HasPrev: page > 1,
		HasNext: page < pages,
	}
}

// Empty reports whether the whole list holds no items.
func (p Page[T]) Empty() bool { return p.Total == 0 }

// From is the 1-based number of this page's first item, for "showing 1–10 of
// 42"; 0 when the page has no items.
func (p Page[T]) From() int {
	if len(p.Items) == 0 {
		return 0
	}
	return (p.Page-1)*p.Size + 1
}

// To is the 1-based number of this page's last item; 0 when the page has no
// items.
func (p Page[T]) To() int {
	if len(p.Items) == 0 {
		return 0
	}
	return p.From() + len(p.Items) - 1
}

// Window is up to n page numbers centred on the current page, for a pager's
// numbered controls: a stable-width run clamped to the list, e.g. with n=5 on
// page 7 of 20 it is [5 6 7 8 9]. It returns nil when the list has no pages.
func (p Page[T]) Window(n int) []int {
	if p.Pages == 0 || n < 1 {
		return nil
	}
	if n > p.Pages {
		n = p.Pages
	}
	start := p.Page - n/2
	if start < 1 {
		start = 1
	}
	if start+n-1 > p.Pages {
		start = p.Pages - n + 1
	}
	out := make([]int, n)
	for i := range out {
		out[i] = start + i
	}
	return out
}
