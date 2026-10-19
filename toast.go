package ui

import (
	"strconv"
	"time"
)

// ToastPosition is a ToastContainer's corner.
type ToastPosition string

const (
	ToastTopRight    ToastPosition = "top-right"
	ToastTopLeft     ToastPosition = "top-left"
	ToastBottomRight ToastPosition = "bottom-right"
	ToastBottomLeft  ToastPosition = "bottom-left"
)

// ToastContainer is the fixed region toasts stack in. Place one per page (a
// live view renders a Toast per message it holds inside it):
//
//	<ui.ToastContainer>
//		for _, m := range p.Messages {
//			<ui.Toast key={m.ID} id={m.ID} variant={ui.AlertSuccess} title={m.Title}/>
//		}
//	</ui.ToastContainer>
type ToastContainer struct {
	// Ref names the region for ShowToast to reveal a toast inside it; the
	// default is "toasts".
	Ref string
	// Position is the corner the stack sits in; ToastTopRight by default.
	Position ToastPosition
	Class    string
	Children Node
}

// ToastContainerProps is ToastContainer, by its former name.
//
// Deprecated: use ToastContainer: ui.ToastContainer{…}, or
// <ui.ToastContainer …> in a template.
type ToastContainerProps = ToastContainer

func (c ToastContainer) ref() string {
	if c.Ref == "" {
		return "toasts"
	}
	return c.Ref
}

func (c ToastContainer) classes() string {
	pos := c.Position
	if pos == "" {
		pos = ToastTopRight
	}
	return withClass("vk-toasts vk-toasts--"+string(pos), c.Class)
}

// Toast is one notification. Its tone matches Alert's variants:
//
//	<ui.Toast variant={ui.AlertSuccess} title="Saved" timeout={4 * time.Second}/>
type Toast struct {
	// Variant is the tone: AlertInfo (default), AlertSuccess, AlertWarning or
	// AlertError, shared with Alert.
	Variant AlertVariant
	// Title is an optional bold heading.
	Title string
	// ID identifies the toast for DismissToastRef and for a server list's key;
	// it renders as both id and ref so ShowToast/DismissToastRef can find it.
	ID string
	// Dismissible adds a close button; default true.
	Closed bool
	// Timeout, when set, fades the toast out after it in the browser (a CSS
	// animation, no JavaScript). The server removing it from its render is the
	// reliable way to drop it.
	Timeout  time.Duration
	Class    string
	Children Node
}

// ToastProps is Toast, by its former name.
//
// Deprecated: use Toast: ui.Toast{…}, or <ui.Toast …> in a template.
type ToastProps = Toast

func (t Toast) classes() string {
	variant := t.Variant
	if variant == "" {
		variant = AlertInfo
	}
	return withClass("vk-toast vk-toast--"+string(variant), t.Class)
}

// life is the style that fades the toast out after its Timeout, or nil.
func (t Toast) life() any {
	if t.Timeout <= 0 {
		return nil
	}
	return Style{"--vk-toast-life": strconv.FormatInt(t.Timeout.Milliseconds(), 10) + "ms"}
}

// ShowToast reveals the toast named ref (one pre-rendered hidden in the
// container), for a browser-only notification.
func ShowToast(ref string) Commands { return JS.Show(Ref(ref)) }

// DismissToast hides the toast the event came from (the close button's), no
// round trip.
func DismissToast() Commands { return JS.Hide(Closest(".vk-toast")) }

// DismissToastRef hides the toast named ref.
func DismissToastRef(ref string) Commands { return JS.Hide(Ref(ref)) }
