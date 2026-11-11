package ui

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"time"
)

// How a component a Call opens shows: its presentation — the call's As,
// else its root's (a Dialog, Drawer, Popover or Notice), else a dialog
// frame — or, Into a region of the page, in place there. The options a
// Call gives travel in its token, signed with its props.

// A Presentation is a component a Call can show another in: Dialog,
// Drawer, Popover and Notice. As the root of a component's markup it is
// that component's presentation; given to As, it is the call's.
type Presentation interface {
	Node
	parts() presentParts
	withParts(p presentParts) Presentation
	presentKind() string // its name in a token
}

// presentParts are what a presentation shows: a title, a footer and the
// component's markup.
type presentParts struct{ Title, Footer, Children Node }

var presentations = map[string]reflect.Type{
	"dialog":  reflect.TypeFor[Dialog](),
	"drawer":  reflect.TypeFor[Drawer](),
	"popover": reflect.TypeFor[Popover](),
	"notice":  reflect.TypeFor[Notice](),
}

// callOpts are a Call's options as its token carries them.
type callOpts struct {
	As    string          `json:"as,omitempty"`   // a presentation's kind
	With  json.RawMessage `json:"with,omitempty"` // its settings
	Into  string          `json:"into,omitempty"` // "#id"
	Bare  bool            `json:"bare,omitempty"` // no inline-card chrome
	Key   string          `json:"key,omitempty"`
	Size  string          `json:"size,omitempty"`
	Owner string          `json:"owner,omitempty"` // the instance an Into's component belongs to (a live page's)
}

func (o *callOpts) apply(opts []CallOption) {
	for _, f := range opts {
		if f != nil {
			f(o)
		}
	}
}

// as is the call's presentation, from As.
func (o *callOpts) as() (Presentation, error) {
	if o.As == "" {
		return nil, nil
	}
	t, ok := presentations[o.As]
	if !ok {
		return nil, fmt.Errorf("ui: no presentation %q", o.As)
	}
	v := reflect.New(t)
	if len(o.With) > 0 {
		if err := json.Unmarshal(o.With, v.Interface()); err != nil {
			return nil, err
		}
	}
	return v.Elem().Interface().(Presentation), nil
}

// options are o as CallOptions again.
func (o callOpts) options() []CallOption {
	return []CallOption{func(to *callOpts) { *to = o }}
}

// As shows the component a Call opens in p, whatever its own root is: a
// Drawer, a Popover by the element that opened it, a Notice. p's settings
// (a Drawer's Side, a Size) are its own; its title and content come from
// the component: the title and footer of the component's own root
// presentation, when it has one, and its markup. Panics when p sets them.
//
//	<button onClick={ui.JS.Call(components.OrderDetails{ID: o.ID}, ui.As(ui.Drawer{Side: "right"}))}>Details</button>
func As(p Presentation) CallOption {
	if ps := p.parts(); ps.Title != nil || ps.Footer != nil || ps.Children != nil {
		panic(fmt.Sprintf("ui: As(%T{…}) sets its title or content: As takes a presentation's settings, the component brings what it shows", p))
	}
	with, err := marshal(p)
	if err != nil {
		panic(fmt.Sprintf("ui: As(%T{…}): %v", p, err))
	}
	return func(o *callOpts) { o.As, o.With = p.presentKind(), with }
}

var keyText = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

// CallKey names the component a Call opens: a second Call with the same
// key, while the first is open, gives it the new props instead of opening
// another. Any text names one; keep it the same for the same thing
// ("order-" + id). (ui.Key is a component's key in a list.)
//
//	<button onClick={ui.JS.Call(components.Cart{}, ui.CallKey("cart"))}>Cart</button>
func CallKey(k string) CallOption {
	if !keyText.MatchString(k) {
		sum := sha256.Sum256([]byte(k))
		k = "k" + base64.RawURLEncoding.EncodeToString(sum[:12])
	}
	return func(o *callOpts) { o.Key = k }
}

// sizeOption is a Size given to a Call: the width of the presentation it
// shows in — SizeSm, SizeMd (the default), SizeLg, SizeXl, SizeFull.
// Panics on another.
//
//	<button onClick={ui.JS.Call(components.Report{ID: r.ID}, ui.SizeXl)}>Report</button>
func sizeOption(s Size) CallOption {
	if s == SizeMd {
		return nil
	}
	if !slices.Contains([]Size{SizeSm, SizeLg, SizeXl, SizeFull}, s) {
		panic(fmt.Sprintf("ui: JS Call's Size %q: SizeSm, SizeMd, SizeLg, SizeXl or SizeFull", s))
	}
	return func(o *callOpts) { o.Size = string(s) }
}

// rootPresentation is the presentation at view's root, all of it.
func rootPresentation(n Node) (Presentation, bool) {
	for range 8 {
		switch x := n.(type) {
		case Presentation:
			return x, true
		case *lazyFrame:
			if x == nil || x.shape != "h" || len(x.dyns) != 1 {
				return nil, false
			}
			d, ok := x.dyns[0].(Node)
			if !ok {
				return nil, false
			}
			n = d
		case keyedNode:
			n = x.n
		default:
			return nil, false
		}
	}
	return nil, false
}

// callRoot is what a presentation puts on its root as a Call's: the
// attributes CallView gave it, with the kind of presentation the runtime
// shows, once.
func callRoot(ctx context.Context, kind string) ([]Attr, context.Context) {
	attrs, ctx := callRootAttrs(ctx)
	if attrs != nil {
		attrs = append(attrs, Attr{"data-vk-present", kind})
	}
	return attrs, ctx
}

// presentation renders a presentation's element: tag with class and the
// call's attributes, a header with the title and a close button, the body
// and the footer.
func presentation(tag, class string, attrs []Attr, ctx context.Context, p presentParts, header bool) Node {
	attrs = append([]Attr{{"class", class}}, attrs...)
	var kids []Node
	if header {
		h := []Node{}
		if p.Title != nil {
			h = append(h, El("h2", []Attr{{"class", "vk-dialog__title"}}, p.Title))
		}
		h = append(h, callClose(ctx, "vk-dialog__close"))
		kids = append(kids, El("div", []Attr{{"class", "vk-dialog__header"}}, h...))
	}
	kids = append(kids, El("div", []Attr{{"class", "vk-dialog__body"}}, p.Children))
	if p.Footer != nil {
		kids = append(kids, El("div", []Attr{{"class", "vk-dialog__footer"}}, p.Footer))
	}
	return withContext(func(c context.Context) context.Context {
		return context.WithValue(c, callRootKey{}, []Attr(nil))
	}, El(tag, attrs, kids...))
}

func (d Dialog) parts() presentParts { return presentParts{d.Title, d.Footer, d.Children} }
func (d Dialog) withParts(p presentParts) Presentation {
	d.Title, d.Footer, d.Children = p.Title, p.Footer, p.Children
	return d
}
func (Dialog) presentKind() string { return "dialog" }

// Drawer is a dialog sliding in from an edge of the window: a Call's
// presentation as Dialog is, for details beside the page.
//
//	<ui.Drawer title={ui.Text("Order " + Number)} side="right">…</ui.Drawer>
type Drawer struct {
	Title Node
	// Side is "right" (the default), "left" or "bottom".
	Side     string
	Footer   Node
	Class    string
	Children Node
}

func (d Drawer) View() Node {
	return ctxNode(func(ctx context.Context) Node {
		call, ctx := callRoot(ctx, "modal")
		side := d.Side
		if side != "left" && side != "bottom" {
			side = "right"
		}
		return presentation("dialog", withClass("vk-drawer vk-drawer--"+side, d.Class), call, ctx, d.parts(), true)
	})
}

func (d Drawer) Render(ctx context.Context, w io.Writer) error { return d.View().Render(ctx, w) }
func (d Drawer) parts() presentParts                           { return presentParts{d.Title, d.Footer, d.Children} }
func (d Drawer) withParts(p presentParts) Presentation {
	d.Title, d.Footer, d.Children = p.Title, p.Footer, p.Children
	return d
}
func (Drawer) presentKind() string { return "drawer" }

// Popover is a small panel by the element that opened it, not modal:
// Escape or a click elsewhere closes it.
type Popover struct {
	Title    Node
	Class    string
	Children Node
}

func (p Popover) View() Node {
	return ctxNode(func(ctx context.Context) Node {
		call, ctx := callRoot(ctx, "popover")
		return presentation("dialog", withClass("vk-popover", p.Class), call, ctx, p.parts(), p.Title != nil)
	})
}

func (p Popover) Render(ctx context.Context, w io.Writer) error { return p.View().Render(ctx, w) }
func (p Popover) parts() presentParts                           { return presentParts{p.Title, nil, p.Children} }
func (p Popover) withParts(ps presentParts) Presentation {
	p.Title, p.Children = ps.Title, ps.Children
	return p
}
func (Popover) presentKind() string { return "popover" }

// Notice is a toast a Call opens: a message in the corner of the window,
// not modal, closing itself after Timeout (4s; negative: when closed).
//
//	<button onClick={ui.JS.Push(Save).Success(ui.JS.Call(ui.Notice{Text: "Saved", Variant: ui.AlertSuccess}))}>Save</button>
type Notice struct {
	Title    string
	Text     string
	Variant  AlertVariant
	Timeout  time.Duration
	Children Node `json:"-"`
}

func (n Notice) View() Node {
	return ctxNode(func(ctx context.Context) Node {
		call, ctx := callRoot(ctx, "toast")
		variant := n.Variant
		if variant == "" {
			variant = AlertInfo
		}
		t := n.Timeout
		if t == 0 {
			t = 4 * time.Second
		}
		attrs := append([]Attr{{"class", "vk-toast vk-toast--" + string(variant)}, {"role", "status"}}, call...)
		if t > 0 && call != nil {
			attrs = append(attrs, Attr{"data-vk-timeout", strconv.FormatInt(t.Milliseconds(), 10)})
		}
		var body []Node
		if n.Title != "" {
			body = append(body, El("div", []Attr{{"class", "vk-toast__title"}}, Text(n.Title)))
		}
		if n.Text != "" {
			body = append(body, Text(n.Text))
		}
		body = append(body, n.Children)
		return withContext(func(c context.Context) context.Context { return context.WithValue(c, callRootKey{}, []Attr(nil)) },
			El("div", attrs, El("div", []Attr{{"class", "vk-toast__body"}}, body...), callClose(ctx, "vk-toast__close")))
	})
}

func (n Notice) Render(ctx context.Context, w io.Writer) error { return n.View().Render(ctx, w) }
func (n Notice) parts() presentParts                           { return presentParts{Children: n.Children} }
func (n Notice) withParts(p presentParts) Presentation {
	n.Children = p.Children
	return n
}
func (Notice) presentKind() string { return "notice" }

// ConfirmBox asks the visitor to confirm, in a dialog a Call opens: its
// OnYes runs when they do, its OnNo when they don't, and it closes. A
// page's JS.Confirm shows one too where the runtime has an overlay host.
//
//	<button onClick={ui.JS.Call(ui.ConfirmBox{Title: "Delete order?", Text: "It can't be undone.", Danger: true, OnYes: Remove})}>Delete</button>
type ConfirmBox struct {
	Title  string
	Text   string
	Danger bool
	// Yes and No are the buttons' words: "OK" and "Cancel" by default, in
	// the visitor's language.
	Yes, No string
	OnYes   Action
	OnNo    Action
}

func (c ConfirmBox) View() Node {
	var title Node
	if c.Title != "" {
		title = Text(c.Title)
	}
	return Dialog{Size: "sm", Class: "vk-confirm", Title: title, Children: El("p", nil, Text(c.Text)), Footer: ctxNode(func(ctx context.Context) Node {
		yes, no := c.Yes, c.No
		if yes == "" {
			yes = tr(ctx, "confirm.yes")
		}
		if no == "" {
			no = tr(ctx, "confirm.no")
		}
		then := func(a Action) Commands {
			if a == nil {
				return JS.Close()
			}
			return actionChain("ConfirmBox", a).Close()
		}
		yesClass := "vk-btn vk-btn--primary"
		if c.Danger {
			yesClass = "vk-btn vk-btn--danger"
		}
		return Fragment(
			El("button", []Attr{{"type", "button"}, {"class", "vk-btn"}, {"data-vk-no", true}, {"onClick", then(c.OnNo)}}, Text(no)),
			El("button", []Attr{{"type", "button"}, {"class", yesClass}, {"data-vk-yes", true}, {"onClick", then(c.OnYes)}}, Text(yes)),
		)
	})}
}

func (c ConfirmBox) Render(ctx context.Context, w io.Writer) error { return c.View().Render(ctx, w) }

func init() {
	RegisterCall(ConfirmBox{})
	RegisterCall(Notice{})
}
