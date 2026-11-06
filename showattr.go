package ui

import (
	"encoding/json"
	"fmt"
	"time"
)

// An element's show attribute shows it while its value holds and hides it
// (the hidden attribute) while it doesn't: a bool, or an expression of
// states the browser keeps following.
//
//	<div show={open} transition={ui.Fade(150 * time.Millisecond)}>…</div>
//
// Its transition attribute animates the change: the enter phase when it
// shows, the leave phase — waited for before it is hidden — when it
// hides. A change while one runs settles to the newest. The page's first
// render, and a server render, show it at once, and so does a browser that
// prefers reduced motion.

// ShowTransition is how a show attribute's element appears and
// disappears: Fade, Scale, Slide, or Animate's classes.
type ShowTransition struct {
	ms           int64
	enter, leave phase
}

// phase is a transition phase: CSS classes (each space-separated) or
// inline styles while it runs, at its start and at its end.
type phase struct {
	Classes *[3]string    `json:"c,omitempty"`
	Styles  *[3]StyleList `json:"s,omitempty"`
}

// StyleList is inline CSS declarations, by property.
type StyleList map[string]string

// Phase is one side of Animate's transition: classes while it runs (during),
// at its start (from) and at its end (to).
type Phase struct {
	during, from, to string
	leave            bool
}

// Enter is the classes an element shows with.
func Enter(during, from, to string) Phase { return Phase{during, from, to, false} }

// Leave is the classes an element hides with.
func Leave(during, from, to string) Phase { return Phase{during, from, to, true} }

// Animate is a transition of classes, d long:
//
//	transition={ui.Animate(200*time.Millisecond,
//		ui.Enter("transition duration-200", "opacity-0 scale-95", "opacity-100 scale-100"),
//		ui.Leave("transition duration-150", "opacity-100", "opacity-0"))}
func Animate(d time.Duration, phases ...Phase) ShowTransition {
	t := ShowTransition{ms: d.Milliseconds()}
	for _, p := range phases {
		c := &[3]string{p.during, p.from, p.to}
		if p.leave {
			t.leave.Classes = c
		} else {
			t.enter.Classes = c
		}
	}
	return t
}

// Fade fades the element in and out over d.
func Fade(d time.Duration) ShowTransition { return styled(d, StyleList{"opacity": "0"}) }

// Scale fades the element in and out over d, growing it from 95%.
func Scale(d time.Duration) ShowTransition {
	return styled(d, StyleList{"opacity": "0", "transform": "scale(0.95)"})
}

// Side is where Slide moves an element from.
type Side string

// The sides an element slides from.
const (
	Top    Side = "top"
	Bottom Side = "bottom"
	Left   Side = "left"
	Right  Side = "right"
)

// Slide slides the element in from side, fading, and back out, over d.
func Slide(from Side, d time.Duration) ShowTransition {
	off := map[Side]string{Top: "translateY(-0.5rem)", Bottom: "translateY(0.5rem)", Left: "translateX(-0.5rem)", Right: "translateX(0.5rem)"}[from]
	if off == "" {
		off = "translateY(-0.5rem)"
	}
	return styled(d, StyleList{"opacity": "0", "transform": off})
}

// styled is a transition of inline styles from hidden, out, to shown.
func styled(d time.Duration, out StyleList) ShowTransition {
	during := StyleList{"transition-property": "opacity, transform", "transition-duration": fmt.Sprintf("%dms", d.Milliseconds()), "transition-timing-function": "ease"}
	in := StyleList{}
	for k := range out {
		in[k] = map[string]string{"opacity": "1", "transform": "none"}[k]
	}
	return ShowTransition{ms: d.Milliseconds(),
		enter: phase{Styles: &[3]StyleList{during, out, in}},
		leave: phase{Styles: &[3]StyleList{during, in, out}}}
}

func (t ShowTransition) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		MS    int64 `json:"ms"`
		Enter phase `json:"enter"`
		Leave phase `json:"leave"`
	}{t.ms, t.enter, t.leave})
}

// showAttr writes a show attribute: hidden while its value doesn't hold,
// and the binding the browser follows when it reads states.
func (h *HTMLRenderer) showAttr(tag string, e *Element, v any) error {
	switch v := v.(type) {
	case bool:
		if !v {
			return h.attrNamed(tag, e, Attr{Name: "hidden", Value: true})
		}
		return nil
	case Binding:
		on, ok := v.Init.(bool)
		if !ok {
			return fmt.Errorf("ui: show reads a %T, not a bool", v.Init)
		}
		if !on {
			if err := h.attrNamed(tag, e, Attr{Name: "hidden", Value: true}); err != nil {
				return err
			}
		}
		j, err := v.json(h.ctx)
		if err != nil {
			return fmt.Errorf("show: %w", err)
		}
		h.write(` data-vk-v="`)
		h.writeEscaped(j)
		h.writeByte('"')
		return h.err
	}
	return fmt.Errorf("ui: show takes a bool or an expression of states, not %T", v)
}

// transitionAttr writes a show attribute's transition.
func (h *HTMLRenderer) transitionAttr(t ShowTransition) error {
	j, err := json.Marshal(t)
	if err != nil {
		return err
	}
	h.write(` data-vk-transition="`)
	h.writeEscaped(string(j))
	h.writeByte('"')
	return h.err
}
