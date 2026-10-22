package ui

// ToastPosition is a ToastContainer's corner.
type ToastPosition string

const (
	ToastTopRight    ToastPosition = "top-right"
	ToastTopLeft     ToastPosition = "top-left"
	ToastBottomRight ToastPosition = "bottom-right"
	ToastBottomLeft  ToastPosition = "bottom-left"
)

// ToastContainerProps is ToastContainer, by its former name.
//
// Deprecated: use ToastContainer: ui.ToastContainer{…}, or
// <ui.ToastContainer …> in a template.
type ToastContainerProps = ToastContainer

// ToastProps is Toast, by its former name.
//
// Deprecated: use Toast: ui.Toast{…}, or <ui.Toast …> in a template.
type ToastProps = Toast

// ShowToast reveals the toast named ref (one pre-rendered hidden in the
// container), for a browser-only notification.
func ShowToast(ref string) Commands { return JS.Show(Ref(ref)) }

// DismissToast hides the toast the event came from (the close button's), no
// round trip.
func DismissToast() Commands { return JS.Hide(Closest(".vk-toast")) }

// DismissToastRef hides the toast named ref.
func DismissToastRef(ref string) Commands { return JS.Hide(Ref(ref)) }
