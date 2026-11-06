package ui

import (
	"context"
	"io"
	"sync/atomic"
)

// Await is the section of a live component's markup showing one of its
// deferred groups — the fields @ui.Defer("orders") marks:
//
//	<ui.Await key="orders">
//	  <slot:pending><OrdersSkeleton rows={10} /></slot:pending>
//	  <slot:error let={err}>Couldn't load the orders: {err.Error()}</slot:error>
//	  for _, o := range orders {
//	    <OrderRow order={o} />
//	  }
//	</ui.Await>
//
// It is the branches a component would write itself, rendered and patched
// as they are:
//
//	if Loading("orders") { … } else if err := LoadErr("orders"); err != nil { … } else { … }
//
// Pending shows while the group loads, Error once its loader failed, and
// Children once its fields are set; Children runs only then. A slot left
// out shows the app's default (SetAwaitDefaults), else what a <ui.Suspense>
// shows. In markup the compiler sets Of to the component whose markup it
// is; in Go, set it to the component.
type Await struct {
	Of       Deferring
	Key      string
	Pending  Node
	Error    func(err error) Node
	Children func() Node
}

// Deferring is a component with deferred groups: a pointer to a struct
// embedding Live.
type Deferring interface {
	Loading(group string) bool
	LoadErr(group string) error
}

// AwaitDefaults are what an Await shows for a slot its markup leaves out.
type AwaitDefaults struct {
	Pending Node
	Error   func(err error) Node
}

var awaitDefaults atomic.Pointer[AwaitDefaults]

// SetAwaitDefaults sets what every Await of the app shows for a slot its
// markup leaves out: Pending while a group loads, Error once its loader
// failed. A nil one keeps the built-in default. Call it once, before the
// app serves:
//
//	ui.SetAwaitDefaults(ui.AwaitDefaults{
//		Pending: ui.Skeleton{Lines: 3},
//		Error:   func(err error) ui.Node { return ui.Alert{Variant: ui.AlertError, Children: ui.Text(err.Error())} },
//	})
func SetAwaitDefaults(d AwaitDefaults) { awaitDefaults.Store(&d) }

func (a Await) View() Node {
	if a.Of == nil {
		panic("ui: Await needs Of, the live component deferring " + a.Key)
	}
	if a.Of.Loading(a.Key) {
		return a.pending()
	}
	if err := a.Of.LoadErr(a.Key); err != nil {
		return a.failed(err)
	}
	if a.Children == nil {
		return nil
	}
	return a.Children()
}

func (a Await) Render(ctx context.Context, w io.Writer) error {
	if n := a.View(); n != nil {
		return n.Render(ctx, w)
	}
	return nil
}

func (a Await) pending() Node {
	if a.Pending != nil {
		return a.Pending
	}
	if d := awaitDefaults.Load(); d != nil && d.Pending != nil {
		return d.Pending
	}
	return RawHTML(`<span aria-busy="true"></span>`)
}

func (a Await) failed(err error) Node {
	if a.Error != nil {
		return a.Error(err)
	}
	if d := awaitDefaults.Load(); d != nil && d.Error != nil {
		return d.Error(err)
	}
	return partError(err, nil, nil)
}
