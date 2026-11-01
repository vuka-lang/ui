package live

import (
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/vuka-lang/ui/v2"
)

// Component libraries (templUI, shadcn-templ) give an element they render
// without an id a random one, "id-" + crypto/rand.Text(), so the same state
// renders differently every time: each patch would carry new ids and the
// library's scripts would lose their elements. A session renames them after
// their place: the instance that renders them and how many came before in
// its markup, the same in the page's HTTP render, its join and every patch
// of the same state. A raw id met again in the same pass — written twice, or
// handed to a child instance — keeps the name it was first given.

const (
	randomIDPrefix = "id-"
	randomIDLen    = 26 // crypto/rand.Text: base32, A–Z and 2–7
)

// ids are a pass's names for the random ids it met.
type ids struct {
	pass  int
	names map[string]string
}

// stableIDs renames the random ids in s, an instance's markup.
func (s *Session) stableIDs(in *instance, text string) string {
	i := nextRandomID(text, 0)
	if i < 0 {
		return text
	}
	if s.ids.pass != s.pass || s.ids.names == nil {
		s.ids = ids{pass: s.pass, names: map[string]string{}}
	}
	var b strings.Builder
	last := 0
	for ; i >= 0; i = nextRandomID(text, i+1) {
		end := i + len(randomIDPrefix) + randomIDLen
		raw := text[i:end]
		name, ok := s.ids.names[raw]
		if !ok {
			in.idn++
			name = randomIDPrefix + in.id + "-" + strconv.Itoa(in.idn)
			s.ids.names[raw] = name
		}
		b.WriteString(text[last:i])
		b.WriteString(name)
		last = end
	}
	b.WriteString(text[last:])
	return b.String()
}

// stableTreeIDs renames the random ids in t's dynamics, in the order its
// HTML has them.
func (s *Session) stableTreeIDs(in *instance, t *ui.Tree) {
	for i, d := range t.Dyn {
		switch d := d.(type) {
		case string:
			t.Dyn[i] = s.stableIDs(in, d)
		case *ui.Tree:
			s.stableTreeIDs(in, d)
		case *ui.TreeList:
			for _, it := range d.Items {
				s.stableTreeIDs(in, it)
			}
		}
	}
}

// nextRandomID is the offset of the next random id in s from from, or -1.
func nextRandomID(s string, from int) int {
	for from < len(s) {
		j := strings.Index(s[from:], randomIDPrefix)
		if j < 0 {
			return -1
		}
		if i := from + j; isRandomID(s, i) {
			return i
		}
		from += j + 1
	}
	return -1
}

// isRandomID reports whether s holds a random id at i: "id-" and 26
// characters of rand.Text's alphabet, not part of a longer word.
func isRandomID(s string, i int) bool {
	end := i + len(randomIDPrefix) + randomIDLen
	if end > len(s) || i > 0 && idChar(s[i-1]) || end < len(s) && idChar(s[end]) && s[end] != '-' {
		return false
	}
	for _, c := range []byte(s[i+len(randomIDPrefix) : end]) {
		if !(c >= 'A' && c <= 'Z' || c >= '2' && c <= '7') {
			return false
		}
	}
	return true
}

func idChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}

// A handler's id inside a key is "c1:" + its path + ":" + its number. The
// path is each key around it, outermost first, joined by "/", and written
// in letters, digits, "_.-" and a few marks, so that it needs no escaping in
// an attribute, in JSON or in a selector whatever the key holds, and two keys
// never write alike:
//
//	7                    an int
//	~7                   an int64
//	row-42               a string; %XX for any other byte, or a first digit or "-"
//	~~<22 characters>    the hash of any other key (it would show a Go type's
//	                     name), a string longer than maxKeyLen or an empty one
//
// A string's "id-" is written "id%2D": stableIDs must not take a key for a
// library's random id. "!" starts a duplicate key's path (keyDups). "^3/"
// starts the path of an instance mounted across pages, here three: key 7 of
// the next page is no row of this one, and an id sent before a navigation
// must not reach it.

// maxKeyLen is how long a string key is written before its hash stands in.
const maxKeyLen = 48

// keyPath is the path of a key of in's render inside the keys of path.
func keyPath(in *instance, path string, key any) string {
	if path == "" {
		if in.pageKeys == 0 {
			return keySegment(key)
		}
		path = "^" + strconv.Itoa(in.pageKeys)
	}
	return path + "/" + keySegment(key)
}

func keySegment(key any) string {
	switch k := key.(type) {
	case int:
		return strconv.Itoa(k)
	case int64:
		return "~" + strconv.FormatInt(k, 10)
	case string:
		if seg := escapeKey(k); seg != "" && len(seg) <= maxKeyLen {
			return seg
		}
	}
	// 128 bits, so no two keys a page can hold meet.
	h := sha256.Sum256([]byte(ui.KeyText(key)))
	return "~~" + base64.RawURLEncoding.EncodeToString(h[:16])
}

func escapeKey(s string) string {
	safe := func(i int) bool {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == '.':
			return true
		case c >= '0' && c <= '9':
			return i > 0
		case c == '-':
			return i > 0 && !(i >= 2 && s[i-2:i] == "id")
		}
		return false
	}
	n := 0
	for i := range len(s) {
		if !safe(i) {
			n++
		}
	}
	if n == 0 {
		return s
	}
	const hex = "0123456789ABCDEF"
	b := make([]byte, 0, len(s)+2*n)
	for i := range len(s) {
		if safe(i) {
			b = append(b, s[i])
		} else {
			b = append(b, '%', hex[s[i]>>4], hex[s[i]&15])
		}
	}
	return string(b)
}

// keyedHandler registers fn under the keys around sc and returns its id.
func (in *instance) keyedHandler(sc *scope, fn any) string {
	if in.keyed == nil {
		in.keyed = map[string]any{}
	}
	if !in.dups.has(sc.path) {
		id := in.id + ":" + sc.path + ":" + strconv.Itoa(*sc.n)
		name := id[len(in.id)+1:]
		if _, taken := in.keyed[name]; !taken {
			in.keyed[name] = fn
			*sc.n++
			return id
		}
	}
	name := in.dups.next(sc.path, in.s.pass)
	in.keyed[name] = fn
	return in.id + ":" + name
}

// keyDups are the keys two items of an instance's render share: a list's
// duplicates, or a key of two lists. The morph pairs such items by position
// (keyedPlan), so their handlers go by position too — their number among
// those handlers — under a generation: "c1:!3:0". An item's id then can't
// tell it from its twin, so the generation moves whenever which handlers
// those are does, and an id of the last one names nothing.
//
// A render finds out only as it meets the second item, the first having
// named its handlers by their key, and which handlers it has only at its
// end; when they changed, the session renders the page again, once (renderAll),
// so that what it sends has the new generation throughout. A key stays
// duplicate for as long as the renders have handlers under it, even one.
type keyDups struct {
	keys  map[string]int // each duplicate key: the pass that last had a handler under it
	gen   int
	n     int
	sig   uint64 // which handlers this render's are, in order
	shown uint64 // and the last render's, whose generation it is
}

func (d *keyDups) has(path string) bool {
	_, ok := d.keys[path]
	return ok
}

func (d *keyDups) begin() { d.n, d.sig = 0, 0 }

// next is the name of a handler under a duplicate key.
func (d *keyDups) next(path string, pass int) string {
	if d.keys == nil {
		d.keys = map[string]int{}
	}
	d.keys[path] = pass
	d.sig = d.sig*31 + sig(path)
	name := "!" + strconv.Itoa(d.gen) + ":" + strconv.Itoa(d.n)
	d.n++
	return name
}

// end ends a render of the instance.
func (d *keyDups) end(s *Session) {
	for k, pass := range d.keys {
		if pass != s.pass {
			delete(d.keys, k)
		}
	}
	if d.sig == d.shown {
		return
	}
	d.shown = d.sig
	if d.sig != 0 && !s.redo {
		d.gen++
		s.again = true
	}
}
