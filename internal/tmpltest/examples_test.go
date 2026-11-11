package tmpltest_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/vuka-lang/orm/v2"
	"github.com/vuka-lang/orm/v2/ormtest"
	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/internal/tmpltest"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// TestDataTableExample renders the DataTable and ActionMenu examples
// (books.vuka.html, row_actions.vuka.html) on a live page over a database:
// the page the URL asks for, the row menu's handler, the reload button.
func TestDataTableExample(t *testing.T) {
	books := orm.Objects[tmpltest.Book]()
	ctx := ormtest.Open(t, books)
	ormtest.Seed(t, ctx, books,
		&tmpltest.Book{Title: "Emma", Author: "Austen", Pages: 474},
		&tmpltest.Book{Title: "Dune", Author: "Herbert", Pages: 412},
		&tmpltest.Book{Title: "Persuasion", Author: "Austen", Pages: 249},
	)
	u, _ := url.Parse("/books?q=austen&sort=pages&dir=desc")
	s := live.NewSession(ui.WithPageURL(ctx, u, nil), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "books", Props: &tmpltest.Books{}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Text("#records tbody"); !strings.HasPrefix(got, "EmmaAusten474") || !strings.Contains(got, "Persuasion") || strings.Contains(got, "Dune") {
		t.Fatalf("rows %q\n%s", got, c.HTML())
	}
	if got := c.Text(".vk-datatable__count"); !strings.HasPrefix(got, "Showing 1–2 of 2 (filtered from 3)") {
		t.Fatalf("count %q", got)
	}
	if got, _ := c.Attr(`#records-menu-1 a.vk-menu__item`, "href"); got != "/books/1" {
		t.Fatalf("Open links to %q\n%s", got, c.HTML())
	}
	if err := c.Click(`#records-menu-2 button.vk-menu__item`); err != nil {
		t.Fatal(err)
	}
	if got := c.Text(".deleted"); got != "Deleted book 3" {
		t.Fatalf("after Delete: %q", got)
	}
	if err := c.Click(`section > button`); err != nil {
		t.Fatal(err)
	}
}
