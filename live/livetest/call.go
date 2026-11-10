package livetest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
)

// openFrame is a component a Call opened: the key the client named it by,
// the element whose commands opened it, and what runs once it closes.
type openFrame struct {
	key    string
	opener string
	closed json.RawMessage
}

// openEvent is the event a Call step's open sends: its token, the key the
// client names the component by, and the props it reads, read now.
func (c *Client) openEvent(this *node, a *opArgs) (ui.Event, string, error) {
	reads := map[string]any{}
	for name, raw := range a.Reads {
		v, err := c.arg(this, raw)
		if err != nil {
			return ui.Event{}, "", err
		}
		reads[name] = v
	}
	c.calls++
	key := "t" + strconv.Itoa(c.calls)
	data, err := json.Marshal(map[string]any{"key": key, "reads": reads})
	if err != nil {
		return ui.Event{}, "", err
	}
	return ui.Event{Target: a.H, Type: "vkcall", Value: a.T, Data: data}, key, nil
}

// frameOf is the opened component e is in: the nearest element around it
// with data-vk-call, else the nearest <dialog>.
func frameOf(e *node) *node {
	var dialog *node
	for ; e != nil; e = e.parent {
		if _, ok := e.attr("data-vk-call"); ok {
			return e
		}
		if e.tag == "dialog" && dialog == nil {
			dialog = e
		}
	}
	return dialog
}

// closedGone runs the Closed hooks of the opened components the page
// shows no more, where the commands that opened them ran.
func (c *Client) closedGone() error {
	var errs []error
	for _, f := range slices.Clone(c.frames) {
		if c.Exists(`[data-vk-call="` + f.key + `"]`) {
			continue
		}
		c.frames = slices.DeleteFunc(c.frames, func(x *openFrame) bool { return x == f })
		if len(f.closed) == 0 {
			continue
		}
		if this := c.at(f.opener); this != nil {
			errs = append(errs, c.run(this, f.closed, ui.Event{}, nil))
		}
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Call opens component as ui.Call does from a handler: on the session,
// mounted now, and shown once the reply — applied here — is in.
func (c *Client) Call(component any) error {
	p, err := c.S.Call(component)
	if err != nil {
		return err
	}
	if err := c.Apply(live.Marshal(live.PatchMessage(0, p))); err != nil {
		return err
	}
	return c.flush()
}

// Dialog is the component the page opened last that is still open (a Call's,
// shown in its presentation, a <dialog> with data-vk-call), for a test to
// read, or one that doesn't exist when none is.
func (c *Client) Dialog() Element {
	var last *node
	c.page().walk(func(e *node) bool {
		if _, ok := e.attr("data-vk-call"); ok {
			last = e
		}
		return true
	})
	return Element{c, last}
}

// Dialogs is how many components Calls opened are open.
func (c *Client) Dialogs() int {
	n := 0
	c.page().walk(func(e *node) bool {
		if _, ok := e.attr("data-vk-call"); ok {
			n++
		}
		return true
	})
	return n
}

// Escape presses Escape in the last opened component: its dialog's cancel,
// which closes it.
func (c *Client) Escape() error {
	d := c.Dialog()
	if d.n == nil {
		return fmt.Errorf("livetest: no open dialog")
	}
	if err := c.fire(d.n, ui.Event{Type: "vkclose"}, false); err != nil {
		return err
	}
	return c.flush()
}
