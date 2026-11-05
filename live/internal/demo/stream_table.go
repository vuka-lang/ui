package demo

import (
	"context"

	"github.com/vuka-lang/ui/v2"
)

// StreamTable is KeyedTable's rows as a stream: the session holds none of
// them between replies, and each row's handlers are pushes of the
// table's methods with the row's id. It is written as the compiler writes
// a .vuka.html file's stream (its markup: a loop over t.rows.Rows() inside
// <tbody>), so the demo builds with any vuka command.
type StreamTable struct {
	ui.Live
	N    int
	Hits *[]int
	rows ui.Stream[int]
	next int `vuka:"-"` // Render doesn't read it
}

func (t *StreamTable) mount(ctx context.Context) error {
	var rows []int
	for t.next < t.N {
		rows = append(rows, t.next)
		t.next++
	}
	t.rows.Reset(rows...)
	return nil
}

func (t *StreamTable) Insert() {
	t.rows.Prepend(t.next)
	t.next++
}

func (t *StreamTable) Hit(id int) { *t.Hits = append(*t.Hits, id) }

func (t *StreamTable) Remove(id int) { t.rows.Delete(id) }

func (t *StreamTable) Render() ui.Node {
	var rowNodes []ui.Node
	for _, id := range t.rows.Rows() {
		rowNodes = append(rowNodes, ui.HTML(`
<tr key={id}>
  <td>row {id}</td>
  <td>
    <button class="hit" onClick={ui.JS.Push(t.Hit, id)}>hit {id}</button>
    <button class="del" onClick={ui.JS.Push(t.Remove, id)}>del {id}</button>
  </td>
</tr>`, id, ui.JS.Push(t.Hit, id), ui.JS.Push(t.Remove, id)))
	}
	block := t.rows.Block(rowNodes)
	return ui.HTML(`
<div>
  <button class="insert" onClick={t.Insert}>insert</button>
  <table>
    <tbody {block.Attrs()...}>{block}</tbody>
  </table>
</div>`, t.Insert, block.Attrs(), block)
}

func (t *StreamTable) LiveSchema(l *ui.LiveSchema) {
	l.Mount(t.mount)
}
