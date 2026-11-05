package livetest

import (
	"cmp"
	"encoding/json"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

// Streams (ui.Stream): the client keeps, per container, the rows it holds
// and the batch it applied last, and applies each batch the server renders
// once, as a browser does — a batch it applied is ignored, the next one is
// applied, one further ahead is out of step, another generation's resets
// the rows. What it holds is what Stream reads, and what the page's HTML
// has inside each container.

// Row is a row a stream's container holds: its key (data-vk-row), its
// HTML and its text.
type Row struct {
	Key, HTML, Text string
}

type streamState struct {
	gen  string
	n    int
	rows []Row
}

// Stream is the rows the container of the stream name of instance id
// holds, in the page's order: what a browser would show.
func (c *Client) Stream(id, name string) []Row {
	st := c.streams[id+"/"+name]
	if st == nil {
		return nil
	}
	return slices.Clone(st.rows)
}

// Reconnect joins s, a new session for the same page, as a browser does
// when its connection dropped: the page — and the rows its streams hold —
// stays, the session mounts afresh.
func (c *Client) Reconnect(s *live.Session) error {
	c.S, c.trees, c.statics, c.strs = s, nil, nil, nil
	m, err := c.Send(live.ClientMessage{Type: live.Join, V: c.V, Ext: c.Ext})
	if err != nil {
		return err
	}
	if m.Type != live.Render {
		return fmt.Errorf("join: %s %s", m.Type, m.Error)
	}
	return nil
}

// streamed is h with each stream container's content the rows the client
// holds for it; apply applies the batch each container names first.
func (c *Client) streamed(h string, apply bool) (string, error) {
	if !strings.Contains(h, ` data-vk-stream="`) {
		return h, nil
	}
	if c.streams == nil {
		c.streams = map[string]*streamState{}
	}
	var b strings.Builder
	i := 0
	for {
		at := strings.Index(h[i:], ` data-vk-stream="`)
		if at < 0 {
			b.WriteString(h[i:])
			return b.String(), nil
		}
		at += i
		start := strings.LastIndexByte(h[:at], '<')
		el, openEnd := openTag(h, start)
		end := elementEnd(h, start, el.tag)
		if end < 0 {
			return "", fmt.Errorf("no end of the stream container <%s>", el.tag)
		}
		inner := h[openEnd : end-len("</"+el.tag+">")]
		name, _ := el.attr("data-vk-stream")
		key := instanceAt(h, start, openEnd) + "/" + name
		st := c.streams[key]
		if apply {
			var err error
			if st, err = applyBatch(st, el, inner); err != nil {
				return "", fmt.Errorf("stream %s: %w", key, err)
			}
			c.streams[key] = st
		}
		if st == nil {
			st = &streamState{}
		}
		b.WriteString(h[i:start])
		b.WriteString("<" + el.tag)
		for _, a := range el.attrs {
			if a[0] == "data-vk-rows" || a[0] == "data-vk-empty" {
				continue
			}
			b.WriteString(" " + a[0] + `="` + html.EscapeString(a[1]) + `"`)
		}
		b.WriteString(` data-vk-rows="` + strconv.Itoa(len(st.rows)) + `"`)
		if len(st.rows) == 0 {
			b.WriteString(` data-vk-empty=""`)
		}
		b.WriteString(">")
		for _, r := range st.rows {
			b.WriteString(r.HTML)
		}
		b.WriteString("</" + el.tag + ">")
		i = end
	}
}

// instanceAt is the id of the instance whose markup holds the element
// at h[at:], whose open tag ends at open: the nearest element with a
// data-vk-id around it or the element itself (an instance's root), else
// the page's.
func instanceAt(h string, at, open int) string {
	id, best := live.RootID, -1
	for off := 0; ; {
		j := strings.Index(h[off:open], `data-vk-id="`)
		if j < 0 {
			return id
		}
		j += off
		start := strings.LastIndexByte(h[:j], '<')
		el, _ := openTag(h, start)
		if end := elementEnd(h, start, el.tag); end > at && start > best {
			best = start
			id, _ = el.attr("data-vk-id")
		}
		off = j + 1
	}
}

// applyBatch applies the batch a container's attributes name to st, the
// rows the client holds for it (nil: none yet), with inner the rows the
// render carries.
func applyBatch(st *streamState, el *node, inner string) (*streamState, error) {
	batch, _ := el.attr("data-vk-batch")
	gen, ns, _ := strings.Cut(batch, ".")
	n, err := strconv.Atoi(ns)
	if err != nil {
		return nil, fmt.Errorf("bad data-vk-batch %q", batch)
	}
	var ops [][]any
	if o, ok := el.attr("data-vk-ops"); ok && o != "" {
		if err := json.Unmarshal([]byte(o), &ops); err != nil {
			return nil, fmt.Errorf("bad data-vk-ops %q: %w", o, err)
		}
	}
	limit := 0
	if l, ok := el.attr("data-vk-limit"); ok {
		limit, _ = strconv.Atoi(l)
	}
	trim, _ := el.attr("data-vk-trim") // the end the limit trims, "start" or "end"; else the end away from the rows
	cut := func(rows []Row, away string) []Row {
		if limit == 0 || len(rows) <= limit {
			return rows
		}
		if cmp.Or(trim, away) == "start" {
			return slices.Delete(rows, 0, len(rows)-limit)
		}
		return rows[:limit]
	}
	kids := rowsIn(inner)
	switch {
	case st == nil && len(ops) == 0:
		// met for the first time, its batch applied already (the page's
		// HTTP render)
		return &streamState{gen: gen, n: n, rows: kids}, nil
	case st == nil:
		st = &streamState{gen: gen, n: n - 1}
	case st.gen == gen && n <= st.n:
		return st, nil // applied: a morph of the container again
	case st.gen == gen && n > st.n+1:
		return nil, fmt.Errorf("out of step: batch %s after %s.%d", batch, st.gen, st.n)
	case st.gen != gen && (len(ops) == 0 || ops[0][0] != "reset"):
		return nil, fmt.Errorf("batch %s of a new generation doesn't start with a reset", batch)
	}
	byKey := map[string]Row{}
	for _, r := range kids {
		byKey[r.Key] = r
	}
	rows := st.rows
	find := func(k string) int { return slices.IndexFunc(rows, func(r Row) bool { return r.Key == k }) }
	resetting := false
	for i, op := range ops {
		kind, _ := op[0].(string)
		switch kind {
		case "reset":
			resetting = true
			keep := map[string]bool{}
			for _, o := range ops[i+1:] {
				if o[0] == "insert" {
					keep[o[1].(string)] = true
				}
			}
			rows = slices.DeleteFunc(rows, func(r Row) bool { return !keep[r.Key] })
		case "delete":
			if j := find(op[1].(string)); j >= 0 {
				rows = slices.Delete(rows, j, j+1)
			}
		case "insert":
			k := op[1].(string)
			r, ok := byKey[k]
			if !ok {
				return nil, fmt.Errorf("batch %s inserts %s, which it doesn't carry", batch, k)
			}
			pos, _ := op[2].(string)
			j := find(k)
			if j >= 0 && !resetting {
				rows[j] = r // morphed in place
				continue
			}
			if j >= 0 {
				rows = slices.Delete(rows, j, j+1) // moved into the reset's order
			}
			if at, ok := op[2].(float64); ok || pos == "start" { // before row at (InsertAt), or the first
				j := min(max(int(at), 0), len(rows))
				rows = cut(slices.Insert(rows, j, r), "end")
			} else {
				rows = cut(append(rows, r), "start")
			}
		default:
			return nil, fmt.Errorf("batch %s has an unknown operation %v", batch, op)
		}
	}
	rows = cut(rows, "start")
	return &streamState{gen: gen, n: n, rows: slices.Clip(rows)}, nil
}

// streamAttrs is a container's attributes, written back from their
// compact form ({"st":…}) as the server renders them: what a client
// joining with live.ExtStream does.
func streamAttrs(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return live.StreamAttrs(b)
}

// rowsIn is the rows in a container's content: its elements.
func rowsIn(inner string) []Row {
	var out []Row
	for i := 0; i < len(inner); {
		lt := strings.IndexByte(inner[i:], '<')
		if lt < 0 {
			break
		}
		start := i + lt
		el, _ := openTag(inner, start)
		end := elementEnd(inner, start, el.tag)
		if end < 0 {
			break
		}
		k, _ := el.attr("data-vk-row")
		h := inner[start:end]
		out = append(out, Row{Key: k, HTML: h, Text: parse(h).textContent()})
		i = end
	}
	return out
}

// Viewport fires a stream container's viewport event as a browser does
// when the container's edge row comes into view: edge is "top" or
// "bottom", sel the container, and the event's value the key of the row
// at that edge — the first, or the last. A container holding no rows
// fires nothing, as in a browser.
func (c *Client) Viewport(sel, edge string) error {
	e := c.find(sel)
	if e == nil {
		return fmt.Errorf("livetest: no element %s", sel)
	}
	if _, ok := e.attr("data-vk-stream"); !ok {
		return fmt.Errorf("livetest: %s isn't a stream's container", sel)
	}
	if edge != "top" && edge != "bottom" {
		return fmt.Errorf("livetest: edge %q isn't top or bottom", edge)
	}
	var rows []*node
	for _, k := range e.kids {
		if _, ok := k.attr("data-vk-row"); ok && k.tag != "" {
			rows = append(rows, k)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	row := rows[0]
	if edge == "bottom" {
		row = rows[len(rows)-1]
	}
	key, _ := row.attr("data-vk-row")
	return c.Fire(sel, ui.Event{Type: "viewport" + edge, Value: key})
}
