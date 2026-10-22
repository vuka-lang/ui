package live_test

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
	"github.com/vuka-lang/ui/live/livetest"
)

// Post counts likes optimistically: the click shows the new count at once,
// and the server's reply confirms it or takes it back.
type Post struct {
	ui.Live
	Ctl          *PostCtl
	likes, saves int
}

// PostCtl is a test's handle on a Post: the session renders a copy.
type PostCtl struct{ fail bool } // the next Like or Save fails

// Likes is a broadcast setting the count.
type Likes int

func (p *Post) Like() error {
	if p.Ctl.fail {
		p.Ctl.fail = false
		return errors.New("try again")
	}
	p.likes++
	return nil
}

func (p *Post) Save() error {
	if p.Ctl.fail {
		p.Ctl.fail = false
		return errors.New("not saved")
	}
	p.saves++
	return nil
}

func (p *Post) Nothing()                    {}
func (p *Post) Fails() error                { return errors.New("no") }
func (p *Post) Update(n Likes)              { p.likes = int(n) }
func (p *Post) LiveSchema(l *ui.LiveSchema) { l.Mount(p.mount) }
func (p *Post) mount(context.Context) error { p.Subscribe("likes"); return nil }

func (p *Post) Render() ui.Node {
	on := func(ch ui.JSChain) ui.Attr { return a("onClick", ui.On(ch)) }
	like := ui.JS.
		AddClass(ui.This, "liked").
		SetText(ui.Ref("likes"), strconv.Itoa(p.likes+1)).
		Push(p.Like).
		OnError(ui.JS.Show(ui.Ref("like-error"))).
		OnSuccess(ui.JS.Dispatch(ui.This, "liked"))
	return ui.El("article", []ui.Attr{a("onLiked", ui.On(ui.JS.SetAttr(ui.This, "data-liked", "yes")))},
		ui.El("button", []ui.Attr{a("id", "like"), a("class", "btn"), on(like)},
			ui.Text("♥ "), ui.El("span", []ui.Attr{a("ref", "likes")}, ui.Child(p.likes))),
		ui.El("button", []ui.Attr{a("id", "keep"), on(ui.JS.SetText(ui.Ref("likes"), "…").Push(p.Like, ui.KeepOnError()).OnError(ui.JS.Show(ui.Ref("like-error"))))}),
		ui.El("button", []ui.Attr{a("id", "slow"), on(ui.JS.AddClass(ui.Ref("likes"), "busy").Push(p.Like).Timeout(time.Second).OnError(ui.JS.Show(ui.Ref("like-error"))))}),
		ui.El("button", []ui.Attr{a("id", "two"), on(ui.JS.
			SetText(ui.Ref("likes"), "+1").Push(p.Like).OnError(ui.JS.SetAttr(ui.This, "data-like", "failed")).
			SetText(ui.Ref("saves"), "saved").Push(p.Save).OnError(ui.JS.SetAttr(ui.This, "data-save", "failed")))}),
		ui.El("p", []ui.Attr{a("ref", "like-error"), a("style", "display: none")}, ui.Text("Couldn't like")),
		ui.El("i", []ui.Attr{a("ref", "saves")}, ui.Child(p.saves)),
		ui.El("button", []ui.Attr{a("id", "nothing"), a("onClick", ui.On(p.Nothing))}),
		ui.El("button", []ui.Attr{a("id", "fails"), a("onClick", ui.On(p.Fails))}),
		ui.El("button", []ui.Attr{a("id", "call"), on(ui.JS.AddClass(ui.Ref("likes"), "calling").Call("pick", ui.Ref("likes").Attr("class"), 7).Push(p.Like).OnError(ui.JS.Show(ui.Ref("like-error"))))}),
	)
}

func post(p *Post) func(context.Context) ui.Node {
	return func(context.Context) ui.Node { return ui.Component("p#1", nil, p) }
}

// TestOptimisticEncoding checks a push's hooks, timeout and keep on the wire,
// the hooks' own pushes given handler ids.
func TestOptimisticEncoding(t *testing.T) {
	s := live.NewSession(context.Background(), post(&Post{Ctl: &PostCtl{}}))
	page, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{
		`[["add_class",{"names":["liked"]}],["set_text",{"to":{"ref":"likes"},"text":"1"}],["push",{"h":"c1:0","on_ok":[["dispatch",{"event":"liked","bubbles":true}]],"on_error":[["show",{"to":{"ref":"like-error"}}]]}]]`,
		`[["set_text",{"to":{"ref":"likes"},"text":"…"}],["push",{"h":"c1:1","on_error":[["show",{"to":{"ref":"like-error"}}]],"keep":true}]]`,
		`[["add_class",{"to":{"ref":"likes"},"names":["busy"]}],["push",{"h":"c1:2","on_error":[["show",{"to":{"ref":"like-error"}}]],"timeout":1000}]]`,
	} {
		if got := jsOf(t, page, "click", i); got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	}
	nested := ui.JS.Push(func() {}).OnError(ui.JS.Push(func() {}, ui.Value(1)))
	b, err := ui.EncodeJS(ui.WithLiveHost(context.Background(), hostFunc(func(any) string { return "h" })), nested)
	if want := `[["push",{"h":"h","on_error":[["push",{"h":"h","value":1}]]}]]`; err != nil || string(b) != want {
		t.Fatalf("a hook's pushes: %s %v", b, err)
	}
}

type hostFunc func(fn any) string

func (h hostFunc) Handler(_ context.Context, fn any) string                      { return h(fn) }
func (h hostFunc) Keyed(ctx context.Context, _ any) context.Context              { return ctx }
func (h hostFunc) Component(context.Context, *ui.ComponentNode, io.Writer) error { return nil }

// TestReplyPerEvent: every event gets one reply carrying its ref, so that a
// runtime can settle the push it came from — a handler changing nothing,
// one failing, and an unknown handler included.
func TestReplyPerEvent(t *testing.T) {
	for _, v := range []int{1, 2} {
		s := live.NewSession(context.Background(), post(&Post{Ctl: &PostCtl{}}))
		if m := s.Handle(live.ClientMessage{Type: live.Join, V: v}); m.Type != live.Render {
			t.Fatal(m.Error)
		}
		for _, tt := range []struct {
			target, want string
		}{
			{"c1:5", `{"type":"patch","ref":7}`},
			{"c1:6", `{"type":"patch","ref":7,"error":"no"}`},
			{"c9:0", `{"type":"error","ref":7,"error":"live: no handler \"c9:0\""}`},
		} {
			got := string(s.HandleJSON([]byte(`{"type":"event","ref":7,"target":"` + tt.target + `","event":"click"}`)))
			if got != tt.want {
				t.Errorf("v%d %s: got %s, want %s", v, tt.target, got, tt.want)
			}
		}
	}
}

func joinPost(t *testing.T, v int) (*livetest.Client, *PostCtl) {
	t.Helper()
	ctl := &PostCtl{}
	c, err := livetest.Join(live.NewSession(context.Background(), post(&Post{Ctl: ctl, likes: 7})), v)
	if err != nil {
		t.Fatal(err)
	}
	return c, ctl
}

func TestOptimisticSuccess(t *testing.T) {
	for _, v := range []int{1, 2} {
		c, _ := joinPost(t, v)
		c.Hold()
		if err := c.Click("#like"); err != nil {
			t.Fatal(err)
		}
		if c.Text("#like span") != "8" || !c.HasClass("#like", "liked") || c.Pending() != 1 {
			t.Fatalf("v%d: predicted at once: %q %s", v, c.Text("#like span"), c.HTML())
		}
		if err := c.Deliver(); err != nil {
			t.Fatal(err)
		}
		if c.Text("#like span") != "8" || !c.HasClass("#like", "liked") || c.Pending() != 0 {
			t.Fatalf("v%d: confirmed: %s", v, c.HTML())
		}
		if x, _ := c.Attr("article", "data-liked"); x != "yes" || c.Visible("p") {
			t.Fatalf("v%d: OnSuccess ran, OnError didn't: %s", v, c.HTML())
		}
		if err := c.Release(); err != nil || c.Click("#like") != nil || c.Text("#like span") != "9" {
			t.Fatalf("v%d: the server's next render: %v %s", v, err, c.HTML())
		}
	}
}

func TestOptimisticError(t *testing.T) {
	for _, v := range []int{1, 2} {
		c, p := joinPost(t, v)
		c.Hold()
		p.fail = true
		c.Click("#like")
		if c.Text("#like span") != "8" || c.Visible("p") {
			t.Fatalf("v%d: predicted: %s", v, c.HTML())
		}
		if err := c.Deliver(); err != nil {
			t.Fatal(err)
		}
		if c.Text("#like span") != "7" || c.HasClass("#like", "liked") || !c.HasClass("#like", "btn") || !c.Visible("p") || c.Pending() != 0 {
			t.Fatalf("v%d: reverted, error shown: %s", v, c.HTML())
		}
		if x, _ := c.Attr("article", "data-liked"); x != "" {
			t.Fatalf("v%d: OnSuccess ran on an error", v)
		}
	}
}

func TestOptimisticServerWins(t *testing.T) {
	for _, v := range []int{1, 2} {
		c, p := joinPost(t, v)
		c.Hold()
		p.fail = true
		c.Click("#like")
		if err := c.Info("likes", Likes(20)); err != nil { // the server's text changes before the reply
			t.Fatal(err)
		}
		if c.Text("#like span") != "20" {
			t.Fatalf("v%d: the server's new text replaces the prediction: %s", v, c.HTML())
		}
		c.Deliver()
		if c.Text("#like span") != "20" || c.HasClass("#like", "liked") || !c.Visible("p") {
			t.Fatalf("v%d: the server's value stays; the rest reverts: %s", v, c.HTML())
		}
	}
}

func TestOptimisticKeepOnError(t *testing.T) {
	c, p := joinPost(t, 2)
	p.fail = true
	if err := c.Click("#keep"); err != nil {
		t.Fatal(err)
	}
	if c.Text("#like span") != "…" || !c.Visible("p") {
		t.Fatalf("kept, error shown: %s", c.HTML())
	}
}

func TestOptimisticTimeout(t *testing.T) {
	c, _ := joinPost(t, 2)
	c.Hold()
	c.Click("#slow")
	if !c.HasClass("#like span", "busy") {
		t.Fatal("predicted")
	}
	c.Advance(999 * time.Millisecond)
	if !c.HasClass("#like span", "busy") || c.Pending() != 1 {
		t.Fatal("not yet")
	}
	c.Advance(time.Millisecond)
	if c.HasClass("#like span", "busy") || !c.Visible("p") || c.Pending() != 0 {
		t.Fatalf("timed out: reverted, error shown: %s", c.HTML())
	}
	// The late reply is applied, its push settled already: the server's
	// render stands, the error stays shown.
	if err := c.Deliver(); err != nil || c.Text("#like span") != "8" || !c.Visible("p") {
		t.Fatalf("late reply: %v %s", err, c.HTML())
	}
}

// TestOptimisticTwoPushes: each push settles its own predictions.
func TestOptimisticTwoPushes(t *testing.T) {
	c, _ := joinPost(t, 2)
	c.Hold()
	c.Click("#two")
	if c.Text("#like span") != "+1" || c.Text("i") != "saved" || c.Pending() != 2 {
		t.Fatalf("both predicted: %s", c.HTML())
	}
	if err := c.Deliver(); err != nil {
		t.Fatal(err)
	}
	c2, p2 := joinPost(t, 2)
	c2.Hold()
	c2.Click("#two")
	p2.fail = true // Like, sent first, fails; Save succeeds
	if err := c2.Deliver(); err != nil {
		t.Fatal(err)
	}
	if c2.Text("#like span") != "7" || c2.Text("i") != "1" {
		t.Fatalf("Like reverted, Save confirmed: %s", c2.HTML())
	}
	if l, _ := c2.Attr("#two", "data-like"); l != "failed" {
		t.Fatalf("Like's OnError: %s", c2.HTML())
	}
	if s, ok := c2.Attr("#two", "data-save"); ok {
		t.Fatalf("Save's OnError ran: %q", s)
	}
	if c.Text("#like span") != "8" || c.Text("i") != "1" {
		t.Fatalf("both confirmed: %s", c.HTML())
	}
}

// TestCall: a called function gets its context and reads; false stops the
// chain; an error fails the push it predicts for, unsent.
func TestCall(t *testing.T) {
	c, _ := joinPost(t, 2)
	var got []any
	var el string
	result := any(true)
	c.RegisterFunc("pick", func(ctx livetest.CallContext, args []any) (any, error) {
		got, el = args, ctx.El.Text()+"|"+must(ctx.El.Attr("id"))
		return result, nil
	})
	if err := c.Click("#call"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "calling" || got[1] != float64(7) || el != "|call" || c.Text("#like span") != "8" {
		t.Fatalf("args read when reached, the push sent: %v %q %s", got, el, c.HTML())
	}
	result = false
	c.Click("#call")
	if c.Text("#like span") != "8" || !c.HasClass("#like span", "calling") {
		t.Fatalf("false stops the chain; what ran stays: %s", c.HTML())
	}
	c2, _ := joinPost(t, 2)
	c2.RegisterFunc("pick", func(livetest.CallContext, []any) (any, error) { return nil, errors.New("boom") })
	c2.Click("#call")
	if c2.Text("#like span") != "7" || c2.HasClass("#like span", "calling") || !c2.Visible("p") || len(c2.Errors) != 1 || !strings.Contains(c2.Errors[0], "boom") {
		t.Fatalf("a throw fails the push unsent: %v %s", c2.Errors, c2.HTML())
	}
	c3, _ := joinPost(t, 2)
	c3.Click("#call")
	if len(c3.Errors) != 1 || !c3.Visible("p") || c3.HasClass("#like span", "calling") {
		t.Fatalf("an unknown function fails too: %v", c3.Errors)
	}
}

func must(s string, _ bool) string { return s }
