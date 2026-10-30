package tmpltest

import "github.com/vuka-lang/ui/v2"

// The vuka command tools.mod pins predates ui.RegisterOptions: the methods
// docs.vuka.html's Options commands name are registered here, as a newer
// one does in docs_html.go.
func init() { ui.RegisterOptions((*Docs).CitiesOf) }
