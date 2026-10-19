package ui_test

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui"
)

var updateKit = flag.Bool("update-kit", false, "rewrite testdata/kit.golden")

type kitCase struct {
	name string
	node func() ui.Node
}

func kitPick(string) {}
func kitPage(int)    {}
func kitSel(bool)    {}
func kitSave()       {}

var kitBooks = []book{{1, "Dune", "Herbert"}, {2, "Emma", "Austen"}, {3, "Ulysses & Co", "Joyce"}}

func kitCols() []ui.Column[book] {
	return []ui.Column[book]{
		{Header: ui.Text("ID"), Cell: func(b book) ui.Node { return ui.Text(b.ID) }, Align: "right", Sort: "id"},
		{Header: ui.Text("Title"), Cell: func(b book) ui.Node { return ui.Text(b.Title) }, Class: "w-1/2", Sort: "title"},
		{Header: ui.Text("Center"), Align: "center", Class: "x"},
		{Header: ui.Text("Plain"), Cell: func(b book) ui.Node { return ui.Text("·") }},
	}
}

func kitCases() []kitCase {
	btn := func(p ui.ButtonProps) func() ui.Node { return func() ui.Node { return ui.Button(p) } }
	field := func(f func(ui.FieldProps) ui.Node, p ui.FieldProps) func() ui.Node {
		return func() ui.Node { return f(p) }
	}
	cs := []kitCase{
		{"actions", func() ui.Node {
			return ui.Actions(ui.Cancel(ui.CloseModal("m")), ui.Button(ui.ButtonProps{Variant: ui.Primary, Children: ui.Text("Save")}))
		}},
		{"actions/empty", func() ui.Node { return ui.Actions() }},
		{"cancel/label", func() ui.Node { return ui.Cancel(kitSave, "Close") }},
		{"cancel/empty-label", func() ui.Node { return ui.Cancel(ui.CloseModal("m"), "") }},

		{"alert", func() ui.Node { return ui.Alert(ui.AlertProps{Children: ui.Text("hi")}) }},
		{"alert/success-title", func() ui.Node {
			return ui.Alert(ui.AlertProps{Variant: ui.AlertSuccess, Title: "Saved", Children: ui.Text("live")})
		}},
		{"alert/warning-dismiss-class", func() ui.Node {
			return ui.Alert(ui.AlertProps{Variant: ui.AlertWarning, Dismissible: true, Class: "mt-2", Children: ui.Text("careful")})
		}},
		{"alert/error-nochildren", func() ui.Node { return ui.Alert(ui.AlertProps{Variant: ui.AlertError, Title: "Oops"}) }},

		{"badge", func() ui.Node { return ui.Badge(ui.BadgeProps{Children: ui.Text("New")}) }},
		{"badge/success", func() ui.Node { return ui.Badge(ui.BadgeProps{Variant: ui.Success, Children: ui.Text("Signed")}) }},
		{"badge/danger-soft-class", func() ui.Node {
			return ui.Badge(ui.BadgeProps{Variant: ui.Danger, Soft: true, Class: "ml-1", Children: ui.Text("Failed")})
		}},

		{"button", btn(ui.ButtonProps{Children: ui.Text("Go")})},
		{"button/primary-sm-icon", btn(ui.ButtonProps{Variant: ui.Primary, Size: ui.SizeSm, Icon: ui.Icon("plus"), Children: ui.Text("New")})},
		{"button/lg-ghost", btn(ui.ButtonProps{Variant: ui.Ghost, Size: ui.SizeLg, Children: ui.Text("Ghost")})},
		{"button/icon-only", btn(ui.ButtonProps{IconOnly: true, Icon: ui.Icon("x"), Title: "Close", Children: ui.Text("Close")})},
		{"button/icon-only-nolabel", btn(ui.ButtonProps{IconOnly: true, Icon: ui.Icon("x")})},
		{"button/loading", btn(ui.ButtonProps{Loading: true, Icon: ui.Icon("plus"), Children: ui.Text("Wait")})},
		{"button/disabled", btn(ui.ButtonProps{Disabled: true, Children: ui.Text("No")})},
		{"button/submit-form", btn(ui.ButtonProps{Type: "submit", Form: "f1", ID: "b1", Title: "Send", Class: "w-full", Children: ui.Text("Send")})},
		{"button/reset", btn(ui.ButtonProps{Type: "reset", Variant: ui.Danger, Children: ui.Text("Reset")})},
		{"button/js", btn(ui.ButtonProps{On: ui.OpenModal("edit"), Variant: ui.Info, Children: ui.Text("Edit")})},
		{"button/handler", btn(ui.ButtonProps{On: kitSave, Variant: ui.Success, Children: ui.Text("Save")})},
		{"button/attrs", btn(ui.ButtonProps{Attrs: []ui.Attr{{Name: "data-x", Value: "1"}, {Name: "aria-label", Value: "X"}}, Variant: ui.Warning, Children: ui.Text("X")})},
		{"button/href", btn(ui.ButtonProps{Href: "/back", Children: ui.Text("Back")})},
		{"button/href-all", btn(ui.ButtonProps{Href: "/books", Nav: true, ID: "l1", Title: "Books", Loading: true, Variant: ui.Primary, Size: ui.SizeSm, Class: "c",
			Attrs: []ui.Attr{{Name: "data-y", Value: "2"}}, Children: ui.Text("Books")})},
		{"button/href-disabled-icon", btn(ui.ButtonProps{Href: "/x", Disabled: true, Icon: ui.Icon("check"), Children: ui.Text("X")})},
		{"button/nochildren", btn(ui.ButtonProps{})},
		{"submit", func() ui.Node { return ui.Submit(ui.ButtonProps{Children: ui.Text("Save")}) }},
		{"submit/variant-loading", func() ui.Node {
			return ui.Submit(ui.ButtonProps{Variant: ui.Danger, Loading: true, Form: "f", Children: ui.Text("Delete")})
		}},
		{"spinner", ui.Spinner},

		{"card", func() ui.Node { return ui.Card(ui.CardProps{Children: ui.Text("body")}) }},
		{"card/title", func() ui.Node { return ui.Card(ui.CardProps{Title: ui.Text("Members"), Children: ui.Text("body")}) }},
		{"card/header", func() ui.Node {
			return ui.Card(ui.CardProps{Header: ui.Badge(ui.BadgeProps{Children: ui.Text("3")}), Children: ui.Text("body")})
		}},
		{"card/all", func() ui.Node {
			return ui.Card(ui.CardProps{Title: ui.Text("Members"), Header: ui.Actions(ui.Button(ui.ButtonProps{Children: ui.Text("Invite")})),
				NoPad: true, Class: "shadow", Children: ui.Text("body")})
		}},
		{"card/empty", func() ui.Node { return ui.Card(ui.CardProps{}) }},

		{"icon", func() ui.Node { return ui.Icon("check") }},
		{"icon/classes", func() ui.Node { return ui.Icon("search", "a", "", "b") }},
		{"icon/unknown", func() ui.Node { return ui.Icon("nope") }},

		{"image", func() ui.Node { return ui.Image(ui.ImageProps{Src: "/a.webp", Alt: "A"}) }},
		{"image/priority-all", func() ui.Node {
			return ui.Image(ui.ImageProps{Src: "/a.webp", Srcset: "/a-1.webp 1x, /a-2.webp 2x", Sizes: "100vw", Alt: "A", Width: 1600, Height: 600,
				Class: "rounded", Priority: true})
		}},
		{"image/eager-plain", func() ui.Node {
			return ui.Image(ui.ImageProps{Src: "/logo.svg", Eager: true, Plain: true, Class: "h-8"})
		}},
		{"image/overlay", func() ui.Node {
			return ui.Image(ui.ImageProps{Src: "/hero.webp", Overlay: ui.El("h1", nil, ui.Text("Hi"))})
		}},
		{"image/overlay-scrim", func() ui.Node {
			return ui.Image(ui.ImageProps{Src: "/hero.webp", Width: 10, Scrim: true, WrapClass: "h-64", Overlay: ui.Text("Over")})
		}},

		{"modal", func() ui.Node { return ui.Modal(ui.ModalProps{Children: ui.Text("body")}) }},
		{"modal/full", func() ui.Node {
			return ui.Modal(ui.ModalProps{Ref: "approval", Icon: ui.Icon("check-circle"), Title: ui.Text("Preview"), Subtitle: "Review before signing",
				Footer: ui.Actions(ui.Cancel(ui.CloseModal("approval"))), Size: "lg", Class: "x", Children: ui.Text("body")})
		}},
		{"modal/subtitle-only-sm", func() ui.Node { return ui.Modal(ui.ModalProps{Subtitle: "Sub", Size: "sm", Children: ui.Text("b")}) }},
		{"modal/closed-xl-open", func() ui.Node {
			return ui.Modal(ui.ModalProps{Ref: "c", Closed: true, Open: true, Size: "xl", Children: ui.Text("b")})
		}},
		{"modal/closed-notitle", func() ui.Node { return ui.Modal(ui.ModalProps{Closed: true, Size: "md"}) }},
		{"modal/onclose", func() ui.Node {
			return ui.Modal(ui.ModalProps{Ref: "srv", Open: true, OnClose: kitSave, Title: ui.Text("Server"), Children: ui.Text("b")})
		}},
		{"modal/commands", func() ui.Node {
			return ui.El("div", nil, ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.OpenModal("m")}}), ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.CloseModal("m")}}))
		}},

		{"tabs/href", func() ui.Node {
			return ui.Tabs(ui.TabsProps{Tabs: []ui.Tab{
				{Label: "Placements", Href: "/tasks?type=placements", Active: true, Count: ui.Count(42)},
				{Label: "Permits", Href: "/tasks?type=permits", Patch: true, Icon: "users", ID: "t2", Count: ui.Count(1234)},
			}, Class: "mb-0"})
		}},
		{"tabs/live", func() ui.Node {
			return ui.Tabs(ui.TabsProps{Tabs: []ui.Tab{
				{Label: "A", On: func() { kitPick("a") }, Active: true, ID: "a"},
				{Label: "B", Icon: "list"},
				{Label: "C & D", On: ui.JS.Show(ui.Ref("x")), Count: ui.Count(0)},
			}})
		}},
		{"tabs/empty", func() ui.Node { return ui.Tabs(ui.TabsProps{}) }},

		{"theme/styletag", ui.StyleTag},

		{"tile", func() ui.Node {
			return ui.Tile(ui.TileProps{Label: "Applications", Icon: "file-text", Value: int64(1234567)})
		}},
		{"tile/variants", func() ui.Node {
			return ui.TileGrid(
				ui.Tile(ui.TileProps{Label: "A", Value: 5, Variant: ui.Success, Icon: "check", Class: "x"}),
				ui.Tile(ui.TileProps{Label: "B", Value: -12840}),
				ui.Tile(ui.TileProps{Label: "C", Value: uint32(1000), Variant: ui.Danger, Icon: "x"}),
				ui.Tile(ui.TileProps{Label: "D", Value: "n/a"}),
				ui.Tile(ui.TileProps{Label: "E", Value: 3.5}),
				ui.Tile(ui.TileProps{Label: "F", Value: nil}),
				ui.Tile(ui.TileProps{Label: "G", Value: uint(7), Icon: "info", Variant: ui.Info}),
				ui.Tile(ui.TileProps{Label: "H", Value: int32(-1000), Icon: "warning", Variant: ui.Warning}),
				ui.Tile(ui.TileProps{Label: "I", Value: uint64(999)}),
			)
		}},
		{"tilegrid/empty", func() ui.Node { return ui.TileGrid() }},

		{"toasts", func() ui.Node { return ui.ToastContainer(ui.ToastContainerProps{}) }},
		{"toasts/all", func() ui.Node {
			return ui.ToastContainer(ui.ToastContainerProps{Ref: "n", Position: ui.ToastBottomLeft, Class: "x", Children: ui.Fragment(
				ui.Toast(ui.ToastProps{Children: ui.Text("plain")}),
				ui.Toast(ui.ToastProps{ID: "saved", Variant: ui.AlertSuccess, Title: "Saved", Timeout: 4 * time.Second, Class: "y", Children: ui.Text("Done")}),
				ui.Toast(ui.ToastProps{Variant: ui.AlertError, Closed: true, Title: "No"}),
			)})
		}},
		{"toast/commands", func() ui.Node {
			return ui.El("div", nil,
				ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.ShowToast("t")}}),
				ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.DismissToast()}}),
				ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.DismissToastRef("t")}}))
		}},

		{"form", func() ui.Node { return ui.Form(ui.FormProps{Children: ui.Text("x")}) }},
		{"form/all", func() ui.Node {
			return ui.Form(ui.FormProps{ID: "sign", OnSubmit: func(struct{}) {}, OnChange: ui.JS.Show(ui.Ref("x")), Method: "get", Action: "/sign",
				Error: "Could not reach the provider.", CSRF: ui.El("input", []ui.Attr{{Name: "type", Value: "hidden"}, {Name: "name", Value: "csrf"}}),
				Class: "grid", Children: ui.Fragment(
					ui.Input(ui.FieldProps{Name: "username", Label: "Username", Value: "ada", Disabled: true}),
					ui.Submit(ui.ButtonProps{Children: ui.Text("Sign")}),
				)})
		}},
		{"form/nochildren", func() ui.Node { return ui.Form(ui.FormProps{}) }},
		{"field", field(ui.Field, ui.FieldProps{Name: "n", Label: "N", Type: "number", Min: "1", Max: "9", Step: "2", Value: "3"})},
		{"field/nolabel", field(ui.Field, ui.FieldProps{Name: "q"})},
		{"input/all", field(ui.Input, ui.FieldProps{Name: "email", Label: "Email", Type: "email", Value: "a@b", Required: true, Help: "We never share it.",
			Placeholder: "you@x", Autocomplete: "email", Class: "c", Attrs: []ui.Attr{{Name: "data-x", Value: "1"}}})},
		{"input/error", field(ui.Input, ui.FieldProps{Name: "name", Label: "Name", Error: "required", Help: "h", Required: true})},
		{"password", field(ui.Password, ui.FieldProps{Name: "password", Label: "Password", Required: true})},
		{"password/autocomplete", field(ui.Password, ui.FieldProps{Name: "pw", Autocomplete: "new-password", Error: "short"})},
		{"textarea", field(ui.Textarea, ui.FieldProps{Name: "note", Label: "Note", Value: "hi <b>"})},
		{"textarea/rows", field(ui.Textarea, ui.FieldProps{Name: "note", Rows: 5, Placeholder: "…", Error: "e", Disabled: true})},
		{"select", field(ui.Select, ui.FieldProps{Name: "cat", Label: "Category", Value: "b",
			Options: []ui.Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}})},
		{"select/placeholder", field(ui.Select, ui.FieldProps{Name: "cat", Placeholder: "Pick", Options: []ui.Option{{Value: "a", Label: "A"}}, Error: "e", Required: true})},
		{"select/placeholder-chosen", field(ui.Select, ui.FieldProps{Name: "cat", Placeholder: "Pick", Value: "a", Options: []ui.Option{{Value: "a", Label: "A"}}})},
		{"checkbox", field(ui.Checkbox, ui.FieldProps{Name: "agree", Label: "I agree", Value: "true"})},
		{"checkbox/off", field(ui.Checkbox, ui.FieldProps{Name: "agree", Label: "No", Disabled: true, Help: "h", Error: "e", Class: "c",
			Attrs: []ui.Attr{{Name: "data-z", Value: "z"}}})},

		{"table", func() ui.Node { return ui.Table(ui.TableProps[book]{Columns: kitCols(), Rows: kitBooks}) }},
		{"table/all", func() ui.Node {
			return ui.Table(ui.TableProps[book]{Columns: kitCols(), Rows: kitBooks,
				RowKey:      func(b book) any { return b.ID },
				RowHref:     func(b book) string { return "/books/" + b.Title },
				RowActions:  func(b book) ui.Node { return ui.Button(ui.ButtonProps{Size: ui.SizeSm, Children: ui.Text("Edit")}) },
				Selectable:  true,
				RowSelected: func(b book) bool { return b.ID == 2 },
				OnSelect:    func(b book) any { return kitSel },
				OnSelectAll: kitSel, SomeSelected: true,
				Index: true, IndexFrom: 10, Flush: true, Class: "x"})
		}},
		{"table/plain-select", func() ui.Node {
			return ui.Table(ui.TableProps[book]{Columns: kitCols()[:2], Rows: kitBooks[:2], Selectable: true, SelectName: "ids",
				RowKey: func(b book) any { return b.ID }, RowSelected: func(b book) bool { return true }, OnSelectAll: kitSel, AllSelected: true, SomeSelected: true})
		}},
		{"table/select-nokey", func() ui.Node {
			return ui.Table(ui.TableProps[book]{Columns: kitCols()[:1], Rows: kitBooks[:1], Selectable: true})
		}},
		{"table/empty", func() ui.Node {
			return ui.Table(ui.TableProps[book]{Columns: kitCols(), Selectable: true, Index: true})
		}},
		{"table/empty-icon", func() ui.Node {
			return ui.Table(ui.TableProps[book]{Columns: kitCols()[:1], EmptyIcon: ui.Icon("list-checks"), Empty: ui.Text("No tasks available."),
				RowActions: func(book) ui.Node { return nil }})
		}},
		{"table/empty-custom", func() ui.Node { return ui.Table(ui.TableProps[book]{Empty: ui.Text("none")}) }},

		{"datatable", func() ui.Node {
			return ui.DataTable(ui.DataTableProps[book]{Columns: kitCols(), Page: ui.NewPage(kitBooks, 3, 1, 10)})
		}},
		{"datatable/empty", func() ui.Node {
			return ui.DataTable(ui.DataTableProps[book]{Columns: kitCols(), Page: ui.NewPage([]book{}, 0, 1, 10), Class: "x"})
		}},
		{"datatable/all", func() ui.Node {
			return ui.DataTable(ui.DataTableProps[book]{Columns: kitCols(), Page: ui.NewPage(kitBooks, 200, 7, 10),
				OnPage: kitPage, OnSize: kitPage, Sizes: []int{10, 25, 50}, OnSearch: kitPick, Search: "du", SearchPlaceholder: "Find books",
				OnSort: kitPick, Sort: "title", SortDir: "desc", Window: 3,
				RowKey: func(b book) any { return b.ID }, RowHref: func(b book) string { return "/b" },
				Selectable: true, OnSelect: func(b book) any { return kitSel }, RowSelected: func(b book) bool { return false },
				OnSelectAll: kitSel, Index: true, Empty: ui.Text("none"), Flush: true, Class: "x",
				Toolbar: ui.Button(ui.ButtonProps{Children: ui.Text("New")}),
				Tabs:    ui.Tabs(ui.TabsProps{Tabs: []ui.Tab{{Label: "All", Href: "/", Active: true}}})})
		}},
		{"datatable/sort-asc-search", func() ui.Node {
			return ui.DataTable(ui.DataTableProps[book]{Columns: kitCols(), Page: ui.NewPage(kitBooks, 30, 1, 10),
				OnSearch: ui.JS.Show(ui.Ref("x")), OnSort: kitPick, Sort: "id", SortDir: "asc", OnPage: kitPage, OnSize: kitPage})
		}},
		{"datatable/toolbar-static-pager", func() ui.Node {
			return ui.DataTable(ui.DataTableProps[book]{Columns: kitCols(), Page: ui.NewPage(kitBooks, 30, 3, 10), Toolbar: ui.Text("t"), Window: -1})
		}},
		{"datatable/tabs-only", func() ui.Node {
			return ui.DataTable(ui.DataTableProps[book]{Columns: kitCols()[:1], Page: ui.NewPage(kitBooks, 3, 1, 10), Tabs: ui.Text("tabs"), OnSort: kitPick})
		}},
	}
	return cs
}

// TestKitGolden renders every component and variant, statically and in a
// live session, against testdata/kit.golden.
func TestKitGolden(t *testing.T) {
	var b strings.Builder
	for _, c := range kitCases() {
		b.WriteString("=== " + c.name + "\n")
		s, err := ui.String(context.Background(), c.node())
		if err != nil {
			b.WriteString("error: " + err.Error() + "\n")
		} else {
			b.WriteString(s + "\n")
		}
		cl := join(t, 1, func(context.Context) ui.Node { return ui.El("main", nil, c.node()) })
		b.WriteString("--- live\n" + cl.HTML() + "\n")
	}
	got := b.String()
	const file = "testdata/kit.golden"
	if *updateKit {
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("kit.golden differs at line %d:\n got: %s\nwant: %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("kit.golden differs in length: got %d lines, want %d", len(gl), len(wl))
	}
}
