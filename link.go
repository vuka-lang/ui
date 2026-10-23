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
	Target string
	Rel    string
	// Active marks the link as the current page's in the browser:
	// aria-current="page", and ActiveClass, while the address matches Href
	// — ActiveExact the same path, ActivePrefix the path or one below it.
	// The runtime sets them as the page loads and after every navigation,
	// so a layout's menu needs no server render to follow the page. An
	// element around active links marked data-vk-active-within gets the
	// attribute's value "true" while it holds the current link, "" when it
	// doesn't, and a <details> so marked opens when the current link comes
	// to be inside it — a menu's group opens on the current page, and stays
	// as the visitor leaves it, open or closed:
	//
	//	<details data-vk-active-within>
	//	  <summary>Orders</summary>
	//	  <ui.Link href="/orders/open" active="prefix">Open</ui.Link>
	//	</details>
	Active LinkActive
	// ActiveClass is a class the runtime adds to an active link, and
	// removes when it is no longer active.
	ActiveClass string
	// Prefetch, PrefetchOff, asks the runtime not to fetch the link's page
	// before it is followed; PrefetchViewport fetches it as soon as the
	// link is in view.
	Prefetch LinkPrefetch
	Attrs    []Attr // any other attributes
	Children Node
}

// LinkActive is how a link's href matches the current address: Link's
// Active.
type LinkActive string

const (
	// ActiveExact: the address's path is the link's.
	ActiveExact LinkActive = "exact"
	// ActivePrefix: the address's path is the link's or below it,
	// /books/7 for /books.
	ActivePrefix LinkActive = "prefix"
)

// LinkPrefetch is when the runtime fetches a link's page before it is
// followed: Link's Prefetch. By default it does on hover, focus or touch.
type LinkPrefetch string

const (
	// PrefetchOff: never before the link is followed.
	PrefetchOff LinkPrefetch = "off"
	// PrefetchViewport: as soon as the link is in view.
	PrefetchViewport LinkPrefetch = "viewport"
)

// Link is a link a live page follows over its connection:
//
//	<ui.Link href="/books/7">Dune</ui.Link>            another page: its components mount
//	<ui.Link href="/books?page=2" patch>Next</ui.Link>  this page, other parameters
//	<ui.Link href="/books" active="prefix" activeClass="on">Books</ui.Link>  current while under /books
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
	for _, a := range [...]Attr{{"id", p.ID}, {"class", p.Class}, {"title", p.Title}, {"target", p.Target}, {"rel", p.Rel},
		{"data-vk-active", string(p.Active)}, {"data-vk-active-class", string(p.ActiveClass)}, {"data-vk-prefetch", string(p.Prefetch)}} {
		if a.Value != "" {
			attrs = append(attrs, a)
		}
	}
	return El("a", append(attrs, p.Attrs...), Child(p.Children))
}
