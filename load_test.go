package ui_test

import (
	"context"
	"slices"
	"testing"

	"github.com/vuka-lang/orm/v2"
	"github.com/vuka-lang/orm/v2/ormtest"
	"github.com/vuka-lang/ui/v2"
)

type Book struct {
	ID     int64 `orm:"pk"`
	Title  string
	Author string
	Pages  int
}

var Books = orm.For[Book]()

func books(t *testing.T) context.Context {
	t.Helper()
	ctx := ormtest.Open(t, Books)
	ormtest.Seed(t, ctx, Books,
		&Book{Title: "Dune", Author: "Herbert", Pages: 412},
		&Book{Title: "Emma", Author: "Austen", Pages: 474},
		&Book{Title: "Ulysses", Author: "Joyce", Pages: 730},
		&Book{Title: "Persuasion", Author: "Austen", Pages: 249},
		&Book{Title: "Beloved", Author: "Morrison", Pages: 324},
	)
	return ctx
}

func titles(p ui.Page[Book]) []string {
	var out []string
	for _, b := range p.Items {
		out = append(out, b.Title)
	}
	return out
}

var byAuthor = func(qs orm.QuerySet[Book], term string) orm.QuerySet[Book] {
	return qs.Filter(orm.Q{"author__icontains": term})
}

func TestLoadPage(t *testing.T) {
	ctx := books(t)
	src := ui.Source[Book]{
		Search:  byAuthor,
		Sorts:   map[string]string{"title": "title", "length": "pages"},
		Default: "-pages",
	}
	for _, c := range []struct {
		name        string
		q           ui.TableQuery
		want        []string
		total, all  int
		page, pages int
	}{
		{"default order", ui.TableQuery{Page: 1, Size: 2}, []string{"Ulysses", "Emma"}, 5, 5, 1, 3},
		{"page cut", ui.TableQuery{Page: 3, Size: 2}, []string{"Persuasion"}, 5, 5, 3, 3},
		{"past the last page", ui.TableQuery{Page: 9, Size: 2}, []string{"Persuasion"}, 5, 5, 3, 3},
		{"sorted", ui.TableQuery{Page: 1, Size: 3, Sort: "title"}, []string{"Beloved", "Dune", "Emma"}, 5, 5, 1, 2},
		{"sorted descending by a mapped field", ui.TableQuery{Page: 1, Size: 2, Sort: "length", Desc: true}, []string{"Ulysses", "Emma"}, 5, 5, 1, 3},
		{"an unlisted key is ignored", ui.TableQuery{Page: 1, Size: 2, Sort: "author", Sortable: []string{"author"}}, []string{"Ulysses", "Emma"}, 5, 5, 1, 3},
		{"searched", ui.TableQuery{Page: 1, Size: 10, Search: "AUST", Sort: "title"}, []string{"Emma", "Persuasion"}, 2, 5, 1, 1},
		{"nothing found", ui.TableQuery{Page: 1, Size: 10, Search: "nobody"}, nil, 0, 5, 1, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := ui.LoadPage(ctx, c.q, src)
			if err != nil {
				t.Fatal(err)
			}
			if got := titles(p); !slices.Equal(got, c.want) || p.Total != c.total || p.All != c.all || p.Page != c.page || p.Pages != c.pages {
				t.Fatalf("got %v total %d all %d page %d/%d, want %v total %d all %d page %d/%d", got, p.Total, p.All, p.Page, p.Pages, c.want, c.total, c.all, c.page, c.pages)
			}
		})
	}
}

// Without Sorts, a key the table's columns declare is the field's name;
// one they don't is ignored. Without Search the term is ignored. Rows
// narrows the set.
func TestLoadPageKeyOnce(t *testing.T) {
	ctx := books(t)
	austen := Books.Filter(orm.Q{"author": "Austen"})
	for _, c := range []struct {
		name string
		q    ui.TableQuery
		src  ui.Source[Book]
		want []string
	}{
		{"column key", ui.TableQuery{Page: 1, Size: 2, Sort: "title", Sortable: []string{"title"}}, ui.Source[Book]{}, []string{"Beloved", "Dune"}},
		{"undeclared key", ui.TableQuery{Page: 1, Size: 5, Sort: "title", Desc: true, Sortable: []string{"pages"}}, ui.Source[Book]{Default: "pages"}, []string{"Persuasion", "Beloved", "Dune", "Emma", "Ulysses"}},
		{"no Search", ui.TableQuery{Page: 1, Size: 5, Search: "Austen"}, ui.Source[Book]{Default: "id"}, []string{"Dune", "Emma", "Ulysses", "Persuasion", "Beloved"}},
		{"Rows", ui.TableQuery{Page: 1, Size: 5, Sort: "title", Desc: true, Sortable: []string{"title"}}, ui.Source[Book]{Rows: &austen}, []string{"Persuasion", "Emma"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := ui.LoadPage(ctx, c.q, c.src)
			if err != nil {
				t.Fatal(err)
			}
			if got := titles(p); !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

// A database error is LoadPage's.
func TestLoadPageError(t *testing.T) {
	if _, err := ui.LoadPage(context.Background(), ui.TableQuery{Page: 1, Size: 10}, ui.Source[Book]{}); err == nil {
		t.Fatal("no database: want an error")
	}
}
