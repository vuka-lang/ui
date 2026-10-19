package live_test

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/live/internal/demo"
	"github.com/vuka-lang/ui/live/livetest"
)

// The demo package's components written with templates, as a person writes
// them: TestTemplateMatchesJSX checks a session renders and patches them
// exactly as it does the compiled JSX.

type tCounter struct {
	ui.Live
	Label string
	n     ui.Assign[int]
}

func (c *tCounter) Inc() { c.n.Update(func(n int) int { return n + 1 }) }

func (c *tCounter) Render() ui.Node {
	return ui.HTML(`<span>{c.Label}: {c.n.Get()} <button onClick={c.Inc}>+</button></span>`, c.Label, c.n.Get(), c.Inc)
}

type item struct {
	ID   int
	Name string
}

// tBoard is demo.Board.
type tBoard struct {
	ui.Live
	Title    string
	Children ui.Node
	items    []item
	next     int
	mode     string
	note     string
	open     bool
}

func (b *tBoard) Mount() {
	for _, n := range []string{"milk", "eggs", "tea"} {
		b.add(n)
	}
}

func (b *tBoard) add(name string) {
	b.next++
	b.items = append(b.items, item{b.next, name})
}

func (b *tBoard) Do(w string) {
	n := len(b.items)
	switch {
	case w == "add":
		b.add("new" + strconv.Itoa(b.next))
	case w == "front":
		b.next++
		b.items = append([]item{{b.next, "front"}}, b.items...)
	case w == "drop" && n > 0:
		b.items = b.items[1:]
	case w == "pop" && n > 0:
		b.items = b.items[:n-1]
	case w == "rotate" && n > 1:
		b.items = append(b.items[1:], b.items[0])
	case w == "reverse":
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			b.items[i], b.items[j] = b.items[j], b.items[i]
		}
	case w == "swap" && n > 2:
		b.items[0], b.items[n-1] = b.items[n-1], b.items[0]
	case w == "rename" && n > 0:
		b.items[n/2].Name += "!"
	case w == "mid" && n > 1:
		it := b.items[0]
		rest := append([]item{}, b.items[1:]...)
		b.items = append(append(rest[:len(rest)/2:len(rest)/2], it), rest[len(rest)/2:]...)
	case w == "clear":
		b.items = nil
	default:
		b.mode = w
	}
}

func (b *tBoard) Toggle() { b.open = !b.open }

func (b *tBoard) Note(s string) { b.note = s }

func (b *tBoard) Remove(id int) {
	for i, it := range b.items {
		if it.ID == id {
			b.items = append(b.items[:i:i], b.items[i+1:]...)
			return
		}
	}
}

func (b *tBoard) Render() ui.Node {
	var items []ui.Node
	for _, it := range b.items {
		items = append(items, ui.HTML(`<li key={it.ID}>{it.Name} {counter}<button onClick={remove}>x</button></li>`,
			it.ID, it.Name, &tCounter{Label: it.Name}, func() { b.Remove(it.ID) }))
	}
	var numbered []ui.Node
	for i, it := range b.items {
		numbered = append(numbered, ui.HTML(`<li key={it.ID}>{i}: {it.Name}</li>`, it.ID, i, it.Name))
	}
	var state ui.Node
	switch {
	case b.open:
		state = ui.HTML(`<p onClick={b.Toggle}>open {b.note}</p>`, b.Toggle, b.note)
	case len(b.items) == 0:
		state = ui.HTML(`<p>empty</p>`)
	default:
		state = ui.HTML(`<p>closed</p>`)
	}
	var mode ui.Node
	switch b.mode {
	case "big":
		mode = ui.HTML(`<b>BIG</b>`)
	case "note":
		mode = ui.HTML(`<input onInput={b.Note} value={b.note} />`, b.Note, b.note)
	default:
		mode = ui.El("i", nil, ui.Text("opaque "+b.mode), ui.Key(nil, &tCounter{Label: "inner"}))
	}
	var keyed []ui.Node
	for _, it := range b.items {
		keyed = append(keyed, ui.Key(it.ID, &tCounter{Label: "k" + it.Name}))
	}
	script := ui.El("script", nil, ui.Child("var mode = '"+b.mode+"' </script>"))
	return ui.HTML(`
<section class="board {b.mode}">
  <h2 title={b.note}>{b.Title} ({len(b.items)})</h2>
  {b.Children}
  <input onInput={b.Do} value={b.mode}>
  <button onClick={b.Toggle}>toggle</button>
  <ul>{items}</ul>
  <ol>{numbered}</ol>
  {state}
  {mode}
  {script}
  {keyed}
</section>`, b.mode, b.note, b.Title, len(b.items), b.Children, b.Do, b.Toggle, items, numbered, state, mode, script, keyed)
}

func tPage(context.Context) ui.Node {
	return ui.HTML(`<main>{one}{two}{solo}</main>`, &tBoard{Title: "one", Children: ui.HTML(`<em>first</em>`)}, &tBoard{Title: "two"}, &tCounter{Label: "solo"})
}

// tCellTable is demo.CellTable.
type tCellTable struct {
	ui.Live
	N     int
	cells []int
}

func (t *tCellTable) Mount() { t.cells = make([]int, t.N) }

func (t *tCellTable) Bump(i int) { t.cells[i]++ }

func (t *tCellTable) Render() ui.Node {
	rows := make([]ui.Node, len(t.cells))
	for i, v := range t.cells {
		rows[i] = ui.HTML(`
<tr key={i}>
  <td>row {i}</td>
  <td class="n">{v}</td>
  <td><button onClick={bump}>+</button></td>
</tr>`, i, v, func() { t.Bump(i) })
	}
	return ui.HTML(`<table>{rows}</table>`, rows)
}

func tCounters(n int) func(context.Context) ui.Node {
	return func(context.Context) ui.Node {
		cs := make([]ui.Node, n)
		for i := range n {
			cs[i] = ui.Key(i, &tCounter{Label: "c" + strconv.Itoa(i)})
		}
		return ui.HTML(`<main>{cs}</main>`, cs)
	}
}

// TestTemplateMatchesJSX sends the same random events to a compiled JSX page
// and its template twin, over v1 and v2: every reply and the page each
// client holds are the same, byte for byte.
func TestTemplateMatchesJSX(t *testing.T) {
	pairs := []struct {
		name      string
		jsx, tmpl func(context.Context) ui.Node
	}{
		{"page", demo.Page, tPage},
		{"counters", demo.Counters(20), tCounters(20)},
		{"cells", func(context.Context) ui.Node { return ui.Component("t#1", nil, &demo.CellTable{N: 20}) },
			func(context.Context) ui.Node { return ui.Component("t#1", nil, &tCellTable{N: 20}) }},
	}
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = true
	for _, p := range pairs {
		for seed := range uint64(8) {
			for _, v := range []int{1, 2} {
				r := rand.New(rand.NewPCG(seed, 11))
				sj, st := live.NewSession(context.Background(), p.jsx), live.NewSession(context.Background(), p.tmpl)
				cj, err := livetest.Join(sj, v)
				if err != nil {
					t.Fatal(err)
				}
				ct, err := livetest.Join(st, v)
				if err != nil {
					t.Fatal(err)
				}
				for i := range 120 {
					if cj.HTML() != ct.HTML() {
						t.Fatalf("%s v%d seed %d step %d:\njsx      %s\ntemplate %s", p.name, v, seed, i, cj.HTML(), ct.HTML())
					}
					ev, ok := cj.RandomEvent(r, demo.Words)
					if !ok {
						break
					}
					mj, errj := cj.Event(ev)
					mt, errt := ct.Event(ev)
					bj, _ := json.Marshal(mj)
					bt, _ := json.Marshal(mt)
					if errj != nil || errt != nil || string(bj) != string(bt) {
						t.Fatalf("%s v%d seed %d step %d %+v:\njsx      %s %v\ntemplate %s %v", p.name, v, seed, i, ev, bj, errj, bt, errt)
					}
				}
			}
		}
	}
}

func BenchmarkTemplateCellTable(b *testing.B) {
	page := func(context.Context) ui.Node { return ui.Component("b#1", nil, &tCellTable{N: 1000}) }
	versions(b, page, func(i int) string { return "c1:" + strconv.Itoa(i*37%1000) })
}

func BenchmarkTemplateCounters(b *testing.B) {
	versions(b, tCounters(100), func(i int) string { return "c" + strconv.Itoa(1+i*7%100) + ":0" })
}
