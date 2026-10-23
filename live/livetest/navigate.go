package livetest

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/vuka-lang/ui/v2/live"
)

// Navigation (the live protocol's "Navigation"): a click on a ui.Link, a
// patch or navigate command, Navigate and Patch send a navigate message; a
// patch reply is applied like any patch, and the client's URL becomes the
// page's. Once a navigate (not a patch) is applied, the optimistic pushes
// still waiting are dropped without their hooks, as a browser drops them
// when the page changes. A redirect reply is the full page load a browser
// would make: the URL changes, the page the client holds doesn't.

// navigation is a navigate message waiting to be sent: replace takes the
// current history entry's place, pop is a move back or forward.
type navigation struct {
	href    string
	patch   bool
	replace bool
	pop     bool
}

// URL is the page's URL as the client last moved it; "" before it moved.
func (c *Client) URL() string { return c.url }

// SetURL sets the page's URL, the first entry of its history: where it was
// loaded from, which Back returns to.
func (c *Client) SetURL(u string) {
	c.url = u
	c.history, c.entry = []string{u}, 0
}

// History is the page's history entries, oldest first, as moves over the
// connection made them, and the index of the current one.
func (c *Client) History() ([]string, int) { return c.history, c.entry }

// Back moves the page to the previous history entry, as the browser's back
// button does: a patch, when the entry is on the same path, else a
// navigate.
func (c *Client) Back() (live.ServerMessage, error) { return c.pop(-1) }

// Forward moves the page to the next history entry.
func (c *Client) Forward() (live.ServerMessage, error) { return c.pop(1) }

func (c *Client) pop(d int) (live.ServerMessage, error) {
	i := c.entry + d
	if i < 0 || i >= len(c.history) {
		return live.ServerMessage{}, fmt.Errorf("livetest: no history entry %+d", d)
	}
	href := c.history[i]
	c.entry = i
	m, err := c.navigate(navigation{href: href, patch: samePath(href, c.url), pop: true})
	if err != nil {
		return m, err
	}
	return m, c.flush()
}

func samePath(a, b string) bool {
	pa, _, _ := strings.Cut(a, "?")
	pb, _, _ := strings.Cut(b, "?")
	return pa == pb
}

// record adds a move's URL to the history, or puts it in place of the
// current entry.
func (c *Client) record(n navigation, u string) {
	switch {
	case n.pop:
	case len(c.history) == 0:
		c.history, c.entry = []string{u}, 0
	case n.replace:
		c.history[c.entry] = u
	default:
		c.history = append(c.history[:c.entry+1], u)
		c.entry++
	}
}

// Navigate moves the page to href over the connection: another page, whose
// components mount.
func (c *Client) Navigate(href string) (live.ServerMessage, error) {
	m, err := c.navigate(navigation{href: href})
	if err != nil {
		return m, err
	}
	return m, c.flush()
}

// Patch moves the page to href, the same page with other parameters: its
// components stay mounted.
func (c *Client) Patch(href string) (live.ServerMessage, error) {
	m, err := c.navigate(navigation{href: href, patch: true})
	if err != nil {
		return m, err
	}
	return m, c.flush()
}

func (c *Client) navigate(n navigation) (live.ServerMessage, error) {
	m, err := c.send(live.ClientMessage{Type: live.Navigate, URL: n.href, Patch: n.patch}, nil)
	if err != nil {
		return m, err
	}
	switch m.Type {
	case live.PatchMsg:
		c.url = cmp.Or(m.URL, n.href)
		c.record(n, c.url)
		if !n.patch {
			for _, p := range c.pending {
				p.settled = true
			}
			c.pending = nil
			c.busy = nil // a new page: its elements were never marked
		}
	case live.Redirect:
		c.url = m.URL
	case live.Error:
		return m, fmt.Errorf("livetest: navigate to %s: %s", n.href, m.Error)
	}
	return m, nil
}

// linkOf is the ui.Link a click on e follows: e or its nearest ancestor
// that is an a element marked data-vk-link, with no target but _self.
func (c *Client) linkOf(e *node) *node {
	for ; e != nil; e = e.parent {
		if e.tag != "a" {
			continue
		}
		if _, ok := c.get(e, "data-vk-link"); !ok {
			return nil
		}
		if t, _ := c.get(e, "target"); t != "" && t != "_self" {
			return nil
		}
		return e
	}
	return nil
}

// follow queues the navigation a click on link makes.
func (c *Client) follow(link *node) {
	if link == nil {
		return
	}
	href, _ := c.get(link, "href")
	mode, _ := c.get(link, "data-vk-link")
	c.queue = append(c.queue, sending{nav: &navigation{href: href, patch: mode == "patch", replace: c.hasAttr(link, "data-vk-replace")}})
}

func (c *Client) hasAttr(e *node, name string) bool {
	_, ok := c.get(e, name)
	return ok
}
