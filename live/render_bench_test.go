package live_test

// The render benchmarks: pages of realistic shapes, compiled by the Vuka
// compiler (internal/benchpages), measured on the paths a live page takes —
// its first HTTP render, its socket's join, an event's patch, a broadcast's
// delivery. They measure; RENDER_BENCH.md beside them is what they found
// and how to run them steadily. TestRenderBenchPages runs each one's setup
// once in the normal test run, so a fixture that stops rendering what it
// should fails there rather than in a benchmark nobody ran.

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/internal/benchpages"
)

func benchPage(c ui.Stateful) func(context.Context) ui.Node {
	return func(context.Context) ui.Node { return ui.Component("bench#1", nil, c) }
}

func usersPage(n int, plain bool) func(context.Context) ui.Node {
	return func(context.Context) ui.Node {
		return ui.Component("bench#1", nil, &benchpages.UsersPage{N: n, Plain: plain})
	}
}

// rowPromote is the target of the Promote item of the row of user id: the
// page is c1, the rows c2… in order.
func rowPromote(id int) string { return "c" + strconv.Itoa(id+1) + ":0" }

func clickMsg(target string) []byte {
	return []byte(`{"type":"event","target":"` + target + `","event":"click"}`)
}

func pushMsg(target string, args string) []byte {
	return []byte(`{"type":"event","target":"` + target + `","event":"push","args":[` + args + `]}`)
}

// replyTo sends msg and fails on an error reply.
func replyTo(tb testing.TB, s *live.Session, msg []byte) []byte {
	out := s.HandleJSON(msg)
	if strings.HasPrefix(string(out), `{"type":"error"`) {
		tb.Fatalf("%s: %s", msg, out)
	}
	return out
}

func joinV2(tb testing.TB, page func(context.Context) ui.Node) (*live.Session, []byte) {
	s := live.NewSession(context.Background(), page)
	return s, replyTo(tb, s, []byte(`{"type":"join","v":2}`))
}

// noVerify turns off the skip verification tests run with: a benchmark
// measures what production does.
func noVerify(b *testing.B) {
	v := live.Verify
	live.Verify = false
	b.Cleanup(func() { live.Verify = v })
}

// coldBench measures a page's first render: as HTML, what the page's HTTP
// response carries, and as trees, what its socket's join sends.
func coldBench(b *testing.B, page func(context.Context) ui.Node) {
	b.Run("http", func(b *testing.B) {
		noVerify(b)
		b.ReportAllocs()
		size := 0
		for b.Loop() {
			s := live.NewSession(context.Background(), page)
			h, err := s.Render()
			if err != nil {
				b.Fatal(err)
			}
			size = len(h)
			s.Close()
		}
		b.ReportMetric(float64(size), "B/html")
	})
	b.Run("join", func(b *testing.B) {
		noVerify(b)
		b.ReportAllocs()
		size := 0
		for b.Loop() {
			s, out := joinV2(b, page)
			size = len(out)
			s.Close()
		}
		b.ReportMetric(float64(size), "B/join")
	})
}

// eventBench sends msg(i) to one joined session per iteration, reporting
// the reply's bytes — even and odd events apart when split, as the modal's
// opens and closes — and the Render calls each event executed.
func eventBench(b *testing.B, page func(context.Context) ui.Node, msg func(i int) []byte, split ...string) {
	noVerify(b)
	s, _ := joinV2(b, page)
	defer s.Close()
	b.ReportAllocs()
	var bytes [2]int
	i := 0
	r0 := live.Renders(s)
	gc := gcCount()
	for b.Loop() {
		bytes[i%2] += len(replyTo(b, s, msg(i)))
		i++
	}
	reportGCs(b, gc, i)
	if len(split) == 2 {
		b.ReportMetric(float64(bytes[0])/float64((i+1)/2), "B/"+split[0])
		b.ReportMetric(float64(bytes[1])/float64(max(i/2, 1)), "B/"+split[1])
	} else {
		b.ReportMetric(float64(bytes[0]+bytes[1])/float64(i), "B/patch")
	}
	b.ReportMetric(float64(live.Renders(s)-r0)/float64(i), "renders/op")
}

func gcCount() uint32 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.NumGC
}

// reportGCs reports the garbage collections the loop ran per op, read
// before and after it, outside its timing.
func reportGCs(b *testing.B, before uint32, ops int) {
	b.ReportMetric(float64(gcCount()-before)/float64(ops), "gcs/op")
}

// BenchmarkRenderTable is a users table of N row components, keyed, each
// with a few props, a badge and an actions menu of ui.ActionMenu items: its
// first render, and an event changing one row's role (Promote: the page's
// state, so the page renders again and places every row). event-plain is
// the same event on rows whose menu items are plain buttons (PlainUserRow),
// the control: an ActionMenu reads the render's context, for the menu it
// closes — a value only its own render sets, so its row is kept as a plain
// one is (the two render the same number of components per event).
func BenchmarkRenderTable(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run("rows="+strconv.Itoa(n), func(b *testing.B) {
			coldBench(b, usersPage(n, false))
			for _, plain := range []bool{false, true} {
				name := map[bool]string{false: "event", true: "event-plain"}[plain]
				b.Run(name, func(b *testing.B) {
					eventBench(b, usersPage(n, plain), func(i int) []byte { return clickMsg(rowPromote(1 + i*37%n)) })
				})
			}
		})
	}
}

// BenchmarkRenderWide is M small components, an event touching one. own:
// the tile's count is its own state, the event renders it alone. props:
// the count is the page's, passed as a prop, so the page renders and
// places all M. slice: props, plus a slice prop every tile is passed.
func BenchmarkRenderWide(b *testing.B) {
	for _, m := range []int{50, 500} {
		for _, mode := range []string{"own", "props", "slice"} {
			b.Run("tiles="+strconv.Itoa(m)+"/"+mode, func(b *testing.B) {
				eventBench(b, benchPage(&benchpages.GridPage{M: m, Mode: mode}), func(i int) []byte {
					return clickMsg("c" + strconv.Itoa(2+i*37%m) + ":0")
				})
			})
		}
	}
}

// BenchmarkRenderModal is a list of 20 users, each with an Edit button
// opening a server-owned modal holding the user's five-field form
// (ui.FormState): its first render, then open / close / reopen another
// user, one event per op.
func BenchmarkRenderModal(b *testing.B) {
	page := func(context.Context) ui.Node { return ui.Component("bench#1", nil, &benchpages.EditPage{N: 20}) }
	coldBench(b, page)
	b.Run("open-close", func(b *testing.B) {
		eventBench(b, page, editCycle, "open", "close")
	})
}

// editCycle is the modal's events in turn: open a user's, close it.
func editCycle(i int) []byte {
	if i%2 == 1 {
		return clickMsg("c1:2") // the close button
	}
	id := 1 + i/2%20
	return pushMsg("c1:"+strconv.Itoa(id)+":0", strconv.Itoa(id))
}

// BenchmarkRenderBroadcast is one broadcast delivered to K joined sessions
// of the same 100-row page, each changing one row: what a socket server's
// per-connection goroutines do between them for one Broadcast — InfoBatch
// and the patch's JSON — run here one after another, so ns/op is the CPU
// one delivery costs the process (writing to the sockets aside).
func BenchmarkRenderBroadcast(b *testing.B) {
	for _, k := range []int{10, 100} {
		b.Run("conns="+strconv.Itoa(k), func(b *testing.B) {
			noVerify(b)
			ss := feedSessions(b, k)
			b.ReportAllocs()
			bytes, i := 0, 0
			r0 := 0
			for _, s := range ss {
				r0 += live.Renders(s)
			}
			roles := benchpages.Users(100)
			gc := gcCount()
			for b.Loop() {
				u := &roles[i*37%100]
				u.Role = benchpages.NextRole(u.Role)
				msg := benchpages.RoleChanged{ID: u.ID, Role: u.Role}
				for _, s := range ss {
					bytes += len(deliverTo(b, s, msg))
				}
				i++
			}
			reportGCs(b, gc, i)
			r := 0
			for _, s := range ss {
				r += live.Renders(s)
			}
			b.ReportMetric(float64(bytes)/float64(i*k), "B/patch")
			b.ReportMetric(float64(r-r0)/float64(i), "renders/op")
		})
	}
}

func feedSessions(tb testing.TB, k int) []*live.Session {
	ss := make([]*live.Session, k)
	for j := range ss {
		ss[j], _ = joinV2(tb, benchPage(&benchpages.FeedPage{N: 100}))
		tb.Cleanup(ss[j].Close)
	}
	return ss
}

// deliver is what a connection does with a broadcast: the session's
// InfoBatch, and the patch's message.
func deliverTo(tb testing.TB, s *live.Session, msg any) []byte {
	p, err := s.InfoBatch([]live.Delivery{{Topic: "users", Msg: msg}})
	if err != nil {
		tb.Fatal(err)
	}
	return live.Marshal(live.PatchMessage(0, p))
}

// TestRenderBenchPages runs each benchmark's setup once and checks the
// fixture renders what the benchmark means to measure, with the skip
// verification tests run with on.
func TestRenderBenchPages(t *testing.T) {
	t.Run("table", func(t *testing.T) {
		for _, plain := range []bool{false, true} {
			s := live.NewSession(context.Background(), usersPage(10, plain))
			h, err := s.Render()
			if err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(h, `<tr class="row"`); n != 10 {
				t.Fatalf("%d rows rendered, want 10", n)
			}
			s, _ = joinV2(t, usersPage(10, plain))
			out := string(replyTo(t, s, clickMsg(rowPromote(3))))
			// user 3 is a viewer: promoted, an editor
			if !strings.Contains(out, "editor") || !strings.Contains(out, `"id":"c4"`) {
				t.Fatalf("promote: %s", out)
			}
		}
	})
	t.Run("wide", func(t *testing.T) {
		for _, mode := range []string{"own", "props", "slice"} {
			s, _ := joinV2(t, benchPage(&benchpages.GridPage{M: 50, Mode: mode}))
			out := string(replyTo(t, s, clickMsg("c7:0")))
			if !strings.Contains(out, `"id":"c7"`) || !strings.Contains(out, `"1"`) {
				t.Fatalf("%s: bump: %s", mode, out)
			}
		}
	})
	t.Run("modal", func(t *testing.T) {
		s, _ := joinV2(t, benchPage(&benchpages.EditPage{N: 20}))
		// the close goes back to the closed markup, which the connection keeps
		for i, want := range []string{`value=\"User 1\"`, `"u":{"2":{"r":`, `value=\"User 2\"`} {
			if out := string(replyTo(t, s, editCycle(i))); !strings.Contains(out, want) {
				t.Fatalf("event %d: no %s in %s", i, want, out)
			}
		}
	})
	t.Run("broadcast", func(t *testing.T) {
		for _, s := range feedSessions(t, 3) {
			out := string(deliverTo(t, s, benchpages.RoleChanged{ID: 2, Role: "viewer"}))
			if !strings.Contains(out, "viewer") || !strings.Contains(out, `"id":"c3"`) {
				t.Fatalf("broadcast: %s", out)
			}
		}
	})
}
