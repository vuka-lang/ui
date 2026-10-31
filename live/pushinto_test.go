package live_test

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

type Shop struct {
	ui.Live
	quotes int
}

type shopQuote struct {
	SKU string
	Qty int
}

func (p *Shop) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(ui.JS.Push(p.Price, "sku-1", ui.This.Value()).Into("#price"))}}, ui.Text("price")),
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(ui.JS.Push(p.Total).Into("#total"))}}, ui.Text("total")),
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(ui.JS.Push(p.Bump))}}, ui.Text("bump")),
		ui.El("p", nil, ui.Child(p.quotes)),
	)
}

func (p *Shop) Price(sku string, qty int) (string, error) {
	p.quotes++
	return sku + " x" + strings.Repeat("I", qty), nil
}
func (p *Shop) Total(q shopQuote) ui.Node { return ui.Text(q.SKU) }
func (p *Shop) Bump()                     { p.quotes++ }

// A live page's Push runs on its instance with its arguments in order (or
// a struct from the form), changes its fields, and the patch carries the
// result for the browser to put Into its target.
func TestPushInto(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("shop", nil, &Shop{}) })
	m := s.Handle(live.ClientMessage{Type: live.Join, Ref: 1})
	ids := regexp.MustCompile(`&#34;h&#34;:&#34;(c\d+:\d+)&#34;`).FindAllStringSubmatch(m.HTML, -1)
	if len(ids) != 3 {
		t.Fatalf("handlers: %s", m.HTML)
	}
	if strings.Contains(m.HTML, "&#34;m&#34;") {
		t.Errorf("a live page's push goes over HTTP: %s", m.HTML)
	}
	ev := func(id string, args string, form map[string][]string) live.ServerMessage {
		var e ui.Event
		e.Target, e.Type = id, "click"
		if args != "" {
			json.Unmarshal([]byte(args), &e.Args)
		}
		e.Form = form
		return s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: 2, Event: e})
	}
	m = ev(ids[0][1], `["sku-1","3"]`, nil)
	if m.Type != live.PatchMsg || string(m.Result) != `{"value":"sku-1 xIII"}` || !strings.Contains(m.HTML+string(mustJSON(m.Updates)), "1") {
		t.Fatalf("Price: %+v %s", m, m.Result)
	}
	m = ev(ids[0][1], `["sku-1","three"]`, nil)
	if m.Type != live.Error && m.Error == "" || m.Result != nil {
		t.Errorf("a read that won't convert: %+v", m)
	}
	m = ev(ids[1][1], ``, map[string][]string{"sku": {"abc"}, "qty": {"2"}})
	if string(m.Result) != `{"html":"abc"}` {
		t.Errorf("Total: %+v %s", m, m.Result)
	}
	m = ev(ids[2][1], ``, nil)
	if m.Type != live.PatchMsg || m.Result != nil {
		t.Errorf("Bump: %+v", m)
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
