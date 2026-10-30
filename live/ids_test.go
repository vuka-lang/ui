package live_test

import (
	"context"
	"crypto/rand"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// libraryWidget is a component library's widget: an element given a
// random id when it has none, as templUI and shadcn-templ do, and an
// element of its own that refers to it.
func libraryWidget(label string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		id := "id-" + rand.Text()
		_, err := io.WriteString(w, `<div id="`+id+`" aria-labelledby="`+id+`-label"><span id="`+id+`-label">`+label+`</span></div>`)
		return err
	})
}

type widgets struct {
	ui.Live
	n int
}

func (c *widgets) Inc() { c.n++ }
func (c *widgets) Render() ui.Node {
	return ui.El("section", nil,
		ui.El("button", []ui.Attr{a("onClick", ui.On(c.Inc))}, ui.Text("+")),
		libraryWidget("first "+strconv.Itoa(c.n)),
		libraryWidget("second"))
}

var rawID = regexp.MustCompile(`id-[A-Z2-7]{26}`)

// TestStableLibraryIDs: random ids are named after their place, the same in
// the HTTP render, the join and every patch of the same state.
func TestStableLibraryIDs(t *testing.T) {
	page := func(context.Context) ui.Node { return ui.Component("w", nil, &widgets{}) }
	s := live.NewSession(context.Background(), page)
	first, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if rawID.MatchString(first) {
		t.Fatalf("a random id is left: %s", first)
	}
	if !strings.Contains(first, `<div id="id-c1-1" aria-labelledby="id-c1-1-label"><span id="id-c1-1-label">first 0</span>`) ||
		!strings.Contains(first, `<div id="id-c1-2"`) {
		t.Fatalf("the ids aren't named after their place: %s", first)
	}
	again, _ := s.Render()
	if again != first {
		t.Fatalf("the same state renders differently:\n%s\n%s", first, again)
	}
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if c.HTML() != first {
		t.Fatalf("the join differs from the HTTP render:\n%s\n%s", c.HTML(), first)
	}
	if _, err := c.Event(click(c, "button")); err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(first, "first 0", "first 1", 1); c.HTML() != want {
		t.Fatalf("after the event:\n%s\nwant\n%s", c.HTML(), want)
	}
}
