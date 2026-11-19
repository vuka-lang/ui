package live_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

var tickerDraws int

type Ticker struct {
	ui.Live
	seen []int
}

func (t *Ticker) LiveSchema(l *ui.LiveSchema) { l.Mount(t.mount) }
func (t *Ticker) mount(context.Context) error { t.Subscribe("tick"); return nil }

func (t *Ticker) UpdateInt(n int) { t.seen = append(t.seen, n) }

func (t *Ticker) Render() ui.Node {
	tickerDraws++
	return ui.El("p", nil, ui.Text(fmt.Sprint(t.seen)))
}

func TestInfoBatchOrdersMessagesAndRendersOnce(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("t", nil, &Ticker{}) })
	if m := s.Handle(live.ClientMessage{Type: live.Join, V: 2}); m.Type != live.Render {
		t.Fatalf("join: %+v", m)
	}
	before := tickerDraws
	p, err := s.InfoBatch([]live.Delivery{{Topic: "tick", Msg: 1}, {Topic: "other", Msg: 9}, {Topic: "tick", Msg: 2}, {Topic: "", Msg: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if n := tickerDraws - before; n != 1 {
		t.Errorf("rendered %d times, want 1", n)
	}
	if len(p.Trees) != 1 || !strings.Contains(string(treeJSON(p.Trees)), "[1 2 3]") {
		t.Errorf("patch: %s", treeJSON(p.Trees))
	}
	if p, err := s.InfoBatch([]live.Delivery{{Topic: "other", Msg: 1}}); err != nil || len(p.Trees) != 0 {
		t.Errorf("a batch nobody took: %+v, %v", p, err)
	}
}
