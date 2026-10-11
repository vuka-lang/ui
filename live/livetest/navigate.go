package livetest

import (
	"cmp"
	"fmt"

	"github.com/vuka-lang/ui/live"
)

// Navigation (the live protocol's "Navigation"): a click on a ui.Link, a
// patch or navigate command, Navigate and Patch send a navigate message; a
// patch reply is applied like any patch, and the client's URL becomes the
// page's. Once a navigate (not a patch) is applied, the optimistic pushes
// still waiting are dropped without their hooks, as a browser drops them
// when the page changes. A redirect reply is the full page load a browser
// would make: the URL changes, the page the client holds doesn't.

// navigation is a navigate message waiting to be sent.
type navigation struct {
	href  string
	patch bool
}

// URL is the page's URL as the client last moved it; "" before it moved.
func (c *Client) URL() string { return c.url }

// Navigate moves the page to href over the connection: another page, whose
// components mount.
func (c *Client) Navigate(href string) (live.ServerMessage, error) {
	m, err := c.navigate(navigation{href, false})
	if err != nil {
		return m, err
	}
	return m, c.flush()
}

// Patch moves the page to href, the same page with other parameters: its
// components stay mounted.
func (c *Client) Patch(href string) (live.ServerMessage, error) {
	m, err := c.navigate(navigation{href, true})
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
	c.queue = append(c.queue, sending{nav: &navigation{href, mode == "patch"}})
}
