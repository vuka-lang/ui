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

// pageCtx is the session's context with the page's URL as an event left
// it, so what a render builds from the URL (pagers, sort links) is current.
func (s *Session) pageCtx() context.Context {
	if s.url != nil {
		return ui.WithPageURLReplaced(s.ctx, s.url)
	}
	return s.ctx
}

// tableParams are the parameters the DataTables the rendered instances
// resolved keep in the address.
func (s *Session) tableParams() map[string]bool {
	out := map[string]bool{}
	for _, in := range s.order() {
		if in.tables != nil {
			for k := range in.tables.Keys() {
				for _, p := range ui.TableParams(k) {
					out[p] = true
				}
			}
		}
	}
	return out
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
	had     map[string]bool // the parameters the instances rendered before the event keep
	write   bool            // the edits changed the address
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
	e := &urlEdit{s: s, q: cloneValues(s.query()), had: s.urlParams()}
	s.afterRender = nil
	for _, in := range s.order() {
		if in.comp != nil && ui.HasURLFields(in.comp) {
			in.url.Snapshot(in.comp)
		}
	}
	return ui.WithURLEditor(ctx, e), e
}

// urlParams are the parameters the rendered instances keep in the address.
func (s *Session) urlParams() map[string]bool {
	out := map[string]bool{}
	for _, in := range s.order() {
		if in.comp != nil && ui.HasURLFields(in.comp) {
			for _, k := range ui.URLParams(in.comp, in.src, in.urlKey) {
				out[k] = true
			}
		}
	}
	for k := range s.tableParams() {
		out[k] = true
	}
	return out
}

// endURL ends an event: the URL-bound fields it changed join its edits, and
// when the query changed the session's URL is the new one, for the render
// to read. The patch to send is queued for after the render (finishURL),
// which takes out the parameters of the instances it no longer renders.
func (s *Session) endURL(e *urlEdit) {
	for _, in := range s.order() {
		if in.comp != nil && ui.HasURLFields(in.comp) {
			if in.url.Changes(in.comp, in.src, in.urlKey, e.q) {
				e.changed = true
			}
		}
	}
	cur := s.pageURL()
	if cur == nil {
		return
	}
	if e.changed {
		if enc := e.q.Encode(); enc != s.query().Encode() {
			s.setURL(&url.URL{Path: cur.Path, RawPath: cur.RawPath, RawQuery: enc})
			e.write = true
		}
	}
	s.afterRender = append(s.afterRender, func() { s.finishURL(e) })
}

// finishURL, once the event's render is done, drops from the address the
// parameters of the instances that render no longer holds, and sends the
// patch for it and the event's edits.
func (s *Session) finishURL(e *urlEdit) {
	cur := s.pageURL()
	if cur == nil {
		return
	}
	q := cloneValues(s.query())
	if len(e.had) > 0 {
		now := s.urlParams()
		for k := range e.had {
			if _, ok := q[k]; ok && !now[k] {
				delete(q, k)
				e.write = true
			}
		}
	}
	if !e.write {
		return
	}
	next := &url.URL{Path: cur.Path, RawPath: cur.RawPath, RawQuery: q.Encode()}
	s.setURL(next)
	if e.push {
		s.root.js = append(s.root.js, ui.JS.Patch(next.RequestURI()))
		return
	}
	s.root.js = append(s.root.js, ui.JS.PatchReplace(next.RequestURI()))
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
