package templdemo

import (
	"context"
	"io"
	"strings"

	"github.com/vuka-lang/ui/v2"
)

// Opaque renders n as one piece of markup, as templ's own code does: the
// baseline the tests measure recorded trees against.
func Opaque(n ui.Node) ui.Node {
	return ui.NodeFunc(func(ctx context.Context, w io.Writer) error {
		var b strings.Builder
		if err := n.Render(ctx, &b); err != nil {
			return err
		}
		_, err := io.WriteString(w, b.String())
		return err
	})
}
