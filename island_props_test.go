package ui_test

import (
	"context"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/uigen"
	"github.com/vuka-lang/vuka"
)

type picker struct {
	Day      string               `json:"day"`
	Max      *float64             `json:"max,omitempty"`
	Open     bool                 `json:"open"`
	When     time.Time            `json:"when"`
	Events   []pickerEvent        `json:"events"`
	OnSelect func(day string)     `json:"onSelect"`
	OnClose  ui.Commands          `json:"onClose"`
	Model    ui.State[string]     `json:"model"`
	Extra    map[string]time.Time `json:"extra,omitempty"`
}

type pickerEvent struct {
	At    time.Time `json:"at"`
	Title string    `json:"title"`
}

func init() { ui.Island("islands/Picker.vue")(vuka.TypeOf[picker]("app.picker")) }

func props(t *testing.T, s string) string {
	t.Helper()
	i := strings.Index(s, `data-vk-props="`)
	if i < 0 {
		t.Fatalf("no data-vk-props in %s", s)
	}
	rest := s[i+len(`data-vk-props="`):]
	return html.UnescapeString(rest[:strings.IndexByte(rest, '"')])
}

func TestIslandTaggedProps(t *testing.T) {
	when := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	open := uigen.State("o1", true)
	tab := uigen.State("t1", "a")
	s, info := page(t, ui.MountIsland("p#1", picker{
		Day:     "mon",
		When:    when,
		Events:  []pickerEvent{{At: when, Title: "x"}},
		OnClose: open.Set(false),
		Model:   tab,
	}, ui.StateProp("open", open), ui.Callback("onPick", uigen.Toggle(ui.JS, open)), ui.Once()))
	p := props(t, s)
	for _, want := range []string{
		`"day":"mon"`,
		`"when":{"$vk":"date","v":"2026-10-15T09:00:00Z"}`,
		`"events":[{"at":{"$vk":"date","v":"2026-10-15T09:00:00Z"},"title":"x"}]`,
		`"onClose":{"$vk":"fn","e":"onclose"}`,
		`"onPick":{"$vk":"fn","e":"onpick"}`,
		`"open":{"$vk":"state","i":true,"s":"c0:o1:1"}`,
		`"model":{"$vk":"state","i":"a","s":"c0:t1:1"}`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("props lack %s:\n%s", want, p)
		}
	}
	for _, no := range []string{`"max"`, `"extra"`, `"onSelect"`} {
		if strings.Contains(p, no) {
			t.Errorf("props have %s:\n%s", no, p)
		}
	}
	for _, want := range []string{`data-vk-js-onclose="`, `data-vk-js-onpick="`, `data-vk-once`} {
		if !strings.Contains(s, want) {
			t.Errorf("host lacks %s:\n%s", want, s)
		}
	}
	if !info.JS || info.Live {
		t.Errorf("info = %+v; want JS, not live", info)
	}
}

func TestIslandFuncPropIsLive(t *testing.T) {
	s, info := page(t, ui.MountIsland("p#1", picker{OnSelect: func(string) {}, Max: ui.Ptr(3.0)}))
	if !info.Live {
		t.Errorf("a handler prop makes the page live: %+v", info)
	}
	_ = s
	s, _ = page(t, ui.MountIsland("p#1", picker{Max: ui.Ptr(3.0)}, ui.Callback("onSelect", nil)))
	if p := props(t, s); !strings.Contains(p, `"max":3`) {
		t.Errorf("a pointer prop sends its value: %s", p)
	}
}

func TestIslandDerivedPropReadOnly(t *testing.T) {
	n := uigen.State("n1", 2)
	d := uigen.Plus(n, n)
	s, _ := page(t, ui.MountIsland("p#1", picker{}, ui.StateProp("double", d)))
	if p := props(t, s); !strings.Contains(p, `"double":{"$vk":"state","x":["+",{"i":2,"s":"c0:n1:1"},{"i":2,"s":"c0:n1:1"}]}`) {
		t.Errorf("derived prop: %s", p)
	}
}

func TestIslandPropsRenderInContext(t *testing.T) {
	// Outside RenderPage the props still render (states keyed by site).
	s, err := ui.String(context.Background(), ui.MountIsland("p#1", picker{}, ui.StateProp("open", uigen.State("o2", false))))
	if err != nil || !strings.Contains(props(t, s), `"open":{"$vk":"state","i":false,"s":"o2:`) {
		t.Errorf("%v %s", err, s)
	}
}
