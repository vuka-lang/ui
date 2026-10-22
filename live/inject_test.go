package live_test

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

type notes struct{ list []string }

type injector struct{ n *notes }

func (i injector) Inject(t reflect.Type) func(context.Context) (reflect.Value, error) {
	if t != reflect.TypeFor[*notes]() {
		return nil
	}
	return func(context.Context) (reflect.Value, error) { return reflect.ValueOf(i.n), nil }
}

type noteForm struct {
	Text string `form:"text" validate:"required"`
}

// noteBoard's handlers take the event's payload and a service, by type.
type noteBoard struct {
	ui.Live
	shown string
}

func (b *noteBoard) Add(ctx context.Context, f noteForm, n *notes) error {
	n.list = append(n.list, f.Text)
	b.shown = strings.Join(n.list, ",")
	return nil
}

func (b *noteBoard) Clear(n *notes, v url.Values) { n.list = nil; b.shown = "cleared " + v.Get("why") }

func (b *noteBoard) Render() ui.Node {
	return ui.El("div", nil,
		ui.El("form", []ui.Attr{{Name: "onSubmit", Value: ui.On(b.Add)}}),
		ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(b.Clear)}}),
		ui.El("p", nil, ui.Text(b.shown)))
}

func TestInjectedHandlers(t *testing.T) {
	n := &notes{}
	ctx := ui.WithInjector(context.Background(), injector{n})
	s := live.NewSession(ctx, func(context.Context) ui.Node { return ui.Component("b", nil, &noteBoard{}) })
	defer s.Close()
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if p, err := s.Event(ui.Event{Target: "c1:0", Type: "submit", Form: url.Values{"text": {"milk"}}}); err != nil || p.Error != "" {
		t.Fatalf("Add: %v %q", err, p.Error)
	}
	if !strings.Contains(s.HTML(), "<p>milk</p>") || len(n.list) != 1 {
		t.Errorf("after Add: %s, %q", s.HTML(), n.list)
	}
	if p, _ := s.Event(ui.Event{Target: "c1:0", Type: "submit", Form: url.Values{}}); p.Error == "" {
		t.Error("an invalid form ran the handler")
	}
	if _, err := s.Event(ui.Event{Target: "c1:1", Type: "click", Form: url.Values{"why": {"done"}}}); err != nil || !strings.Contains(s.HTML(), "<p>cleared done</p>") {
		t.Errorf("Clear: %v, %s", err, s.HTML())
	}

	// A session whose context carries no injector can't fill the service.
	bare := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("b", nil, &noteBoard{}) })
	defer bare.Close()
	bare.Render()
	if p, _ := bare.Event(ui.Event{Target: "c1:1", Type: "click"}); p.Error == "" {
		t.Errorf("no injector: %q", p.Error)
	}
}
