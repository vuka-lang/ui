package ui

// LinkProps is a Link's attributes; the children are its content.
type LinkProps struct {
	Href string
	// Patch asks for the same live route with other parameters: the page
	// stays mounted when the server says so (an Update taking the URL),
	// else mounts again.
	Patch bool
	ID    string
	Class string
	Title string
	// Target and Rel as an a element's: a link with a target other than
	// _self is left to the browser.
	Target   string
	Rel      string
	Attrs    []Attr // any other attributes
	Children Node
}

// Link is a link a live page follows over its connection:
//
//	<ui.Link href="/books/7">Dune</ui.Link>            another page: its components mount
//	<ui.Link href="/books?page=2" patch>Next</ui.Link>  this page, other parameters
//
// It renders an a element with href, marked data-vk-link (data-vk-link="patch"
// with Patch), so it is an ordinary link wherever the runtime doesn't take
// it: without JavaScript, on a page that isn't connected, opened in a new
// tab or with a modifier key, or to another origin. Href is sanitized as an
// a element's (templ.URL), and the runtime follows only paths and http and
// https URLs.
func Link(p LinkProps) Node {
	mark := any(true)
	if p.Patch {
		mark = "patch"
	}
	attrs := []Attr{{"href", p.Href}, {"data-vk-link", mark}}
	for _, a := range [...]Attr{{"id", p.ID}, {"class", p.Class}, {"title", p.Title}, {"target", p.Target}, {"rel", p.Rel}} {
		if a.Value != "" {
			attrs = append(attrs, a)
		}
	}
	return El("a", append(attrs, p.Attrs...), Child(p.Children))
}
