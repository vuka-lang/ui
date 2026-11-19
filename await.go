package ui

import (
	"cmp"
	"context"
	"io"
	"strconv"
	"sync/atomic"
	"time"
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
//
// Wait, Delay and MinShow keep a fast load from flashing Pending (see
// <ui.Suspense>). On a live page, with Delay or MinShow in effect, the
// section is a <vk-await> (display: contents) in every state, which is what
// the runtime holds.
type Await struct {
	Of  Deferring
	Key string
	// And are the groups that load after Key: the section is pending while
	// any of them loads, failed once one failed, else shows Children.
	And      []string
	Pending  Node
	Error    func(err error) Node
	Children func() Node
	// Wait holds a live page's first render, the HTTP one and the join's,
	// for up to this long for the groups' loaders, so a loader that finishes
	// in time ships Children and Pending never goes on the wire. The longest
	// Wait of a page's sections is the page's one budget. 0 holds nothing.
	Wait time.Duration
	// Delay is how long Pending stays invisible once shown (its space is
	// reserved): a load that finishes sooner never shows it. 0 is the
	// default, DefaultAwaitDelay; Instant shows Pending at once.
	Delay time.Duration
	// MinShow is how long Pending, once visible, stays before Children
	// replaces it, so it never blinks. 0 holds nothing.
	MinShow time.Duration
}

// DefaultAwaitDelay is the Delay of an Await that sets none and whose
// app's AwaitDefaults set none: a load faster than a blink of an eye (about
// 150 ms, where waiting starts to be noticed) never shows Pending.
const DefaultAwaitDelay = 150 * time.Millisecond

// Instant, as an Await's Delay, shows Pending at once.
const Instant time.Duration = -1

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
	// Wait, Delay and MinShow are the timings of an Await that sets none
	// (Instant for no Delay).
	Wait, Delay, MinShow time.Duration
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
	wait, delay, minShow := a.timings()
	pending, err := a.state()
	if pending && wait > 0 {
		a.holdFor(wait)
	}
	var n Node
	state := "done"
	switch {
	case pending:
		n, state = a.pending(), "pending"
	case err != nil:
		n, state = a.failed(err), "error"
	case a.Children != nil:
		n = a.Children()
	}
	if delay <= 0 && minShow <= 0 {
		return n
	}
	if s, ok := a.Of.(interface{ ID() string }); !ok || s.ID() == "" {
		return n
	}
	style := "display: contents"
	attrs := []Attr{{"data-vk-await", state}}
	if delay > 0 {
		if state == "pending" {
			style += "; visibility: hidden"
		}
		attrs = append(attrs, Attr{"data-vk-delay", strconv.FormatInt(delay.Milliseconds(), 10)})
	}
	if minShow > 0 {
		attrs = append(attrs, Attr{"data-vk-min", strconv.FormatInt(minShow.Milliseconds(), 10)})
	}
	attrs = append(attrs, Attr{"style", style})
	return HTML(`<vk-await {attrs...}>{n}</vk-await>`, attrs, n)
}

// keys are the groups the section shows, in order.
func (a Await) keys() []string { return append([]string{a.Key}, a.And...) }

// state is whether any group loads, else the first group's failure.
func (a Await) state() (pending bool, err error) {
	for _, k := range a.keys() {
		if a.Of.Loading(k) {
			return true, nil
		}
		if err := a.Of.LoadErr(k); err != nil {
			return false, err
		}
	}
	return false, nil
}

func (a Await) holdFor(d time.Duration) {
	h, ok := a.Of.(interface {
		holdFor(string, time.Duration)
	})
	if !ok {
		return
	}
	for _, k := range a.keys() {
		h.holdFor(k, d)
	}
}

// timings are the section's Wait, Delay and MinShow: its own, else the
// app's defaults, else DefaultAwaitDelay for Delay.
func (a Await) timings() (wait, delay, minShow time.Duration) {
	var d AwaitDefaults
	if p := awaitDefaults.Load(); p != nil {
		d = *p
	}
	wait, delay, minShow = cmp.Or(a.Wait, d.Wait), cmp.Or(a.Delay, d.Delay, DefaultAwaitDelay), cmp.Or(a.MinShow, d.MinShow)
	return max(wait, 0), max(delay, 0), max(minShow, 0)
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
