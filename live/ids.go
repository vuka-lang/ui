package live

import (
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
