package live_test

import (
	"context"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/uigen"
)

// GuardTable is CellTable with a title above the table: a click on the
// title changes it alone, a click on a row's button the row's cell. It is
// written as the compiler writes a .vuka.html file whose loop reads only
// tracked state (ui.Guard); with Unguarded, as it writes one that doesn't.
type GuardTable struct {
	ui.Live
	N         int
	Unguarded bool
	Built     *int // counts the loop's runs
	title     uigen.Assign[int]
	cells     uigen.Assign[[]int]
}

func (t *GuardTable) mount(ctx context.Context) error {
	t.cells.Set(make([]int, t.N))
	return nil
}

func (t *GuardTable) Title() { t.title.Set(t.title.Get() + 1) }

func (t *GuardTable) Bump(i int) {
	t.cells.Update(func(cells []int) []int { cells[i]++; return cells })
}

func (t *GuardTable) rows() []ui.Node {
	if t.Built != nil {
		*t.Built++
	}
	var items []ui.Node
	for i, v := range t.cells.Get() {
		items = append(items, ui.HTML(`
<tr key={i}>
  <td>row {i}</td>
  <td class="n">{v}</td>
  <td><button onClick={func() { t.Bump(i) }}>+</button></td>
</tr>`, i, v, func() { t.Bump(i) }))
	}
	return items
}

func (t *GuardTable) Render() ui.Node {
	var items any
	if t.Unguarded {
		items = t.rows()
	} else {
		items = ui.Guard(0x67756172647461, t.rows, t.cells.Version())
	}
	return ui.HTML(`<div><h1 onClick={t.Title}>Table {t.title.Get()}</h1><table>{items}</table></div>`, t.Title, t.title.Get(), items)
}

func (t *GuardTable) LiveSchema(l *ui.LiveSchema) {
	l.Mount(t.mount)
}
