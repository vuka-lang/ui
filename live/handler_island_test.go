package live

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/vuka-lang/ui/v2"
)

// An island function prop's argument arrives as the event's JSON data; a
// scalar handler parameter takes it as it takes an event's value.
func TestHandlerScalarData(t *testing.T) {
	var gotS string
	var gotN int
	var gotB bool
	var gotF float64
	for _, c := range []struct {
		fn   any
		data string
	}{
		{func(s string) { gotS = s }, `"mon"`},
		{func(_ context.Context, n int) error { gotN = n; return nil }, `7`},
		{func(b bool) { gotB = b }, `true`},
		{func(f float64) { gotF = f }, `2.5`},
	} {
		if err := callHandler(context.Background(), c.fn, ui.Event{Target: "c1:0", Type: "emit", Data: json.RawMessage(c.data)}); err != nil {
			t.Fatal(err)
		}
	}
	if gotS != "mon" || gotN != 7 || !gotB || gotF != 2.5 {
		t.Errorf("got %q %d %v %v", gotS, gotN, gotB, gotF)
	}
	if err := callHandler(context.Background(), func(int) {}, ui.Event{Target: "c1:0", Type: "emit", Data: json.RawMessage(`{"a":1}`)}); err == nil {
		t.Error("an object into an int should fail")
	}
}
