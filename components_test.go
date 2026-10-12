package ui_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/live/livetest"
)

func on(ch ui.JSChain) ui.Attr { return ui.Attr{Name: "onClick", Value: ui.On(ch)} }
func onh(fn any) ui.Attr       { return ui.Attr{Name: "onClick", Value: ui.On(fn)} }
func txt(s string) ui.Node     { return ui.Text(s) }
func root(id string) string    { return `[data-vk-id="` + id + `"] ` }

func join(t *testing.T, v int, render func(context.Context) ui.Node) *livetest.Client {
	t.Helper()
	c, err := livetest.Join(live.NewSession(context.Background(), render), v)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// --- Page[T] ---------------------------------------------------------------

func TestNewPage(t *testing.T) {
	p := ui.NewPage([]int{1, 2, 3, 4, 5}, 42, 1, 5)
	if p.Total != 42 || p.Pages != 9 || p.Page != 1 || p.HasPrev || !p.HasNext {
		t.Fatalf("page 1: %+v", p)
	}
	if p.From() != 1 || p.To() != 5 {
		t.Fatalf("range: %d-%d", p.From(), p.To())
	}
	last := ui.NewPage([]int{41, 42}, 42, 99, 5) // clamped to the last page
	if last.Page != 9 || last.HasNext || !last.HasPrev {
		t.Fatalf("clamp: %+v", last)
	}
	empty := ui.NewPage([]int{}, 0, 1, 10)
	if !empty.Empty() || empty.Pages != 0 || empty.HasNext || empty.From() != 0 || empty.To() != 0 {
		t.Fatalf("empty: %+v", empty)
	}
}

func TestPageWindow(t *testing.T) {
	p := ui.NewPage([]int{}, 200, 7, 10) // 20 pages, on page 7
	if got := p.Window(5); !eqInts(got, []int{5, 6, 7, 8, 9}) {
		t.Fatalf("mid window: %v", got)
	}
	first := ui.NewPage([]int{}, 200, 1, 10)
	if got := first.Window(5); !eqInts(got, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("start window: %v", got)
	}
	if got := ui.NewPage([]int{}, 20, 1, 10).Window(5); !eqInts(got, []int{1, 2}) {
		t.Fatalf("short window: %v", got)
	}
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Modal: browser-side open/close ---------------------------------------

type modalHost struct{ ui.Live }

func (m *modalHost) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{{Name: "id", Value: "open"}, on(ui.OpenModal("edit"))}, txt("Open")),
		ui.Modal(ui.ModalProps{Ref: "edit", Title: txt("Edit"), Children: txt("body")}),
	)
}

func TestModalOpenClose(t *testing.T) {
	for _, v := range []int{1, 2} {
		c := join(t, v, func(context.Context) ui.Node { return ui.Component("m#1", nil, &modalHost{}) })
		rootSel := root("c1") + ".vk-modal-root"
		if c.Visible(rootSel) {
			t.Fatalf("v%d: modal shows before opening", v)
		}
		if err := c.Click(root("c1") + "#open"); err != nil {
			t.Fatal(err)
		}
		if !c.Visible(rootSel) {
			t.Fatalf("v%d: open did not show the modal: %s", v, c.HTML())
		}
		if err := c.Click(root("c1") + ".vk-modal__close"); err != nil {
			t.Fatal(err)
		}
		if c.Visible(rootSel) {
			t.Fatalf("v%d: close did not hide the modal", v)
		}
		if _, ok := c.Attr(root("c1")+".vk-modal", "aria-modal"); !ok {
			t.Fatalf("v%d: dialog missing aria-modal", v)
		}
	}
}

// --- Modal: server-owned open state ---------------------------------------

type liveModalHost struct {
	ui.Live
	open ui.Assign[bool]
}

func (m *liveModalHost) Open()  { m.open.Set(true) }
func (m *liveModalHost) Close() { m.open.Set(false) }
func (m *liveModalHost) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{{Name: "id", Value: "open"}, onh(m.Open)}, txt("Open")),
		ui.Modal(ui.ModalProps{Ref: "edit", Open: m.open.Get(), OnClose: m.Close, Children: txt("body")}),
	)
}

func TestModalServerOwned(t *testing.T) {
	for _, v := range []int{1, 2} {
		c := join(t, v, func(context.Context) ui.Node { return ui.Component("m#1", nil, &liveModalHost{}) })
		rootSel := root("c1") + ".vk-modal-root"
		if c.Visible(rootSel) {
			t.Fatalf("v%d: shows before the server opens it", v)
		}
		if err := c.Click(root("c1") + "#open"); err != nil {
			t.Fatal(err)
		}
		if !c.Visible(rootSel) {
			t.Fatalf("v%d: server open did not show it: %s", v, c.HTML())
		}
		if err := c.Click(root("c1") + ".vk-modal__close"); err != nil {
			t.Fatal(err)
		}
		if c.Visible(rootSel) {
			t.Fatalf("v%d: server close did not hide it", v)
		}
	}
}

// --- Alert ----------------------------------------------------------------

type alertHost struct{ ui.Live }

func (a *alertHost) Render() ui.Node {
	return ui.El("div", nil, ui.Alert(ui.AlertProps{Variant: ui.AlertSuccess, Title: "Saved", Dismissible: true, Children: txt("All good")}))
}

func TestAlertDismiss(t *testing.T) {
	for _, v := range []int{1, 2} {
		c := join(t, v, func(context.Context) ui.Node { return ui.Component("a#1", nil, &alertHost{}) })
		if !c.HasClass(root("c1")+".vk-alert", "vk-alert--success") {
			t.Fatalf("v%d: variant class missing: %s", v, c.HTML())
		}
		if !c.Visible(root("c1") + ".vk-alert") {
			t.Fatalf("v%d: alert hidden initially", v)
		}
		if err := c.Click(root("c1") + ".vk-alert__close"); err != nil {
			t.Fatal(err)
		}
		if c.Visible(root("c1") + ".vk-alert") {
			t.Fatalf("v%d: dismiss did not hide the alert", v)
		}
	}
}

// --- Toast ----------------------------------------------------------------

type toastHost struct{ ui.Live }

func (h *toastHost) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{{Name: "id", Value: "show"}, on(ui.ShowToast("saved"))}, txt("Notify")),
		ui.ToastContainer(ui.ToastContainerProps{Children: ui.Fragment(
			styleHidden(ui.Toast(ui.ToastProps{ID: "saved", Variant: ui.AlertSuccess, Title: "Saved", Children: txt("Done")})),
		)}),
	)
}

// styleHidden renders a toast hidden until ShowToast reveals it.
func styleHidden(n ui.Node) ui.Node {
	return ui.El("div", []ui.Attr{{Name: "style", Value: ui.Style{"display": "none"}}, {Name: "ref", Value: "saved"}}, n)
}

func TestToastShowDismiss(t *testing.T) {
	for _, v := range []int{1, 2} {
		c := join(t, v, func(context.Context) ui.Node { return ui.Component("t#1", nil, &toastHost{}) })
		wrap := root("c1") + `[data-vk-ref="saved"]`
		if c.Visible(wrap) {
			t.Fatalf("v%d: toast shows before pushing", v)
		}
		if err := c.Click(root("c1") + "#show"); err != nil {
			t.Fatal(err)
		}
		if !c.Visible(wrap) {
			t.Fatalf("v%d: show did not reveal the toast: %s", v, c.HTML())
		}
		if err := c.Click(root("c1") + ".vk-toast__close"); err != nil {
			t.Fatal(err)
		}
		if c.Visible(root("c1") + ".vk-toast") {
			t.Fatalf("v%d: dismiss did not hide the toast", v)
		}
	}
}

// --- Table (presentational) -----------------------------------------------

func TestTableRenders(t *testing.T) {
	node := ui.Table(ui.TableProps[book]{
		Columns: []ui.Column[book]{
			{Header: txt("Title"), Cell: func(b book) ui.Node { return txt(b.Title) }},
			{Header: txt("Author"), Cell: func(b book) ui.Node { return txt(b.Author) }, Align: "right"},
		},
		Rows:   catalog[:2],
		RowKey: func(b book) any { return b.ID },
	})
	html, err := ui.String(context.Background(), node)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<table", "<th", "Title", "Author", "vk-col--right", catalog[0].Title, catalog[1].Author} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
	empty, _ := ui.String(context.Background(), ui.Table(ui.TableProps[book]{
		Columns: []ui.Column[book]{{Header: txt("Title"), Cell: func(b book) ui.Node { return txt(b.Title) }}},
	}))
	if !strings.Contains(empty, "vk-table__empty") {
		t.Fatalf("no empty state: %s", empty)
	}
}

// --- DataTable: paging, search, sort over a live view ---------------------

type book struct {
	ID            int
	Title, Author string
}

var catalog = []book{
	{1, "Things Fall Apart", "Achebe"}, {2, "Arrow of God", "Achebe"},
	{3, "Petals of Blood", "Ngugi"}, {4, "Weep Not, Child", "Ngugi"},
	{5, "Americanah", "Adichie"}, {6, "Half of a Yellow Sun", "Adichie"},
	{7, "Paradise", "Gurnah"}, {8, "By the Sea", "Gurnah"},
	{9, "Afterlives", "Gurnah"}, {10, "Disgrace", "Coetzee"},
	{11, "Nervous Conditions", "Dangarembga"}, {12, "So Long a Letter", "Ba"},
}

const pageSize = 5

type booksHost struct {
	ui.Live
	page  ui.Assign[int]
	query ui.Assign[string]
	sortK ui.Assign[string]
	dir   ui.Assign[string]
}

func (h *booksHost) Mount()          { h.page.Set(1) }
func (h *booksHost) GoTo(n int)      { h.page.Set(n) }
func (h *booksHost) Search(q string) { h.query.Set(q); h.page.Set(1) }
func (h *booksHost) SortBy(k string) {
	if h.sortK.Get() == k && h.dir.Get() == "asc" {
		h.dir.Set("desc")
	} else {
		h.sortK.Set(k)
		h.dir.Set("asc")
	}
	h.page.Set(1)
}

func (h *booksHost) rows() []book {
	var out []book
	q := strings.ToLower(h.query.Get())
	for _, b := range catalog {
		if q == "" || strings.Contains(strings.ToLower(b.Title), q) || strings.Contains(strings.ToLower(b.Author), q) {
			out = append(out, b)
		}
	}
	if k := h.sortK.Get(); k != "" {
		sort.SliceStable(out, func(i, j int) bool {
			less := out[i].Title < out[j].Title
			if k == "author" {
				less = out[i].Author < out[j].Author
			}
			if h.dir.Get() == "desc" {
				return !less
			}
			return less
		})
	}
	return out
}

func (h *booksHost) Render() ui.Node {
	rows := h.rows()
	total := len(rows)
	page := h.page.Get()
	if page < 1 {
		page = 1
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pg := ui.NewPage(rows[start:end], total, page, pageSize)
	return ui.DataTable(ui.DataTableProps[book]{
		Columns: []ui.Column[book]{
			{Header: txt("Title"), Cell: func(b book) ui.Node { return txt(b.Title) }, Sort: "title"},
			{Header: txt("Author"), Cell: func(b book) ui.Node { return txt(b.Author) }, Sort: "author"},
		},
		Page:     pg,
		Search:   h.query.Get(),
		Sort:     h.sortK.Get(),
		SortDir:  h.dir.Get(),
		OnPage:   h.GoTo,
		OnSearch: h.Search,
		OnSort:   h.SortBy,
		RowKey:   func(b book) any { return b.ID },
	})
}

func TestDataTablePagingSearchSort(t *testing.T) {
	for _, v := range []int{1, 2} {
		c := join(t, v, func(context.Context) ui.Node { return ui.Component("b#1", nil, &booksHost{}) })
		r := root("c1")

		// Page 1 shows the first five rows and a pager.
		if n := strings.Count(c.HTML(), "<tr"); n < 5 {
			t.Fatalf("v%d: expected rows, got %d: %s", v, n, c.HTML())
		}
		if !strings.Contains(c.Text(r+".vk-datatable__count"), "of 12") {
			t.Fatalf("v%d: count wrong: %q", v, c.Text(r+".vk-datatable__count"))
		}

		// Next page.
		if err := c.Click(r + `button[aria-label="Next page"]`); err != nil {
			t.Fatal(err)
		}
		if got, _ := c.Attr(r+`button[aria-label="Page 2"]`, "aria-current"); got != "page" {
			t.Fatalf("v%d: page 2 not current: %q", v, got)
		}

		// Jump to a numbered page.
		if err := c.Click(r + `button[aria-label="Page 3"]`); err != nil {
			t.Fatal(err)
		}
		if got, _ := c.Attr(r+`button[aria-label="Page 3"]`, "aria-current"); got != "page" {
			t.Fatalf("v%d: page 3 not current", v)
		}

		// Sort by Title (the first sort control) on the unfiltered list; this
		// also resets to page 1.
		if err := c.Click(r + ".vk-sort"); err != nil {
			t.Fatal(err)
		}
		if got, _ := c.Attr(r+".vk-sort", "aria-sort"); got != "ascending" {
			t.Fatalf("v%d: title not ascending: %q", v, got)
		}
		if before(c.HTML(), "Afterlives", "Americanah") != true {
			t.Fatalf("v%d: ascending order wrong: %s", v, bodyOf(c.HTML()))
		}
		// A second click flips to descending.
		if err := c.Click(r + ".vk-sort"); err != nil {
			t.Fatal(err)
		}
		if got, _ := c.Attr(r+".vk-sort", "aria-sort"); got != "descending" {
			t.Fatalf("v%d: title not descending: %q", v, got)
		}
		if !before(c.HTML(), "Weep Not, Child", "Things Fall Apart") {
			t.Fatalf("v%d: descending order wrong: %s", v, bodyOf(c.HTML()))
		}

		// Search filters and resets to page 1.
		if err := c.Fire(r+`input[name="q"]`, ui.Event{Type: "input", Value: "achebe"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.Text(r+".vk-datatable__count"), "of 2") {
			t.Fatalf("v%d: search did not filter to 2: %q (%s)", v, c.Text(r+".vk-datatable__count"), c.HTML())
		}
	}
}

// before reports whether a appears before b in s.
func before(s, a, b string) bool {
	ia, ib := strings.Index(s, a), strings.Index(s, b)
	return ia >= 0 && ib >= 0 && ia < ib
}

func bodyOf(html string) string {
	if i := strings.Index(html, "<tbody"); i >= 0 {
		return html[i:]
	}
	return html
}
