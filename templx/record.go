package templx

import (
	"context"
	"io"

	"github.com/vuka-lang/ui"
)

// Rec records a templ component's render into a live render tree: ui.Rec.
// The vuka command compiles the .templ files of a module requiring ui to
// call it, so on a live page their markup diffs as JSX does.
type Rec = ui.Rec

// Record is ui.Record. Generated code calls it.
func Record(ctx context.Context, w io.Writer) Rec { return ui.Record(ctx, w) }
