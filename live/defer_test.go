package live

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/vuka"
)

type deferPage struct {
	ui.Live
	Title string         `json:"title"`
	Stats map[string]int `json:"stats" defer:""`
	Rows  []string       `json:"rows" defer:""`
}

func (p *deferPage) Render() ui.Node { return ui.IslandPage(p) }

func (p *deferPage) LoadStats(ctx context.Context) error {
	p.Stats = map[string]int{"open": 3}
	return nil
}

func (p *deferPage) LoadRows(ctx context.Context) error { return errors.New("db: connection refused") }

func init() { ui.Island("islands/Orders.vue")(vuka.TypeOf[deferPage]("live.deferPage")) }

func TestSessionDeferred(t *testing.T) {
	s := NewSession(context.Background(), func(ctx context.Context) ui.Node {
		return ui.Component("page", nil, &deferPage{Title: "Orders"})
	})
	s.SetErrorText(func(err error) string { return "Internal Server Error" })
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "open") || !strings.Contains(html, "&#34;p&#34;:[&#34;stats&#34;,&#34;rows&#34;]") {
		t.Fatalf("first render: %s", html)
	}
	ds := s.Deferred()
	if len(ds) != 2 || len(s.Deferred()) != 0 {
		t.Fatalf("deferred %d", len(ds))
	}
	var got []string
	for _, d := range ds {
		d.Run(context.Background())
		p, err := s.Loaded(d)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Updates) != 1 {
			t.Fatalf("patch %+v", p)
		}
		got = append(got, p.Updates[0].HTML)
	}
	if !strings.Contains(got[0], "&#34;stats&#34;:{&#34;open&#34;:3}") {
		t.Errorf("stats: %s", got[0])
	}
	if !strings.Contains(got[1], "&#34;e&#34;:{&#34;rows&#34;:{&#34;message&#34;:&#34;Internal Server Error&#34;}}") || strings.Contains(got[1], "connection refused") {
		t.Errorf("rows' error: %s", got[1])
	}
}
