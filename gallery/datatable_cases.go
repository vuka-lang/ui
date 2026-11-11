package gallery

import (
	"context"
	"errors"
	"io"
	"net/url"

	"github.com/vuka-lang/ui/v2"
)

type atURL struct {
	u string
	n ui.Node
}

func (a atURL) Render(ctx context.Context, w io.Writer) error {
	u, _ := url.Parse(a.u)
	return a.n.Render(ui.WithPageURL(ctx, u, nil), w)
}

// dataTableCases are a DataTable's states, kept in the URL with Load.
func dataTableCases() []Case {
	cols := []ui.Column[book]{
		{Header: ui.Text("Title"), Sort: "title", Cell: func(b book) ui.Node { return ui.Text(b.Title) }},
		{Header: ui.Text("Author"), Sort: "author", Cell: func(b book) ui.Node { return ui.Text(b.Author) }},
		{Header: ui.Text("ID"), Align: "right", Cell: func(b book) ui.Node { return ui.Text(b.ID) }},
	}
	load := func(total int, err error) func(context.Context, ui.TableQuery) (ui.Page[book], error) {
		return func(_ context.Context, q ui.TableQuery) (ui.Page[book], error) {
			if total == 0 {
				return ui.Page[book]{}, err
			}
			return ui.Page[book]{Items: books, Total: total, All: 42}, err
		}
	}
	actions := func(b book) ui.Node {
		return ui.Fragment(ui.ActionMenu{Icon: "file-text", Href: "/books/1", Children: ui.Text("Open")},
			ui.ActionMenu{Icon: "trash", Danger: true, On: ui.JS.Hide("#x"), Children: ui.Text("Delete")})
	}
	table := func(at string, d ui.DataTable[book]) func() ui.Node {
		d.Columns, d.ID = cols, "t"
		return func() ui.Node { return atURL{at, d} }
	}
	return []Case{
		{"datatable/load", table("/books", ui.DataTable[book]{Load: load(42, nil), Actions: actions, Toolbar: ui.Button{Children: ui.Text("New book")}})},
		{"datatable/load/empty", table("/books", ui.DataTable[book]{Load: load(0, nil), EmptyIcon: ui.Icon("list"), Empty: ui.Text("No books yet.")})},
		{"datatable/load/loading", table("/books", ui.DataTable[book]{Load: load(42, nil), Loading: true})},
		{"datatable/load/error", table("/books", ui.DataTable[book]{Load: load(0, errors.New("down"))})},
		{"datatable/load/sorted", table("/books?sort=title&dir=desc", ui.DataTable[book]{Load: load(42, nil)})},
		{"datatable/load/paged", table("/books?page=3", ui.DataTable[book]{Load: load(42, nil), Index: true})},
		{"datatable/load/searched", table("/books?q=aus", ui.DataTable[book]{Load: load(3, nil)})},
		{"datatable/load/no-match", table("/books?q=zzz", ui.DataTable[book]{Load: load(0, nil)})},
		{"datatable/client", table("/books", ui.DataTable[book]{Rows: books, Client: true, SearchBy: func(b book) string { return b.Title }})},
		{"actionmenu", func() ui.Node {
			items := actions(book{})
			return ui.HTML(`<div class="vk-menu__panel" role="menu">{items}</div>`, items)
		}},
	}
}
