// Package templdemo is templ components, compiled by the vuka command to
// record live render trees, and pages using them, for the live package's
// tests. The *_templ.go and *_html.go files are generated:
// vuka gen -inplace ./live/internal/templdemo
package templdemo

import "github.com/vuka-lang/ui/v2"

// InboxPage and BooksPage are pages sharing the templ Layout.
func InboxPage() ui.Node { return InboxView{} }

func BooksPage(books []string) ui.Node { return BooksView{Books: books} }
