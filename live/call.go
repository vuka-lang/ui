package live

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/vuka-lang/ui/v2"
)

// Components opened with Call (ui.JS.Call(component), ui.Call): each is an
// instance of the session, a child of the page's at a place of its own,
// rendered after the page's markup in a Teleport to the overlay host
// (#vk-overlays, which the document shell holds), so live patches, its
// handlers, forms and focus work as anywhere on the page, and it stays
// open while the page renders again. Closing it drops it from the page's
// render: the instance is unmounted. A stateless component (or a stateful
// one whose Mount failed) is rendered by a holder instance, so its
// handlers and ui.Close find it too.

// OverlayHost is the id of the element the opened components are
// teleported to: the document shell's, or the layout's own.
const OverlayHost = "vk-overlays"

// callOpen is the handler a Call step's open names (scope.CallTarget).
const callOpen = "vkcall"

// openCall is a component the page opened.
type openCall struct {
	key   string
	in    *instance
	opts  []ui.CallOption // how it shows: As, Into, CallKey, Size
	into  string          // the element it opened in, "#side"; "" for the overlay host
	owner string          // the instance it belongs to, Into a region of its
}

// callHolder renders a stateless component a Call opened, or the error
// state of a stateful one whose Mount failed.
type callHolder struct {
	ui.Live
	key    string
	view   ui.Node
	failed error
	close  func()
	opts   []ui.CallOption
}

func (h *callHolder) Render() ui.Node {
	return ui.CallView(h.key, h.view, h.close, h.failed, h.opts...)
}

var callKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

func newNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// CallBinding is what the tokens of the session's Calls are bound to.
func (sc *scope) CallBinding() string { return sc.in.s.callNonce }

// CallOwner is the instance whose render shows the Call.
func (sc *scope) CallOwner() string { return sc.in.id }

// CallTarget is the handler a Call step's open event names.
func (sc *scope) CallTarget() string { return RootID + ":" + callOpen }

// CallAction keeps fn, an Action prop of a Call the instance renders, under
// its place in the render (the keys around it, and how many came before):
// the same at each render while the markup around it is, as a handler's
// key is.
func (sc *scope) CallAction(_ context.Context, fn any) string {
	occ := "\x00act " + sc.keys
	sc.in.occ[occ]++
	k := sc.keys + "a" + strconv.Itoa(sc.in.occ[occ])
	if sc.in.acts == nil {
		sc.in.acts = map[string]any{}
	}
	sc.in.acts[k] = fn
	return sc.in.id + ":" + k
}

// action is the Action prop id names (CallAction).
func (s *Session) action(id string) (any, bool) {
	inst, k, ok := strings.Cut(id, ":")
	in := s.byID[inst]
	if !ok || in == nil {
		return nil, false
	}
	fn, ok := in.acts[k]
	return fn, ok
}

// openEvent answers a Call step's open: its token in the event's value,
// the key the browser named it by and the props it read in its data.
func (s *Session) openEvent(ctx context.Context, ev ui.Event) error {
	var d struct {
		Key   string          `json:"key"`
		Reads json.RawMessage `json:"reads"`
	}
	if len(ev.Data) > 0 {
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return publicErr{err, "live: the call's data doesn't parse"}
		}
	}
	if !callKey.MatchString(d.Key) {
		return publicErr{errors.New("live: bad call key"), "live: the call has no key"}
	}
	c, err := ui.ParseCall(ctx, s.callNonce, ev.Value, d.Reads)
	if err != nil {
		return err
	}
	if k := c.Key(); k != "" && k != d.Key {
		return publicErr{errors.New("live: the call's key isn't its token's"), "live: the call's key isn't its token's"}
	}
	comp, err := c.Value(s.action)
	if err != nil {
		return err
	}
	_, owner := c.Into()
	return s.open(ctx, d.Key, comp, c.Options(), owner)
}

// open mounts comp, a pointer to a component's new value, as the opened
// component key names, shown as opts say, rendered from the next render
// on; owner is the instance it belongs to when it opens Into a region. Its
// Mount's error is open's, the component showing its frame's error state,
// after its OnError prop runs. A component open under key already takes
// comp's props instead (CallKey).
func (s *Session) open(ctx context.Context, key string, comp any, opts []ui.CallOption, owner string) error {
	if c := s.callByKey(key); c != nil {
		return s.update(c, comp, opts)
	}
	_, into := ui.CallOptions(opts)
	if into != "" {
		// a region shows one opened component: the one opened last
		s.calls = slices.DeleteFunc(s.calls, func(c *openCall) bool { return c.into == into })
	}
	s.next++
	in := &instance{s: s, id: "c" + strconv.Itoa(s.next), children: map[string]*instance{}, fresh: true}
	closeIt := func() { s.dropCall(key) }
	oc := &openCall{key: key, in: in, opts: opts, into: into, owner: owner}
	failed := ui.InjectFields(ctx, comp)
	st, live := comp.(ui.Stateful)
	switch {
	case failed != nil:
	case live:
		if failed = ui.CheckStateful(st); failed == nil {
			in.comp, in.call = st, oc
			ui.AttachLive(st, in)
			in.trackMethods(st)
			failed = mount(ctx, st)
		}
	default:
		in.comp = &callHolder{key: key, view: comp.(ui.Node), close: closeIt, opts: opts}
	}
	if failed != nil {
		in.comp, in.call, in.methods = &callHolder{key: key, failed: failed, close: closeIt, opts: opts}, nil, nil
	}
	in.typ = reflect.TypeOf(in.comp)
	if h, ok := in.comp.(*callHolder); ok {
		ui.AttachLive(h, in)
	}
	s.calls = append(s.calls, oc)
	s.root.children[callPlace(key)] = in
	s.byID[in.id] = in
	if failed == nil {
		return nil
	}
	switch a := ui.OnErrorOf(comp).(type) {
	case nil:
	case ui.Commands:
		s.root.js = append(s.root.js, a)
	case ui.CommandsOn:
		s.root.js = append(s.root.js, a.Commands())
	default:
		if err := callHandler(ctx, a, ui.Event{Type: "vkerror"}); err != nil {
			failed = errors.Join(failed, err)
		}
	}
	return failed
}

// callByKey is the component open under key, or nil.
func (s *Session) callByKey(key string) *openCall {
	for _, c := range s.calls {
		if c.key == key {
			return c
		}
	}
	return nil
}

// update gives c, an opened component, comp's props, and opts, as a
// second Call with its key does: a stateful one keeps its state, its tag's
// props set as a parent's render sets them.
func (s *Session) update(c *openCall, comp any, opts []ui.CallOption) error {
	c.opts = opts
	if h, ok := c.in.comp.(*callHolder); ok {
		n, stateless := comp.(ui.Node)
		if _, live := comp.(ui.Stateful); live || !stateless || h.failed != nil {
			return fmt.Errorf("live: a component is open under the key %s already, of another type", c.key)
		}
		h.view, h.opts, c.in.set = n, opts, true
		return nil
	}
	st, ok := comp.(ui.Stateful)
	if !ok || reflect.TypeOf(st) != c.in.typ {
		return fmt.Errorf("live: a component is open under the key %s already, of another type", c.key)
	}
	copyProps(c.in.comp, st)
	c.in.trackMethods(st)
	c.in.set = true
	return nil
}

// dropOrphans drops the components opened Into a region whose instance is
// gone, reporting whether it dropped any.
func (s *Session) dropOrphans() bool {
	n := len(s.calls)
	s.calls = slices.DeleteFunc(s.calls, func(c *openCall) bool { return c.owner != "" && s.byID[c.owner] == nil })
	return len(s.calls) < n
}

// callPlace is an opened component's place among the page's children.
func callPlace(key string) string { return "vk-call#k" + keyText(key) }

// dropCall closes the opened component key names: the next render leaves
// it out, unmounting it.
func (s *Session) dropCall(key string) {
	s.calls = slices.DeleteFunc(s.calls, func(c *openCall) bool { return c.key == key })
}

// overlays is what the page's render ends with: each opened component, in
// a Teleport to the overlay host.
func (s *Session) overlays() []ui.Node {
	out := make([]ui.Node, len(s.calls))
	for i, c := range s.calls {
		to := c.into
		if to == "" {
			to = "#" + OverlayHost
		}
		out[i] = ui.Teleport{To: to, ID: "vk-call-" + c.key, Children: &ui.ComponentNode{Site: "vk-call", Key: c.key, Props: c.in.comp, Routed: true}}
	}
	return out
}

// callOf is the opened component an event's target is in: its instance's,
// or the nearest around it.
func (s *Session) callOf(target string) *openCall {
	id, _, _ := strings.Cut(target, ":")
	var within func(in *instance) bool
	within = func(in *instance) bool {
		if in.id == id {
			return true
		}
		return slices.ContainsFunc(in.kids, within)
	}
	for i := len(s.calls) - 1; i >= 0; i-- {
		if within(s.calls[i].in) {
			return s.calls[i]
		}
	}
	return nil
}

// caller answers ui.Call and ui.Close in an event's handler.
type caller struct {
	s      *Session
	target string
}

func (c caller) OpenCall(ctx context.Context, component any, opts []ui.CallOption) error {
	comp, err := ui.CallValue(component)
	if err != nil {
		return err
	}
	key, into := ui.CallOptions(opts)
	if key == "" {
		c.s.callSeq++
		key = "s" + strconv.Itoa(c.s.callSeq)
	}
	owner := ""
	if into != "" {
		owner, _, _ = strings.Cut(c.target, ":")
	}
	return c.s.open(ctx, key, comp, opts, owner)
}

func (c caller) CloseCall(context.Context) {
	if oc := c.s.callOf(c.target); oc != nil {
		c.s.dropCall(oc.key)
	}
}

// Call opens component as ui.Call does in a handler, then renders the
// page again: what changed, the opened component among it. Its Mount's
// error is the patch's.
func (s *Session) Call(component any, opts ...ui.CallOption) (Patch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	comp, err := ui.CallValue(component)
	if err != nil {
		return Patch{}, err
	}
	key, _ := ui.CallOptions(opts)
	if key == "" {
		s.callSeq++
		key = "s" + strconv.Itoa(s.callSeq)
	}
	oerr := s.open(s.ctx, key, comp, opts, "")
	p, err := s.patch()
	if oerr != nil {
		p.Err, p.Error = oerr, s.errorText(oerr)
	}
	return p, err
}
