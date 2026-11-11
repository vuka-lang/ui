package live_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

type shelfBook struct {
	ID     int
	Title  string
	Author string
}

var shelf = []shelfBook{
	{1, "Dune", "Herbert"}, {2, "Emma", "Austen"}, {3, "Ulysses", "Joyce"},
	{4, "Persuasion", "Austen"}, {5, "Beloved", "Morrison"},
}

// loadShelf is a Load over shelf, the way a store pages: it counts every
// call.
func loadShelf(calls *int) func(ctx context.Context, q ui.TableQuery) (ui.Page[shelfBook], error) {
	return func(ctx context.Context, q ui.TableQuery) (ui.Page[shelfBook], error) {
		*calls++
		var found []shelfBook
		for _, b := range shelf {
			if q.Search == "" || strings.Contains(strings.ToLower(b.Author), strings.ToLower(q.Search)) {
				found = append(found, b)
			}
		}
		if q.Sort == "title" {
			for i := range found {
				for j := i + 1; j < len(found); j++ {
					if (found[j].Title < found[i].Title) != q.Desc {
						found[i], found[j] = found[j], found[i]
					}
				}
			}
		}
		start := min(q.Offset(), len(found))
		return ui.Page[shelfBook]{Items: found[start:min(start+q.Size, len(found))], Total: len(found), All: len(shelf)}, nil
	}
}

func shelfColumns() []ui.Column[shelfBook] {
	return []ui.Column[shelfBook]{
		{Header: ui.Text("Title"), Sort: "title", Cell: func(b shelfBook) ui.Node { return ui.Text(b.Title) }},
		{Header: ui.Text("Author"), Cell: func(b shelfBook) ui.Node { return ui.Text(b.Author) }},
	}
}

// TestDataTableLoad drives a table with Load: sorting, paging, searching
// and the page size write the page's URL (its other parameters kept), the
// table renders the page Load returns for it, ui.JS.Reload("#records")
// calls Load again, and a row's ActionMenu item runs its handler.
func TestDataTableLoad(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(fmt.Sprint("v", v), func(t *testing.T) {
			calls, deleted := 0, 0
			render := func() ui.Node {
				return ui.El("main", nil,
					ui.El("button", []ui.Attr{a("id", "reload"), a("onClick", ui.On(ui.JS.Reload("#records")))}),
					ui.DataTable[shelfBook]{
						Columns: shelfColumns(), Load: loadShelf(&calls), Sizes: []int{2, 5},
						RowKey: func(b shelfBook) any { return b.ID },
						Actions: func(b shelfBook) ui.Node {
							return ui.ActionMenu{Icon: "trash", Danger: true, On: func() { deleted = b.ID }, Children: ui.Text("Delete")}
						},
					})
			}
			s, c := urlSession(t, v, "/books?x=1", render)
			check := func(url, count string, order ...string) {
				t.Helper()
				html := c.HTML()
				if c.URL() != url || c.Text(".vk-datatable__count") != count || html != s.HTML() {
					t.Fatalf("at %q %q, want %q %q\n%s", c.URL(), c.Text(".vk-datatable__count"), url, count, html)
				}
				var got []string
				for _, o := range order {
					if !strings.Contains(html, ">"+o+"<") {
						t.Fatalf("at %q: no row %q\n%s", url, o, html)
					}
					got = append(got, o)
				}
				for i := 1; i < len(got); i++ {
					if strings.Index(html, ">"+got[i-1]+"<") > strings.Index(html, ">"+got[i]+"<") {
						t.Fatalf("at %q: rows not in order %v\n%s", url, order, html)
					}
				}
			}
			if !c.Exists("#records.vk-datatable") {
				t.Fatalf("the root carries the region id\n%s", c.HTML())
			}
			check("/books?x=1", "Showing 1–2 of 5Rows per page 25", "Dune", "Emma")

			// A sortable header is a link: ascending, then descending.
			if err := c.Click(`a.vk-sort`); err != nil {
				t.Fatal(err)
			}
			check("/books?sort=title&x=1", "Showing 1–2 of 5Rows per page 25", "Beloved", "Dune")
			if got, _ := c.Attr("a.vk-sort", "aria-sort"); got != "ascending" {
				t.Fatalf("aria-sort %q", got)
			}
			if err := c.Click(`a.vk-sort`); err != nil {
				t.Fatal(err)
			}
			check("/books?dir=desc&sort=title&x=1", "Showing 1–2 of 5Rows per page 25", "Ulysses", "Persuasion")

			// The pager's links.
			if err := c.Click(`a[aria-label="Page 3"]`); err != nil {
				t.Fatal(err)
			}
			check("/books?dir=desc&page=3&sort=title&x=1", "Showing 5–5 of 5Rows per page 25", "Beloved")

			// Typing searches, back on the first page.
			if err := c.Fire(".vk-datatable__search", ui.Event{Type: "input", Value: "aus"}); err != nil {
				t.Fatal(err)
			}
			check("/books?dir=desc&q=aus&sort=title&x=1", "Showing 1–2 of 2 (filtered from 5)Rows per page 25", "Persuasion", "Emma")
			if got, _ := c.Attr(".vk-datatable__search", "value"); got != "aus" {
				t.Fatalf("search box %q", got)
			}

			// Nothing found: the clear link drops the search.
			if err := c.Fire(".vk-datatable__search", ui.Event{Type: "input", Value: "nobody"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(c.Text(".vk-table__empty"), "Nothing matches “nobody”") {
				t.Fatalf("no match: %s", c.HTML())
			}
			if err := c.Click(".vk-datatable__clear"); err != nil {
				t.Fatal(err)
			}
			check("/books?dir=desc&sort=title&x=1", "Showing 1–2 of 5Rows per page 25", "Ulysses")

			// The page size.
			if err := c.Fire(".vk-datatable__size select", ui.Event{Type: "change", Value: "5"}); err != nil {
				t.Fatal(err)
			}
			check("/books?dir=desc&size=5&sort=title&x=1", "Showing 1–5 of 5Rows per page 25", "Ulysses", "Persuasion", "Emma", "Dune", "Beloved")

			// A region reload calls Load again.
			before := calls
			if err := c.Click("#reload"); err != nil {
				t.Fatal(err)
			}
			if calls != before+1 {
				t.Fatalf("Load calls %d after a reload, want %d", calls, before+1)
			}

			// A row's menu item.
			if err := c.Click(`#records-menu-3 .vk-menu__item`); err != nil {
				t.Fatal(err)
			}
			if deleted != 3 {
				t.Fatalf("deleted %d, want 3", deleted)
			}
			if got, _ := c.Attr(`#records-menu-3 .vk-menu__item`, "popovertarget"); got != "records-menu-3" {
				t.Fatalf("an item closes its menu: popovertarget %q", got)
			}
		})
	}
}

// TestDataTableRowsKeyed: Rows paged on the server, its parameters under
// Key, beside a second table; a URL asking for a page past the last shows
// the last; a sort key Sorts doesn't order by stays plain.
func TestDataTableRowsKeyed(t *testing.T) {
	render := func() ui.Node {
		return ui.El("main", nil,
			ui.DataTable[shelfBook]{
				ID: "a", Key: "a", Columns: shelfColumns(), Rows: shelf, Sizes: []int{2},
				SearchBy: func(b shelfBook) string { return b.Author },
				Sorts:    map[string]func(shelfBook) any{"title": func(b shelfBook) any { return b.Title }},
			},
			ui.DataTable[shelfBook]{ID: "b", Key: "b", Columns: shelfColumns(), Rows: shelf, Sizes: []int{2}})
	}
	s, c := urlSession(t, 2, "/p?a.page=9&b.page=2", render)
	if got := c.Text("#a .vk-datatable__count"); got != "Showing 5–5 of 5Rows per page 2" {
		t.Fatalf("a: %q", got)
	}
	if got := c.Text("#b .vk-datatable__count"); got != "Showing 3–4 of 5Rows per page 2" {
		t.Fatalf("b: %q", got)
	}
	if c.Exists("#b a.vk-sort") || c.Exists("#b .vk-datatable__search") {
		t.Fatalf("b has no Sorts or SearchBy: no sort link, no search\n%s", c.HTML())
	}
	if err := c.Click("#a a.vk-sort"); err != nil {
		t.Fatal(err)
	}
	if c.URL() != "/p?a.sort=title&b.page=2" || c.HTML() != s.HTML() {
		t.Fatalf("at %q\n%s", c.URL(), c.HTML())
	}
	if got := c.Text("#a tbody"); !strings.HasPrefix(got, "BelovedMorrisonDune") {
		t.Fatalf("a sorted: %q", got)
	}
	if err := c.Fire("#a .vk-datatable__search", ui.Event{Type: "input", Value: "austen"}); err != nil {
		t.Fatal(err)
	}
	if c.URL() != "/p?a.q=austen&a.sort=title&b.page=2" || c.Text("#a tbody") != "EmmaAustenPersuasionAusten" {
		t.Fatalf("at %q: %q", c.URL(), c.Text("#a tbody"))
	}
	if got := c.Text("#b .vk-datatable__count"); got != "Showing 3–4 of 5Rows per page 2" {
		t.Fatalf("b moved: %q", got)
	}
}

// TestDataTableLoadError: a failing Load renders the table's error state,
// not the page's.
func TestDataTableLoadError(t *testing.T) {
	render := func() ui.Node {
		return ui.El("main", nil, ui.DataTable[shelfBook]{Columns: shelfColumns(), Load: func(context.Context, ui.TableQuery) (ui.Page[shelfBook], error) {
			return ui.Page[shelfBook]{}, errors.New("db: down")
		}})
	}
	_, c := urlSession(t, 2, "/p", render)
	if got := c.Text(`#records [role="alert"]`); !strings.Contains(got, "couldn’t be loaded") || strings.Contains(c.HTML(), "db: down") {
		t.Fatalf("error state %q\n%s", got, c.HTML())
	}
}

// TestDataTableClient: Client renders every row marked for the browser,
// with the text it is found by, and the controls the runtime drives.
func TestDataTableClient(t *testing.T) {
	render := func() ui.Node {
		return ui.El("main", nil, ui.DataTable[shelfBook]{Columns: shelfColumns(), Rows: shelf, Client: true, Sizes: []int{2, 5},
			SearchBy: func(b shelfBook) string { return b.Title + " " + b.Author }})
	}
	_, c := urlSession(t, 2, "/p?page=2", render)
	html := c.HTML()
	for _, want := range []string{`data-vk-table="client"`, `data-vk-text="beloved morrison"`, `data-vk-table-search`, `data-vk-table-size`, `data-vk-table-go="next"`, `data-vk-table-empty="match"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("no %s\n%s", want, html)
		}
	}
	if n := strings.Count(html, "data-vk-table-row"); n != len(shelf) {
		t.Fatalf("%d rows rendered, want every row", n)
	}
	if got := c.Text("[data-vk-table-pages]"); got != "3" {
		t.Fatalf("pages %q", got)
	}
}
