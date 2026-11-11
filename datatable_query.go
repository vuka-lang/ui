package ui

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TableQuery is the page a DataTable asks its Load for: as the page's URL
// says, checked against the table. Page is from 1, as asked (Load cuts what
// there is); Size is one of the table's sizes; Sort is one of its columns'
// sort keys, or empty; Search is trimmed.
type TableQuery struct {
	Page, Size int
	Search     string
	Sort       string
	Desc       bool
	// Sortable is the sort keys the table's columns declare: what LoadPage
	// orders by, as field names, when its Source lists no Sorts.
	Sortable []string
}

// Offset is how many rows come before the page: (Page-1)*Size.
func (q TableQuery) Offset() int { return (max(q.Page, 1) - 1) * q.Size }

// tableSizes are the page sizes a DataTable kept in the URL offers without
// Sizes.
var tableSizes = []int{10, 25, 50, 100}

// tableURL is the page's address as a DataTable kept in it reads and writes
// it: its path, its query, and the table's Key prefixing its parameters.
type tableURL struct {
	path   string
	values url.Values
	key    string
}

func urlOf(ctx context.Context, key string) tableURL {
	t := tableURL{key: key, values: url.Values{}}
	if u := PageURL(ctx); u != nil {
		t.path, t.values = u.Path, u.Query()
	}
	return t
}

// param is the name of the table's parameter: q, page, size, sort, dir,
// under Key when it has one ("orders.page").
func (u tableURL) param(name string) string {
	if u.key == "" {
		return name
	}
	return u.key + "." + name
}

func (u tableURL) get(name string) string { return u.values.Get(u.param(name)) }

// query reads the table's query from the URL: sizes are the sizes it
// offers (the first the default), sortable its columns' sort keys.
func (u tableURL) query(sizes []int, sortable []string) TableQuery {
	q := TableQuery{Search: strings.TrimSpace(u.get("q")), Sortable: sortable}
	if len(q.Search) > 200 {
		q.Search = q.Search[:200]
	}
	if s := u.get("sort"); slices.Contains(sortable, s) {
		q.Sort, q.Desc = s, u.get("dir") == "desc"
	}
	q.Size, _ = strconv.Atoi(u.get("size"))
	if !slices.Contains(sizes, q.Size) {
		q.Size = sizes[0]
	}
	q.Page, _ = strconv.Atoi(u.get("page"))
	q.Page = max(q.Page, 1)
	return q
}

// href is the page's URL with the table's parameters set (pairs of name,
// value; an empty value drops the parameter), the others kept.
func (u tableURL) href(set ...string) string {
	v := url.Values{}
	for k, vs := range u.values {
		v[k] = slices.Clone(vs)
	}
	for i := 0; i+1 < len(set); i += 2 {
		if set[i+1] == "" {
			v.Del(u.param(set[i]))
		} else {
			v.Set(u.param(set[i]), set[i+1])
		}
	}
	if len(v) == 0 {
		return cmp.Or(u.path, "?")
	}
	return u.path + "?" + v.Encode()
}

func (u tableURL) pageHref(n int) string {
	if n <= 1 {
		return u.href("page", "")
	}
	return u.href("page", strconv.Itoa(n))
}

// sortHref is a sortable header's link: ascending, then descending, then
// the table's own order.
func (u tableURL) sortHref(q TableQuery, key string) string {
	switch {
	case q.Sort != key:
		return u.href("sort", key, "dir", "", "page", "")
	case !q.Desc:
		return u.href("sort", key, "dir", "desc", "page", "")
	}
	return u.href("sort", "", "dir", "", "page", "")
}

// kept is the query's parameters that aren't the table's, for its search
// form to send again.
func (u tableURL) kept() [][2]string {
	own := map[string]bool{}
	for _, n := range []string{"q", "page", "size", "sort", "dir"} {
		own[u.param(n)] = true
	}
	var out [][2]string
	for _, k := range slices.Sorted(func(yield func(string) bool) {
		for k := range u.values {
			if !own[k] && !yield(k) {
				return
			}
		}
	}) {
		for _, s := range u.values[k] {
			out = append(out, [2]string{k, s})
		}
	}
	return out
}

// edit is a live handler writing the table's parameter name: the value the
// event carries, the page back to the first.
func (u tableURL) edit(name string) func(ctx context.Context, v string) {
	return func(ctx context.Context, v string) {
		EditURL(ctx, func(q url.Values) {
			v = strings.TrimSpace(v)
			if v == "" {
				q.Del(u.param(name))
			} else {
				q.Set(u.param(name), v)
			}
			q.Del(u.param("page"))
		}, false)
	}
}

// pageOf is the page q asks for of rows, a DataTable's Rows: those search
// finds q.Search in, ordered by sorts[q.Sort], cut to q's page.
func pageOf[T any](rows []T, q TableQuery, search func(T) string, sorts map[string]func(T) any) Page[T] {
	if q.Search != "" && search != nil {
		term := strings.ToLower(q.Search)
		rows = slices.DeleteFunc(slices.Clone(rows), func(r T) bool { return !strings.Contains(strings.ToLower(search(r)), term) })
	}
	if by := sorts[q.Sort]; by != nil {
		rows = slices.Clone(rows)
		slices.SortStableFunc(rows, func(a, b T) int {
			if q.Desc {
				a, b = b, a
			}
			return compareCells(by(a), by(b))
		})
	}
	p := NewPage[T](nil, len(rows), q.Page, q.Size)
	start := min((p.Page-1)*p.Size, len(rows))
	p.Items = rows[start:min(start+p.Size, len(rows))]
	return p
}

// compareCells orders two values a Sorts func returned: numbers and times
// by value, anything else by its text, digit runs as numbers.
func compareCells(a, b any) int {
	switch x := a.(type) {
	case int:
		if y, ok := b.(int); ok {
			return cmp.Compare(x, y)
		}
	case int64:
		if y, ok := b.(int64); ok {
			return cmp.Compare(x, y)
		}
	case float64:
		if y, ok := b.(float64); ok {
			return cmp.Compare(x, y)
		}
	case time.Time:
		if y, ok := b.(time.Time); ok {
			return x.Compare(y)
		}
	case bool:
		if y, ok := b.(bool); ok && x != y {
			return map[bool]int{false: -1, true: 1}[x]
		}
	}
	return naturalCompare(strings.ToLower(fmt.Sprint(a)), strings.ToLower(fmt.Sprint(b)))
}

func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		da, db := digits(a), digits(b)
		if da > 0 && db > 0 {
			na, _ := strconv.ParseUint(a[:min(da, 18)], 10, 64)
			nb, _ := strconv.ParseUint(b[:min(db, 18)], 10, 64)
			if n := cmp.Compare(na, nb); n != 0 {
				return n
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			return cmp.Compare(a[0], b[0])
		}
		a, b = a[1:], b[1:]
	}
	return cmp.Compare(len(a), len(b))
}

func digits(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}
