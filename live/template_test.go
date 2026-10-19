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

var counterT = ui.Template(`<span>{{0}}: {{1}} <button onClick="{{2}}">+</button></span>`)

func (c *tCounter) Render() ui.Node { return counterT.Render(c.Label, c.n.Get(), c.Inc) }

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

var (
	boardT = ui.Template(`
<section className="board {{0}}">
  <h2 title="{{1}}">{{2}} ({{3}})</h2>
  {{4}}
  <input onInput="{{5}}" value="{{6}}">
  <button onClick="{{7}}">toggle</button>
  <ul>{{8}}</ul>
  <ol>{{9}}</ol>
  {{10}}
  {{11}}
  <script>{{12}}</script>
  {{13}}
</section>`)
	boardItemT = ui.Template(`<li key="{{0}}">{{1}} {{2}}<button onClick="{{3}}">x</button></li>`)
	numberedT  = ui.Template(`<li>{{0}}: {{1}}</li>`)
	openT      = ui.Template(`<p onClick="{{0}}">open {{1}}</p>`)
	emptyT     = ui.Template(`<p>empty</p>`)
	closedT    = ui.Template(`<p>closed</p>`)
	bigT       = ui.Template(`<b>BIG</b>`)
	noteT      = ui.Template(`<input onInput="{{0}}" value="{{1}}" />`)
)

func (b *tBoard) Render() ui.Node {
	numbered := make([]ui.Node, len(b.items))
	for i, it := range b.items {
		numbered[i] = numberedT.Render(i, it.Name)
	}
	var state ui.Node
	switch {
	case b.open:
		state = openT.Render(b.Toggle, b.note)
	case len(b.items) == 0:
		state = emptyT.Render()
	default:
		state = closedT.Render()
	}
	var mode ui.Node
	switch b.mode {
	case "big":
		mode = bigT.Render()
	case "note":
		mode = noteT.Render(b.Note, b.note)
	default:
		mode = ui.El("i", nil, ui.Text("opaque "+b.mode), ui.Key(nil, &tCounter{Label: "inner"}))
	}
	return boardT.Render(b.mode, b.note, b.Title, len(b.items), b.Children, b.Do, b.mode, b.Toggle,
		ui.Each(b.items, func(it item) ui.Node {
			return boardItemT.Render(it.ID, it.Name, &tCounter{Label: it.Name}, func() { b.Remove(it.ID) })
		}),
		numbered, ui.If(true, state), ui.If(true, mode),
		"var mode = '"+b.mode+"' </script>",
		ui.Each(b.items, func(it item) ui.Node { return ui.Key(it.ID, &tCounter{Label: "k" + it.Name}) }),
	)
}

var (
	pageT = ui.Template(`<main>{{0}}{{1}}{{2}}</main>`)
	emT   = ui.Template(`<em>first</em>`)
)

func tPage(context.Context) ui.Node {
	return pageT.Render(&tBoard{Title: "one", Children: emT.Render()}, &tBoard{Title: "two"}, &tCounter{Label: "solo"})
}

// tCellTable is demo.CellTable.
type tCellTable struct {
	ui.Live
	N     int
	cells []int
}

func (t *tCellTable) Mount() { t.cells = make([]int, t.N) }

func (t *tCellTable) Bump(i int) { t.cells[i]++ }

var (
	cellTableT = ui.Template(`<table>{{0}}</table>`)
	cellRowT   = ui.Template(`
<tr key="{{0}}">
  <td>row {{0}}</td>
  <td className="n">{{1}}</td>
  <td><button onClick="{{2}}">+</button></td>
</tr>`)
)

func (t *tCellTable) Render() ui.Node {
	rows := make([]ui.Node, len(t.cells))
	for i, v := range t.cells {
		rows[i] = cellRowT.Render(i, v, func() { t.Bump(i) })
	}
	return cellTableT.Render(rows)
}

var countersT = ui.Template(`<main>{{0}}</main>`)

func tCounters(n int) func(context.Context) ui.Node {
	return func(context.Context) ui.Node {
		cs := make([]ui.Node, n)
		for i := range n {
			cs[i] = ui.Key(i, &tCounter{Label: "c" + strconv.Itoa(i)})
		}
		return countersT.Render(cs)
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
