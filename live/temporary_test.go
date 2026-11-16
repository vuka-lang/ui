package live_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// report is a page holding a large one-shot list: two thousand rows of
// about a hundred bytes, in a Temporary region unless Plain.
type report struct {
	ui.Live
	Plain  bool
	wide   bool // the region sits in another element: its surroundings change
	rows   []string
	clicks int
	picked string
}

func (r *report) LiveSchema(l *ui.LiveSchema) { l.Mount(r.mount) }

func (r *report) mount(context.Context) error { r.rows = reportRows("row", 2000); return nil }

func (r *report) Inc()    { r.clicks++ }
func (r *report) Reload() { r.rows = reportRows("new", 2000) }
func (r *report) Shrink() { r.rows = r.rows[:3] }
func (r *report) Edit()   { r.rows[0] = "edited in place" }
func (r *report) Widen()  { r.wide = !r.wide }

func reportRows(prefix string, n int) []string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = fmt.Sprintf("%s %d %s", prefix, i, strings.Repeat("·", 40))
	}
	return rows
}

func (r *report) Render() ui.Node {
	rows := make([]ui.Node, len(r.rows))
	for i, row := range r.rows {
		pick := ui.On(func() { r.picked = row })
		rows[i] = ui.HTML(`<tr key={i}><td>{row}</td><td><button class={"pick p" + strconv.Itoa(i)} onClick={pick}>pick</button></td></tr>`, i, row, "pick p"+strconv.Itoa(i), pick)
	}
	body := ui.HTML(`<table>{rows}</table>`, rows)
	if !r.Plain {
		body = ui.Temporary(ui.TemporaryProps{Of: r.rows, Children: body})
	}
	box := ui.HTML(`<div id="box">{body}</div>`, body)
	if r.wide {
		box = ui.HTML(`<article id="box">{body}</article>`, body)
	}
	return ui.HTML(`<section>
  <p id="count">clicks {r.clicks} picked {r.picked}</p>
  <button id="inc" onClick={r.Inc}></button>
  <button id="reload" onClick={r.Reload}></button>
  <button id="shrink" onClick={r.Shrink}></button>
  <button id="edit" onClick={r.Edit}></button>
  <button id="widen" onClick={r.Widen}></button>
  {box}
</section>`, r.clicks, r.picked, ui.On(r.Inc), ui.On(r.Reload), ui.On(r.Shrink), ui.On(r.Edit), ui.On(r.Widen), box)
}

func reportPage(plain bool) func(context.Context) ui.Node {
	return func(context.Context) ui.Node {
		return ui.El("main", nil, ui.Component("report", nil, &report{Plain: plain}))
	}
}

func joinReport(t *testing.T, plain bool) (*live.Session, *livetest.Client) {
	t.Helper()
	s := live.NewSession(context.Background(), reportPage(plain))
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func clickBytes(t *testing.T, c *livetest.Client, sel string) int {
	t.Helper()
	before := c.Bytes
	if err := c.Click(sel); err != nil {
		t.Fatal(err)
	}
	return c.Bytes - before
}

func wantRow(t *testing.T, c *livetest.Client, rows int, text ...string) {
	t.Helper()
	h := c.HTML()
	if n := strings.Count(h, "<tr"); n != rows {
		t.Fatalf("the client holds %d rows, want %d", n, rows)
	}
	for _, s := range text {
		if !strings.Contains(h, s) {
			t.Fatalf("the client's page lacks %q", s)
		}
	}
}

// TestTemporaryForgets: the rows arrive with the join, and the session
// keeps nothing of them after; the same page without Temporary keeps
// their tree and shadow for the connection's life.
func TestTemporaryForgets(t *testing.T) {
	sp, cp := joinReport(t, true)
	st, ct := joinReport(t, false)
	wantRow(t, cp, 2000, "row 1999 ")
	wantRow(t, ct, 2000, "row 1999 ")
	pf, pb := live.Retained(sp)
	tf, tb := live.Retained(st)
	t.Logf("retained: plain %d frames %d bytes, temporary %d frames %d bytes", pf, pb, tf, tb)
	if pf < 4000 || pb < 100_000 {
		t.Fatalf("the plain page keeps %d frames, %d bytes: the measure is off", pf, pb)
	}
	if tf > 20 || tb > 1000 {
		t.Fatalf("the temporary page keeps %d frames, %d bytes", tf, tb)
	}
	if h := st.HTML(); strings.Contains(h, "row 1999") {
		t.Fatal("the session's HTML still holds the forgotten rows")
	}
}

// TestTemporaryUnchanged: a render for another reason leaves the rows as
// the browser has them — they aren't in the patch — and their handlers,
// which the render named again, still answer.
func TestTemporaryUnchanged(t *testing.T) {
	s, c := joinReport(t, false)
	if n := clickBytes(t, c, "#inc"); n > 400 {
		t.Fatalf("an unrelated event's reply is %d bytes", n)
	}
	wantRow(t, c, 2000, "clicks 1", "row 1999 ")
	if n := clickBytes(t, c, ".p7"); n > 400 {
		t.Fatalf("a row's event's reply is %d bytes", n)
	}
	wantRow(t, c, 2000, "picked row 7 ")
	if f, b := live.Retained(s); f > 20 || b > 1000 {
		t.Fatalf("the session keeps %d frames, %d bytes after the events", f, b)
	}
	// a page sitting idle releases its shadows and rebuilds them: the
	// forgotten region stays forgotten
	s.Trim()
	if n := clickBytes(t, c, "#inc"); n > 400 {
		t.Fatalf("an event after Trim's reply is %d bytes", n)
	}
	wantRow(t, c, 2000, "clicks 2", "row 1999 ")
}

// TestTemporarySetAgain: a new value is sent whole, then forgotten again;
// the same slice changed in place isn't another value.
func TestTemporarySetAgain(t *testing.T) {
	s, c := joinReport(t, false)
	if n := clickBytes(t, c, "#reload"); n < 100_000 {
		t.Fatalf("a new value's reply is %d bytes: the rows weren't sent", n)
	}
	wantRow(t, c, 2000, "new 1999 ")
	if strings.Contains(c.HTML(), "row 1 ") {
		t.Fatal("the old rows are still on the page")
	}
	if f, b := live.Retained(s); f > 20 || b > 1000 {
		t.Fatalf("the session keeps %d frames, %d bytes after sending the new value", f, b)
	}
	if n := clickBytes(t, c, "#inc"); n > 400 {
		t.Fatalf("an unrelated event's reply is %d bytes", n)
	}
	clickBytes(t, c, "#shrink") // another length of the same array
	wantRow(t, c, 3, "new 2 ")
	clickBytes(t, c, "#edit")
	if strings.Contains(c.HTML(), "edited in place") {
		t.Fatal("an edit in place was sent: it isn't another value")
	}
	wantRow(t, c, 3, "new 0 ")
}

// TestTemporarySurroundings: an element around the region changing its
// markup sends what it holds whole, the region's content as it is.
func TestTemporarySurroundings(t *testing.T) {
	s, c := joinReport(t, false)
	clickBytes(t, c, "#widen")
	if !strings.Contains(c.HTML(), `<article id="box">`) {
		t.Fatalf("the box didn't change: %.300s", c.HTML())
	}
	wantRow(t, c, 2000, "row 1999 ")
	clickBytes(t, c, "#widen")
	wantRow(t, c, 2000, "row 1999 ")
	if f, b := live.Retained(s); f > 20 || b > 1000 {
		t.Fatalf("the session keeps %d frames, %d bytes", f, b)
	}
}

// TestTemporaryJoins: a fresh mount renders and sends the rows; so does a
// page adopted from its HTTP render, whose join starts the connection
// afresh; and a v1 connection gets them as HTML.
func TestTemporaryJoins(t *testing.T) {
	_, c := joinReport(t, false)
	if err := c.Reconnect(live.NewSession(context.Background(), reportPage(false))); err != nil {
		t.Fatal(err)
	}
	wantRow(t, c, 2000, "row 1999 ")

	s := live.NewSession(context.Background(), reportPage(false))
	page, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "row 1999 ") {
		t.Fatal("the HTTP render lacks the rows")
	}
	if err := s.Adopt(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	c, err = livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantRow(t, c, 2000, "row 1999 ")
	if n := clickBytes(t, c, "#inc"); n > 400 {
		t.Fatalf("an unrelated event's reply is %d bytes", n)
	}

	s = live.NewSession(context.Background(), reportPage(false))
	c, err = livetest.Join(s, 1)
	if err != nil {
		t.Fatal(err)
	}
	clickBytes(t, c, "#inc")
	wantRow(t, c, 2000, "clicks 1", "row 1999 ")
}

// TestTemporaryAhead: a page rendered ahead and taken by a navigation is new
// to the client: its rows are sent, then forgotten.
func TestTemporaryAhead(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.El("main", nil) })
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	ahead := live.NewSession(context.Background(), reportPage(false))
	if err := ahead.RenderAhead(); err != nil {
		t.Fatal(err)
	}
	p, err := s.NavigateFrom(live.Route{Render: reportPage(false)}, ahead, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(live.Marshal(live.PatchMessage(0, p))); err != nil {
		t.Fatal(err)
	}
	wantRow(t, c, 2000, "row 1999 ")
	if f, b := live.Retained(s); f > 20 || b > 1000 {
		t.Fatalf("the session keeps %d frames, %d bytes", f, b)
	}
	if n := clickBytes(t, c, "#inc"); n > 400 {
		t.Fatalf("an unrelated event's reply is %d bytes", n)
	}
	wantRow(t, c, 2000, "clicks 1", "row 1999 ")
}

// deferredReport loads its rows after the first render (ui.Defer): the
// load's patch sends them, and the session forgets them after it.
type deferredReport struct {
	ui.Live
	rows   []string
	clicks int
}

func (r *deferredReport) LiveSchema(l *ui.LiveSchema) {
	l.Field(&r.rows, ui.Defer("rows"))
	l.Loads("rows", r.load)
}

func (r *deferredReport) load(context.Context) error { r.rows = reportRows("row", 2000); return nil }
func (r *deferredReport) Inc()                       { r.clicks++ }

func (r *deferredReport) Render() ui.Node {
	var body ui.Node = ui.HTML(`<p>loading</p>`)
	if !r.Loading("rows") {
		rows := make([]ui.Node, len(r.rows))
		for i, row := range r.rows {
			rows[i] = ui.HTML(`<tr key={i}><td>{row}</td></tr>`, i, row)
		}
		body = ui.Temporary(ui.TemporaryProps{Of: r.rows, Children: ui.HTML(`<table>{rows}</table>`, rows)})
	}
	return ui.HTML(`<section><p id="count">clicks {r.clicks}</p><button id="inc" onClick={r.Inc}></button><div>{body}</div></section>`, r.clicks, ui.On(r.Inc), body)
}

func TestTemporaryDeferred(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.El("main", nil, ui.Component("report", nil, &deferredReport{}))
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantRow(t, c, 0, "loading")
	for _, d := range s.Deferred() {
		d.Run(context.Background())
		p, err := s.Loaded(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Apply(live.Marshal(live.PatchMessage(0, p))); err != nil {
			t.Fatal(err)
		}
	}
	wantRow(t, c, 2000, "row 1999 ")
	if f, b := live.Retained(s); f > 20 || b > 1000 {
		t.Fatalf("the session keeps %d frames, %d bytes", f, b)
	}
	if n := clickBytes(t, c, "#inc"); n > 400 {
		t.Fatalf("an unrelated event's reply is %d bytes", n)
	}
	wantRow(t, c, 2000, "clicks 1", "row 1999 ")
}
