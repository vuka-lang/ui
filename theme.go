package ui

// ui.css is the only authored source of theme values. Keep Styles a constant
// for existing callers; cssgen copies the stylesheets verbatim.
//go:generate go run ./internal/cssgen

// StyleTag is a <style> element holding Styles, to place in a page's head so
// the components are themed:
//
//	func layout(page ui.Node) ui.Node {
//		return ui.El("html", nil, ui.El("head", nil, ui.StyleTag()), ui.El("body", nil, page))
//	}
func StyleTag() Node {
	return El("style", []Attr{{Name: "data-vk-ui", Value: true}}, RawHTML(Styles))
}
