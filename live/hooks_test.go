package live_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// Cart pushes with a lifecycle: each hook logs its phase through a call.
type Cart struct {
	ui.Live
	Ctl      *CartCtl
	removed  int
	refreshs int
}

type CartCtl struct{ fail bool }

func (p *Cart) Remove() error {
	if p.Ctl.fail {
		return errors.New("still in use")
	}
	p.removed++
	return nil
}

func (p *Cart) Refresh() { p.refreshs++ }

func logStep(s string) ui.Commands { return ui.JS.Call("log", s) }

// remove is a reusable chain, wrapped by its callers.
func (p *Cart) remove() ui.Commands { return ui.JS.Push(p.Remove).After(logStep("inner after")) }

func (p *Cart) Render() ui.Node {
	b := func(id string, ch ui.Commands) ui.Node {
		return ui.El("button", []ui.Attr{a("id", id), a("onClick", ui.On(ch))}, ui.Text(id))
	}
	full := p.remove().
		Before(logStep("before")).
		Success(logStep("success")).
		Error(ui.JS.SetText(ui.Ref("err"), ui.LoadError).Then(logStep("error"))).
		After(logStep("after"))
	return ui.El("div", nil,
		b("full", full),
		b("confirm", ui.JS.Busy(ui.Ref("list")).Push(p.Remove).Before(ui.JS.Confirm("Remove?").Then(logStep("before"))).After(logStep("after"))),
		b("gate", ui.JS.Push(p.Remove).Before(ui.JS.If(ui.Ref("agree").Attr("data-ok")).Call("log", "agreed")).After(logStep("after"))),
		b("busy", ui.JS.Push(p.Remove).Before(ui.JS.Busy(ui.Ref("list"))).After(logStep("after"))),
		b("slow", ui.JS.Push(p.Remove).Timeout(time.Second).Error(logStep("error")).After(logStep("after"))),
		b("method", ui.JS.Push(p.Remove).Success(p.Refresh).After(logStep("after"))),
		b("twice", ui.JS.Push(p.Remove).Success(logStep("s1")).OnSuccess(logStep("s2")).OnError(logStep("e1")).Error(logStep("e2")).After(logStep("a1")).After(logStep("a2"))),
		b("then", ui.JS.Push(p.Remove).Success(logStep("success")).Then(logStep("at once"))),
		b("agree", ui.JS.SetAttr(ui.Ref("agree"), "data-ok", "")),
		ui.El("span", []ui.Attr{a("ref", "agree")}),
		ui.El("ul", []ui.Attr{a("ref", "list")}),
		ui.El("p", []ui.Attr{a("ref", "err")}),
		ui.El("i", []ui.Attr{a("id", "count")}, ui.Textf("%d %d", p.removed, p.refreshs)),
	)
}

func joinCart(t *testing.T) (*livetest.Client, *CartCtl, *[]string) {
	t.Helper()
	ctl := &CartCtl{}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("cart#1", nil, &Cart{Ctl: ctl}) })
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	var steps []string
	c.RegisterFunc("log", func(_ livetest.CallContext, args []any) (any, error) {
		steps = append(steps, args[0].(string))
		return nil, nil
	})
	return c, ctl, &steps
}

func TestPushLifecycleOrder(t *testing.T) {
	for _, fail := range []bool{false, true} {
		c, ctl, steps := joinCart(t)
		ctl.fail = fail
		if err := c.Click("#full"); err != nil {
			t.Fatal(err)
		}
		want := "before,success,inner after,after"
		if fail {
			want = "before,error,inner after,after"
			if c.Text("p") != "still in use" {
				t.Errorf("ui.LoadError in Error: %q", c.Text("p"))
			}
		}
		if got := strings.Join(*steps, ","); got != want {
			t.Errorf("fail=%v: got %s, want %s", fail, got, want)
		}
	}
}

func TestPushLifecycleVeto(t *testing.T) {
	c, _, steps := joinCart(t)
	c.Confirm = func(string) bool { return false }
	if err := c.Click("#confirm"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*steps, ","); got != "" || c.Text("#count") != "0 0" || c.Pending() != 0 {
		t.Fatalf("a declined confirm: nothing sent, no hook: %q %s", got, c.HTML())
	}
	if c.Busy("ul") {
		t.Fatalf("a vetoed push's Busy is taken off: %s", c.HTML())
	}
	c.Confirm = nil
	if err := c.Click("#confirm"); err != nil || strings.Join(*steps, ",") != "before,after" || c.Text("#count") != "1 0" {
		t.Fatalf("accepted: %v %q %s", err, *steps, c.HTML())
	}
	*steps = nil
	c.Click("#gate")
	if got := strings.Join(*steps, ","); got != "" || c.Text("#count") != "1 0" {
		t.Fatalf("an if not taken: nothing sent: %q %s", got, c.HTML())
	}
	c.Click("#agree")
	c.Click("#gate")
	if got := strings.Join(*steps, ","); got != "agreed,after" || c.Text("#count") != "2 0" {
		t.Fatalf("an if taken: %q %s", got, c.HTML())
	}
}

func TestPushLifecycleBusy(t *testing.T) {
	c, ctl, steps := joinCart(t)
	ctl.fail = true
	c.Hold()
	c.Click("#busy")
	if !c.Busy("ul") {
		t.Fatalf("busy from Before while waiting: %s", c.HTML())
	}
	c.Deliver()
	if c.Busy("ul") || strings.Join(*steps, ",") != "after" {
		t.Fatalf("off on an error, then After: %q %s", *steps, c.HTML())
	}
}

func TestPushLifecycleTimeout(t *testing.T) {
	c, _, steps := joinCart(t)
	c.Hold()
	c.Click("#slow")
	c.Advance(time.Second)
	if got := strings.Join(*steps, ","); got != "error,after" {
		t.Fatalf("a timeout is an error: %q", got)
	}
}

func TestPushLifecycleMethodHook(t *testing.T) {
	c, _, steps := joinCart(t)
	if err := c.Click("#method"); err != nil {
		t.Fatal(err)
	}
	if c.Text("#count") != "1 1" || strings.Join(*steps, ",") != "after" {
		t.Fatalf("Success pushed the method: %q %s", *steps, c.HTML())
	}
}

func TestPushLifecycleTwice(t *testing.T) {
	c, ctl, steps := joinCart(t)
	c.Click("#twice")
	ctl.fail = true
	c.Click("#twice")
	if got := strings.Join(*steps, ","); got != "s1,s2,a1,a2,e1,e2,a1,a2" {
		t.Fatalf("hooks given twice run both, in order: %q", got)
	}
}

func TestPushLifecycleStepsAfterRunAtOnce(t *testing.T) {
	c, _, steps := joinCart(t)
	c.Hold()
	c.Click("#then")
	if got := strings.Join(*steps, ","); got != "at once" {
		t.Fatalf("before the reply: %q", got)
	}
	c.Deliver()
	if got := strings.Join(*steps, ","); got != "at once,success" {
		t.Fatalf("after it: %q", got)
	}
}

func TestPushHooksEncoding(t *testing.T) {
	ctx := ui.WithLiveHost(context.Background(), hostFunc(func(any) string { return "h" }))
	enc := func(ch ui.JSChain) string {
		b, err := ui.EncodeJS(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	f := func() {}
	base := ui.JS.Push(f)
	wrapped := base.Before(ui.JS.Confirm("Delete?")).Success(ui.JS.Hide("#row")).Error(ui.JS.Show("#err")).After(ui.JS.PopFocus())
	if got, want := enc(base), `[["push",{"h":"h"}]]`; got != want {
		t.Errorf("the chain wrapped is as it was: %s", got)
	}
	if got, want := enc(wrapped), `[["push",{"h":"h","before":[["confirm",{"msg":"Delete?"}]],"on_ok":[["hide",{"to":{"sel":"#row"}}]],"on_error":[["show",{"to":{"sel":"#err"}}]],"after":[["pop_focus",{}]]}]]`; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if a, b := enc(base.OnSuccess(ui.JS.Hide("#x")).OnError(ui.JS.Show("#y"))), enc(base.Success(ui.JS.Hide("#x")).Error(ui.JS.Show("#y"))); a != b {
		t.Errorf("OnSuccess and OnError are Success and Error: %s %s", a, b)
	}
	if got, want := enc(base.Success(f).After(ui.On(f))), `[["push",{"h":"h","on_ok":[["push",{"h":"h"}]],"after":[["push",{"h":"h"}]]}]]`; got != want {
		t.Errorf("a func is pushed: %s", got)
	}
	if got, want := enc(base.Success(ui.JS.Hide("#a")).Success(ui.JS.Hide("#b"))), `[["push",{"h":"h","on_ok":[["hide",{"to":{"sel":"#a"}}],["hide",{"to":{"sel":"#b"}}]]}]]`; got != want {
		t.Errorf("twice: %s", got)
	}
	if got, want := enc(ui.JS.On("#row").Push(f).Before(ui.JS.Confirm("?")).After(ui.JS.PopFocus()).Hide()), `[["push",{"h":"h","before":[["confirm",{"msg":"?"}]],"after":[["pop_focus",{}]]}],["hide",{"to":{"sel":"#row"}}]]`; got != want {
		t.Errorf("CommandsOn: %s", got)
	}
	for name, f := range map[string]func(){
		"Before without push": func() { ui.JS.Before(ui.JS) },
		"After without push":  func() { ui.JS.Hide("#x").After(ui.JS) },
		"Success, an int":     func() { ui.JS.Push(f).Success(3) },
		"Error, nil":          func() { ui.JS.Push(f).Error(nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
}
