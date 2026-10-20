package live_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/live"
)

// Replies is a page whose form clears its bound field once sent.
type Replies struct {
	ui.Live
	ID      int
	Reply   string
	replies []string
}

type repliesParams struct{ ID int }

func (r *Replies) LiveSchema(l *ui.LiveSchema) { l.Field(&r.Reply, ui.Bind{}) }
func (r *Replies) Send()                       { r.replies = append(r.replies, r.Reply); r.Reply = "" }
func (r *Replies) Update(repliesParams)        {}
func (r *Replies) Render() ui.Node {
	return ui.HTML(`<form onSubmit={r.Send}><input {ui.BindValue(r, &r.Reply)...}/><p>{r.ID} {fmt.Sprint(r.replies)}</p></form>`,
		r.Send, ui.BindValue(r, &r.Reply), r.ID, fmt.Sprint(r.replies))
}

func routed(r *Replies) *ui.ComponentNode {
	return &ui.ComponentNode{Site: "replies", Props: r, Routed: true, URL: []string{"ID"}}
}

func input(t *testing.T, s *live.Session, target, value string) string {
	t.Helper()
	p, err := s.Event(ui.Event{Target: target, Type: "input", Value: value})
	if err != nil || p.Error != "" {
		t.Fatalf("%s: %v %s", target, err, p.Error)
	}
	var b strings.Builder
	for _, u := range p.Updates {
		b.WriteString(u.HTML)
	}
	return b.String()
}

// A routed page owns its exported fields: what its handler changes stays
// changed, a bound field included, and a patch sets only those its URL
// binds.
func TestRoutedPageOwnsItsFields(t *testing.T) {
	tag := routed(&Replies{ID: 2})
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return tag })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if out := input(t, s, "c1:1", "hello"); !strings.Contains(out, `value="hello"`) {
		t.Fatalf("typed: %s", out)
	}
	if out := input(t, s, "c1:0", ""); !strings.Contains(out, `value=""`) || !strings.Contains(out, "2 [hello]") {
		t.Fatalf("sent: the field isn't cleared: %s", out)
	}
	input(t, s, "c1:1", "draft")
	next := &Replies{ID: 3}
	p, err := s.Navigate(live.Route{
		Render:  func(context.Context) ui.Node { return routed(next) },
		Updates: map[ui.Stateful]any{next: repliesParams{3}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if out := p.Updates[0].HTML; !strings.Contains(out, `value="draft"`) || !strings.Contains(out, "3 [hello]") {
		t.Fatalf("patched: %s", out)
	}
}

// Thread renders Replies as a component: its render passes Reply, which
// wins over what the component set.
type Thread struct {
	ui.Live
	reply string
}

func (t *Thread) Render() ui.Node {
	return ui.HTML(`<div>{r}</div>`, &Replies{ID: 1, Reply: t.reply})
}

func TestParentPropsWin(t *testing.T) {
	th := &Thread{reply: "from the parent"}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("thread", nil, th) })
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if out := input(t, s, "c2:0", ""); !strings.Contains(out, `value="from the parent"`) {
		t.Fatalf("the parent's prop: %s", out)
	}
}
