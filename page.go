package ui

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
)

// PageInfo is what RenderPage learned of a page while rendering it.
type PageInfo struct {
	// Live: the page renders a stateful component, an event handler or a JS
	// command pushing to one, so it takes a live session (package live),
	// whose render writes it differently. RenderPage stops at the first.
	Live bool
	// JS: the page has JS commands (data-vk-js-<event>), which the
	// runtime script runs.
	JS bool
	// Opaque: the page holds HTML the renderer didn't write, a templ
	// component's (or another opaque Node's) or Safe HTML naming a data-vk-
	// attribute, which only its HTML tells live or JS apart.
	Opaque bool
}

// RenderPage writes n to w as a live session's first render of a page
// writes it when nothing in it is live (keyed elements marked
// data-vk-key), and reports what it met. When info.Live it stops at the
// first live thing, w holding part of the page, and the page is the
// session's to render.
func RenderPage(ctx context.Context, n Node, w io.Writer) (info PageInfo, err error) {
	p := &pageProbe{}
	err = renderHTML(WithLiveHost(ctx, p), w, n)
	if p.info.Live {
		return p.info, nil
	}
	return p.info, err
}

// pageProbe is RenderPage's LiveHost: it renders nothing live, noting
// what it meets.
type pageProbe struct {
	info   PageInfo
	states map[string]int
}

// StateKey names a state as a live session names those of its page (c0),
// so the page's states keep their keys when a session renders it again.
func (p *pageProbe) StateKey(_ context.Context, site string) string {
	if p.states == nil {
		p.states = map[string]int{}
	}
	p.states[site]++
	return "c0:" + site + ":" + strconv.Itoa(p.states[site])
}

var errPageLive = errors.New("ui: the page is live")

func (p *pageProbe) Component(context.Context, *ComponentNode, io.Writer) error {
	p.info.Live = true
	return errPageLive
}

func (p *pageProbe) Handler(context.Context, any) string {
	p.info.Live = true
	return ""
}

func (p *pageProbe) Keyed(ctx context.Context, _ any) context.Context { return ctx }

// probe is the RenderPage the renderer runs under, or nil.
func (h *HTMLRenderer) probe() *pageProbe {
	p, _ := liveHost(h.ctx).(*pageProbe)
	return p
}

// noteRaw notes Safe HTML that may hold what only its HTML tells.
func (h *HTMLRenderer) noteRaw(html string) {
	if strings.Contains(html, "data-vk-") {
		if p := h.probe(); p != nil {
			p.info.Opaque = true
		}
	}
}
