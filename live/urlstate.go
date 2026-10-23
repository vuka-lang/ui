package live

import (
	"context"
	"net/url"

	"github.com/vuka-lang/ui/v2"
)

// The page's URL state: what its components' @ui.URL fields read, and what
// an event changes of it — by assigning such a field, or through
// ui.EditURL (web's Query) — sent as a patch of the page, the other
// parameters kept, once the event's reply is applied. The patch takes the
// current history entry's place unless an edit asked for a new one.

// pageURL is the page's URL as its events see it: the one an edit made,
// else the one its context carries (ui.PageURL); nil when unknown.
func (s *Session) pageURL() *url.URL {
	if s.url != nil {
		return s.url
	}
	return ui.PageURL(s.ctx)
}

// query is the page's query, parsed once per URL.
func (s *Session) query() url.Values {
	if s.q == nil {
		s.q = url.Values{}
		if u := s.pageURL(); u != nil {
			s.q = u.Query()
		}
	}
	return s.q
}

// setURL makes u the page's URL, as a navigation brought it; nil: the one
// the session's context carries.
func (s *Session) setURL(u *url.URL) { s.url, s.q = u, nil }

// urlEdit is the ui.URLEditor of one event: the edits its code makes, of
// the session's query.
type urlEdit struct {
	s       *Session
	q       url.Values
	changed bool
	push    bool
}

func (e *urlEdit) EditURL(edit func(url.Values), push bool) {
	edit(e.q)
	e.changed = true
	e.push = e.push || push
}

func (e *urlEdit) Query() url.Values { return cloneValues(e.q) }

// beginURL starts an event: ctx carrying its editor, and each instance's
// URL-bound fields recorded, to see which the event changes.
func (s *Session) beginURL(ctx context.Context) (context.Context, *urlEdit) {
	e := &urlEdit{s: s, q: cloneValues(s.query())}
	for _, in := range s.order() {
		if in.comp != nil && ui.HasURLFields(in.comp) {
			in.url.Snapshot(in.comp)
		}
	}
	return ui.WithURLEditor(ctx, e), e
}

// endURL ends an event: the URL-bound fields it changed join its edits, and
// when the query changed the session's URL is the new one, for the render
// to read; the returned push is the patch to send once the reply is
// applied, nil when there is none (or the page's URL isn't known).
func (s *Session) endURL(e *urlEdit) []ui.Commands {
	for _, in := range s.order() {
		if in.comp != nil && ui.HasURLFields(in.comp) {
			if in.url.Changes(in.comp, in.src, in.urlKey, e.q) {
				e.changed = true
			}
		}
	}
	cur := s.pageURL()
	if !e.changed || cur == nil {
		return nil
	}
	enc := e.q.Encode()
	if enc == s.query().Encode() {
		return nil
	}
	next := &url.URL{Path: cur.Path, RawPath: cur.RawPath, RawQuery: enc}
	s.setURL(next)
	href := next.RequestURI()
	if e.push {
		return []ui.Commands{ui.JS.Patch(href)}
	}
	return []ui.Commands{ui.JS.PatchReplace(href)}
}

// fillURL sets the URL-bound fields of in's component from the page's URL
// as its tag n renders; it reports whether a prop changed, which the
// render's props comparison missed.
func (s *Session) fillURL(in *instance, n *ui.ComponentNode) bool {
	if in.comp == nil || ui.IsIsland(in.typ) || !ui.HasURLFields(in.comp) {
		return false
	}
	if in.urlKey == "" {
		in.urlKey = n.URLKey
	}
	return in.url.Fill(s.ctx, in.comp, n.Props, in.urlKey, n.Routed, s.query())
}

func cloneValues(q url.Values) url.Values {
	out := make(url.Values, len(q))
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}
