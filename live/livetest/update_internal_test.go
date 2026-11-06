package livetest

import (
	"encoding/json"
	"testing"
)

// TestUpdateInIndexOrder: a change's dynamics are applied in index order,
// as the browser applies them, so a string defined by reference at a lower
// index is there for a higher one, however Go orders the map.
func TestUpdateInIndexOrder(t *testing.T) {
	for range 20 {
		c := &Client{strs: map[int]string{}}
		f := &frame{d: make([]any, 13)}
		var u any
		json.Unmarshal([]byte(`{"3":{"r":82,"v":" hidden"},"12":{"r":82}}`), &u)
		if err := c.update(f, u); err != nil {
			t.Fatal(err)
		}
		if f.d[3] != " hidden" || f.d[12] != " hidden" {
			t.Fatalf("dynamics 3 and 12: %q, %q", f.d[3], f.d[12])
		}
	}
}
