package live

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
)

// The compact form of a stream container's attributes on a v2 connection
// (the "stream" extension, which a client asks for as it joins): the
// attribute set a container renders — data-vk-stream, data-vk-batch,
// data-vk-limit, data-vk-trim, data-vk-ops and, for a batch starting with
// a reset, data-vk-rows and data-vk-empty — goes as one dynamic of its own,
//
//	{"st":{"n":"messages","b":"q7.12","l":200,"o":[["d","9030"],["e","9031"]]}}
//
// its operations plain JSON rather than JSON inside an HTML attribute
// inside a JSON string, and their names one letter: ["r"] a reset,
// ["d",key] a delete, ["e",key] an insert at the end, ["s",key] one at the
// start, [i,key] one before row i. "tr" is the end the limit trims ("s",
// "e") when the stream fixed it. The client writes the attributes back
// (data-vk-rows and data-vk-empty from the operations, as the server
// does), so the container's HTML — and everything the runtime does with
// it — is the attribute form's. The page's HTTP render, v1 and a client
// joining without the extension get the attributes as they are.

// ExtStream is the v2 extension a client joins with to get stream
// containers' attributes in their compact form.
const ExtStream = "stream"

// Extensions are the v2 extensions this session speaks.
var Extensions = []string{ExtStream}

// streamAttrs writes s, a container's stream attributes as rendered, in
// their compact form; false, writing nothing, when s isn't one the client
// would write back the same.
func (e *enc) streamAttrs(s string) bool {
	attrs, ok := parseAttrs(s)
	if !ok || len(attrs) < 2 || attrs[0].name != "data-vk-stream" || attrs[1].name != "data-vk-batch" {
		return false
	}
	out := struct {
		N  string  `json:"n"`
		B  string  `json:"b"`
		L  int     `json:"l,omitempty"`
		Tr string  `json:"tr,omitempty"`
		O  [][]any `json:"o,omitempty"`
	}{N: attrs[0].value, B: attrs[1].value}
	rest := attrs[2:]
	take := func(name string) (string, bool) {
		if len(rest) > 0 && rest[0].name == name {
			v := rest[0].value
			rest = rest[1:]
			return v, true
		}
		return "", false
	}
	if l, ok := take("data-vk-limit"); ok {
		n, err := strconv.Atoi(l)
		if err != nil || n <= 0 {
			return false
		}
		out.L = n
	}
	if tr, ok := take("data-vk-trim"); ok {
		switch tr {
		case "start", "end":
			out.Tr = tr[:1]
		default:
			return false
		}
	}
	if o, ok := take("data-vk-ops"); ok {
		var ops [][]any
		if json.Unmarshal([]byte(o), &ops) != nil || len(ops) == 0 {
			return false
		}
		for _, op := range ops {
			c, ok := compactOp(op)
			if !ok {
				return false
			}
			out.O = append(out.O, c)
		}
		if out.O[0][0] == "r" {
			// a reset's rows, which the client counts as the server did
			rows := streamRows(out.O, out.L)
			if v, ok := take("data-vk-rows"); !ok || v != strconv.Itoa(rows) {
				return false
			}
			if rows == 0 {
				if _, ok := take("data-vk-empty"); !ok {
					return false
				}
			}
		}
	}
	if len(rest) > 0 {
		return false
	}
	j, err := json.Marshal(out)
	if err != nil {
		return false
	}
	e.raw(`{"st":`)
	e.b = append(e.b, j...)
	e.raw("}")
	return true
}

// compactOp is an operation of data-vk-ops in its compact form.
func compactOp(op []any) ([]any, bool) {
	kind, _ := op[0].(string)
	switch {
	case kind == "reset" && len(op) == 1:
		return []any{"r"}, true
	case kind == "delete" && len(op) == 2:
		k, ok := op[1].(string)
		return []any{"d", k}, ok
	case kind == "insert" && len(op) == 3:
		k, ok := op[1].(string)
		if !ok {
			return nil, false
		}
		switch at := op[2].(type) {
		case string:
			if at == "end" || at == "start" {
				return []any{at[:1], k}, true
			}
		case float64:
			if at >= 0 && at == float64(int(at)) {
				return []any{int(at), k}, true
			}
		}
	}
	return nil, false
}

// streamRows is how many rows a batch starting with a reset leaves: its
// inserts, at most the limit.
func streamRows(ops [][]any, limit int) int {
	n := 0
	for _, op := range ops {
		if op[0] != "r" && op[0] != "d" {
			n++
		}
	}
	if limit > 0 {
		n = min(n, limit)
	}
	return n
}

type attrKV struct{ name, value string }

// parseAttrs reads attributes as the HTML renderer writes them: each
// ` name="value"`, or ` name` alone.
func parseAttrs(s string) ([]attrKV, bool) {
	var out []attrKV
	for s != "" {
		if s[0] != ' ' {
			return nil, false
		}
		s = s[1:]
		i := strings.IndexAny(s, ` ="`)
		if i < 0 {
			out = append(out, attrKV{name: s})
			break
		}
		name := s[:i]
		s = s[i:]
		if name == "" {
			return nil, false
		}
		if !strings.HasPrefix(s, `="`) {
			if s != "" && s[0] != ' ' {
				return nil, false
			}
			out = append(out, attrKV{name: name})
			continue
		}
		s = s[2:]
		end := strings.IndexByte(s, '"')
		if end < 0 {
			return nil, false
		}
		out = append(out, attrKV{name, html.UnescapeString(s[:end])})
		s = s[end+1:]
	}
	return out, true
}

// hasStream reports whether markup holds a stream container: its rows
// and its batch are sent once, never kept by reference.
func hasStream(s string) bool { return strings.Contains(s, ` data-vk-stream="`) }
