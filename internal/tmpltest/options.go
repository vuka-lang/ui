package tmpltest

import "github.com/vuka-lang/ui/v2"

// The vuka command tools.mod pins predates ui.RegisterPush: the methods
// docs.vuka.html's Push and Options steps name are registered here, as a
// newer one does in docs_html.go.
func init() {
	for _, m := range []any{(*Docs).CitiesOf, (*Docs).RegionsOf, (*Docs).Older, (*Docs).Newer, (*Docs).Details} {
		ui.RegisterPush(m)
	}
}
