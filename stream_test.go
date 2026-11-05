package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

type streamItem struct {
	ID   int
	Text string
}

// sent is the operations the next batch of s carries, each as the
// browser reads it, and the rows' texts in their order; the batch is then
// flushed, as a written reply flushes it.
func sent(t *testing.T, s *Stream[streamItem]) (ops [][]any, texts []string) {
	t.Helper()
	var rows []Node
	for _, r := range s.Rows() {
		rows = append(rows, El("li", []Attr{{Name: "key", Value: r.ID}}, Text(r.Text)))
	}
	b := s.Block(rows)
	if b.err != nil {
		t.Fatal(b.err)
	}
	for _, a := range b.Attrs() {
		if a.Name == "data-vk-ops" {
			if err := json.Unmarshal([]byte(fmt.Sprint(a.Value)), &ops); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, op := range b.ops {
		if op.node != nil {
			h, err := String(context.Background(), op.node)
			if err != nil {
				t.Fatal(err)
			}
			texts = append(texts, h)
		}
	}
	s.shown = true
	s.streamFlush()
	return ops, texts
}

func newTestStream(t *testing.T) *Stream[streamItem] {
	t.Helper()
	s := &Stream[streamItem]{}
	s.streamInit("items")
	sent(t, s) // the mount's reset
	return s
}

func wantOps(t *testing.T, got [][]any, want ...[]any) {
	t.Helper()
	if len(want) == 0 && len(got) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %v, want %v", got, want)
	}
}

func items(ids ...int) []streamItem {
	out := make([]streamItem, len(ids))
	for i, id := range ids {
		out[i] = streamItem{id, fmt.Sprint("r", id)}
	}
	return out
}

// InsertAt puts its rows before the browser's row i, in the order given:
// each goes at i, last first; a negative i is 0, and one past the rows is
// sent as is, the browser putting the rows at the end.
func TestStreamInsertAt(t *testing.T) {
	s := newTestStream(t)
	s.InsertAt(2, items(7, 8)...)
	if got := s.Rows(); !reflect.DeepEqual(got, items(8, 7)) {
		t.Fatalf("Rows %v, want 8 then 7: the order their operations apply", got)
	}
	ops, _ := sent(t, s)
	wantOps(t, ops, []any{"insert", "8", 2.0}, []any{"insert", "7", 2.0})

	s.InsertAt(-3, items(1)...)
	s.InsertAt(99, items(2)...)
	ops, _ = sent(t, s)
	wantOps(t, ops, []any{"insert", "1", 0.0}, []any{"insert", "2", 99.0})

	s.InsertAt(0)
	if ops, _ = sent(t, s); ops != nil {
		t.Fatalf("an InsertAt of no rows sends %v", ops)
	}
}

// A batch names a key once: a second insert of a key keeps the first's
// position with the second's row, a delete after an insert drops the
// insert, an insert after a delete is a move, and a reset drops what was
// queued before it — InsertAt among them as any insert.
func TestStreamInsertAtCompaction(t *testing.T) {
	s := newTestStream(t)
	s.Insert(streamItem{1, "first"})
	s.InsertAt(3, streamItem{1, "second"})
	ops, texts := sent(t, s)
	wantOps(t, ops, []any{"insert", "1", "end"})
	if want := []string{"<li>second</li>"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("rows %v, want %v", texts, want)
	}

	s.InsertAt(1, streamItem{2, "two"})
	s.Insert(streamItem{2, "two!"})
	ops, texts = sent(t, s)
	wantOps(t, ops, []any{"insert", "2", 1.0})
	if want := []string{"<li>two!</li>"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("rows %v, want %v", texts, want)
	}

	s.InsertAt(1, items(3)...)
	s.Delete(3)
	ops, _ = sent(t, s)
	wantOps(t, ops, []any{"delete", "3"})

	s.Delete(4)
	s.InsertAt(0, items(4)...)
	ops, _ = sent(t, s)
	wantOps(t, ops, []any{"delete", "4"}, []any{"insert", "4", 0.0})

	s.InsertAt(2, items(5, 6)...)
	s.Reset(items(9)...)
	ops, _ = sent(t, s)
	wantOps(t, ops, []any{"reset"}, []any{"insert", "9", "end"})
}

// A limit bounds the inserts queued at the stream's ends, never
// InsertAt's: where those end up depends on the rows the browser holds.
func TestStreamInsertAtLimit(t *testing.T) {
	s := newTestStream(t)
	s.Limit(2)
	s.InsertAt(1, items(1, 2, 3)...)
	s.Prepend(items(4, 5, 6)...)
	ops, _ := sent(t, s)
	wantOps(t, ops,
		[]any{"insert", "3", 1.0}, []any{"insert", "2", 1.0}, []any{"insert", "1", 1.0},
		[]any{"insert", "5", "start"}, []any{"insert", "4", "start"})
}

// A limit whose end is fixed (TrimStart, TrimEnd) keeps a Reset's rows at
// the other end, and bounds the queue at each end by what the browser keeps
// of it: inserts going in at the end it trims are dropped newest first.
func TestStreamTrimEnd(t *testing.T) {
	s := newTestStream(t)
	s.Limit(3).TrimEnd()
	s.Reset(items(1, 2, 3, 4, 5)...)
	_, texts := sent(t, s)
	if len(texts) != 3 || texts[0] != `<li>r1</li>` || texts[2] != `<li>r3</li>` {
		t.Fatalf("a reset trimming the end keeps %v", texts)
	}
	s.Limit(3).TrimStart()
	s.Reset(items(1, 2, 3, 4, 5)...)
	_, texts = sent(t, s)
	if len(texts) != 3 || texts[0] != `<li>r3</li>` || texts[2] != `<li>r5</li>` {
		t.Fatalf("a reset trimming the start keeps %v", texts)
	}

	s.Limit(2).TrimEnd()
	s.Insert(items(1, 2, 3)...)
	s.Prepend(items(4, 5, 6)...)
	ops, _ := sent(t, s)
	wantOps(t, ops,
		[]any{"insert", "1", "end"}, []any{"insert", "2", "end"},
		[]any{"insert", "5", "start"}, []any{"insert", "4", "start"})
	s.Limit(2).TrimStart()
	s.Insert(items(1, 2, 3)...)
	s.Prepend(items(4, 5, 6)...)
	ops, _ = sent(t, s)
	wantOps(t, ops,
		[]any{"insert", "2", "end"}, []any{"insert", "3", "end"},
		[]any{"insert", "6", "start"}, []any{"insert", "5", "start"})

	b := s.Block(nil)
	var trim any
	for _, a := range b.Attrs() {
		if a.Name == "data-vk-trim" {
			trim = a.Value
		}
	}
	if trim != "start" {
		t.Fatalf("data-vk-trim is %v", trim)
	}
	s.Limit(2)
	for _, a := range s.Block(nil).Attrs() {
		if a.Name == "data-vk-trim" {
			t.Fatal("Limit again leaves the end it trims fixed")
		}
	}
}
