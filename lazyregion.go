package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
)

// LazyProps are what Lazy takes.
type LazyProps struct {
	// When shows the region while it holds, as Show's does: a bool, a
	// ui.State[bool] or an expression of states (the compiler writes a
	// Binding). The content renders the first time it holds.
	When any
	// Each renders the content again each time the browser shows the
	// region, for fresh data, instead of once.
	Each bool
	// Bare leaves showing and hiding to an element around the region that
	// When shows already (a component's lazy attribute: <Modal show={open}
	// lazy>): the region only waits to render its content.
	Bare bool
	// Loading is what shows until the content arrives: by default an empty
	// placeholder, busy, saying "Loading…" to assistive technology.
	Loading  Node
	Children Node
}

// Lazy renders its children only once the browser first shows them, and
// shows them while When holds, as Show does. The page's first render has
// an empty region in their place (Loading, busy): the children aren't
// rendered and their server work — a field's choices, a component's Mount
// — doesn't run. When the browser first shows the region it shows at once,
// Loading in it, and asks for the content: an event over a live page's
// connection, a request to a page that isn't live (the page renders again
// for that region only, RenderRegion). The content stays rendered after
// that, unless Each, which renders it again each time the region shows.
//
//	<ui.Lazy when={open}>
//	  <ProductEditor id={id}/>
//	</ui.Lazy>
//
// A region When shows at the first render renders its content then. A
// render with no runtime to ask for it — ui.String, a static page — and a
// region in a stream row render the content at once. An error rendering the
// content shows in the region, not as the page's.
func Lazy(p LazyProps) Node {
	return ctxNode(func(ctx context.Context) Node {
		open, bind, err := whenOf("Lazy", p.When)
		if err != nil {
			return Try(nil, err)
		}
		attrs := make([]Attr, 0, 8)
		if bind != nil && !p.Bare {
			attrs = append(attrs, Attr{"data-vk-show", bind})
		}
		if open || p.Bare {
			attrs = append(attrs, Attr{"style", "display: contents"})
		} else {
			attrs = append(attrs, Attr{"hidden", true})
		}
		if p.Each {
			attrs = append(attrs, Attr{"data-vk-each", true})
		}
		waiting := func(more ...Attr) Node {
			attrs = append(attrs, Attr{"data-vk-init", true}, Attr{"aria-busy", "true"})
			return El("vk-lazy", append(attrs, more...), lazyLoading(ctx, p.Loading))
		}
		if eager(ctx) {
			return El("vk-lazy", attrs, lazyContent(p.Children))
		}
		switch h := liveHost(ctx).(type) {
		case *pageProbe:
			id := nextRegion(ctx)
			attrs = append(attrs, Attr{"data-vk-region", id})
			content := withContext(func(ctx context.Context) context.Context {
				return context.WithValue(ctx, regionKey{}, &regionScope{prefix: id + "."})
			}, lazyContent(p.Children))
			switch {
			case h.want == id:
				return h.capture(ctx, El("vk-lazy", attrs, content))
			case open || strings.HasPrefix(h.want, id+"."):
				return El("vk-lazy", attrs, content)
			}
			h.info.JS = true // the runtime asks for the content
			return waiting()
		case LazyHost:
			if inStreamRow(ctx) != nil {
				break // a row the server forgets: nothing to render it into later
			}
			key := h.LazyKey(ctx)
			if open {
				h.ShowLazy(key)
			}
			shown := On(InitHandler(func(context.Context) error { h.ShowLazy(key); return nil }))
			if !h.LazyShown(key) {
				return waiting(Attr{"onVkInit", shown})
			}
			if p.Each {
				attrs = append(attrs, Attr{"onVkInit", shown})
			}
		}
		return El("vk-lazy", attrs, lazyContent(p.Children))
	})
}

// LazyHost is a live host keeping which lazy regions the browser showed,
// so their content renders from then on: package live's sessions.
type LazyHost interface {
	// LazyKey names the next region the render meets, the same at each
	// render while the page's markup around it is the same.
	LazyKey(ctx context.Context) string
	LazyShown(key string) bool
	ShowLazy(key string)
}

// lazyContent is a region's content: shown, so the forms in it aren't
// hidden ones, and its error the region's.
func lazyContent(children Node) Node {
	return withContext(func(ctx context.Context) context.Context {
		return context.WithValue(ctx, browserHiddenKey{}, false)
	}, ErrorBoundary(func(err error) Node { return partError(err, nil, nil) }, children))
}

// lazyLoading is what a region shows until its content arrives: in an
// element of its own, which the runtime watches for the region showing.
func lazyLoading(ctx context.Context, loading Node) Node {
	attrs := []Attr{{"class", "vk-lazy-loading"}, {"role", "status"}, {"aria-busy", "true"}}
	if loading == nil {
		attrs = append(attrs, Attr{"aria-label", tr(ctx, "lazy.loading")})
	}
	return El("div", attrs, loading)
}

// A region's id, on a page that isn't live: its place among the regions
// (and hidden forms waiting for their Init) around it in the render,
// counted from 0 — "2" — and, inside another region, the outer region's
// id before it: "2.0". So a region's id is the same at each render of the
// page, whichever regions the render opened.
type regionScope struct {
	prefix string
	n      int
}

type regionKey struct{}

// nextRegion is the id of the next region the render meets.
func nextRegion(ctx context.Context) string {
	sc, _ := ctx.Value(regionKey{}).(*regionScope)
	if sc == nil {
		p, ok := liveHost(ctx).(*pageProbe)
		if !ok {
			return ""
		}
		sc = &p.regions
	}
	id := sc.prefix + strconv.Itoa(sc.n)
	sc.n++
	return id
}

// capture keeps n's HTML as the region RenderRegion asked for, and renders
// it in place.
func (p *pageProbe) capture(ctx context.Context, n Node) Node {
	var b bytes.Buffer
	if err := renderHTML(ctx, &b, n); err != nil {
		return Try(nil, err)
	}
	p.found, p.region = true, b.String()
	return RawHTML(p.region)
}

// ErrNoRegion is RenderRegion's error when the page's render met no region
// of the id asked for: the page has changed since the browser loaded it.
var ErrNoRegion = errors.New("ui: the page has no such lazy region")

// RenderRegion renders n, a page that isn't live, as RenderPage does, and
// returns the HTML of its lazy region id — a Lazy, or a form waiting for
// its form type's Init — with its content: what the runtime asks the page
// for when the browser first shows the region. Its id is the region's
// data-vk-region. The regions around it render open, the others as the
// page's first render has them.
func RenderRegion(ctx context.Context, n Node, id string) (string, error) {
	p := &pageProbe{want: id}
	err := renderHTML(WithLiveHost(ctx, p), io.Discard, n)
	switch {
	case p.found:
		return p.region, nil
	case err != nil && !p.info.Live:
		return "", err
	}
	return "", ErrNoRegion
}

type eagerKey struct{}

// WithEager is ctx rendering lazy regions and hidden forms at once, the
// content and Init they wait for in the browser: a page rendered with no
// browser to ask for them later, such as for a form posted without a
// connection.
func WithEager(ctx context.Context) context.Context {
	return context.WithValue(ctx, eagerKey{}, true)
}

func eager(ctx context.Context) bool {
	v, _ := ctx.Value(eagerKey{}).(bool)
	return v
}
