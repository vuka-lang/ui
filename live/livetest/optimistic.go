package livetest

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/vuka-lang/ui"
)

// Optimistic pushes (the live protocol's "Optimistic pushes"): the commands
// before a push are its predictions; the client records what they change
// and, when the push fails — its reply carries an error, or no reply comes
// in time — reverts it, where the server hasn't rendered a new value since.
//
// Replies are applied as soon as an event is sent, unless the client holds
// its events (Hold): then a test sees the predicted page before the server
// answers, delivers the events with Deliver, and runs timeouts with Advance.

// value is an attribute's value, or its absence.
type value struct {
	s   string
	has bool
}

// change is one change a command made: the element and attribute (or
// textAttr), its value before and after, and the server's value then.
type change struct {
	path, name            string
	before, after, server value
}

// pending is an optimistic push not yet settled by its reply.
type pending struct {
	this        string // the path of This, for its hooks
	changes     []change
	onOK, onErr json.RawMessage
	keep        bool
	deadline    time.Duration
	settled     bool
}

// sending is an event waiting to be sent, and the push it is the event of.
type sending struct {
	ev  ui.Event
	p   *pending
	nav *navigation // a navigate message instead of the event
}

// DefaultTimeout is how long a push waits for its reply unless it says.
const DefaultTimeout = 10 * time.Second

func (c *Client) predict(this *node, a *opArgs, seg []change) *pending {
	t := DefaultTimeout
	if a.Timeout > 0 {
		t = time.Duration(a.Timeout) * time.Millisecond
	}
	p := &pending{this: this.path, changes: seg, onOK: a.OnOK, onErr: a.OnError, keep: a.Keep, deadline: c.now + t}
	c.pending = append(c.pending, p)
	return p
}

// Hold makes the client keep the events commands push, and the events of
// data-vk-on handlers, until Deliver: what a test sees in between is the
// page before the server answers.
func (c *Client) Hold() { c.held = true }

// Deliver sends the events held, in order, applying each reply; the events
// those replies' commands push are sent too. The client goes on holding.
func (c *Client) Deliver() error {
	c.held = false
	defer func() { c.held = true }()
	return c.flush()
}

// Release stops holding events and delivers those held.
func (c *Client) Release() error {
	c.held = false
	return c.flush()
}

// Pending is the number of optimistic pushes not yet settled.
func (c *Client) Pending() int {
	n := 0
	for _, p := range c.pending {
		if !p.settled {
			n++
		}
	}
	return n
}

// Advance moves the client's clock on by d: each push whose timeout
// passes without its reply fails. A reply coming later is applied, its
// push settled already.
func (c *Client) Advance(d time.Duration) error {
	c.now += d
	for _, p := range slices.Clone(c.pending) {
		if p.settled || p.deadline > c.now {
			continue
		}
		if !p.keep {
			c.revert(p.changes)
		}
		if err := c.settle(p, false); err != nil {
			return err
		}
	}
	return c.flush()
}

// settle marks p settled and runs its hook, on_ok or on_error.
func (c *Client) settle(p *pending, ok bool) error {
	p.settled = true
	hook := p.onErr
	if ok {
		hook = p.onOK
	}
	if len(hook) == 0 {
		return nil
	}
	if this := c.at(p.this); this != nil {
		return c.run(this, hook, ui.Event{}, nil)
	}
	return nil
}

// at is the element of the page at path ("" is the root).
func (c *Client) at(path string) *node {
	root := c.page()
	if path == "" {
		return root
	}
	var found *node
	root.walk(func(e *node) bool {
		if e.path == path {
			found = e
		}
		return found == nil
	})
	return found
}

// revert undoes changes, latest first, each where the server still renders
// what it did then and the page still shows what the change set.
func (c *Client) revert(changes []change) {
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		e := c.at(ch.path)
		if e == nil {
			continue
		}
		server := serverValue(e, ch.name)
		var shown value
		shown.s, shown.has = c.get(e, ch.name)
		if server != ch.server || shown != ch.after {
			continue
		}
		m := c.over[e.path]
		if ch.before == server {
			delete(m, ch.name)
			if len(m) == 0 {
				delete(c.over, e.path)
			}
			continue
		}
		m[ch.name] = &override{server: server.s, serverHas: server.has, val: ch.before.s, valHas: ch.before.has}
	}
}

// failLocally fails the push a failed call predicts for, the next push in
// rest, without sending it: the changes of its segment so far are reverted
// and its on_error runs.
func (c *Client) failLocally(this *node, rest []op, acc *[]change) error {
	seg := *acc
	*acc = nil
	for _, o := range rest {
		if o.name != "push" {
			continue
		}
		if o.args.H == "" {
			return nil
		}
		if !o.args.Keep {
			c.revert(seg)
		}
		return c.settle(&pending{this: this.path, onErr: o.args.OnError}, false)
	}
	return nil
}

// Func stands in for a JavaScript function a call command calls: it gets
// the call's context and arguments (element reads resolved) and returns
// its result — false stops the chain — or an error, as a throw.
type Func func(ctx CallContext, args []any) (any, error)

// CallContext is what a called function gets besides its arguments.
type CallContext struct {
	Event ui.Event // the event the commands run for
	El    Element  // the element the call's target names, This without one
	Root  Element  // This's instance root
}

// Element is an element of the page, read as it shows after commands.
type Element struct {
	c *Client
	n *node
}

// Exists reports whether there is an element.
func (e Element) Exists() bool { return e.n != nil }

// Attr is the element's attribute.
func (e Element) Attr(name string) (string, bool) {
	if e.n == nil {
		return "", false
	}
	return e.c.get(e.n, name)
}

// Text is the element's text.
func (e Element) Text() string {
	if e.n == nil {
		return ""
	}
	return e.c.text(e.n)
}

// RegisterFunc makes fn the function call commands naming name call.
// A call of a name not registered fails, as an unknown function does in a
// browser.
func (c *Client) RegisterFunc(name string, fn Func) {
	if c.funcs == nil {
		c.funcs = map[string]Func{}
	}
	c.funcs[name] = fn
}

// call runs a call command; ok false stops the chain.
func (c *Client) call(this *node, a *opArgs, ev ui.Event) (ok bool, err error) {
	fn := c.funcs[a.Fn]
	if fn == nil {
		return false, fmt.Errorf("livetest: no function %s", a.Fn)
	}
	ctx := CallContext{Event: ev, Root: Element{c, instanceOf(this)}}
	ts, err := c.targets(this, a.To)
	if err != nil {
		return false, err
	}
	if len(ts) > 0 {
		ctx.El = Element{c, ts[0]}
	}
	args := make([]any, len(a.Args))
	for i, raw := range a.Args {
		if args[i], err = c.arg(this, raw); err != nil {
			return false, err
		}
	}
	res, err := fn(ctx, args)
	if err != nil {
		return false, fmt.Errorf("%s: %w", a.Fn, err)
	}
	return res != false, nil
}

// arg is a call's argument: a JSON value, or an element read.
func (c *Client) arg(this *node, raw json.RawMessage) (any, error) {
	var read map[string]*cond
	if json.Unmarshal(raw, &read) == nil && len(read) == 1 && read["$read"] != nil {
		k := read["$read"]
		ts, err := c.targets(this, k.To)
		if err != nil {
			return nil, err
		}
		if len(ts) == 0 {
			if k.Exists {
				return false, nil
			}
			return nil, nil
		}
		switch e := ts[0]; {
		case k.Value:
			return c.value(e), nil
		case k.Checked:
			_, on := c.get(e, "checked")
			return on, nil
		case k.Attr != "":
			if v, ok := c.get(e, k.Attr); ok {
				return v, nil
			}
			return nil, nil
		}
		return c.holds(this, k)
	}
	var v any
	return v, json.Unmarshal(raw, &v)
}
