package ui_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/uigen"
)

func contains(t *testing.T, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Fatalf("missing %q in %s", w, s)
		}
	}
}

// --- Button ---------------------------------------------------------------

func TestButtonRenders(t *testing.T) {
	s := render(t, ui.Button{Variant: ui.Primary, Size: ui.SizeSm, Icon: ui.Icon("plus"), Children: ui.Text("New")})
	contains(t, s, "<button", `type="button"`, "vk-btn", "vk-btn--primary", "vk-btn--sm", "vk-btn__icon", `data-vk-icon="plus"`, "New")

	// A link button.
	link := render(t, ui.Button{Href: "/back", Nav: true, Children: ui.Text("Back")})
	contains(t, link, "<a", `href="/back"`, "data-vk-link", "vk-btn")
}

func TestButtonLoading(t *testing.T) {
	s := render(t, ui.Submit{Loading: true, Children: ui.Text("Save")})
	// A loading submit button is busy, disabled, spins, and carries the hook
	// the form-busy CSS targets.
	contains(t, s, `aria-busy="true"`, "disabled", "vk-spinner", "vk-btn--submit", `type="submit"`, "vk-btn--primary")
	if strings.Contains(s, `data-vk-icon`) {
		t.Fatalf("a loading button shows the spinner, not an icon: %s", s)
	}
}

// --- Badge / Tile / Card / Icon ------------------------------------------

func TestBadge(t *testing.T) {
	contains(t, render(t, ui.Badge{Variant: ui.Success, Children: ui.Text("Signed")}),
		"vk-badge", "vk-badge--success", "Signed")
	contains(t, render(t, ui.Badge{Variant: ui.Danger, Soft: true, Children: ui.Text("Failed")}),
		"vk-badge--danger", "vk-badge--soft")
}

func TestTileFormatsNumber(t *testing.T) {
	s := render(t, ui.Tile{Label: "Applications", Icon: "file-text", Value: int64(1234567)})
	contains(t, s, "vk-tile", "vk-tile__value", "1,234,567", "Applications", `data-vk-icon="file-text"`)

	grid := render(t, ui.TileGrid(ui.Tile{Label: "A", Value: 5}, ui.Tile{Label: "B", Value: 0}))
	contains(t, grid, "vk-tile-grid")
}

func TestCard(t *testing.T) {
	s := render(t, ui.Card{
		Title:    ui.Text("Members"),
		Header:   ui.Actions(ui.Button{Children: ui.Text("Invite")}),
		Children: ui.Text("body"),
	})
	contains(t, s, "vk-card", "vk-card__header", "vk-card__title", "Members", "vk-card__header-actions", "vk-actions", "Invite", "vk-card__body", "body")
}

func TestIconUnknownIsVisible(t *testing.T) {
	s := render(t, ui.Icon("does-not-exist"))
	contains(t, s, "<svg", "vk-icon", `data-vk-icon="does-not-exist"`)
	if len(ui.IconNames()) == 0 {
		t.Fatal("no icons registered")
	}
}

// --- Tabs: live switch ----------------------------------------------------

type tabsHost struct {
	ui.Live
	active uigen.Assign[string]
}

func (h *tabsHost) Mount()        { h.active.Set("a") }
func (h *tabsHost) Pick(v string) { h.active.Set(v) }
func (h *tabsHost) Render() ui.Node {
	cur := h.active.Get()
	return ui.Tabs{Tabs: []ui.Tab{
		{Label: "Alpha", ID: "tab-a", On: func() { h.Pick("a") }, Active: cur == "a", Count: ui.Count(3)},
		{Label: "Beta", ID: "tab-b", On: func() { h.Pick("b") }, Active: cur == "b"},
	}}
}

func TestTabsSwitch(t *testing.T) {
	for _, v := range []int{1, 2} {
		c := join(t, v, func(context.Context) ui.Node { return ui.Component("tabs#1", nil, &tabsHost{}) })
		r := root("c1")
		if !c.HasClass(r+"#tab-a", "vk-tab--active") {
			t.Fatalf("v%d: tab A not active initially: %s", v, c.HTML())
		}
		contains(t, c.HTML(), "vk-tab__count", "3")
		if err := c.Click(r + "#tab-b"); err != nil {
			t.Fatal(err)
		}
		if !c.HasClass(r+"#tab-b", "vk-tab--active") {
			t.Fatalf("v%d: tab B not active after click", v)
		}
		if c.HasClass(r+"#tab-a", "vk-tab--active") {
			t.Fatalf("v%d: tab A still active after switching", v)
		}
	}
}

// --- DataTable: select-all + per-row live selection -----------------------

type selHost struct {
	ui.Live
	sel uigen.Assign[map[int]bool]
}

func (h *selHost) Mount() { h.sel.Set(map[int]bool{}) }
func (h *selHost) Toggle(id int) {
	h.sel.Update(func(m map[int]bool) map[int]bool {
		n := map[int]bool{}
		for k, v := range m {
			n[k] = v
		}
		n[id] = !n[id]
		return n
	})
}
func (h *selHost) All() {
	h.sel.Update(func(m map[int]bool) map[int]bool {
		on := len(m) < 3 // simple toggle-all
		n := map[int]bool{}
		for _, b := range catalog[:3] {
			n[b.ID] = on
		}
		return n
	})
}
func (h *selHost) count() int {
	n := 0
	for _, v := range h.sel.Get() {
		if v {
			n++
		}
	}
	return n
}
func (h *selHost) Render() ui.Node {
	rows := catalog[:3]
	return ui.DataTable[book]{
		Columns:     []ui.Column[book]{{Header: txt("Title"), Cell: func(b book) ui.Node { return txt(b.Title) }}},
		Page:        ui.NewPage(rows, len(rows), 1, 10),
		Index:       true,
		Selectable:  true,
		RowKey:      func(b book) any { return b.ID },
		RowSelected: func(b book) bool { return h.sel.Get()[b.ID] },
		OnSelect:    func(b book) any { id := b.ID; return func() { h.Toggle(id) } },
		OnSelectAll: h.All,
		AllSelected: h.count() == 3,
	}
}

func TestDataTableSelection(t *testing.T) {
	for _, v := range []int{1, 2} {
		c := join(t, v, func(context.Context) ui.Node { return ui.Component("sel#1", nil, &selHost{}) })
		r := root("c1")
		// Index and select columns present.
		contains(t, c.HTML(), "vk-col--index", "vk-col--select", `aria-label="Select all"`)
		boxes := strings.Count(c.HTML(), `aria-label="Select row"`)
		if boxes != 3 {
			t.Fatalf("v%d: expected 3 row checkboxes, got %d", v, boxes)
		}
		// Tick the first row (a checkbox fires change, not click).
		if err := c.Fire(r+`input[aria-label="Select row"]`, ui.Event{Type: "change"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.HTML(), "vk-row--selected") {
			t.Fatalf("v%d: row not highlighted after select: %s", v, c.HTML())
		}
		// Select-all.
		if err := c.Fire(r+`input[aria-label="Select all"]`, ui.Event{Type: "change"}); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(c.HTML(), "vk-row--selected"); n != 3 {
			t.Fatalf("v%d: expected 3 selected rows after select-all, got %d", v, n)
		}
	}
}

func TestTableEmptyWithIcon(t *testing.T) {
	s := render(t, ui.Table[book]{
		Columns:   []ui.Column[book]{{Header: txt("Title"), Cell: func(b book) ui.Node { return txt(b.Title) }}},
		Flush:     true,
		EmptyIcon: ui.Icon("list-checks"),
		Empty:     ui.Text("No tasks available."),
	})
	contains(t, s, "vk-table--flush", "vk-table__empty", "vk-empty", "vk-empty__icon", `data-vk-icon="list-checks"`, "No tasks available.")
}

// --- Dialog (Modal) extras ------------------------------------------------

func TestModalIconSubtitleSize(t *testing.T) {
	s := render(t, ui.Modal{
		Ref: "approval", Open: true, Size: "xl",
		Icon: ui.Icon("check-circle"), Title: ui.Text("Preview"), Subtitle: "Review before signing",
		Footer:   ui.Actions(ui.Cancel(ui.CloseModal("approval"))),
		Children: ui.Text("body"),
	})
	contains(t, s, "vk-modal--xl", "vk-modal__icon", `data-vk-icon="check-circle"`, "vk-modal__heading",
		"vk-modal__title", "Preview", "vk-modal__subtitle", "Review before signing", "vk-modal__footer")
}

// --- Form: submit fires, errors render, loading state ---------------------

type signForm struct {
	Password string `form:"password"`
}

type formHost struct {
	ui.Live
	signed uigen.Assign[bool]
	errMsg uigen.Assign[string]
}

func (h *formHost) Save(ctx context.Context, f signForm) error {
	if f.Password == "" {
		h.errMsg.Set("Password is required.")
		return nil
	}
	h.errMsg.Set("")
	h.signed.Set(true)
	return nil
}
func (h *formHost) Render() ui.Node {
	if h.signed.Get() {
		return ui.El("div", []ui.Attr{{Name: "id", Value: "done"}}, txt("Signed."))
	}
	return ui.El("div", nil, ui.Form{ID: "sign", OnSubmit: h.Save, Children: ui.Fragment(
		ui.Input{Name: "username", Label: "Username", Value: "ada", Disabled: true},
		ui.Password{Name: "password", Label: "Password", Required: true, Error: h.errMsg.Get()},
		ui.Submit{Children: txt("Sign")},
	)})
}

func TestFormSubmitAndErrors(t *testing.T) {
	for _, v := range []int{1, 2} {
		c := join(t, v, func(context.Context) ui.Node { return ui.Component("form#1", nil, &formHost{}) })
		r := root("c1")
		contains(t, c.HTML(), "vk-form", `id="sign"`, "data-vk-on-submit", "vk-input", "vk-field__req",
			"vk-btn--submit", "Sign")

		// Submit empty → a field error renders.
		if err := c.Fire(r+"#sign", ui.Event{Type: "submit", Form: url.Values{"password": {""}}}); err != nil {
			t.Fatal(err)
		}
		contains(t, c.HTML(), "vk-field--invalid", "vk-field__error", "Password is required.", `aria-invalid="true"`)

		// Submit with a value → the handler runs and the view advances.
		if err := c.Fire(r+"#sign", ui.Event{Type: "submit", Form: url.Values{"password": {"secret"}}}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.HTML(), "Signed.") {
			t.Fatalf("v%d: form submit did not run the handler: %s", v, c.HTML())
		}
	}
}

func TestFormLevelError(t *testing.T) {
	s := render(t, ui.Form{Error: "Could not reach the provider.", Children: ui.Text("x")})
	contains(t, s, "vk-alert", "vk-alert--error", "Could not reach the provider.")
}

func TestSelectAndCheckbox(t *testing.T) {
	sel := render(t, ui.Select{Name: "cat", Label: "Category", Value: "b",
		Options: []ui.Option{{Value: "a", Label: "Alpha"}, {Value: "b", Label: "Beta"}}})
	contains(t, sel, "vk-select", "<option", "Alpha", "Beta", `value="b" selected`)

	cb := render(t, ui.Checkbox{Name: "agree", Label: "I agree", Value: "true"})
	contains(t, cb, "vk-field--check", "vk-check", "vk-check__box", `type="checkbox"`, "checked", "I agree")

	ta := render(t, ui.Textarea{Name: "note", Label: "Note", Rows: 5, Value: "hi"})
	contains(t, ta, "vk-textarea", `rows="5"`, "hi")
}
